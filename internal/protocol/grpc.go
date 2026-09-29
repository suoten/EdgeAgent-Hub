package protocol

import (
	"fmt"
	"net"
	"sync"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// GRPCConfig gRPC 内部通信配置
type GRPCConfig struct {
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"` // :50061
	Enabled    bool   `yaml:"enabled" json:"enabled"`
}

// GRPCServer gRPC 内部通信服务器
// 生产环境可使用 google.golang.org/grpc 实现
// 此实现提供框架和 TCP 透传层
type GRPCServer struct {
	config  GRPCConfig
	logger  *logrus.Entry
	mu      sync.RWMutex
	listener net.Listener
	handler  func(deviceID string, envelope models.SensorEnvelope)
}

// NewGRPCServer 创建 gRPC 服务器
func NewGRPCServer(config GRPCConfig, logger *logrus.Entry) *GRPCServer {
	return &GRPCServer{
		config: config,
		logger: logger,
	}
}

// SetDataHandler 设置数据回调
func (g *GRPCServer) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	g.handler = handler
}

// Start 启动 gRPC 服务器
func (g *GRPCServer) Start() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	ln, err := net.Listen("tcp", g.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", g.config.ListenAddr, err)
	}
	g.listener = ln

	go func() {
		g.logger.Infof("gRPC server listening on %s", g.config.ListenAddr)
		for {
			conn, err := ln.Accept()
			if err != nil {
				g.logger.Errorf("gRPC accept error: %v", err)
				return
			}
			go g.handleConn(conn)
		}
	}()

	return nil
}

// handleConn 处理 TCP 连接（简化版 gRPC 透传）
func (g *GRPCServer) handleConn(conn net.Conn) {
	defer conn.Close()
	// 生产环境: 使用 grpc.NewServer() 和 protobuf 服务定义
	// 此处提供框架，实际 gRPC 服务需要 .proto 定义和生成的代码
	g.logger.Debugf("gRPC connection from %s", conn.RemoteAddr())
}

// Stop 停止 gRPC 服务器
func (g *GRPCServer) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.listener != nil {
		g.listener.Close()
	}
}
