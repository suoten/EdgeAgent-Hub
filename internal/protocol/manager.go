package protocol

import (
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// ProtocolManager 管理所有协议适配器
type ProtocolManager struct {
	modbusClients map[string]*ModbusClient
	opcuaClients  map[string]*OPCUAClient
	logger        *logrus.Entry
	mu            sync.RWMutex
	handler       func(deviceID string, envelope models.SensorEnvelope)
}

// NewProtocolManager 创建协议管理器
func NewProtocolManager(logger *logrus.Entry) *ProtocolManager {
	return &ProtocolManager{
		modbusClients: make(map[string]*ModbusClient),
		opcuaClients:  make(map[string]*OPCUAClient),
		logger:        logger,
	}
}

// SetDataHandler 设置统一数据回调
func (pm *ProtocolManager) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	pm.handler = handler
}

// PublishEnvelope 手动发布传感器信封（供 Webhook 等外部协议使用）
func (pm *ProtocolManager) PublishEnvelope(deviceID string, envelope models.SensorEnvelope) {
	if pm.handler != nil {
		pm.handler(deviceID, envelope)
	}
}

// AddModbusDevice 添加 Modbus TCP 设备
func (pm *ProtocolManager) AddModbusDevice(config ModbusDeviceConfig) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.modbusClients[config.DeviceID]; exists {
		return nil // 已存在，跳过
	}

	client := NewModbusClient(config, pm.logger)
	client.SetDataHandler(func(deviceID string, envelope models.SensorEnvelope) {
		if pm.handler != nil {
			pm.handler(deviceID, envelope)
		}
	})

	if err := client.Connect(); err != nil {
		pm.logger.Warnf("Failed to connect Modbus device %s: %v", config.DeviceID, err)
		// 连接失败也加入管理，后续轮询会自动重连
	}
	client.StartPolling()
	pm.modbusClients[config.DeviceID] = client
	pm.logger.Infof("Added Modbus device: %s (%s:%d)", config.DeviceID, config.Host, config.Port)
	return nil
}

// AddOPCUADevice 添加 OPC UA 设备
func (pm *ProtocolManager) AddOPCUADevice(config OPCUADeviceConfig) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.opcuaClients[config.DeviceID]; exists {
		return nil
	}

	client := NewOPCUAClient(config, pm.logger)
	client.SetDataHandler(func(deviceID string, envelope models.SensorEnvelope) {
		if pm.handler != nil {
			pm.handler(deviceID, envelope)
		}
	})

	if err := client.Connect(); err != nil {
		pm.logger.Warnf("Failed to connect OPC UA device %s: %v", config.DeviceID, err)
	}
	client.StartPolling()
	pm.opcuaClients[config.DeviceID] = client
	pm.logger.Infof("Added OPC UA device: %s (%s)", config.DeviceID, config.Endpoint)
	return nil
}

// RemoveDevice 移除设备
func (pm *ProtocolManager) RemoveDevice(deviceID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if client, exists := pm.modbusClients[deviceID]; exists {
		client.Stop()
		delete(pm.modbusClients, deviceID)
	}
	if client, exists := pm.opcuaClients[deviceID]; exists {
		client.Stop()
		delete(pm.opcuaClients, deviceID)
	}
}

// ListDevices 列出所有设备
func (pm *ProtocolManager) ListDevices() []ProtocolDeviceInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var devices []ProtocolDeviceInfo
	for id, client := range pm.modbusClients {
		devices = append(devices, ProtocolDeviceInfo{
			DeviceID:  id,
			Protocol:  "modbus_tcp",
			Connected: client.IsConnected(),
		})
	}
	for id, client := range pm.opcuaClients {
		devices = append(devices, ProtocolDeviceInfo{
			DeviceID:  id,
			Protocol:  "opcua",
			Connected: client.IsConnected(),
		})
	}
	return devices
}

// Stop 停止所有设备
func (pm *ProtocolManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, client := range pm.modbusClients {
		client.Stop()
	}
	for _, client := range pm.opcuaClients {
		client.Stop()
	}
	pm.modbusClients = make(map[string]*ModbusClient)
	pm.opcuaClients = make(map[string]*OPCUAClient)
}

// ProtocolDeviceInfo 协议设备信息
type ProtocolDeviceInfo struct {
	DeviceID  string `json:"device_id"`
	Protocol  string `json:"protocol"`
	Connected bool   `json:"connected"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}
