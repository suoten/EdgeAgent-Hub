package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  host: "127.0.0.1"
  port: 9090
nats:
  url: "nats://localhost:4222"
mqtt:
  broker: "tcp://localhost:1883"
  client_id: "test-hub"
storage:
  db_path: "data/test.db"
  retention_days: 7
inference:
  onnx:
    enabled: true
    endpoint: "http://localhost:50052"
    execution_provider: "cpu"
orchestrator:
  workflow_dir: "workflows"
security:
  jwt_secret: "test-secret"
  admin:
    username: "admin"
observability:
  log_level: "debug"
  log_format: "text"
`

	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.NATS.URL != "nats://localhost:4222" {
		t.Errorf("expected NATS URL nats://localhost:4222, got %s", cfg.NATS.URL)
	}
}

func TestDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 空配置，测试默认值
	os.WriteFile(configPath, []byte("{}"), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Storage.RetentionDays != 30 {
		t.Errorf("expected default retention 30, got %d", cfg.Storage.RetentionDays)
	}
	if cfg.Security.JWTAlgorithm != "HS256" {
		t.Errorf("expected default algorithm HS256, got %s", cfg.Security.JWTAlgorithm)
	}
}

func TestDurationUnmarshal(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  read_timeout: 45s
  write_timeout: 60s
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.ReadTimeout.Duration != 45*time.Second {
		t.Errorf("expected 45s, got %v", cfg.Server.ReadTimeout.Duration)
	}
	if cfg.Server.WriteTimeout.Duration != 60*time.Second {
		t.Errorf("expected 60s, got %v", cfg.Server.WriteTimeout.Duration)
	}
}
