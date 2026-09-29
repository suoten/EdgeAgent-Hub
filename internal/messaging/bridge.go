package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// Bridge 实现 MQTT-NATS 双向桥接
// MQTT 消息自动桥接到 NATS subject (如 sensors/vibration/line1 → edgelite.sensors.vibration.line1)
// NATS 智能体响应可选择性桥接回 MQTT
type Bridge struct {
	natsConn *nats.Conn
	mqttClient mqtt.Client
	logger     *logrus.Entry

	// MQTT → NATS 桥接
	mqttToNATSEnabled bool
	// NATS → MQTT 桥接
	natsToMQTTEnabled bool

	// 消息计数器
	messagesIn  atomic.Int64
	messagesOut atomic.Int64

	// 订阅管理
	natsSubs   []*nats.Subscription
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewBridge 创建新的 MQTT-NATS 桥接器
func NewBridge(natsURL string, mqttBroker, mqttClientID, mqttUser, mqttPass string,
	mqttQoS int, mqttKeepAlive time.Duration, mqttCleanSession bool,
	mqttTopics []string, logger *logrus.Entry) (*Bridge, error) {

	ctx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		mqttToNATSEnabled: true,
		natsToMQTTEnabled: true,
		logger:            logger,
		ctx:               ctx,
		cancel:            cancel,
	}

	// 连接 NATS
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warnf("NATS disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Infof("NATS reconnected to %s", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Error("NATS connection closed")
		}),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to NATS %s: %w", natsURL, err)
	}
	b.natsConn = nc
	logger.Infof("NATS connected to %s", natsURL)

	// 连接 MQTT
	mqttOpts := mqtt.NewClientOptions()
	mqttOpts.AddBroker(mqttBroker)
	mqttOpts.SetClientID(mqttClientID)
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetCleanSession(mqttCleanSession)
	mqttOpts.SetKeepAlive(mqttKeepAlive)
	if mqttUser != "" {
		mqttOpts.SetUsername(mqttUser)
		mqttOpts.SetPassword(mqttPass)
	}
	mqttOpts.SetOnConnectHandler(func(c mqtt.Client) {
		logger.Infof("MQTT connected to %s", mqttBroker)
	})
	mqttOpts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		logger.Warnf("MQTT connection lost: %v", err)
	})
	mqttOpts.SetDefaultPublishHandler(func(c mqtt.Client, msg mqtt.Message) {
		// 默认处理: 桥接到 NATS
		b.bridgeMQTTToNATS(msg.Topic(), msg.Payload())
	})

	b.mqttClient = mqtt.NewClient(mqttOpts)
	if token := b.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		// MQTT 连接失败不阻止服务启动，自动重连会在后台尝试恢复
		logger.Warnf("MQTT initial connection failed: %v (will retry in background)", token.Error())
	} else {
		logger.Infof("MQTT connected to %s", mqttBroker)
	}

	// 订阅 MQTT 主题
	for _, topic := range mqttTopics {
		if token := b.mqttClient.Subscribe(topic, byte(mqttQoS), func(c mqtt.Client, msg mqtt.Message) {
			b.bridgeMQTTToNATS(msg.Topic(), msg.Payload())
		}); token.Wait() && token.Error() != nil {
			logger.Warnf("Failed to subscribe to MQTT topic %s: %v", topic, token.Error())
		} else {
			logger.Infof("Subscribed to MQTT topic: %s", topic)
		}
	}

	return b, nil
}

// bridgeMQTTToNATS 将 MQTT 消息桥接到 NATS
func (b *Bridge) bridgeMQTTToNATS(topic string, payload []byte) {
	if !b.mqttToNATSEnabled {
		return
	}

	b.messagesIn.Add(1)

	// MQTT topic → NATS subject: sensors/vibration/line1 → edgelite.sensors.vibration.line1
	subject := mqttTopicToNATSSubject(topic)

	// 发布到 NATS
	if err := b.natsConn.Publish(subject, payload); err != nil {
		b.logger.Errorf("Failed to bridge MQTT→NATS [%s→%s]: %v", topic, subject, err)
	} else {
		b.logger.Debugf("Bridged MQTT→NATS: %s → %s (%d bytes)", topic, subject, len(payload))
	}
}

// bridgeNATSToMQTT 将 NATS 消息桥接回 MQTT
func (b *Bridge) bridgeNATSToMQTT(subject string, payload []byte) {
	if !b.natsToMQTTEnabled {
		return
	}

	b.messagesOut.Add(1)

	// NATS subject → MQTT topic: edgelite.sensors.vibration.line1 → sensors/vibration/line1
	topic := natsSubjectToMQTTTopic(subject)

	if token := b.mqttClient.Publish(topic, 1, false, payload); token.Wait() && token.Error() != nil {
		b.logger.Errorf("Failed to bridge NATS→MQTT [%s→%s]: %v", subject, topic, token.Error())
	} else {
		b.logger.Debugf("Bridged NATS→MQTT: %s → %s (%d bytes)", subject, topic, len(payload))
	}
}

// PublishNATS 发布消息到 NATS
func (b *Bridge) PublishNATS(subject string, payload []byte) error {
	b.messagesOut.Add(1)
	return b.natsConn.Publish(subject, payload)
}

// PublishNATSJSON 发布 JSON 消息到 NATS
func (b *Bridge) PublishNATSJSON(subject string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return b.PublishNATS(subject, data)
}

// SubscribeNATS 订阅 NATS subject
func (b *Bridge) SubscribeNATS(subject string, handler func(msg *nats.Msg)) error {
	sub, err := b.natsConn.Subscribe(subject, handler)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NATS subject %s: %w", subject, err)
	}
	b.mu.Lock()
	b.natsSubs = append(b.natsSubs, sub)
	b.mu.Unlock()
	b.logger.Infof("Subscribed to NATS subject: %s", subject)
	return nil
}

// SubscribeNATSQueue 订阅 NATS queue group (负载均衡)
func (b *Bridge) SubscribeNATSQueue(subject, queue string, handler func(msg *nats.Msg)) error {
	sub, err := b.natsConn.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return fmt.Errorf("failed to queue subscribe to NATS subject %s: %w", subject, err)
	}
	b.mu.Lock()
	b.natsSubs = append(b.natsSubs, sub)
	b.mu.Unlock()
	b.logger.Infof("Queue subscribed to NATS subject: %s (queue: %s)", subject, queue)
	return nil
}

// RequestNATS 发送 NATS 请求并等待响应
func (b *Bridge) RequestNATS(subject string, payload []byte, timeout time.Duration) (*nats.Msg, error) {
	return b.natsConn.Request(subject, payload, timeout)
}

// PublishMQTT 发布 MQTT 消息
func (b *Bridge) PublishMQTT(topic string, qos byte, payload []byte) error {
	token := b.mqttClient.Publish(topic, qos, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// SubscribeMQTT 订阅 MQTT 主题
func (b *Bridge) SubscribeMQTT(topic string, qos byte, callback mqtt.MessageHandler) error {
	token := b.mqttClient.Subscribe(topic, qos, callback)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// NATSConn 返回底层 NATS 连接
func (b *Bridge) NATSConn() *nats.Conn {
	return b.natsConn
}

// IsNATSConnected 返回 NATS 连接状态
func (b *Bridge) IsNATSConnected() bool {
	return b.natsConn != nil && b.natsConn.IsConnected()
}

// IsMQTTConnected 返回 MQTT 连接状态
func (b *Bridge) IsMQTTConnected() bool {
	return b.mqttClient != nil && b.mqttClient.IsConnected()
}

// MessagesIn 返回入站消息计数
func (b *Bridge) MessagesIn() int64 {
	return b.messagesIn.Load()
}

// MessagesOut 返回出站消息计数
func (b *Bridge) MessagesOut() int64 {
	return b.messagesOut.Load()
}

// SetNATSToMQTTEnabled 启用/禁用 NATS→MQTT 桥接
func (b *Bridge) SetNATSToMQTTEnabled(enabled bool) {
	b.natsToMQTTEnabled = enabled
}

// SetMQTTToNATSEnabled 启用/禁用 MQTT→NATS 桥接
func (b *Bridge) SetMQTTToNATSEnabled(enabled bool) {
	b.mqttToNATSEnabled = enabled
}

// Close 关闭桥接器
func (b *Bridge) Close() {
	b.cancel()

	b.mu.Lock()
	for _, sub := range b.natsSubs {
		sub.Unsubscribe()
	}
	b.natsSubs = nil
	b.mu.Unlock()

	if b.natsConn != nil {
		b.natsConn.Close()
	}
	if b.mqttClient != nil {
		b.mqttClient.Disconnect(250)
	}
}

// mqttTopicToNATSSubject 将 MQTT topic 转换为 NATS subject
// 例: sensors/vibration/line1 → edgelite.sensors.vibration.line1
func mqttTopicToNATSSubject(topic string) string {
	// MQTT 通配符 + → * (NATS 单级通配符)
	// MQTT 通配符 # → > (NATS 多级通配符)
	topic = strings.ReplaceAll(topic, "+", "*")
	topic = strings.ReplaceAll(topic, "#", ">")
	return "edgelite." + strings.ReplaceAll(topic, "/", ".")
}

// natsSubjectToMQTTTopic 将 NATS subject 转换为 MQTT topic
// 例: edgelite.sensors.vibration.line1 → sensors/vibration/line1
func natsSubjectToMQTTTopic(subject string) string {
	// 去掉 edgelite. 前缀
	if strings.HasPrefix(subject, "edgelite.") {
		subject = strings.TrimPrefix(subject, "edgelite.")
	}
	// NATS 通配符 * → +, > → #
	subject = strings.ReplaceAll(subject, "*", "+")
	subject = strings.ReplaceAll(subject, ">", "#")
	return strings.ReplaceAll(subject, ".", "/")
}
