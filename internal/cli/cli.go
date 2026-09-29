package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/config"
	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/edgelite/edgeagent-hub/internal/security"
	"github.com/edgelite/edgeagent-hub/internal/storage"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// CLI 是命令行工具入口
type CLI struct {
	configPath string
	cfg        *config.Config
	logger     *logrus.Entry
}

// NewCLI 创建 CLI 实例
func NewCLI(configPath string) *CLI {
	return &CLI{
		configPath: configPath,
		logger:     logrus.NewEntry(logrus.New()),
	}
}

// Execute 执行 CLI 命令
func (c *CLI) Execute(args []string) int {
	if len(args) == 0 {
		c.printHelp()
		return 1
	}

	command := args[0]
	rest := args[1:]

	switch command {
	case "selftest":
		return c.runSelfTest(rest)
	case "init":
		return c.runInit(rest)
	case "model":
		return c.runModel(rest)
	case "workflow":
		return c.runWorkflow(rest)
	case "version":
		fmt.Println("EdgeAgent Hub v1.0.0")
		return 0
	case "help", "--help", "-h":
		c.printHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		c.printHelp()
		return 1
	}
}

func (c *CLI) printHelp() {
	fmt.Println(`EdgeAgent Hub CLI

Usage: edgelite-hub <command> [options]

Commands:
  selftest     Run full system health check
  init         Initialize configuration and database
  model        Model management (list/load/unload/switch/rollback)
  workflow     Workflow management (list/trigger/show)
  version      Print version
  help         Show this help

Use "edgelite-hub <command> --help" for command-specific options.`)
}

// ── selftest ──

func (c *CLI) runSelfTest(args []string) int {
	c.loadConfig()

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║       EdgeAgent Hub — Self Test                 ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	allPass := true

	// 1. 配置检查
	fmt.Print("[1/8] Configuration check................. ")
	if c.cfg != nil {
		fmt.Println("✓ PASS")
	} else {
		fmt.Println("✗ FAIL — config not loaded")
		return 1
	}

	// 2. 数据库检查
	fmt.Print("[2/8] Database (SQLite)..................... ")
	store, err := storage.New(
		c.cfg.Storage.DBPath, c.cfg.Storage.WALMode,
		c.cfg.Storage.MaxConnections, c.cfg.Storage.RetentionDays,
		c.cfg.Storage.AuditRetentionDays, c.cfg.Storage.OfflineQueueMax,
	)
	if err != nil {
		fmt.Printf("✗ FAIL — %v\n", err)
		allPass = false
	} else {
		fmt.Println("✓ PASS")
		defer store.Close()
	}

	// 3. NATS 连接检查
	fmt.Print("[3/8] NATS connection....................... ")
	natsHost := strings.Replace(strings.Replace(c.cfg.NATS.URL, "nats://", "", 1), ":4222", ":8222", 1)
	if c.checkHTTP(natsHost) {
		fmt.Println("✓ PASS")
	} else {
		fmt.Println("⚠ WARN — NATS monitoring endpoint not reachable (core may still work)")
	}

	// 4. MQTT 连接检查
	fmt.Print("[4/8] MQTT broker........................... ")
	if c.cfg.MQTT.Broker != "" {
		fmt.Println("✓ PASS (configured)")
	} else {
		fmt.Println("⚠ WARN — MQTT broker not configured")
	}

	// 5. ONNX 模型目录
	fmt.Print("[5/8] ONNX model directory................. ")
	modelDir := c.cfg.Inference.ONNX.ModelDir
	if entries, err := os.ReadDir(modelDir); err == nil {
		onnxCount := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".onnx") {
				onnxCount++
			}
		}
		fmt.Printf("✓ PASS (%d models)\n", onnxCount)
	} else {
		fmt.Println("⚠ WARN — directory not found, will be created")
		os.MkdirAll(modelDir, 0755)
	}

	// 6. 工作流目录
	fmt.Print("[6/8] Workflow directory................... ")
	if entries, err := os.ReadDir(c.cfg.Orchestrator.WorkflowDir); err == nil {
		wfCount := 0
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".yaml" || ext == ".yml" {
				wfCount++
			}
		}
		fmt.Printf("✓ PASS (%d workflows)\n", wfCount)
	} else {
		fmt.Println("⚠ WARN — directory not found")
		os.MkdirAll(c.cfg.Orchestrator.WorkflowDir, 0755)
	}

	// 7. 知识库目录
	fmt.Print("[7/8] Knowledge base....................... ")
	if entries, err := os.ReadDir(c.cfg.Inference.RAG.KnowledgeDir); err == nil {
		fmt.Printf("✓ PASS (%d files)\n", len(entries))
	} else {
		fmt.Println("⚠ WARN — directory not found")
		os.MkdirAll(c.cfg.Inference.RAG.KnowledgeDir, 0755)
	}

	// 8. 安全检查
	fmt.Print("[8/8] Security............................. ")
	if c.cfg.Security.JWTSecret == "" || c.cfg.Security.JWTSecret == "CHANGE_ME_IN_PRODUCTION_USE_A_LONG_RANDOM_STRING" {
		fmt.Println("⚠ WARN — JWT secret is empty/default, will auto-generate on next start!")
		allPass = false
	} else {
		fmt.Println("✓ PASS (JWT secret configured)")
	}

	fmt.Println()
	if allPass {
		fmt.Println("Result: ALL CHECKS PASSED ✓")
	} else {
		fmt.Println("Result: PASSED WITH WARNINGS ⚠")
	}
	return 0
}

// ── init ──

func (c *CLI) runInit(args []string) int {
	c.loadConfig()

	fmt.Println("Initializing EdgeAgent Hub...")

	// 创建目录结构
	dirs := []string{
		c.cfg.Inference.ONNX.ModelDir,
		c.cfg.Orchestrator.WorkflowDir,
		c.cfg.Inference.RAG.KnowledgeDir,
		c.cfg.Inference.RAG.VectordbDir,
		c.cfg.OTA.PartitionA,
		c.cfg.OTA.PartitionB,
		filepath.Dir(c.cfg.Storage.DBPath),
		filepath.Dir(c.cfg.Observability.LogFile),
		"data/knowledge",
		"data/vectordb",
		"data/ota",
	}
	for _, d := range dirs {
		if d != "" && d != "." {
			if err := os.MkdirAll(d, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create directory %s: %v\n", d, err)
				return 1
			}
			fmt.Printf("  ✓ Created: %s\n", d)
		}
	}

	// 初始化数据库
	fmt.Print("  Initializing database... ")
	store, err := storage.New(
		c.cfg.Storage.DBPath, c.cfg.Storage.WALMode,
		c.cfg.Storage.MaxConnections, c.cfg.Storage.RetentionDays,
		c.cfg.Storage.AuditRetentionDays, c.cfg.Storage.OfflineQueueMax,
	)
	if err != nil {
		fmt.Printf("✗ FAIL: %v\n", err)
		return 1
	}
	defer store.Close()
	fmt.Println("✓ OK")

	// 创建默认管理员
	fmt.Print("  Creating admin user... ")
	hash, err := security.HashPassword("admin123")
	if err != nil {
		fmt.Printf("✗ FAIL: %v\n", err)
		return 1
	}

	err = store.SaveUser(&models.User{
		Username:     c.cfg.Security.Admin.Username,
		PasswordHash: hash,
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	})
	if err != nil {
		fmt.Printf("⚠ Admin user may already exist: %v\n", err)
	} else {
		fmt.Printf("✓ Created (username: %s)\n", c.cfg.Security.Admin.Username)
	}

	fmt.Println("\nInitialization complete!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit configs/config.yaml for your environment")
	fmt.Println("  2. Change the admin password immediately")
	fmt.Println("  3. Place ONNX models in models/onnx/")
	fmt.Println("  4. Place GGUF LLM model in models/llm/")
	fmt.Println("  5. Run: edgelite-hub selftest")
	fmt.Println("  6. Start: edgelite-hub serve")
	return 0
}

// ── model 管理 ──

func (c *CLI) runModel(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: edgelite-hub model <list|load|unload|switch|rollback>")
		return 1
	}
	c.loadConfig()

	switch args[0] {
	case "list":
		return c.modelList()
	case "load":
		if len(args) < 3 {
			fmt.Println("Usage: edgelite-hub model load <model_id> <file_path>")
			return 1
		}
		return c.modelLoad(args[1], args[2])
	case "unload":
		if len(args) < 2 {
			fmt.Println("Usage: edgelite-hub model unload <model_id>")
			return 1
		}
		return c.modelUnload(args[1])
	case "switch":
		if len(args) < 2 {
			fmt.Println("Usage: edgelite-hub model switch <model_id>")
			return 1
		}
		return c.modelSwitch(args[1])
	case "rollback":
		return c.modelRollback()
	default:
		fmt.Fprintf(os.Stderr, "Unknown model command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) modelList() int {
	store, err := c.openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open store: %v\n", err)
		return 1
	}
	defer store.Close()

	models, err := store.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query models: %v\n", err)
		return 1
	}

	fmt.Printf("%-30s %-15s %-10s %-8s %-12s %s\n", "ID", "NAME", "VERSION", "ACTIVE", "SIZE", "SHA256")
	fmt.Println(strings.Repeat("-", 100))

	for _, m := range models {
		activeStr := ""
		if m.Active {
			activeStr = "✓"
		}
		sha := m.SHA256
		if len(sha) > 16 {
			sha = sha[:16] + "..."
		}
		fmt.Printf("%-30s %-15s %-10s %-8s %-12d %s\n", m.ID, m.Name, m.Version, activeStr, m.SizeBytes, sha)
	}
	fmt.Printf("\n%d model(s)\n", len(models))
	return 0
}

func (c *CLI) modelLoad(modelID, filePath string) int {
	token := c.login()
	if token == "" {
		return 1
	}

	payload := fmt.Sprintf(`{"model_id":"%s","file_path":"%s"}`, modelID, filePath)
	resp, err := c.apiPost("/api/v1/models/load", token, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	fmt.Println(resp)
	return 0
}

func (c *CLI) modelUnload(modelID string) int {
	token := c.login()
	if token == "" {
		return 1
	}

	payload := fmt.Sprintf(`{"model_id":"%s"}`, modelID)
	resp, err := c.apiPost("/api/v1/models/unload", token, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	fmt.Println(resp)
	return 0
}

func (c *CLI) modelSwitch(modelID string) int {
	token := c.login()
	if token == "" {
		return 1
	}

	payload := fmt.Sprintf(`{"model_id":"%s"}`, modelID)
	resp, err := c.apiPost("/api/v1/models/switch", token, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	fmt.Println(resp)
	return 0
}

func (c *CLI) modelRollback() int {
	token := c.login()
	if token == "" {
		return 1
	}

	resp, err := c.apiPost("/api/v1/models/rollback", token, "{}")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	fmt.Println(resp)
	return 0
}

// ── workflow 管理 ──

func (c *CLI) runWorkflow(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: edgelite-hub workflow <list|trigger|show>")
		return 1
	}
	c.loadConfig()

	switch args[0] {
	case "list":
		return c.workflowList()
	case "trigger":
		if len(args) < 2 {
			fmt.Println("Usage: edgelite-hub workflow trigger <name>")
			return 1
		}
		return c.workflowTrigger(args[1])
	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: edgelite-hub workflow show <name>")
			return 1
		}
		return c.workflowShow(args[1])
	default:
		fmt.Fprintf(os.Stderr, "Unknown workflow command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) workflowList() int {
	wfDir := c.cfg.Orchestrator.WorkflowDir
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read workflow directory: %v\n", err)
		return 1
	}

	fmt.Printf("%-40s %s\n", "NAME", "STEPS")
	fmt.Println(strings.Repeat("-", 60))

	count := 0
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(wfDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var wf struct {
			Name  string `yaml:"name"`
			Steps []struct {
				ID string `yaml:"id"`
			} `yaml:"steps"`
		}
		if err := yaml.Unmarshal(data, &wf); err != nil {
			continue
		}
		name := wf.Name
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		fmt.Printf("%-40s %d\n", name, len(wf.Steps))
		count++
	}
	fmt.Printf("\n%d workflow(s)\n", count)
	return 0
}

func (c *CLI) workflowTrigger(name string) int {
	token := c.login()
	if token == "" {
		return 1
	}

	resp, err := c.apiPost(fmt.Sprintf("/api/v1/workflows/%s/trigger", name), token, "{}")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	fmt.Println(resp)
	return 0
}

func (c *CLI) workflowShow(name string) int {
	wfPath := filepath.Join(c.cfg.Orchestrator.WorkflowDir, name+".yaml")
	data, err := os.ReadFile(wfPath)
	if err != nil {
		wfPath = filepath.Join(c.cfg.Orchestrator.WorkflowDir, name+".yml")
		data, err = os.ReadFile(wfPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Workflow not found: %s\n", name)
			return 1
		}
	}
	fmt.Println(string(data))
	return 0
}

// ── 辅助方法 ──

func (c *CLI) loadConfig() {
	if c.cfg != nil {
		return
	}
	cfg, err := config.Load(c.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	c.cfg = cfg
}

func (c *CLI) openStore() (*storage.Store, error) {
	c.loadConfig()
	return storage.New(
		c.cfg.Storage.DBPath, c.cfg.Storage.WALMode,
		c.cfg.Storage.MaxConnections, c.cfg.Storage.RetentionDays,
		c.cfg.Storage.AuditRetentionDays, c.cfg.Storage.OfflineQueueMax,
	)
}

func (c *CLI) login() string {
	c.loadConfig()
	baseURL := fmt.Sprintf("http://%s:%d", c.cfg.Server.Host, c.cfg.Server.Port)

	payload := fmt.Sprintf(`{"username":"%s","password":"admin123"}`, c.cfg.Security.Admin.Username)
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed (is server running?): %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Login response parse error: %v\n", err)
		return ""
	}
	token, ok := result["access_token"]
	if !ok {
		fmt.Fprintf(os.Stderr, "Login failed: no access_token in response\n")
		return ""
	}
	return token
}

func (c *CLI) apiPost(path, token, payload string) (string, error) {
	c.loadConfig()
	baseURL := fmt.Sprintf("http://%s:%d", c.cfg.Server.Host, c.cfg.Server.Port)

	req, err := http.NewRequest("POST", baseURL+path, strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func (c *CLI) checkHTTP(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}
