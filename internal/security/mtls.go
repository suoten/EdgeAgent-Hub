package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// MTLSCertManager 管理 mTLS 双向认证证书
type MTLSCertManager struct {
	certFile string
	keyFile  string
	caFile   string
	cert     tls.Certificate
	caPool   *x509.CertPool
	mu       sync.RWMutex
	logger   *logrus.Entry
}

// NewMTLSCertManager 创建 mTLS 证书管理器
func NewMTLSCertManager(certFile, keyFile, caFile string, logger *logrus.Entry) (*MTLSCertManager, error) {
	mgr := &MTLSCertManager{
		certFile: certFile,
		keyFile:  keyFile,
		caFile:   caFile,
		logger:   logger,
	}

	if err := mgr.Load(); err != nil {
		return nil, err
	}

	return mgr, nil
}

// Load 加载证书和 CA
func (m *MTLSCertManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 加载服务端证书
	cert, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
	if err != nil {
		return fmt.Errorf("failed to load server cert/key: %w", err)
	}
	m.cert = cert

	// 加载 CA 证书
	caCert, err := os.ReadFile(m.caFile)
	if err != nil {
		return fmt.Errorf("failed to read CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("failed to parse CA cert")
	}
	m.caPool = caPool

	m.logger.Info("mTLS certificates loaded successfully")
	return nil
}

// GetTLSConfig 返回 TLS 配置（用于 HTTP 服务器）
func (m *MTLSCertManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &tls.Config{
		Certificates: []tls.Certificate{m.cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    m.caPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}

// GetClientTLSConfig 返回客户端 TLS 配置（用于 gRPC/HTTP 客户端连接）
func (m *MTLSCertManager) GetClientTLSConfig(serverName string) *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &tls.Config{
		Certificates: []tls.Certificate{m.cert},
		RootCAs:      m.caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}
}

// GetMQTTTLSConfig 返回 MQTT TLS 配置
func (m *MTLSCertManager) GetMQTTTLSConfig() *tls.Config {
	return m.GetTLSConfig()
}

// Reload 热重载证书
func (m *MTLSCertManager) Reload() error {
	m.logger.Info("Reloading mTLS certificates...")
	return m.Load()
}

// VerifyClientCert 验证客户端证书（可用于自定义验证逻辑）
func (m *MTLSCertManager) VerifyClientCert(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// Go TLS 库已自动验证证书链，这里可添加额外的自定义验证逻辑
	// 例如：检查证书的 CN/SAN 是否在允许列表中
	return nil
}
