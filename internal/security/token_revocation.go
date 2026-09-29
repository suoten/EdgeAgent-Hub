package security

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TokenRevocationList Token 撤销列表
type TokenRevocationList struct {
	mu       sync.RWMutex
	revoked  map[string]time.Time // tokenHash -> expiry
	logger   *logrus.Entry
}

// NewTokenRevocationList 创建 Token 撤销列表
func NewTokenRevocationList(logger *logrus.Entry) *TokenRevocationList {
	return &TokenRevocationList{
		revoked: make(map[string]time.Time),
		logger:  logger,
	}
}

// Revoke 撤销一个 Token（记录其哈希和过期时间）
func (t *TokenRevocationList) Revoke(tokenHash string, expiry time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.revoked[tokenHash] = expiry
	t.logger.Infof("Token revoked: %s... (expires: %s)", tokenHash[:8], expiry.Format(time.RFC3339))
}

// IsRevoked 检查 Token 是否已被撤销
func (t *TokenRevocationList) IsRevoked(tokenHash string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, revoked := t.revoked[tokenHash]
	return revoked
}

// Cleanup 清理已过期的撤销记录
func (t *TokenRevocationList) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for hash, expiry := range t.revoked {
		if now.After(expiry) {
			delete(t.revoked, hash)
		}
	}
}

// StartCleanupTask 启动定期清理任务
func (t *TokenRevocationList) StartCleanupTask(interval time.Duration, stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				t.Cleanup()
			}
		}
	}()
}
