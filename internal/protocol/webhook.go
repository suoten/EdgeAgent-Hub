package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// WebhookConfig HTTP Webhook 南向接入配置
type WebhookConfig struct {
	ListenAddr  string   `yaml:"listen_addr" json:"listen_addr"`   // :50060
	Path        string   `yaml:"path" json:"path"`                 // /webhook/data
	AllowedIPs  []string `yaml:"allowed_ips" json:"allowed_ips"`   // IP 白名单
	AuthHeader  string   `yaml:"auth_header" json:"auth_header"`   // X-Webhook-Key
	AuthSecret  string   `yaml:"auth_secret" json:"auth_secret"`   // 共享密钥
}

// WebhookReceiver HTTP Webhook 南向接收器
// 监听 HTTP POST 请求，将数据标准化为 SensorEnvelope 后转发
type WebhookReceiver struct {
	config     WebhookConfig
	logger     *logrus.Entry
	mu         sync.RWMutex
	handler    func(deviceID string, envelope models.SensorEnvelope)
	httpServer *http.Server
}

// NewWebhookReceiver 创建 Webhook 接收器
func NewWebhookReceiver(config WebhookConfig, logger *logrus.Entry) *WebhookReceiver {
	return &WebhookReceiver{
		config: config,
		logger: logger,
	}
}

// SetDataHandler 设置数据回调
func (w *WebhookReceiver) SetDataHandler(handler func(deviceID string, envelope models.SensorEnvelope)) {
	w.handler = handler
}

// Start 启动 HTTP Webhook 监听
func (w *WebhookReceiver) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc(w.config.Path, w.handleWebhook)

	w.httpServer = &http.Server{
		Addr:    w.config.ListenAddr,
		Handler: mux,
	}

	go func() {
		w.logger.Infof("Webhook receiver listening on %s%s", w.config.ListenAddr, w.config.Path)
		if err := w.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			w.logger.Errorf("Webhook server error: %v", err)
		}
	}()

	return nil
}

// Stop 停止 Webhook 接收器
func (w *WebhookReceiver) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.httpServer.Shutdown(ctx)
	}
}

// handleWebhook 处理 Webhook 请求
func (w *WebhookReceiver) handleWebhook(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 鉴权
	if w.config.AuthSecret != "" {
		key := req.Header.Get(w.config.AuthHeader)
		if key != w.config.AuthSecret {
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// IP 白名单检查
	if len(w.config.AllowedIPs) > 0 {
		clientIP := req.RemoteAddr
		allowed := false
		for _, ip := range w.config.AllowedIPs {
			if ip == clientIP {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(rw, "forbidden", http.StatusForbidden)
			return
		}
	}

	// 解析请求体
	var envelope models.SensorEnvelope
	if err := json.NewDecoder(req.Body).Decode(&envelope); err != nil {
		// 尝试作为原始 JSON 解析
		var raw map[string]any
		if err2 := json.NewDecoder(req.Body).Decode(&raw); err2 != nil {
			http.Error(rw, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		// 手动构建信封
		envelope = w.parseRawPayload(raw)
	}

	if envelope.DeviceID == "" {
		envelope.DeviceID = "webhook.device"
	}
	if envelope.Protocol == "" {
		envelope.Protocol = "http_webhook"
	}
	if envelope.Timestamp == 0 {
		envelope.Timestamp = time.Now().UnixMilli()
	}
	if envelope.Metrics == nil {
		envelope.Metrics = make(map[string]float64)
	}
	envelope.Metadata.Source = req.RemoteAddr
	envelope.Metadata.Quality = "good"

	if w.handler != nil {
		w.handler(envelope.DeviceID, envelope)
	}

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte(`{"status":"received"}`))
}

// parseRawPayload 将原始 JSON 解析为 SensorEnvelope
func (w *WebhookReceiver) parseRawPayload(raw map[string]any) models.SensorEnvelope {
	envelope := models.SensorEnvelope{
		Metrics:  make(map[string]float64),
		Metadata: models.EnvelopeMetadata{
			Extra: make(map[string]any),
		},
	}

	if v, ok := raw["device_id"].(string); ok {
		envelope.DeviceID = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		envelope.Timestamp = int64(v)
	}

	// 解析 metrics
	if metrics, ok := raw["metrics"].(map[string]any); ok {
		for k, v := range metrics {
			if f, ok := v.(float64); ok {
				envelope.Metrics[k] = f
			}
		}
	} else {
		// 将顶层所有数字字段视为 metrics
		for k, v := range raw {
			if f, ok := v.(float64); ok {
				if k != "device_id" && k != "timestamp" {
					envelope.Metrics[k] = f
				}
			}
		}
	}

	return envelope
}
