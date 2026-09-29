package models

import (
	"time"
)

// ── 标准信封格式 (SensorEnvelope) ──
// 所有南向数据统一为标准信封格式, 见规划文档 9.2

type SensorEnvelope struct {
	DeviceID  string                 `json:"device_id"`
	Protocol  string                 `json:"protocol"`
	Timestamp int64                  `json:"timestamp"`
	Metrics   map[string]float64     `json:"metrics"`
	Metadata  EnvelopeMetadata        `json:"metadata"`
}

type EnvelopeMetadata struct {
	Unit   map[string]string `json:"unit"`
	Quality string           `json:"quality"`
	Source string           `json:"source"`
	Extra  map[string]any   `json:"extra,omitempty"`
}

// ── 推理结果 ──

type InferenceResult struct {
	ModelID    string                 `json:"model_id"`
	AgentID    string                 `json:"agent_id"`
	DeviceID   string                 `json:"device_id"`
	Timestamp  int64                  `json:"timestamp"`
	Confidence float64                `json:"confidence"`
	Prediction string                 `json:"prediction"`
	Outputs    map[string]any         `json:"outputs,omitempty"`
	LatencyMS  float64                `json:"latency_ms"`
}

// ── 告警 ──

type AlertSeverity string

const (
	AlertSeverityLow    AlertSeverity = "LOW"
	AlertSeverityMedium AlertSeverity = "MEDIUM"
	AlertSeverityHigh   AlertSeverity = "HIGH"
	AlertSeverityCritical AlertSeverity = "CRITICAL"
)

type AlertStatus string

const (
	AlertStatusActive    AlertStatus = "active"
	AlertStatusAcked     AlertStatus = "acked"
	AlertStatusResolved  AlertStatus = "resolved"
)

type Alert struct {
	ID          string         `json:"id"`
	WorkflowID  string         `json:"workflow_id"`
	DeviceID    string         `json:"device_id"`
	Type        string         `json:"type"`
	Severity    AlertSeverity  `json:"severity"`
	Confidence  float64        `json:"confidence"`
	Message     string         `json:"message"`
	Anchored    bool           `json:"anchored"`
	Evidence    AlertEvidence  `json:"evidence"`
	Status      AlertStatus    `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	AckedAt     *time.Time     `json:"acked_at,omitempty"`
	AckedBy     string         `json:"acked_by,omitempty"`
}

type AlertEvidence struct {
	SensorValue    float64  `json:"sensor_value"`
	Threshold      float64  `json:"threshold"`
	VisionConfirmed bool    `json:"vision_confirmed"`
	RAGSources     []string `json:"rag_sources"`
}

// ── 工作流 (编排 DSL) ──

type Workflow struct {
	Name      string         `yaml:"name" json:"name"`
	Trigger   TriggerDef     `yaml:"trigger" json:"trigger"`
	Steps     []StepDef      `yaml:"steps" json:"steps"`
}

type TriggerDef struct {
	Source    string `yaml:"source" json:"source"`       // mqtt / nats / manual / schedule
	Topic     string `yaml:"topic" json:"topic"`         // MQTT topic 或 NATS subject
	Condition string `yaml:"condition" json:"condition"` // 条件表达式
}

type StepDef struct {
	ID         string         `yaml:"id" json:"id"`
	Agent      string         `yaml:"agent" json:"agent"`
	Input      any            `yaml:"input" json:"input"`
	Output     string         `yaml:"output" json:"output"`
	Timeout    string         `yaml:"timeout" json:"timeout"`
	Condition  string         `yaml:"condition" json:"condition"`
	OnTimeout  string         `yaml:"on_timeout" json:"on_timeout"`   // skip / retry / abort
	OnFailure  any            `yaml:"on_failure" json:"on_failure"`   // continue / abort / {agent: ...}
	Parallel   bool           `yaml:"parallel" json:"parallel"`
}

// WorkflowExecution 是一次编排执行的状态
type WorkflowExecution struct {
	WorkflowID  string                 `json:"workflow_id"`
	WorkflowName string                `json:"workflow_name"`
	Status      string                 `json:"status"`         // running / completed / failed / timeout
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Steps       map[string]StepResult  `json:"steps"`
	TriggerData map[string]any         `json:"trigger_data"`
	Error       string                 `json:"error,omitempty"`
}

type StepResult struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"` // pending / running / completed / failed / skipped / timeout
	Output     any            `json:"output"`
	Error      string         `json:"error,omitempty"`
	StartTime  time.Time      `json:"start_time"`
	EndTime    *time.Time     `json:"end_time,omitempty"`
	DurationMS float64        `json:"duration_ms"`
}

// ── 智能体注册 ──

type AgentType string

const (
	AgentTypeONNX    AgentType = "onnx"
	AgentTypeLLM     AgentType = "llm"
	AgentTypeVision  AgentType = "vision"
	AgentTypeRAG     AgentType = "rag"
	AgentTypeNotify  AgentType = "notify"
	AgentTypeTemplate AgentType = "template"
)

type AgentInfo struct {
	ID           string    `json:"id"`
	Type         AgentType `json:"type"`
	Endpoint     string    `json:"endpoint"`
	Capabilities []string  `json:"capabilities"`
	Status       string    `json:"status"`  // online / offline / degraded
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Weight       int       `json:"weight"`
}

// ── 模型管理 ──

type ModelInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Type         string    `json:"type"`  // onnx / gguf
	FilePath     string    `json:"file_path"`
	Partition    string    `json:"partition"`  // A / B
	Active       bool      `json:"active"`
	SizeBytes    int64     `json:"size_bytes"`
	SHA256       string    `json:"sha256"`
	LoadedAt     *time.Time `json:"loaded_at,omitempty"`
}

// ── 审计日志 ──

type AuditEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`     // 用户名或系统
	Action    string    `json:"action"`    // 操作类型
	Resource  string    `json:"resource"`  // 操作对象
	Detail    string    `json:"detail"`
	IP        string    `json:"ip"`
}

// ── 离线队列消息 ──

type QueuedMessage struct {
	ID        int64     `json:"id"`
	Topic     string    `json:"topic"`     // NATS subject 或 MQTT topic
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
	Retries   int       `json:"retries"`
	Status    string    `json:"status"`    // pending / sending / sent / failed
}

// ── 用户 (RBAC) ──

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
}

// ── OTA 更新任务 ──

type OTAStatus string

const (
	OTAStatusPending   OTAStatus = "pending"
	OTAStatusDownloading OTAStatus = "downloading"
	OTAStatusVerifying  OTAStatus = "verifying"
	OTAStatusCanary     OTAStatus = "canary"
	OTAStatusSwitching  OTAStatus = "switching"
	OTAStatusComplete   OTAStatus = "complete"
	OTAStatusFailed     OTAStatus = "failed"
	OTAStatusRolledBack OTAStatus = "rolled_back"
)

type OTATask struct {
	ID           string     `json:"id"`
	ModelName    string     `json:"model_name"`
	Version      string     `json:"version"`
	TargetPartition string  `json:"target_partition"`
	Status       OTAStatus  `json:"status"`
	SHA256       string     `json:"sha256"`
	SizeBytes    int64      `json:"size_bytes"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// ── RAG 检索结果 ──

type RAGSearchResult struct {
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Score      float64 `json:"score"`
}

type RAGAnchoredResponse struct {
	Message      string            `json:"message"`
	Anchored     bool              `json:"anchored"`
	Evidence     map[string]any    `json:"evidence"`
	RAGSources   []string          `json:"rag_sources"`
	FallbackUsed bool              `json:"fallback_used"`
}

// ── 系统健康 ──

type HealthStatus struct {
	Status       string            `json:"status"`  // healthy / degraded / unhealthy
	NATSConnected bool            `json:"nats_connected"`
	MQTTConnected bool            `json:"mqtt_connected"`
	ONNXAvailable bool            `json:"onnx_available"`
	LLMAvailable  bool            `json:"llm_available"`
	RAGAvailable  bool            `json:"rag_available"`
	OfflineMode   bool            `json:"offline_mode"`
	Components    map[string]string `json:"components"`
	Timestamp     time.Time        `json:"timestamp"`
}

// ── 对话请求 ──

type ChatRequest struct {
	Message   string `json:"message"`
	Context   string `json:"context,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`
}

type ChatResponse struct {
	Response  string   `json:"response"`
	Sources   []string `json:"sources,omitempty"`
	Anchored  bool     `json:"anchored"`
}
