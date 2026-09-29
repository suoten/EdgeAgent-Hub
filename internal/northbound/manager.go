package northbound

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/security"
	"github.com/sirupsen/logrus"
)

// Manager 管理北向上报: Webhook、MQTT 北向、数据脱敏
type Manager struct {
	webhookURL     string
	webhookTimeout time.Duration
	desensitizer   *security.Desensitizer
	logger         *logrus.Entry
	httpClient     *http.Client
}

// NewManager 创建北向管理器
func NewManager(webhookURL string, webhookTimeout time.Duration,
	desensitizer *security.Desensitizer, logger *logrus.Entry) *Manager {
	return &Manager{
		webhookURL:     webhookURL,
		webhookTimeout: webhookTimeout,
		desensitizer:   desensitizer,
		logger:         logger,
		httpClient: &http.Client{
			Timeout: webhookTimeout,
		},
	}
}

// WebhookPayload 是北向 Webhook 的标准载荷
type WebhookPayload struct {
	Event     string         `json:"event"`
	Timestamp string         `json:"timestamp"`
	DeviceID  string         `json:"device_id"`
	Alert     *AlertPayload  `json:"alert,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

type AlertPayload struct {
	Type        string   `json:"type"`
	Severity    string   `json:"severity"`
	Confidence  float64  `json:"confidence"`
	Message     string   `json:"message"`
	Anchored    bool     `json:"anchored"`
	Evidence    map[string]any `json:"evidence"`
}

// SendWebhook 发送 Webhook 通知
func (m *Manager) SendWebhook(ctx context.Context, payload *WebhookPayload) error {
	if m.webhookURL == "" {
		m.logger.Debug("Webhook URL not configured, skipping")
		return nil
	}

	// 数据脱敏
	if m.desensitizer != nil {
		payload.DeviceID = m.desensitizer.Desensitize(payload.DeviceID)
		if payload.Alert != nil {
			payload.Alert.Message = m.desensitizer.Desensitize(payload.Alert.Message)
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "EdgeAgent-Hub/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	m.logger.Debugf("Webhook sent: event=%s device=%s", payload.Event, payload.DeviceID)
	return nil
}

// SendAlertWebhook 发送告警 Webhook
func (m *Manager) SendAlertWebhook(ctx context.Context, event, deviceID string, alert *AlertPayload) error {
	payload := &WebhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		DeviceID:  deviceID,
		Alert:     alert,
	}
	return m.SendWebhook(ctx, payload)
}

// ── 钉钉通知 ──

// DingTalkNotifier 钉钉通知器
type DingTalkNotifier struct {
	webhookURL string
	logger     *logrus.Entry
	httpClient *http.Client
}

// NewDingTalkNotifier 创建钉钉通知器
func NewDingTalkNotifier(webhookURL string, logger *logrus.Entry) *DingTalkNotifier {
	if webhookURL == "" {
		return nil
	}
	return &DingTalkNotifier{
		webhookURL: webhookURL,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send 发送钉钉消息
func (d *DingTalkNotifier) Send(ctx context.Context, title, text string) error {
	if d == nil || d.webhookURL == "" {
		return nil
	}

	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  text,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dingtalk payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.webhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// ── SMS 通知 ──

// SMSNotifier SMS 通知器
type SMSNotifier struct {
	apiKey string
	apiURL string
	logger *logrus.Entry
	httpClient *http.Client
}

// NewSMSNotifier 创建 SMS 通知器
func NewSMSNotifier(apiKey, apiURL string, logger *logrus.Entry) *SMSNotifier {
	if apiKey == "" || apiURL == "" {
		return nil
	}
	return &SMSNotifier{
		apiKey: apiKey,
		apiURL: apiURL,
		logger: logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send 发送 SMS
func (s *SMSNotifier) Send(ctx context.Context, phone, message string) error {
	if s == nil || s.apiURL == "" {
		return nil
	}

	payload := map[string]string{
		"api_key": s.apiKey,
		"phone":   phone,
		"message": message,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SMS payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.apiURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SMS send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SMS API returned HTTP %d", resp.StatusCode)
	}

	return nil
}
