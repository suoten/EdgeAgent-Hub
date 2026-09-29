package messaging

import (
	"fmt"
	"net"
	"strconv"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/sirupsen/logrus"
)

// EmbeddedBroker 是一个内嵌的 MQTT broker，用于在没有外部 MQTT broker 时提供消息服务
type EmbeddedBroker struct {
	server   *mochi.Server
	tcpPort  int
	logger   *logrus.Entry
}

// NewEmbeddedBroker 创建并启动一个内嵌 MQTT broker
// 如果指定端口被占用则自动尝试下一个端口
func NewEmbeddedBroker(preferredPort int, logger *logrus.Entry) (*EmbeddedBroker, error) {
	port := preferredPort
	for attempts := 0; attempts < 10; attempts++ {
		if isPortAvailable(port) {
			break
		}
		logger.Infof("Port %d is in use, trying %d", port, port+1)
		port++
	}

	server := mochi.New(nil)

	// 允许匿名访问
	allowHook := &auth.AllowHook{}
	_ = server.AddHook(allowHook, nil)

	// 创建 TCP listener
	tcpListener := listeners.NewTCP(listeners.Config{
		Type:    "tcp",
		ID:      "embedded-tcp",
		Address: fmt.Sprintf(":%d", port),
	})
	if err := server.AddListener(tcpListener); err != nil {
		return nil, fmt.Errorf("failed to add TCP listener on port %d: %w", port, err)
	}

	// 启动 broker（非阻塞）
	go func() {
		if err := server.Serve(); err != nil {
			logger.Errorf("Embedded MQTT broker error: %v", err)
		}
	}()

	logger.Infof("Embedded MQTT broker started on port %d", port)

	return &EmbeddedBroker{
		server:  server,
		tcpPort: port,
		logger:  logger,
	}, nil
}

// Port 返回实际监听的端口
func (b *EmbeddedBroker) Port() int {
	return b.tcpPort
}

// BrokerURL 返回 MQTT broker URL
func (b *EmbeddedBroker) BrokerURL() string {
	return fmt.Sprintf("tcp://127.0.0.1:%d", b.tcpPort)
}

// Close 关闭 broker
func (b *EmbeddedBroker) Close() {
	if b.server != nil {
		_ = b.server.Close()
	}
}

// isPortAvailable 检查端口是否可用
func isPortAvailable(port int) bool {
	addr := ":" + strconv.Itoa(port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
