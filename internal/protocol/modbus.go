package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// ModbusFuncCode 定义 Modbus 功能码
type ModbusFuncCode uint8

const (
	FuncReadCoils          ModbusFuncCode = 0x01
	FuncReadDiscreteInputs ModbusFuncCode = 0x02
	FuncReadHoldingRegs    ModbusFuncCode = 0x03
	FuncReadInputRegs      ModbusFuncCode = 0x04
)

// ModbusRegister 定义一个寄存器映射
type ModbusRegister struct {
	Name      string  `yaml:"name" json:"name"`
	Address   uint16  `yaml:"address" json:"address"`
	Quantity  uint16  `yaml:"quantity" json:"quantity"`
	FuncCode  uint8   `yaml:"func_code" json:"func_code"`
	DataType  string  `yaml:"data_type" json:"data_type"` // uint16, int16, uint32, int32, float32, float64
	Scale     float64 `yaml:"scale" json:"scale"`
	Offset    float64 `yaml:"offset" json:"offset"`
	Unit      string  `yaml:"unit" json:"unit"`
}

// ModbusDeviceConfig 定义一个 Modbus 设备配置
type ModbusDeviceConfig struct {
	DeviceID     string           `yaml:"device_id" json:"device_id"`
	Host         string           `yaml:"host" json:"host"`
	Port         int              `yaml:"port" json:"port"`
	UnitID       uint8            `yaml:"unit_id" json:"unit_id"`
	Timeout      time.Duration    `yaml:"timeout" json:"timeout"`
	PollInterval time.Duration    `yaml:"poll_interval" json:"poll_interval"`
	Registers    []ModbusRegister `yaml:"registers" json:"registers"`
}

// ModbusClient 是一个生产级 Modbus TCP 客户端
type ModbusClient struct {
	config    ModbusDeviceConfig
	conn      net.Conn
	mu        sync.Mutex
	logger    *logrus.Entry
	connected bool
	stopCh    chan struct{}
	handler   func(deviceID string, envelope models.SensorEnvelope)
}

// NewModbusClient 创建 Modbus TCP 客户端
func NewModbusClient(config ModbusDeviceConfig, logger *logrus.Entry) *ModbusClient {
	if config.Port == 0 {
		config.Port = 502
	}
	if config.Timeout == 0 {
		config.Timeout = 3 * time.Second
	}
	if config.PollInterval == 0 {
		config.PollInterval = 1 * time.Second
	}
	return &ModbusClient{
		config: config,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// SetDataHandler 设置数据回调
func (c *ModbusClient) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	c.handler = handler
}

// Connect 连接 Modbus TCP 设备
func (c *ModbusClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	addr := net.JoinHostPort(c.config.Host, fmt.Sprintf("%d", c.config.Port))
	conn, err := net.DialTimeout("tcp", addr, c.config.Timeout)
	if err != nil {
		return fmt.Errorf("modbus connect %s failed: %w", addr, err)
	}
	c.conn = conn
	c.connected = true
	c.logger.Infof("Modbus TCP connected to %s (device: %s)", addr, c.config.DeviceID)
	return nil
}

// Disconnect 断开连接
func (c *ModbusClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	return nil
}

// IsConnected 返回连接状态
func (c *ModbusClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// StartPolling 启动轮询
func (c *ModbusClient) StartPolling() {
	go c.pollLoop()
}

// Stop 停止轮询和连接
func (c *ModbusClient) Stop() {
	close(c.stopCh)
	c.Disconnect()
}

// pollLoop 轮询循环
func (c *ModbusClient) pollLoop() {
	ticker := time.NewTicker(c.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				if err := c.Connect(); err != nil {
					c.logger.Warnf("Modbus reconnect failed: %v", err)
					continue
				}
			}
			envelope, err := c.ReadAll()
			if err != nil {
				c.logger.Warnf("Modbus read failed for %s: %v", c.config.DeviceID, err)
				c.Disconnect()
				continue
			}
			if c.handler != nil && envelope != nil {
				c.handler(c.config.DeviceID, *envelope)
			}
		}
	}
}

// ReadAll 读取所有配置的寄存器
func (c *ModbusClient) ReadAll() (*models.SensorEnvelope, error) {
	metrics := make(map[string]float64)
	units := make(map[string]string)

	for _, reg := range c.config.Registers {
		values, err := c.ReadRegisters(reg.Address, reg.Quantity, reg.FuncCode)
		if err != nil {
			c.logger.Debugf("Modbus read register %s (addr=%d) failed: %v", reg.Name, reg.Address, err)
			continue
		}

		val, err := decodeRegister(values, reg.DataType, reg.Scale, reg.Offset)
		if err != nil {
			c.logger.Warnf("Modbus decode register %s failed: %v", reg.Name, err)
			continue
		}
		metrics[reg.Name] = val
		if reg.Unit != "" {
			units[reg.Name] = reg.Unit
		}
	}

	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics read from device %s", c.config.DeviceID)
	}

	return &models.SensorEnvelope{
		DeviceID:  c.config.DeviceID,
		Protocol:  "modbus_tcp",
		Timestamp: time.Now().UnixMilli(),
		Metrics:   metrics,
		Metadata: models.EnvelopeMetadata{
			Unit:    units,
			Quality: "good",
			Source:  fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		},
	}, nil
}

// ReadRegisters 读取寄存器 (Modbus TCP)
func (c *ModbusClient) ReadRegisters(address, quantity uint16, funcCode uint8) ([]uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// 构建 Modbus TCP ADU
	// Transaction ID (2) + Protocol ID (2) + Length (2) + Unit ID (1) + FuncCode (1) + Address (2) + Quantity (2)
	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], 1)         // Transaction ID
	binary.BigEndian.PutUint16(req[2:], 0)         // Protocol ID (0 = Modbus)
	binary.BigEndian.PutUint16(req[4:], 6)         // Length (6 bytes follow)
	req[6] = c.config.UnitID                       // Unit ID
	req[7] = funcCode                              // Function Code
	binary.BigEndian.PutUint16(req[8:], address)   // Starting Address
	binary.BigEndian.PutUint16(req[10:], quantity) // Quantity

	c.conn.SetDeadline(time.Now().Add(c.config.Timeout))

	if _, err := c.conn.Write(req); err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}

	// 读取响应头 (MBAP Header: 7 bytes)
	header := make([]byte, 7)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, fmt.Errorf("read header failed: %w", err)
	}

	txnID := binary.BigEndian.Uint16(header[0:2])
	if txnID != 1 {
		return nil, fmt.Errorf("transaction ID mismatch: %d", txnID)
	}

	length := binary.BigEndian.Uint16(header[4:6])
	unitID := header[6]
	if unitID != c.config.UnitID {
		return nil, fmt.Errorf("unit ID mismatch: %d", unitID)
	}

	// 读取剩余响应
	remaining := int(length) - 1 // 减去已读的 UnitID
	respBody := make([]byte, remaining)
	if _, err := io.ReadFull(c.conn, respBody); err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	// 检查异常响应
	if respBody[0]&0x80 != 0 {
		if len(respBody) >= 2 {
			return nil, fmt.Errorf("modbus exception code: %d", respBody[1])
		}
		return nil, fmt.Errorf("modbus exception response")
	}

	// respBody[0] = function code
	// respBody[1] = byte count
	// respBody[2:] = data
	if len(respBody) < 2 {
		return nil, fmt.Errorf("response too short")
	}
	byteCount := int(respBody[1])
	data := respBody[2:]
	if len(data) < byteCount {
		return nil, fmt.Errorf("data length mismatch: expected %d, got %d", byteCount, len(data))
	}

	// 解析为 uint16 数组
	values := make([]uint16, byteCount/2)
	for i := 0; i < len(values); i++ {
		values[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}

	return values, nil
}

// decodeRegister 将寄存器值解码为目标数据类型
func decodeRegister(values []uint16, dataType string, scale, offset float64) (float64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("no values to decode")
	}

	var raw float64
	switch dataType {
	case "uint16":
		raw = float64(values[0])
	case "int16":
		v := int16(values[0])
		raw = float64(v)
	case "uint32":
		if len(values) < 2 {
			return 0, fmt.Errorf("uint32 requires 2 registers")
		}
		raw = float64(uint32(values[0])<<16 | uint32(values[1]))
	case "int32":
		if len(values) < 2 {
			return 0, fmt.Errorf("int32 requires 2 registers")
		}
		v := int32(uint32(values[0])<<16 | uint32(values[1]))
		raw = float64(v)
	case "float32":
		if len(values) < 2 {
			return 0, fmt.Errorf("float32 requires 2 registers")
		}
		bits := uint32(values[0])<<16 | uint32(values[1])
		raw = float64(math.Float32frombits(bits))
	case "float64":
		if len(values) < 4 {
			return 0, fmt.Errorf("float64 requires 4 registers")
		}
		bits := uint64(values[0])<<48 | uint64(values[1])<<32 | uint64(values[2])<<16 | uint64(values[3])
		raw = math.Float64frombits(bits)
	default:
		raw = float64(values[0])
	}

	// 应用缩放和偏移: value = raw * scale + offset
	if scale == 0 {
		scale = 1
	}
	return raw*scale + offset, nil
}
