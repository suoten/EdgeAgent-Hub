package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是 EdgeAgent Hub 的全局配置结构
type Config struct {
	Server         ServerConfig         `yaml:"server"`
	NATS           NATSConfig           `yaml:"nats"`
	MQTT           MQTTConfig           `yaml:"mqtt"`
	Storage        StorageConfig        `yaml:"storage"`
	Inference      InferenceConfig      `yaml:"inference"`
	Orchestrator   OrchestratorConfig   `yaml:"orchestrator"`
	Security       SecurityConfig       `yaml:"security"`
	OTA            OTAConfig            `yaml:"ota"`
	Northbound     NorthboundConfig     `yaml:"northbound"`
	Observability  ObservabilityConfig  `yaml:"observability"`
	Offline        OfflineConfig        `yaml:"offline"`
	Resource       ResourceConfig       `yaml:"resource"`
	Protocols      ProtocolsConfig      `yaml:"protocols"`
}

type ServerConfig struct {
	Host         string   `yaml:"host"`
	Port         int      `yaml:"port"`
	ReadTimeout  Duration `yaml:"read_timeout"`
	WriteTimeout Duration `yaml:"write_timeout"`
	TLS          TLSConfig `yaml:"tls"`
	CORSOrigins  []string `yaml:"cors_origins"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

type NATSConfig struct {
	URL           string   `yaml:"url"`
	CredsFile     string   `yaml:"creds_file"`
	MaxMemory     string   `yaml:"max_memory"`
	MaxStore      string   `yaml:"max_store"`
	MaxReconnects int      `yaml:"max_reconnects"`
	ReconnectWait Duration `yaml:"reconnect_wait"`
}

type MQTTConfig struct {
	Broker       string   `yaml:"broker"`
	ClientID     string   `yaml:"client_id"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	QoS          int      `yaml:"qos"`
	KeepAlive    Duration `yaml:"keepalive"`
	CleanSession bool     `yaml:"clean_session"`
	SubscribeTopics []string `yaml:"subscribe_topics"`
	TLS          TLSConfig `yaml:"tls"`
}

type StorageConfig struct {
	DBPath             string `yaml:"db_path"`
	WALMode            bool   `yaml:"wal_mode"`
	MaxConnections     int    `yaml:"max_connections"`
	RetentionDays      int    `yaml:"retention_days"`
	AuditRetentionDays int    `yaml:"audit_retention_days"`
	OfflineQueueMax    int    `yaml:"offline_queue_max"`
}

type InferenceConfig struct {
	ONNX ONNXConfig `yaml:"onnx"`
	LLM  LLMConfig  `yaml:"llm"`
	RAG  RAGConfig  `yaml:"rag"`
}

type ONNXConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Endpoint         string   `yaml:"endpoint"`
	ModelDir         string   `yaml:"model_dir"`
	ExecutionProvider string  `yaml:"execution_provider"`
	BatchWindow      Duration `yaml:"batch_window"`
	MaxBatchSize     int      `yaml:"max_batch_size"`
}

type LLMConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Endpoint     string   `yaml:"endpoint"`
	ModelPath    string   `yaml:"model_path"`
	ContextSize  int      `yaml:"context_size"`
	MaxTokens    int      `yaml:"max_tokens"`
	Temperature  float64  `yaml:"temperature"`
	GPULayers    int      `yaml:"gpu_layers"`

	// 远程 LLM API 配置 (OpenAI 兼容接口)
	// 支持通义千问/DeepSeek/Ollama/OpenAI 等
	APIEnabled   bool     `yaml:"api_enabled"`
	APIBaseURL   string   `yaml:"api_base_url"`
	APIKey       string   `yaml:"api_key"`
	APIModel     string   `yaml:"api_model"`
	APITimeout   Duration `yaml:"api_timeout"`
	APISystemPrompt string `yaml:"api_system_prompt"`
}

type RAGConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Endpoint         string   `yaml:"endpoint"`
	KnowledgeDir     string   `yaml:"knowledge_dir"`
	VectordbDir      string   `yaml:"vectordb_dir"`
	EmbeddingModel   string   `yaml:"embedding_model"`
	EmbeddingDim     int      `yaml:"embedding_dim"`
	TopK             int      `yaml:"top_k"`
	AnchorThreshold  float64  `yaml:"anchor_threshold"`
}

type OrchestratorConfig struct {
	WorkflowDir        string   `yaml:"workflow_dir"`
	MaxConcurrent      int      `yaml:"max_concurrent"`
	DefaultTimeout     Duration `yaml:"default_timeout"`
	HeartbeatInterval  Duration `yaml:"heartbeat_interval"`
	HealthCheckTimeout Duration `yaml:"health_check_timeout"`
}

type SecurityConfig struct {
	JWTSecret          string            `yaml:"jwt_secret"`
	AccessTokenTTL     Duration          `yaml:"access_token_ttl"`
	RefreshTokenTTL    Duration          `yaml:"refresh_token_ttl"`
	JWTAlgorithm       string            `yaml:"jwt_algorithm"`
	MaxLoginAttempts   int               `yaml:"max_login_attempts"`
	LockoutDuration    Duration          `yaml:"lockout_duration"`
	RateLimitPerMinute int               `yaml:"rate_limit_per_minute"`
	ModelSigningPubKey string            `yaml:"model_signing_pubkey"`
	ModelSigningKey    string            `yaml:"model_signing_key"`
	Desensitize        DesensitizeConfig `yaml:"desensitize"`
	Admin              AdminConfig       `yaml:"admin"`
}

type DesensitizeConfig struct {
	Enabled bool     `yaml:"enabled"`
	Patterns []string `yaml:"patterns"`
}

type AdminConfig struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type OTAConfig struct {
	PartitionA      string `yaml:"partition_a"`
	PartitionB      string `yaml:"partition_b"`
	ActivePartition string `yaml:"active_partition"`
	RetentionDays   int    `yaml:"retention_days"`
	CanaryPercent   int    `yaml:"canary_percent"`
	RegistryURL     string `yaml:"registry_url"`
	RegistryUsername string `yaml:"registry_username"`
	RegistryPassword string `yaml:"registry_password"`
}

type NorthboundConfig struct {
	WebhookURL     string           `yaml:"webhook_url"`
	WebhookTimeout Duration         `yaml:"webhook_timeout"`
	MQTT           NorthboundMQTT   `yaml:"mqtt"`
	Notify         NotifyConfig     `yaml:"notify"`
}

type NorthboundMQTT struct {
	Enabled    bool   `yaml:"enabled"`
	Broker     string `yaml:"broker"`
	TopicPrefix string `yaml:"topic_prefix"`
}

type NotifyConfig struct {
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	SMS      SMSConfig      `yaml:"sms"`
}

type DingTalkConfig struct {
	Enabled bool   `yaml:"enabled"`
	Webhook string `yaml:"webhook"`
}

type SMSConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	APIURL  string `yaml:"api_url"`
}

type ObservabilityConfig struct {
	LogLevel             string   `yaml:"log_level"`
	LogFormat            string   `yaml:"log_format"`
	LogFile              string   `yaml:"log_file"`
	LogMaxSize           int      `yaml:"log_max_size"`
	LogMaxDays           int      `yaml:"log_max_days"`
	MetricsPath          string   `yaml:"metrics_path"`
	CollectSystemMetrics bool     `yaml:"collect_system_metrics"`
	MetricsInterval      Duration `yaml:"metrics_interval"`
}

type OfflineConfig struct {
	Enabled           bool     `yaml:"enabled"`
	CheckInterval     Duration `yaml:"check_interval"`
	CheckURL          string   `yaml:"check_url"`
	RetransmitInterval Duration `yaml:"retransmit_interval"`
	MaxRetries        int      `yaml:"max_retries"`
}

type ResourceConfig struct {
	AutoDetect         bool          `yaml:"auto_detect"`
	MinMemoryForLLM    int64         `yaml:"min_memory_for_llm"`
	MinMemoryForVision int64         `yaml:"min_memory_for_vision"`
	RAMTiers           RAMTierConfig `yaml:"ram_tiers"`
}

type RAMTierConfig struct {
	Minimal  int64 `yaml:"minimal"`
	Basic    int64 `yaml:"basic"`
	Standard int64 `yaml:"standard"`
	Full     int64 `yaml:"full"`
}

// ── 协议接入配置 ──

type ProtocolsConfig struct {
	Modbus  ModbusProtocolConfig  `yaml:"modbus"`
	OPCUA   OPCUAProtocolConfig   `yaml:"opcua"`
	BLE     BLEProtocolConfig     `yaml:"ble"`
	ONVIF   ONVIFProtocolConfig   `yaml:"onvif"`
	Webhook WebhookProtocolConfig `yaml:"webhook"`
}

type ModbusProtocolConfig struct {
	Enabled bool                    `yaml:"enabled"`
	Devices []ModbusDeviceConfig    `yaml:"devices"`
}

type ModbusDeviceConfig struct {
	DeviceID     string             `yaml:"device_id" json:"device_id"`
	Host         string             `yaml:"host" json:"host"`
	Port         int                `yaml:"port" json:"port"`
	UnitID       uint8              `yaml:"unit_id" json:"unit_id"`
	Timeout      Duration           `yaml:"timeout" json:"timeout"`
	PollInterval Duration           `yaml:"poll_interval" json:"poll_interval"`
	Registers    []ModbusRegister   `yaml:"registers" json:"registers"`
}

type ModbusRegister struct {
	Name      string  `yaml:"name" json:"name"`
	Address   uint16  `yaml:"address" json:"address"`
	Quantity  uint16  `yaml:"quantity" json:"quantity"`
	FuncCode  uint8   `yaml:"func_code" json:"func_code"`
	DataType  string  `yaml:"data_type" json:"data_type"`
	Scale     float64 `yaml:"scale" json:"scale"`
	Offset    float64 `yaml:"offset" json:"offset"`
	Unit      string  `yaml:"unit" json:"unit"`
}

type OPCUAProtocolConfig struct {
	Enabled bool                  `yaml:"enabled"`
	Devices []OPCUADeviceConfig   `yaml:"devices"`
}

type OPCUADeviceConfig struct {
	DeviceID     string            `yaml:"device_id" json:"device_id"`
	Endpoint     string            `yaml:"endpoint" json:"endpoint"`
	SecurityMode string            `yaml:"security_mode" json:"security_mode"`
	Policy       string            `yaml:"policy" json:"policy"`
	CertFile     string            `yaml:"cert_file" json:"cert_file"`
	KeyFile      string            `yaml:"key_file" json:"key_file"`
	AuthMode     string            `yaml:"auth_mode" json:"auth_mode"`
	Username     string            `yaml:"username" json:"username"`
	Password     string            `yaml:"password" json:"password"`
	Timeout      Duration          `yaml:"timeout" json:"timeout"`
	PollInterval Duration          `yaml:"poll_interval" json:"poll_interval"`
	Nodes        []OPCUANodeConfig `yaml:"nodes" json:"nodes"`
}

type OPCUANodeConfig struct {
	NodeID   string  `yaml:"node_id" json:"node_id"`
	Name     string  `yaml:"name" json:"name"`
	Unit     string  `yaml:"unit" json:"unit"`
	Scale    float64 `yaml:"scale" json:"scale"`
	Offset   float64 `yaml:"offset" json:"offset"`
	Deadband float64 `yaml:"deadband" json:"deadband"`
}

type BLEProtocolConfig struct {
	Enabled bool                `yaml:"enabled"`
	Devices []BLEDeviceConfig   `yaml:"devices"`
}

type BLEDeviceConfig struct {
	DeviceID     string             `yaml:"device_id" json:"device_id"`
	AdapterID    string             `yaml:"adapter_id" json:"adapter_id"`
	DeviceMAC    string             `yaml:"device_mac" json:"device_mac"`
	ServiceUUID  string             `yaml:"service_uuid" json:"service_uuid"`
	CharUUID     string             `yaml:"char_uuid" json:"char_uuid"`
	Timeout      Duration           `yaml:"timeout" json:"timeout"`
	PollInterval Duration           `yaml:"poll_interval" json:"poll_interval"`
}

type ONVIFProtocolConfig struct {
	Enabled bool                 `yaml:"enabled"`
	Devices []ONVIFDeviceConfig  `yaml:"devices"`
}

type ONVIFDeviceConfig struct {
	DeviceID     string             `yaml:"device_id" json:"device_id"`
	URL          string             `yaml:"url" json:"url"`
	Username     string             `yaml:"username" json:"username"`
	Password     string             `yaml:"password" json:"password"`
	Timeout      Duration           `yaml:"timeout" json:"timeout"`
	PollInterval Duration           `yaml:"poll_interval" json:"poll_interval"`
	HasPTZ       bool               `yaml:"has_ptz" json:"has_ptz"`
	RTSPURL      string             `yaml:"rtsp_url" json:"rtsp_url"`
}

type WebhookProtocolConfig struct {
	Enabled    bool     `yaml:"enabled"`
	ListenAddr string   `yaml:"listen_addr"`
	Path       string   `yaml:"path"`
	AuthSecret string   `yaml:"auth_secret"`
	AllowedIPs []string `yaml:"allowed_ips"`
}

// Duration 包装 time.Duration 以支持 YAML 序列化
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// Load 从文件路径加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	cfg.applyDefaults()

	// 如果 JWT 密钥仍是默认值或空，自动生成随机密钥并回写配置文件
	if cfg.Security.JWTSecret == "" || cfg.Security.JWTSecret == "CHANGE_ME_IN_PRODUCTION_USE_A_LONG_RANDOM_STRING" {
		newSecret, err := generateRandomSecret(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		cfg.Security.JWTSecret = newSecret
		if err := cfg.saveToFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: generated JWT secret but failed to write back to config: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "INFO: JWT secret auto-generated and saved to config file")
		}
	}

	return cfg, nil
}

// applyDefaults 为未设置的配置项填充默认值
func (c *Config) applyDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeout.Duration == 0 {
		c.Server.ReadTimeout = Duration{30 * time.Second}
	}
	if c.Server.WriteTimeout.Duration == 0 {
		c.Server.WriteTimeout = Duration{30 * time.Second}
	}

	if c.NATS.URL == "" {
		c.NATS.URL = "nats://127.0.0.1:4222"
	}
	if c.NATS.MaxReconnects == 0 {
		c.NATS.MaxReconnects = -1
	}
	if c.NATS.ReconnectWait.Duration == 0 {
		c.NATS.ReconnectWait = Duration{2 * time.Second}
	}

	if c.MQTT.Broker == "" {
		c.MQTT.Broker = "tcp://127.0.0.1:1883"
	}
	if c.MQTT.ClientID == "" {
		c.MQTT.ClientID = "edgeagent-hub"
	}
	if c.MQTT.QoS == 0 {
		c.MQTT.QoS = 1
	}
	if c.MQTT.KeepAlive.Duration == 0 {
		c.MQTT.KeepAlive = Duration{60 * time.Second}
	}

	if c.Storage.DBPath == "" {
		c.Storage.DBPath = "data/edgeagent.db"
	}
	if c.Storage.MaxConnections == 0 {
		c.Storage.MaxConnections = 10
	}
	if c.Storage.RetentionDays == 0 {
		c.Storage.RetentionDays = 30
	}
	if c.Storage.AuditRetentionDays == 0 {
		c.Storage.AuditRetentionDays = 180
	}
	if c.Storage.OfflineQueueMax == 0 {
		c.Storage.OfflineQueueMax = 10000
	}

	if c.Inference.ONNX.Endpoint == "" {
		c.Inference.ONNX.Endpoint = "http://127.0.0.1:50052"
	}
	if c.Inference.ONNX.ModelDir == "" {
		c.Inference.ONNX.ModelDir = "models/onnx"
	}
	if c.Inference.ONNX.ExecutionProvider == "" {
		c.Inference.ONNX.ExecutionProvider = "cpu"
	}
	if c.Inference.ONNX.MaxBatchSize == 0 {
		c.Inference.ONNX.MaxBatchSize = 8
	}

	if c.Inference.LLM.Endpoint == "" {
		c.Inference.LLM.Endpoint = "http://127.0.0.1:50054"
	}
	if c.Inference.LLM.ContextSize == 0 {
		c.Inference.LLM.ContextSize = 2048
	}
	if c.Inference.LLM.MaxTokens == 0 {
		c.Inference.LLM.MaxTokens = 512
	}
	if c.Inference.LLM.Temperature == 0 {
		c.Inference.LLM.Temperature = 0.3
	}
	if c.Inference.LLM.APITimeout.Duration == 0 {
		c.Inference.LLM.APITimeout = Duration{30 * time.Second}
	}
	if c.Inference.LLM.APISystemPrompt == "" {
		c.Inference.LLM.APISystemPrompt = "你是一个工业设备运维告警助手。根据传感器数据和检索到的知识，生成准确、简洁的告警消息和运维建议。引用数据必须与提供的传感器读数一致，不要编造数据。"
	}

	if c.Inference.RAG.Endpoint == "" {
		c.Inference.RAG.Endpoint = "http://127.0.0.1:50055"
	}
	if c.Inference.RAG.KnowledgeDir == "" {
		c.Inference.RAG.KnowledgeDir = "data/knowledge"
	}
	if c.Inference.RAG.VectordbDir == "" {
		c.Inference.RAG.VectordbDir = "data/vectordb"
	}
	if c.Inference.RAG.EmbeddingModel == "" {
		c.Inference.RAG.EmbeddingModel = "all-MiniLM-L6-v2"
	}
	if c.Inference.RAG.EmbeddingDim == 0 {
		c.Inference.RAG.EmbeddingDim = 384
	}
	if c.Inference.RAG.TopK == 0 {
		c.Inference.RAG.TopK = 5
	}
	if c.Inference.RAG.AnchorThreshold == 0 {
		c.Inference.RAG.AnchorThreshold = 0.85
	}

	if c.Orchestrator.WorkflowDir == "" {
		c.Orchestrator.WorkflowDir = "workflows"
	}
	if c.Orchestrator.MaxConcurrent == 0 {
		c.Orchestrator.MaxConcurrent = 100
	}
	if c.Orchestrator.DefaultTimeout.Duration == 0 {
		c.Orchestrator.DefaultTimeout = Duration{5 * time.Second}
	}
	if c.Orchestrator.HeartbeatInterval.Duration == 0 {
		c.Orchestrator.HeartbeatInterval = Duration{10 * time.Second}
	}
	if c.Orchestrator.HealthCheckTimeout.Duration == 0 {
		c.Orchestrator.HealthCheckTimeout = Duration{30 * time.Second}
	}

	// JWTSecret 为空时使用默认占位符，Load() 函数会在之后自动生成随机密钥
	if c.Security.JWTSecret == "" {
		c.Security.JWTSecret = "CHANGE_ME_IN_PRODUCTION_USE_A_LONG_RANDOM_STRING"
	}
	if c.Security.AccessTokenTTL.Duration == 0 {
		c.Security.AccessTokenTTL = Duration{30 * time.Minute}
	}
	if c.Security.RefreshTokenTTL.Duration == 0 {
		c.Security.RefreshTokenTTL = Duration{168 * time.Hour}
	}
	if c.Security.JWTAlgorithm == "" {
		c.Security.JWTAlgorithm = "HS256"
	}
	if c.Security.MaxLoginAttempts == 0 {
		c.Security.MaxLoginAttempts = 5
	}
	if c.Security.LockoutDuration.Duration == 0 {
		c.Security.LockoutDuration = Duration{15 * time.Minute}
	}
	if c.Security.RateLimitPerMinute == 0 {
		c.Security.RateLimitPerMinute = 60
	}
	if c.Security.Admin.Username == "" {
		c.Security.Admin.Username = "admin"
	}

	if c.OTA.PartitionA == "" {
		c.OTA.PartitionA = "models/partition_a"
	}
	if c.OTA.PartitionB == "" {
		c.OTA.PartitionB = "models/partition_b"
	}
	if c.OTA.ActivePartition == "" {
		c.OTA.ActivePartition = "A"
	}
	if c.OTA.RetentionDays == 0 {
		c.OTA.RetentionDays = 7
	}
	if c.OTA.CanaryPercent == 0 {
		c.OTA.CanaryPercent = 10
	}

	if c.Northbound.WebhookTimeout.Duration == 0 {
		c.Northbound.WebhookTimeout = Duration{10 * time.Second}
	}

	if c.Observability.LogLevel == "" {
		c.Observability.LogLevel = "info"
	}
	if c.Observability.LogFormat == "" {
		c.Observability.LogFormat = "json"
	}
	if c.Observability.LogFile == "" {
		c.Observability.LogFile = "logs/edgeagent.log"
	}
	if c.Observability.LogMaxSize == 0 {
		c.Observability.LogMaxSize = 10
	}
	if c.Observability.LogMaxDays == 0 {
		c.Observability.LogMaxDays = 7
	}
	if c.Observability.MetricsPath == "" {
		c.Observability.MetricsPath = "/metrics"
	}
	// CORSOrigins 为空时不限制（同源），配置后仅允许白名单域名
	if len(c.Server.CORSOrigins) == 0 {
		c.Server.CORSOrigins = []string{"*"} // 默认允许同源请求
	}

	if c.Observability.MetricsInterval.Duration == 0 {
		c.Observability.MetricsInterval = Duration{15 * time.Second}
	}

	if c.Offline.CheckInterval.Duration == 0 {
		c.Offline.CheckInterval = Duration{30 * time.Second}
	}
	if c.Offline.RetransmitInterval.Duration == 0 {
		c.Offline.RetransmitInterval = Duration{10 * time.Second}
	}
	if c.Offline.MaxRetries == 0 {
		c.Offline.MaxRetries = 5
	}

	if c.Resource.MinMemoryForLLM == 0 {
		c.Resource.MinMemoryForLLM = 1024
	}
	if c.Resource.MinMemoryForVision == 0 {
		c.Resource.MinMemoryForVision = 4096
	}
	if c.Resource.RAMTiers.Minimal == 0 {
		c.Resource.RAMTiers.Minimal = 512
	}
	if c.Resource.RAMTiers.Basic == 0 {
		c.Resource.RAMTiers.Basic = 1024
	}
	if c.Resource.RAMTiers.Standard == 0 {
		c.Resource.RAMTiers.Standard = 4096
	}
	if c.Resource.RAMTiers.Full == 0 {
		c.Resource.RAMTiers.Full = 8192
	}
}

// generateRandomSecret 生成加密安全的随机十六进制密钥
func generateRandomSecret(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// saveToFile 将配置回写到 YAML 文件（保留用户手动编辑的其他字段）
func (c *Config) saveToFile(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
