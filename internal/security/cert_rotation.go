package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// CertAutoRotation 证书自动轮换管理器
type CertAutoRotation struct {
	certManager *MTLSCertManager
	certFile    string
	keyFile     string
	caFile      string
	orgName     string
	checkInterval time.Duration
	stopCh      chan struct{}
	logger      *logrus.Entry
	mu          sync.Mutex
}

// NewCertAutoRotation 创建证书自动轮换器
func NewCertAutoRotation(certManager *MTLSCertManager, certFile, keyFile, caFile, orgName string, logger *logrus.Entry) *CertAutoRotation {
	return &CertAutoRotation{
		certManager:   certManager,
		certFile:      certFile,
		keyFile:       keyFile,
		caFile:        caFile,
		orgName:       orgName,
		checkInterval: 24 * time.Hour, // 每天检查一次
		stopCh:        make(chan struct{}),
		logger:        logger,
	}
}

// Start 启动自动轮换检查
func (c *CertAutoRotation) Start() {
	go func() {
		ticker := time.NewTicker(c.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				c.checkAndRotate()
			}
		}
	}()
	c.logger.Info("Certificate auto-rotation started")
}

// Stop 停止自动轮换
func (c *CertAutoRotation) Stop() {
	close(c.stopCh)
}

// checkAndRotate 检查证书有效期并在即将过期时轮换
func (c *CertAutoRotation) checkAndRotate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 读取当前证书
	certPEM, err := os.ReadFile(c.certFile)
	if err != nil {
		c.logger.Warnf("Failed to read cert for rotation check: %v", err)
		return
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		c.logger.Warn("Failed to decode cert PEM")
		return
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		c.logger.Warnf("Failed to parse cert: %v", err)
		return
	}

	// 如果证书在 30 天内过期，自动轮换
	daysUntilExpiry := time.Until(cert.NotAfter).Hours() / 24
	if daysUntilExpiry < 30 {
		c.logger.Infof("Certificate expires in %.0f days, auto-rotating...", daysUntilExpiry)
		if err := c.generateSelfSignedCert(); err != nil {
			c.logger.Errorf("Certificate auto-rotation failed: %v", err)
			return
		}
		if err := c.certManager.Reload(); err != nil {
			c.logger.Errorf("Failed to reload certificates after rotation: %v", err)
			return
		}
		c.logger.Info("Certificate auto-rotation completed successfully")
	}
}

// generateSelfSignedCert 生成新的自签名证书
func (c *CertAutoRotation) generateSelfSignedCert() error {
	// 生成 RSA-2048 密钥对
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// 证书序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	// 证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{c.orgName},
			CommonName:   "edgeagent-hub",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // 1 年有效期
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// 自签名
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// 确保目录存在
	certDir := filepath.Dir(c.certFile)
	os.MkdirAll(certDir, 0755)

	// 写入证书文件
	certOut, err := os.Create(c.certFile)
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	// 写入私钥文件
	keyOut, err := os.Create(c.keyFile)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()

	// CA 文件 = 证书文件（自签名）
	caOut, err := os.Create(c.caFile)
	if err != nil {
		return err
	}
	pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	caOut.Close()

	return nil
}

// GenerateInitialCerts 生成初始自签名证书（如果不存在）
func GenerateInitialCerts(certFile, keyFile, caFile, orgName string) error {
	if _, err := os.Stat(certFile); err == nil {
		return nil // 证书已存在
	}

	rotator := &CertAutoRotation{
		certFile: certFile,
		keyFile:  keyFile,
		caFile:   caFile,
		orgName:  orgName,
		logger:   logrus.NewEntry(logrus.New()),
	}
	return rotator.generateSelfSignedCert()
}
