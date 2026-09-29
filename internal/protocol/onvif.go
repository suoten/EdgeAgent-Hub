package protocol

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// ONVIFDeviceConfig ONVIF 摄像头配置
type ONVIFDeviceConfig struct {
	DeviceID     string        `yaml:"device_id" json:"device_id"`
	URL          string        `yaml:"url" json:"url"`             // http://192.168.1.100/onvif/device_service
	Username     string        `yaml:"username" json:"username"`
	Password     string        `yaml:"password" json:"password"`
	Timeout      time.Duration `yaml:"timeout" json:"timeout"`
	PollInterval time.Duration `yaml:"poll_interval" json:"poll_interval"`
	// PTZ 控制
	HasPTZ       bool          `yaml:"has_ptz" json:"has_ptz"`
	// 视频流
	RTSPURL      string        `yaml:"rtsp_url" json:"rtsp_url"`
}

// ONVIFClient ONVIF 摄像头客户端
// 生产环境可链接 github.com/usepool/onvif 或 gosoap
type ONVIFClient struct {
	config    ONVIFDeviceConfig
	logger    *logrus.Entry
	mu        sync.RWMutex
	stopCh    chan struct{}
	handler   func(deviceID string, envelope models.SensorEnvelope)
	connected bool
	httpClient *http.Client
}

// NewONVIFClient 创建 ONVIF 客户端
func NewONVIFClient(config ONVIFDeviceConfig, logger *logrus.Entry) *ONVIFClient {
	return &ONVIFClient{
		config: config,
		logger: logger,
		stopCh: make(chan struct{}),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// SetDataHandler 设置数据回调
func (c *ONVIFClient) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	c.handler = handler
}

// Connect 连接 ONVIF 设备
func (c *ONVIFClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 生产环境: 发送 WS-Discovery 探测 + 设备能力请求
	// 此处验证 URL 可达
	resp, err := c.httpClient.Get(c.config.URL)
	if err != nil {
		c.logger.Warnf("ONVIF connect failed for %s: %v", c.config.DeviceID, err)
		return err
	}
	resp.Body.Close()

	c.connected = true
	c.logger.Infof("ONVIF connected: %s (%s)", c.config.DeviceID, c.config.URL)
	return nil
}

// IsConnected 返回连接状态
func (c *ONVIFClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// StartPolling 开始轮询 ONVIF 设备状态
func (c *ONVIFClient) StartPolling() {
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

// poll 执行一次 ONVIF 轮询
func (c *ONVIFClient) poll() {
	if !c.IsConnected() {
		if err := c.Connect(); err != nil {
			return
		}
	}

	// 生产环境: 调用 GetStatus / GetEvent
	// 简化: 发送设备状态信封
	envelope := models.SensorEnvelope{
		DeviceID:  c.config.DeviceID,
		Protocol:  "onvif",
		Timestamp: time.Now().UnixMilli(),
		Metrics:   make(map[string]float64),
		Metadata: models.EnvelopeMetadata{
			Source:  c.config.URL,
			Quality: "good",
			Extra: map[string]any{
				"rtsp_url": c.config.RTSPURL,
			},
		},
	}

	if c.handler != nil {
		c.handler(c.config.DeviceID, envelope)
	}
}

// PTZControl PTZ 控制（生产环境实现）
func (c *ONVIFClient) PTZControl(pan, tilt, zoom float64) error {
	if !c.config.HasPTZ {
		return fmt.Errorf("PTZ not supported by device %s", c.config.DeviceID)
	}
	// 生产环境: 发送 ONVIF PTZ SOAP 请求
	c.logger.Debugf("PTZ control: %s pan=%.2f tilt=%.2f zoom=%.2f", c.config.DeviceID, pan, tilt, zoom)
	return nil
}

// Snapshot 获取快照 URL
func (c *ONVIFClient) Snapshot() (string, error) {
	// 生产环境: 调用 GetSnapshotURI
	return fmt.Sprintf("%s/snapshot.jpg", c.config.URL), nil
}

// Stop 停止 ONVIF 客户端
func (c *ONVIFClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.stopCh)
	c.connected = false
	c.logger.Infof("ONVIF disconnected: %s", c.config.DeviceID)
}
