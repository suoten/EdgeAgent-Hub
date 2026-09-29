package protocol

import (
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// BLEDeviceConfig BLE 设备配置
type BLEDeviceConfig struct {
	DeviceID     string        `yaml:"device_id" json:"device_id"`
	AdapterID    string        `yaml:"adapter_id" json:"adapter_id"` // hci0
	DeviceMAC    string        `yaml:"device_mac" json:"device_mac"`
	ServiceUUID  string        `yaml:"service_uuid" json:"service_uuid"`
	CharUUID     string        `yaml:"char_uuid" json:"char_uuid"` // 特征 UUID
	Timeout      time.Duration `yaml:"timeout" json:"timeout"`
	PollInterval time.Duration `yaml:"poll_interval" json:"poll_interval"`
}

// BLEClient BLE 蓝牙客户端
// 生产环境需链接 tinygo-org/bluetooth 或 go-ble/ble 库
// 此实现提供完整的框架和模拟数据回退
type BLEClient struct {
	config   BLEDeviceConfig
	logger   *logrus.Entry
	conn     bool
	mu       sync.RWMutex
	stopCh   chan struct{}
	handler  func(deviceID string, envelope models.SensorEnvelope)
	connected bool
}

// NewBLEClient 创建 BLE 客户端
func NewBLEClient(config BLEDeviceConfig, logger *logrus.Entry) *BLEClient {
	return &BLEClient{
		config: config,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// SetDataHandler 设置数据回调
func (c *BLEClient) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	c.handler = handler
}

// Connect 连接 BLE 设备
func (c *BLEClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 生产环境: 使用 bluetooth.Hub() 扫描并连接设备
	// adapter, err := bluetooth.DefaultAdapter.Enable()
	// device, err := adapter.Connect(bluetooth.Address{MACAddress: ...})
	// 此处为框架实现
	c.logger.Infof("BLE connecting to %s (%s)", c.config.DeviceID, c.config.DeviceMAC)

	// 模拟连接成功（实际部署时替换为真实 BLE 连接）
	c.connected = true
	c.logger.Infof("BLE connected: %s", c.config.DeviceID)
	return nil
}

// IsConnected 返回连接状态
func (c *BLEClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// StartPolling 开始轮询读取 BLE 特征值
func (c *BLEClient) StartPolling() {
	go func() {
		ticker := time.NewTicker(c.config.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				c.poll()
			}
		}
	}()
}

// poll 执行一次 BLE 数据读取
func (c *BLEClient) poll() {
	if !c.IsConnected() {
		if err := c.Connect(); err != nil {
			c.logger.Warnf("BLE reconnect failed for %s: %v", c.config.DeviceID, err)
			return
		}
	}

	// 生产环境: 读取 GATT 特征值
	// data, err := characteristic.Read()
	// 解析 data 为传感器值

	// 框架实现: 通过回调发送空信封
		envelope := models.SensorEnvelope{
			DeviceID:  c.config.DeviceID,
			Protocol:  "ble",
			Timestamp: time.Now().UnixMilli(),
			Metrics:   make(map[string]float64),
			Metadata: models.EnvelopeMetadata{
				Source:  c.config.DeviceMAC,
				Quality: "good",
				Extra:   make(map[string]any),
			},
		}

	if c.handler != nil {
		c.handler(c.config.DeviceID, envelope)
	}
}

// Stop 停止 BLE 客户端
func (c *BLEClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.stopCh)
	c.connected = false
	c.logger.Infof("BLE disconnected: %s", c.config.DeviceID)
}

// Disconnect 断开 BLE 连接
func (c *BLEClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
}
