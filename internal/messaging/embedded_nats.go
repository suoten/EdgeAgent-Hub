package messaging

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/sirupsen/logrus"
)

// EmbeddedNATS 是一个内嵌的 NATS 服务器
type EmbeddedNATS struct {
	natsServer *server.Server
	host       string
	port       int
	logger     *logrus.Entry
}

// NewEmbeddedNATS 创建并启动一个内嵌 NATS 服务器
func NewEmbeddedNATS(preferredPort int, logger *logrus.Entry) (*EmbeddedNATS, error) {
	port := preferredPort
	for attempts := 0; attempts < 10; attempts++ {
		if isPortAvailable(port) {
			break
		}
		logger.Infof("NATS port %d is in use, trying %d", port, port+1)
		port++
	}

	// 创建 NATS 服务器选项
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      port,
		JetStream:            true,
		StoreDir:             "data/nats-store",
		JetStreamMaxMemory:   256 * 1024 * 1024,
		JetStreamMaxStore:    1024 * 1024 * 1024,
		LogFile:   "",
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS server: %w", err)
	}

	// 启动服务器（非阻塞）
	go ns.Start()

	// 等待服务器就绪
	if !ns.ReadyForConnections(5 * time.Second) {
		return nil, fmt.Errorf("NATS server did not become ready within 5 seconds")
	}

	logger.Infof("Embedded NATS server started on 127.0.0.1:%d (JetStream enabled)", port)

	return &EmbeddedNATS{
		natsServer: ns,
		host:       "127.0.0.1",
		port:       port,
		logger:     logger,
	}, nil
}

// URL 返回 NATS 服务器 URL
func (n *EmbeddedNATS) URL() string {
	return fmt.Sprintf("nats://%s:%d", n.host, n.port)
}

// Close 关闭服务器
func (n *EmbeddedNATS) Close() {
	if n.natsServer != nil {
		n.natsServer.Shutdown()
	}
}

// isPortAvailable 已在 embedded_broker.go 中定义
var _ = strconv.Itoa
var _ = net.Listen
