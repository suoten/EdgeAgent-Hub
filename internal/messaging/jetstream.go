package messaging

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// JetStreamManager 管理 NATS JetStream 持久化和编排断点续行
type JetStreamManager struct {
	js        nats.JetStreamContext
	logger    *logrus.Entry
	streams   map[string]*nats.StreamInfo
	consumers map[string]*nats.ConsumerInfo
}

// 预定义 Stream 名称
const (
	StreamOrchestrator = "ORCHESTRATOR"
	StreamAlerts       = "ALERTS"
	StreamInference    = "INFERENCE"
	StreamSensorData   = "SENSORDATA"
)

// 预定义 Subject 前缀
const (
	SubjectOrchestratorExec = "orchestrator.exec."
	SubjectAlertsPersistent = "alerts.persistent."
	SubjectInferenceResult  = "inference.result."
	SubjectSensorPersist    = "sensor.persist."
)

// NewJetStreamManager 创建 JetStream 管理器
func NewJetStreamManager(nc *nats.Conn, logger *logrus.Entry) (*JetStreamManager, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	mgr := &JetStreamManager{
		js:        js,
		logger:    logger,
		streams:   make(map[string]*nats.StreamInfo),
		consumers: make(map[string]*nats.ConsumerInfo),
	}

	return mgr, nil
}

// InitializeStreams 初始化所有预定义 Stream
func (m *JetStreamManager) InitializeStreams() error {
	streamDefs := map[string]*nats.StreamConfig{
		StreamOrchestrator: {
			Name:      StreamOrchestrator,
			Subjects:  []string{SubjectOrchestratorExec + ">"},
			Retention: nats.WorkQueuePolicy,
			MaxMsgs:   100000,
			MaxAge:    24 * time.Hour,
			Storage:   nats.FileStorage,
		},
		StreamAlerts: {
			Name:      StreamAlerts,
			Subjects:  []string{SubjectAlertsPersistent + ">"},
			Retention: nats.LimitsPolicy,
			MaxMsgs:   50000,
			MaxAge:    72 * time.Hour,
			Storage:   nats.FileStorage,
		},
		StreamInference: {
			Name:      StreamInference,
			Subjects:  []string{SubjectInferenceResult + ">"},
			Retention: nats.LimitsPolicy,
			MaxMsgs:   100000,
			MaxAge:    48 * time.Hour,
			Storage:   nats.FileStorage,
		},
		StreamSensorData: {
			Name:      StreamSensorData,
			Subjects:  []string{SubjectSensorPersist + ">"},
			Retention: nats.LimitsPolicy,
			MaxMsgs:   500000,
			MaxAge:    24 * time.Hour,
			Storage:   nats.FileStorage,
		},
	}

	for name, cfg := range streamDefs {
		info, err := m.js.AddStream(cfg)
		if err != nil {
			// 尝试更新已有 Stream
			info, err = m.js.UpdateStream(cfg)
			if err != nil {
				return fmt.Errorf("failed to create/update stream %s: %w", name, err)
			}
		}
		m.streams[name] = info
		m.logger.Infof("JetStream stream '%s' initialized", name)
	}

	return nil
}

// PublishExecutionState 发布编排执行状态（用于断点续行）
func (m *JetStreamManager) PublishExecutionState(execution *models.WorkflowExecution) error {
	subject := SubjectOrchestratorExec + execution.WorkflowID
	data, err := json.Marshal(execution)
	if err != nil {
		return fmt.Errorf("failed to marshal execution state: %w", err)
	}

	_, err = m.js.Publish(subject, data, nats.MsgId(execution.WorkflowID))
	if err != nil {
		return fmt.Errorf("failed to publish execution state: %w", err)
	}

	m.logger.Debugf("Published execution state for workflow %s (status: %s)", execution.WorkflowName, execution.Status)
	return nil
}

// CreateExecutionConsumer 为编排断点续行创建消费者
func (m *JetStreamManager) CreateExecutionConsumer(workflowID string) (*nats.ConsumerInfo, error) {
	consumerName := "orchestrator-" + workflowID
	consumer, err := m.js.AddConsumer(StreamOrchestrator, &nats.ConsumerConfig{
		Name:          consumerName,
		FilterSubject: SubjectOrchestratorExec + workflowID,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer %s: %w", consumerName, err)
	}

	m.logger.Infof("Created execution consumer for workflow %s", workflowID)
	return consumer, nil
}

// RecoverExecutions 恢复未完成的编排执行
func (m *JetStreamManager) RecoverExecutions(handler func(execution *models.WorkflowExecution)) error {
	// 删除可能存在的旧消费者（上次运行残留）
	_ = m.js.DeleteConsumer(StreamOrchestrator, "recovery-scanner")

	// 直接使用 PullSubscribe 创建临时消费者用于扫描未完成执行
	sub, err := m.js.PullSubscribe(
		SubjectOrchestratorExec+">",
		"recovery-scanner",
		nats.ManualAck(),
		nats.MaxDeliver(1),
		nats.AckWait(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to pull subscribe: %w", err)
	}
	defer m.js.DeleteConsumer(StreamOrchestrator, "recovery-scanner")

	msgs, err := sub.Fetch(100, nats.MaxWait(10*time.Second))
	if err != nil && err != nats.ErrTimeout {
		return fmt.Errorf("failed to fetch messages: %w", err)
	}

	for _, msg := range msgs {
		var exec models.WorkflowExecution
		if err := json.Unmarshal(msg.Data, &exec); err != nil {
			m.logger.Warnf("Failed to unmarshal execution state: %v", err)
			msg.Nak()
			continue
		}

		if exec.Status == "running" {
			m.logger.Infof("Recovering incomplete execution: %s (workflow: %s)", exec.WorkflowID, exec.WorkflowName)
			handler(&exec)
		}
		msg.Ack()
	}

	return nil
}

// PublishPersistentAlert 发布持久化告警
func (m *JetStreamManager) PublishPersistentAlert(alert *models.Alert) error {
	subject := SubjectAlertsPersistent + alert.DeviceID
	data, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	_, err = m.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish alert: %w", err)
	}
	return nil
}

// PublishInferenceResult 发布推理结果（持久化）
func (m *JetStreamManager) PublishInferenceResult(result *models.InferenceResult) error {
	subject := SubjectInferenceResult + result.DeviceID
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal inference result: %w", err)
	}

	_, err = m.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish inference result: %w", err)
	}
	return nil
}

// PublishSensorData 持久化传感器数据
func (m *JetStreamManager) PublishSensorData(envelope *models.SensorEnvelope) error {
	subject := SubjectSensorPersist + envelope.DeviceID
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal sensor data: %w", err)
	}

	_, err = m.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish sensor data: %w", err)
	}
	return nil
}

// GetStreamInfo 获取 Stream 信息
func (m *JetStreamManager) GetStreamInfo(streamName string) (*nats.StreamInfo, error) {
	info, err := m.js.StreamInfo(streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info for %s: %w", streamName, err)
	}
	return info, nil
}
