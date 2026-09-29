package protocol

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// OPCUANodeConfig 定义一个 OPC UA 节点配置
type OPCUANodeConfig struct {
	NodeID    string  `yaml:"node_id" json:"node_id"`
	Name      string  `yaml:"name" json:"name"`
	Unit      string  `yaml:"unit" json:"unit"`
	Scale     float64 `yaml:"scale" json:"scale"`
	Offset    float64 `yaml:"offset" json:"offset"`
	Deadband  float64 `yaml:"deadband" json:"deadband"`
}

// OPCUADeviceConfig 定义一个 OPC UA 设备配置
type OPCUADeviceConfig struct {
	DeviceID      string            `yaml:"device_id" json:"device_id"`
	Endpoint      string            `yaml:"endpoint" json:"endpoint"`
	SecurityMode  string            `yaml:"security_mode" json:"security_mode"` // none, sign, signandencrypt
	Policy        string            `yaml:"policy" json:"policy"`
	CertFile      string            `yaml:"cert_file" json:"cert_file"`
	KeyFile       string            `yaml:"key_file" json:"key_file"`
	AuthMode      string            `yaml:"auth_mode" json:"auth_mode"` // anonymous, username
	Username      string            `yaml:"username" json:"username"`
	Password      string            `yaml:"password" json:"password"`
	Timeout       time.Duration     `yaml:"timeout" json:"timeout"`
	PollInterval  time.Duration     `yaml:"poll_interval" json:"poll_interval"`
	Nodes         []OPCUANodeConfig `yaml:"nodes" json:"nodes"`
}

// OPCUAClient 是一个 OPC UA 客户端适配器
// 注意: 这是一个抽象层，实际 OPC UA 通信通过外部 sidecar 或 gopcua 库实现
// 在生产环境中，此处使用 gopcua (github.com/gopcua/opcua)
// 为避免 CGO 依赖，这里提供接口抽象和模拟实现，实际部署时替换为 gopcua 实现
type OPCUAClient struct {
	config    OPCUADeviceConfig
	logger    *logrus.Entry
	connected bool
	mu        sync.Mutex
	stopCh    chan struct{}
	handler   func(deviceID string, envelope models.SensorEnvelope)

	// lastValues 用于死区过滤
	lastValues map[string]float64
}

// NewOPCUAClient 创建 OPC UA 客户端
func NewOPCUAClient(config OPCUADeviceConfig, logger *logrus.Entry) *OPCUAClient {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.PollInterval == 0 {
		config.PollInterval = 2 * time.Second
	}
	if config.SecurityMode == "" {
		config.SecurityMode = "none"
	}
	if config.AuthMode == "" {
		config.AuthMode = "anonymous"
	}
	return &OPCUAClient{
		config:     config,
		logger:     logger,
		stopCh:     make(chan struct{}),
		lastValues: make(map[string]float64),
	}
}

// SetDataHandler 设置数据回调
func (c *OPCUAClient) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	c.handler = handler
}

// Connect 连接 OPC UA 服务器
func (c *OPCUAClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// 在生产环境中，这里会使用 gopcua 建立 OPC UA 会话
	// 例:
	//   opts := []opcua.Option{
	//       opcua.SecurityPolicy(c.config.Policy),
	//       opcua.SecurityMode(c.config.SecurityMode),
	//       opcua.AuthMode(c.config.AuthMode),
	//   }
	//   client := opcua.NewClient(c.config.Endpoint, opts...)
	//   if err := client.Connect(ctx); err != nil { return err }

	c.logger.Infof("OPC UA connecting to %s (device: %s, security: %s)", c.config.Endpoint, c.config.DeviceID, c.config.SecurityMode)
	c.connected = true
	return nil
}

// Disconnect 断开 OPC UA 连接
func (c *OPCUAClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	return nil
}

// IsConnected 返回连接状态
func (c *OPCUAClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// StartPolling 启动轮询
func (c *OPCUAClient) StartPolling() {
	go c.pollLoop()
}

// Stop 停止
func (c *OPCUAClient) Stop() {
	close(c.stopCh)
	c.Disconnect()
}

// pollLoop 轮询循环
func (c *OPCUAClient) pollLoop() {
	ticker := time.NewTicker(c.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				if err := c.Connect(); err != nil {
					c.logger.Warnf("OPC UA reconnect failed: %v", err)
					continue
				}
			}
			envelope, err := c.ReadAll()
			if err != nil {
				c.logger.Warnf("OPC UA read failed for %s: %v", c.config.DeviceID, err)
				continue
			}
			if c.handler != nil && envelope != nil {
				c.handler(c.config.DeviceID, *envelope)
			}
		}
	}
}

// ReadAll 读取所有配置的节点
func (c *OPCUAClient) ReadAll() (*models.SensorEnvelope, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics := make(map[string]float64)
	units := make(map[string]string)
	changed := false

	for _, node := range c.config.Nodes {
		// 在生产环境中，这里会通过 gopcua 读取节点值:
		//   nodeID := opcua.ParseNodeID(node.NodeID)
		//   resp, err := c.client.Read(ctx, &ua.ReadRequest{
		//       NodesToRead: []*ua.ReadValueID{{NodeID: nodeID}},
		//   })
		//   val := resp.Results[0].Value.Value().(float64)

		// 当前实现: 跳过未连接的实际读取
		// 在 gopcua 集成后替换为真实读取
		val, err := c.readNode(node)
		if err != nil {
			c.logger.Debugf("OPC UA read node %s failed: %v", node.Name, err)
			continue
		}

		// 死区过滤
		if node.Deadband > 0 {
			if lastVal, ok := c.lastValues[node.Name]; ok {
				diff := val - lastVal
				if diff < 0 {
					diff = -diff
				}
				if diff < node.Deadband {
					continue // 在死区内，跳过
				}
			}
		}

		c.lastValues[node.Name] = val
		metrics[node.Name] = val
		if node.Unit != "" {
			units[node.Name] = node.Unit
		}
		changed = true
	}

	if !changed {
		return nil, nil
	}

	return &models.SensorEnvelope{
		DeviceID:  c.config.DeviceID,
		Protocol:  "opcua",
		Timestamp: time.Now().UnixMilli(),
		Metrics:   metrics,
		Metadata: models.EnvelopeMetadata{
			Unit:    units,
			Quality: "good",
			Source:  c.config.Endpoint,
		},
	}, nil
}

// readNode 读取单个 OPC UA 节点
// 在生产部署时，此方法将调用 gopcua 实际读取
func (c *OPCUAClient) readNode(node OPCUANodeConfig) (float64, error) {
	// TODO: 集成 gopcua 库进行实际 OPC UA 读取
	// 当前返回错误以表明需要实际 OPC UA 服务器
	_ = context.Background()
	return 0, fmt.Errorf("OPC UA server not connected at %s", c.config.Endpoint)
}
