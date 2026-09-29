package offline

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/messaging"
	"github.com/edgelite/edgeagent-hub/internal/storage"
	"github.com/sirupsen/logrus"
)

// Manager 管理断网自治：缓存队列、离线推理、延迟重传
type Manager struct {
	store             *storage.Store
	bridge            *messaging.Bridge
	logger            *logrus.Entry
	checkInterval     time.Duration
	retransmitInterval time.Duration
	maxRetries        int

	offlineMode       atomic.Bool
	cloudConnected    atomic.Bool
	mu                sync.RWMutex
	cancel            context.CancelFunc
}

// NewManager 创建断网自治管理器
func NewManager(store *storage.Store, bridge *messaging.Bridge,
	checkInterval, retransmitInterval time.Duration, maxRetries int,
	logger *logrus.Entry) *Manager {
	return &Manager{
		store:              store,
		bridge:             bridge,
		logger:             logger,
		checkInterval:      checkInterval,
		retransmitInterval: retransmitInterval,
		maxRetries:         maxRetries,
	}
}

// Start 启动断网自治管理器
func (m *Manager) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	// 启动连接检查
	go m.checkLoop(ctx)

	// 启动重传
	go m.retransmitLoop(ctx)

	m.logger.Info("Offline autonomy manager started")
}

// Stop 停止管理器
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// SetOfflineMode 手动设置离线模式
func (m *Manager) SetOfflineMode(offline bool) {
	if offline {
		m.offlineMode.Store(true)
		m.cloudConnected.Store(false)
		m.logger.Warn("Manually entering offline mode")
	} else {
		m.offlineMode.Store(false)
		m.cloudConnected.Store(true)
		m.logger.Info("Manually exiting offline mode")
	}
}

// IsOfflineMode 返回是否处于离线自治模式
func (m *Manager) IsOfflineMode() bool {
	return m.offlineMode.Load()
}

// IsCloudConnected 返回云端连接状态
func (m *Manager) IsCloudConnected() bool {
	return m.cloudConnected.Load()
}

// Enqueue 将消息加入离线队列
func (m *Manager) Enqueue(topic string, payload []byte) error {
	return m.store.EnqueueOffline(topic, payload)
}

// QueueLength 返回离线队列长度
func (m *Manager) QueueLength() (int, error) {
	return m.store.OfflineQueueLength()
}

// checkLoop 定期检查云端连接状态
func (m *Manager) checkLoop(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkConnection()
		}
	}
}

// checkConnection 检查云端连接
func (m *Manager) checkConnection() {
	// 使用 NATS 连接状态作为云端连接指示
	connected := m.bridge != nil && m.bridge.IsNATSConnected()
	wasConnected := m.cloudConnected.Load()
	m.cloudConnected.Store(connected)

	if connected != wasConnected {
		if connected {
			m.logger.Info("Cloud connection restored, exiting offline mode")
			m.offlineMode.Store(false)
		} else {
			m.logger.Warn("Cloud connection lost, entering offline mode")
			m.offlineMode.Store(true)
		}
	}
}

// retransmitLoop 重传循环
func (m *Manager) retransmitLoop(ctx context.Context) {
	ticker := time.NewTicker(m.retransmitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.cloudConnected.Load() {
				m.retransmit()
			}
		}
	}
}

// retransmit 重传离线队列中的消息
func (m *Manager) retransmit() {
	msgs, err := m.store.DequeueOffline(100)
	if err != nil {
		m.logger.Errorf("Failed to dequeue offline messages: %v", err)
		return
	}

	if len(msgs) == 0 {
		return
	}

	m.logger.Infof("Retransmitting %d offline messages", len(msgs))

	for _, msg := range msgs {
		// 通过 NATS 重传
		err := m.bridge.PublishNATS(msg.Topic, msg.Payload)
		if err != nil {
			m.logger.Errorf("Failed to retransmit message %d: %v", msg.ID, err)
			if msg.Retries >= m.maxRetries {
				m.store.MarkOfflineFailed(msg.ID)
			} else {
				m.store.IncrementOfflineRetry(msg.ID)
			}
		} else {
			m.store.MarkOfflineSent(msg.ID)
			m.logger.Debugf("Retransmitted message %d to %s", msg.ID, msg.Topic)
		}
	}
}
