package agent

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// OOMManager OOM 自动卸载管理器
type OOMManager struct {
	registry      *Registry
	logger        *logrus.Entry
	mu            sync.RWMutex
	maxMemoryMB   int64
	checkInterval time.Duration
	stopCh        chan struct{}
	modelMemory   map[string]int64 // modelID -> memoryMB
	priorities    map[string]int   // modelID -> priority (1=highest, 10=lowest)
}

// NewOOMManager 创建 OOM 管理器
func NewOOMManager(registry *Registry, maxMemoryMB int64, logger *logrus.Entry) *OOMManager {
	return &OOMManager{
		registry:      registry,
		logger:        logger,
		maxMemoryMB:   maxMemoryMB,
		checkInterval: 10 * time.Second,
		stopCh:        make(chan struct{}),
		modelMemory:   make(map[string]int64),
		priorities:    make(map[string]int),
	}
}

// SetModelMemory 设置模型内存占用
func (o *OOMManager) SetModelMemory(modelID string, memoryMB int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.modelMemory[modelID] = memoryMB
}

// SetModelPriority 设置模型优先级
func (o *OOMManager) SetModelPriority(modelID string, priority int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.priorities[modelID] = priority
}

// Start 启动 OOM 监控
func (o *OOMManager) Start() {
	go func() {
		ticker := time.NewTicker(o.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-o.stopCh:
				return
			case <-ticker.C:
				o.checkAndEvict()
			}
		}
	}()
	o.logger.Info("OOM manager started")
}

// Stop 停止 OOM 监控
func (o *OOMManager) Stop() {
	close(o.stopCh)
}

// checkAndEvict 检查内存使用并卸载低优先级模型
func (o *OOMManager) checkAndEvict() {
	o.mu.Lock()
	defer o.mu.Unlock()

	totalMemory := int64(0)
	for _, mem := range o.modelMemory {
		totalMemory += mem
	}

	// 如果超过阈值，开始卸载
	if totalMemory > o.maxMemoryMB {
		o.logger.Warnf("Memory threshold exceeded: %dMB > %dMB, evicting low-priority models",
			totalMemory, o.maxMemoryMB)

		// 按优先级排序（优先级数字大的先卸载）
		type modelPriority struct {
			id       string
			priority int
			memory   int64
		}
		var candidates []modelPriority
		for id, mem := range o.modelMemory {
			pri := o.priorities[id]
			if pri == 0 {
				pri = 5 // 默认优先级
			}
			candidates = append(candidates, modelPriority{id: id, priority: pri, memory: mem})
		}

		// 简单冒泡排序（按优先级降序）
		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].priority > candidates[i].priority {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}

		// 卸载直到内存低于阈值
		for _, candidate := range candidates {
			if totalMemory <= o.maxMemoryMB {
				break
			}

			o.logger.Infof("OOM eviction: unloading model %s (priority: %d, memory: %dMB)",
				candidate.id, candidate.priority, candidate.memory)

			// 标记智能体为离线
			if agent, ok := o.registry.Get(candidate.id); ok {
				agent.Status = "evicted"
			}

			totalMemory -= candidate.memory
			delete(o.modelMemory, candidate.id)
		}
	}
}
