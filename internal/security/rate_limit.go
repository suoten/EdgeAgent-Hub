package security

import (
	"sync"
	"time"
)

// RateLimiter 基于滑动窗口的请求限流器
// 按IP/路径限流，如 60次/分钟
type RateLimiter struct {
	mu          sync.Mutex
	requests    map[string][]time.Time
	maxRequests int
	window      time.Duration
	stopCh      chan struct{}
}

// NewRateLimiter 创建限流器
func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests:    make(map[string][]time.Time),
		maxRequests: maxRequests,
		window:      window,
		stopCh:      make(chan struct{}),
	}
	// 启动清理 goroutine
	go rl.cleanup()
	return rl
}

// Stop 停止限流器的清理 goroutine
func (r *RateLimiter) Stop() {
	close(r.stopCh)
}

// Allow 检查是否允许请求
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// 过滤过期请求
	var valid []time.Time
	for _, t := range r.requests[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.maxRequests {
		r.requests[key] = valid
		return false
	}

	valid = append(valid, now)
	r.requests[key] = valid
	return true
}

// cleanup 定期清理过期记录
func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-r.window)
			for key, times := range r.requests {
				var valid []time.Time
				for _, t := range times {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(r.requests, key)
				} else {
					r.requests[key] = valid
				}
			}
			r.mu.Unlock()
		}
	}
}

// ── 登录保护 ──

// LoginProtector 登录保护器 (5次失败锁定15分钟)
type LoginProtector struct {
	mu               sync.Mutex
	failedAttempts   map[string]int
	lockedUntil      map[string]time.Time
	maxAttempts      int
	lockoutDuration  time.Duration
}

// NewLoginProtector 创建登录保护器
func NewLoginProtector(maxAttempts int, lockoutDuration time.Duration) *LoginProtector {
	return &LoginProtector{
		failedAttempts:  make(map[string]int),
		lockedUntil:     make(map[string]time.Time),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
	}
}

// IsLocked 检查用户是否被锁定
func (p *LoginProtector) IsLocked(username string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	until, ok := p.lockedUntil[username]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	// 锁定期已过，重置
	delete(p.lockedUntil, username)
	delete(p.failedAttempts, username)
	return false
}

// RecordFailure 记录登录失败
func (p *LoginProtector) RecordFailure(username string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.failedAttempts[username]++
	if p.failedAttempts[username] >= p.maxAttempts {
		p.lockedUntil[username] = time.Now().Add(p.lockoutDuration)
	}
}

// RecordSuccess 记录登录成功
func (p *LoginProtector) RecordSuccess(username string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failedAttempts, username)
	delete(p.lockedUntil, username)
}

// RemainingAttempts 返回剩余尝试次数
func (p *LoginProtector) RemainingAttempts(username string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxAttempts - p.failedAttempts[username]
}
