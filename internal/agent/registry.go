package agent

import (
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// Registry 管理智能体注册、发现和健康检查
type Registry struct {
	mu              sync.RWMutex
	agents          map[string]*models.AgentInfo
	logger          *logrus.Entry
	heartbeatTimeout time.Duration
}

// NewRegistry 创建智能体注册中心
func NewRegistry(logger *logrus.Entry, heartbeatTimeout time.Duration) *Registry {
	return &Registry{
		agents:          make(map[string]*models.AgentInfo),
		logger:          logger,
		heartbeatTimeout: heartbeatTimeout,
	}
}

// Register 注册一个智能体
func (r *Registry) Register(info *models.AgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info.LastHeartbeat = time.Now()
	if info.Status == "" {
		info.Status = "online"
	}
	if info.Weight == 0 {
		info.Weight = 1
	}
	r.agents[info.ID] = info
	r.logger.Infof("Agent registered: %s (type=%s, endpoint=%s, capabilities=%v)",
		info.ID, info.Type, info.Endpoint, info.Capabilities)
}

// Deregister 注销一个智能体
func (r *Registry) Deregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
	r.logger.Infof("Agent deregistered: %s", agentID)
}

// Heartbeat 更新智能体心跳
func (r *Registry) Heartbeat(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if agent, ok := r.agents[agentID]; ok {
		agent.LastHeartbeat = time.Now()
		agent.Status = "online"
	}
}

// Get 获取单个智能体信息
func (r *Registry) Get(agentID string) (*models.AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[agentID]
	if !ok {
		return nil, false
	}
	// 返回副本
	copy := *agent
	return &copy, true
}

// List 列出所有智能体
func (r *Registry) List() []models.AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]models.AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		result = append(result, *a)
	}
	return result
}

// DiscoverByCapability 通过能力发现智能体
func (r *Registry) DiscoverByCapability(capability string) []models.AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []models.AgentInfo
	for _, a := range r.agents {
		if a.Status != "online" {
			continue
		}
		for _, cap := range a.Capabilities {
			if cap == capability {
				result = append(result, *a)
				break
			}
		}
	}
	return result
}

// DiscoverByType 通过类型发现智能体
func (r *Registry) DiscoverByType(agentType models.AgentType) []models.AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []models.AgentInfo
	for _, a := range r.agents {
		if a.Status != "online" {
			continue
		}
		if a.Type == agentType {
			result = append(result, *a)
		}
	}
	return result
}

// DiscoverByAgentID 通过 Agent ID 发现智能体 (如 "onnx.vibration_anomaly")
func (r *Registry) DiscoverByAgentID(agentID string) (*models.AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if agent, ok := r.agents[agentID]; ok {
		if agent.Status == "online" {
			copy := *agent
			return &copy, true
		}
	}
	return nil, false
}

// SelectByAgentID 选择一个智能体 (支持负载均衡)
func (r *Registry) SelectByAgentID(agentID string) (*models.AgentInfo, bool) {
	return r.DiscoverByAgentID(agentID)
}

// SelectByCapability 选择一个具有指定能力的智能体 (轮询负载均衡)
func (r *Registry) SelectByCapability(capability string) (*models.AgentInfo, bool) {
	agents := r.DiscoverByCapability(capability)
	if len(agents) == 0 {
		return nil, false
	}
	// 简单轮询: 返回第一个 (后续可改为加权轮询)
	return &agents[0], true
}

// StartHealthCheck 启动健康检查 goroutine
func (r *Registry) StartHealthCheck(interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.checkHealth()
		case <-stopCh:
			return
		}
	}
}

// checkHealth 检查所有智能体心跳
func (r *Registry) checkHealth() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, agent := range r.agents {
		// 内置智能体（endpoint=internal）永远在线，不检查心跳
		if agent.Endpoint == "internal" {
			agent.Status = "online"
			continue
		}

		if now.Sub(agent.LastHeartbeat) > r.heartbeatTimeout {
			if agent.Status != "offline" {
				agent.Status = "offline"
				r.logger.Warnf("Agent heartbeat timeout, marking offline: %s (last: %s)",
					id, agent.LastHeartbeat.Format(time.RFC3339))
			}
		} else if now.Sub(agent.LastHeartbeat) > r.heartbeatTimeout/2 {
			if agent.Status != "degraded" {
				agent.Status = "degraded"
				r.logger.Warnf("Agent heartbeat degraded: %s", id)
			}
		}
	}
}

// OnlineCount 返回在线智能体数量
func (r *Registry) OnlineCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, a := range r.agents {
		if a.Status == "online" {
			count++
		}
	}
	return count
}
