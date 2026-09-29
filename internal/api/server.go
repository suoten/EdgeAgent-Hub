package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/agent"
	"github.com/edgelite/edgeagent-hub/internal/config"
	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/edgelite/edgeagent-hub/internal/offline"
	"github.com/edgelite/edgeagent-hub/internal/orchestrator"
	"github.com/edgelite/edgeagent-hub/internal/ota"
	"github.com/edgelite/edgeagent-hub/internal/observability"
	"github.com/edgelite/edgeagent-hub/internal/security"
	"github.com/edgelite/edgeagent-hub/internal/storage"
	"github.com/edgelite/edgeagent-hub/internal/messaging"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

// Server 是管理 API 服务器
type Server struct {
	cfg            *config.Config
	store          *storage.Store
	bridge         *messaging.Bridge
	registry       *agent.Registry
	orchestrator   *orchestrator.Engine
	otaManager     *ota.Manager
	offlineMgr     *offline.Manager
	authService    *security.AuthService
	rateLimiter    *security.RateLimiter
	loginProtector *security.LoginProtector
	metrics        *observability.Metrics
	healthChecker  *observability.HealthChecker
	promRegistry   *prometheus.Registry
	logger         *logrus.Entry
}

// NewServer 创建 API 服务器
func NewServer(
	cfg *config.Config,
	store *storage.Store,
	bridge *messaging.Bridge,
	registry *agent.Registry,
	orch *orchestrator.Engine,
	otaMgr *ota.Manager,
	offlineMgr *offline.Manager,
	authSvc *security.AuthService,
	rl *security.RateLimiter,
	lp *security.LoginProtector,
	metrics *observability.Metrics,
	hc *observability.HealthChecker,
	promReg *prometheus.Registry,
	logger *logrus.Entry,
) *Server {
	return &Server{
		cfg:            cfg,
		store:          store,
		bridge:         bridge,
		registry:       registry,
		orchestrator:   orch,
		otaManager:     otaMgr,
		offlineMgr:     offlineMgr,
		authService:    authSvc,
		rateLimiter:    rl,
		loginProtector: lp,
		metrics:        metrics,
		healthChecker:  hc,
		promRegistry:   promReg,
		logger:         logger,
	}
}

// SetupRoutes 设置所有 API 路由
func (s *Server) SetupRoutes(e *echo.Echo) {
	// 健康探针 (不需要认证)
	e.GET("/health/live", s.handleLiveness)
	e.GET("/health/ready", s.handleReadiness)
	e.GET("/health/startup", s.handleStartup)

	// Prometheus 指标 (不需要认证)
	e.GET(s.cfg.Observability.MetricsPath, s.handleMetrics)

	// 前端静态文件 (Vue SPA, build output: web/dist/)
	e.Static("/assets", "web/dist/assets")
	e.File("/favicon.svg", "web/dist/favicon.svg")
	e.GET("/", func(c echo.Context) error {
		return c.File("web/dist/index.html")
	})
	// SPA 回退: 所有非 API 路由返回 index.html
	e.GET("/*", func(c echo.Context) error {
		path := c.Path()
		// 如果是 API 或健康检查路由，跳过
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/health") || path == "/metrics" {
			return echo.ErrNotFound
		}
		return c.File("web/dist/index.html")
	})

	// 认证
	e.POST("/api/v1/auth/login", s.handleLogin)
	e.POST("/api/v1/auth/refresh", s.handleRefreshToken)

	// API v1 组 (需要认证)
	v1 := e.Group("/api/v1", s.authMiddleware, s.rateLimitMiddleware)

	// 模型管理
	v1.GET("/models", s.handleListModels)
	v1.POST("/models/load", s.handleLoadModel, s.requirePermission(security.PermModelLoad))
	v1.POST("/models/upload", s.handleUploadModel, s.requirePermission(security.PermModelLoad))
	v1.POST("/models/unload", s.handleUnloadModel, s.requirePermission(security.PermModelUnload))
	v1.POST("/models/switch", s.handleSwitchModel, s.requirePermission(security.PermModelSwitch))
	v1.POST("/models/rollback", s.handleRollbackModel, s.requirePermission(security.PermModelRollback))
	v1.GET("/models/llm-config", s.handleGetLLMConfig)
	v1.PUT("/models/llm-config", s.handleUpdateLLMConfig, s.requirePermission(security.PermModelLoad))

	// 编排管理
	v1.GET("/workflows", s.handleListWorkflows)
	v1.POST("/workflows", s.handleCreateWorkflow, s.requirePermission(security.PermWorkflowCreate))
	v1.PUT("/workflows/:id", s.handleUpdateWorkflow, s.requirePermission(security.PermWorkflowUpdate))
	v1.POST("/workflows/:id/trigger", s.handleTriggerWorkflow, s.requirePermission(security.PermWorkflowTrigger))
	v1.GET("/workflows/executions", s.handleListExecutions)
	v1.GET("/workflows/executions/:id", s.handleGetExecution)

	// 设备管理
	v1.GET("/devices", s.handleListDevices)

	// 告警管理
	v1.GET("/alerts", s.handleListAlerts)
	v1.PUT("/alerts/:id/ack", s.handleAckAlert, s.requirePermission(security.PermAlertAck))

	// 知识管理
	v1.POST("/knowledge/upload", s.handleKnowledgeUpload, s.requirePermission(security.PermKnowledgeUpload))
	v1.GET("/knowledge/search", s.handleKnowledgeSearch, s.requirePermission(security.PermKnowledgeSearch))

	// OTA 管理
	v1.POST("/ota/check", s.handleOTACheck, s.requirePermission(security.PermOTACheck))
	v1.POST("/ota/apply", s.handleOTAApply, s.requirePermission(security.PermOTAApply))
	v1.GET("/ota/tasks", s.handleListOTATasks, s.requirePermission(security.PermOTACheck))

	// 系统监控
	v1.GET("/system/health", s.handleSystemHealth, s.requirePermission(security.PermSystemHealth))
	v1.GET("/system/metrics", s.handleSystemMetrics, s.requirePermission(security.PermSystemMetrics))
	v1.POST("/system/offline-mode", s.handleSetOfflineMode, s.requirePermission(security.PermSystemHealth))

	// 对话交互
	v1.POST("/chat", s.handleChat, s.requirePermission(security.PermChat))

	// 审计日志
	v1.GET("/audit/logs", s.handleAuditLogs, s.requirePermission(security.PermAuditLogs))

	// 安全管理
	v1.GET("/security/users", s.handleListUsers, s.requirePermission(security.PermUserManage))
	v1.POST("/security/users", s.handleCreateUser, s.requirePermission(security.PermUserManage))
	v1.DELETE("/security/users/:username", s.handleDeleteUser, s.requirePermission(security.PermUserManage))
	v1.POST("/auth/change-password", s.handleChangePassword)
	v1.POST("/security/users/:username/reset", s.handleResetUserPassword, s.requirePermission(security.PermUserManage))

	// 智能体管理
	v1.GET("/agents", s.handleListAgents)
	v1.POST("/agents/register", s.handleRegisterAgent)
}

// ── 中间件 ──

// authMiddleware JWT 认证中间件
func (s *Server) authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// 跳过认证的路径已在路由设置时排除
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid authorization format"})
		}

		claims, err := s.authService.ValidateToken(parts[1])
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		}

		// 存储用户信息到上下文
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		return next(c)
	}
}

// rateLimitMiddleware 请求限流中间件
func (s *Server) rateLimitMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ip := c.RealIP()
		if !s.rateLimiter.Allow(ip) {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		}
		return next(c)
	}
}

// requirePermission 权限检查中间件
func (s *Server) requirePermission(perm security.Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, ok := c.Get("role").(models.Role)
			if !ok {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "no role context"})
			}
			if !security.HasPermission(role, perm) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
			}
			return next(c)
		}
	}
}

// ── 健康探针 ──

func (s *Server) handleLiveness(c echo.Context) error {
	return c.JSON(http.StatusOK, s.healthChecker.Liveness())
}

func (s *Server) handleReadiness(c echo.Context) error {
	return c.JSON(http.StatusOK, s.healthChecker.Readiness())
}

func (s *Server) handleStartup(c echo.Context) error {
	return c.JSON(http.StatusOK, s.healthChecker.Startup())
}

// ── Prometheus 指标 ──

func (s *Server) handleMetrics(c echo.Context) error {
	handler := promhttp.HandlerFor(s.promRegistry, promhttp.HandlerOpts{})
	handler.ServeHTTP(c.Response(), c.Request())
	return nil
}

// ── 认证 ──

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// 检查锁定
	if s.loginProtector.IsLocked(req.Username) {
		return c.JSON(http.StatusLocked, map[string]string{"error": "account locked, try later"})
	}

	// 查找用户
	user, err := s.store.GetUser(req.Username)
	if err != nil {
		s.loginProtector.RecordFailure(req.Username)
		s.store.RecordLoginAttempt(req.Username, c.RealIP(), false)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}

	// 验证密码
	if !security.CheckPassword(req.Password, user.PasswordHash) {
		s.loginProtector.RecordFailure(req.Username)
		s.store.RecordLoginAttempt(req.Username, c.RealIP(), false)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}

	// 生成 Token
	accessToken, err := s.authService.GenerateAccessToken(user.Username, user.Role)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
	}

	refreshToken, err := s.authService.GenerateRefreshToken(user.Username, user.Role)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
	}

	s.loginProtector.RecordSuccess(req.Username)
	s.store.RecordLoginAttempt(req.Username, c.RealIP(), true)
	s.store.UpdateLastLogin(user.Username)

	// 审计日志
	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     user.Username,
		Action:    "login",
		Resource:  "auth",
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"role":          string(user.Role),
	})
}

func (s *Server) handleRefreshToken(c echo.Context) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	claims, err := s.authService.ValidateToken(req.RefreshToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	}

	accessToken, err := s.authService.GenerateAccessToken(claims.Username, claims.Role)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"access_token": accessToken,
		"token_type":   "Bearer",
	})
}

// handleUploadModel 处理模型文件上传
func (s *Server) handleUploadModel(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no file provided"})
	}

	// 获取表单参数
	modelID := c.FormValue("model_id")
	if modelID == "" {
		modelID = strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
	}

	// 确定模型类型和目录
	ext := strings.ToLower(filepath.Ext(file.Filename))
	modelType := "onnx"
	modelDir := s.cfg.Inference.ONNX.ModelDir
	if modelDir == "" {
		modelDir = "models/onnx"
	}
	if ext == ".gguf" {
		modelType = "gguf"
		modelDir = "models/llm"
	}

	// 确保目录存在
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create model dir: %v", err)})
	}

	// 保存文件
	safeName := filepath.Base(file.Filename)
	dst := filepath.Join(modelDir, safeName)
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// 获取文件大小
	stat, _ := os.Stat(dst)
	var sizeBytes int64
	if stat != nil {
		sizeBytes = stat.Size()
	}

	// 写入数据库
	now := time.Now()
	m := &models.ModelInfo{
		ID:        modelID,
		Name:      modelID,
		Version:   "1.0.0",
		Type:      modelType,
		FilePath:  dst,
		Partition: s.cfg.OTA.ActivePartition,
		Active:    false,
		SizeBytes: sizeBytes,
		LoadedAt:  &now,
	}
	if err := s.store.SaveModel(m); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to save model: %v", err)})
	}

	// 如果是 ONNX 模型，尝试通过 NATS 通知 sidecar 加载
	if modelType == "onnx" {
		subject := "agent.onnx.load"
		payload, _ := json.Marshal(map[string]string{"model_id": modelID, "file_path": dst})
		_, natsErr := s.bridge.RequestNATS(subject, payload, 5*time.Second)
		if natsErr != nil {
			// sidecar 离线，数据库层面已加载
			s.store.SaveAudit(&models.AuditEntry{
				Timestamp: time.Now(),
				Actor:     getUsername(c),
				Action:    "model:upload",
				Resource:  modelID,
				IP:        c.RealIP(),
			})
			return c.JSON(http.StatusOK, map[string]any{
				"status":    "uploaded",
				"model_id":  modelID,
				"file_path": dst,
				"warning":   "sidecar offline, model saved to database. Start sidecar to enable inference.",
			})
		}
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "model:upload",
		Resource:  modelID,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusCreated, map[string]any{
		"status":    "uploaded",
		"model_id":  modelID,
		"file_path": dst,
		"type":      modelType,
		"size":      sizeBytes,
	})
}

// handleGetLLMConfig 获取 LLM API 配置
func (s *Server) handleGetLLMConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"api_enabled":      s.cfg.Inference.LLM.APIEnabled,
		"api_base_url":     s.cfg.Inference.LLM.APIBaseURL,
		"api_model":        s.cfg.Inference.LLM.APIModel,
		"api_key":          maskAPIKey(s.cfg.Inference.LLM.APIKey),
		"has_api_key":      s.cfg.Inference.LLM.APIKey != "",
		"temperature":      s.cfg.Inference.LLM.Temperature,
		"max_tokens":       s.cfg.Inference.LLM.MaxTokens,
		"system_prompt":    s.cfg.Inference.LLM.APISystemPrompt,
	})
}

// handleUpdateLLMConfig 更新 LLM API 配置
func (s *Server) handleUpdateLLMConfig(c echo.Context) error {
	var req struct {
		APIEnabled   *bool   `json:"api_enabled"`
		APIBaseURL   *string `json:"api_base_url"`
		APIKey       *string `json:"api_key"`
		APIModel     *string `json:"api_model"`
		Temperature  *float64 `json:"temperature"`
		MaxTokens    *int    `json:"max_tokens"`
		SystemPrompt *string `json:"system_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.APIEnabled != nil {
		s.cfg.Inference.LLM.APIEnabled = *req.APIEnabled
	}
	if req.APIBaseURL != nil {
		s.cfg.Inference.LLM.APIBaseURL = *req.APIBaseURL
	}
	if req.APIKey != nil && *req.APIKey != "" {
		s.cfg.Inference.LLM.APIKey = *req.APIKey
	}
	if req.APIModel != nil {
		s.cfg.Inference.LLM.APIModel = *req.APIModel
	}
	if req.Temperature != nil {
		s.cfg.Inference.LLM.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		s.cfg.Inference.LLM.MaxTokens = *req.MaxTokens
	}
	if req.SystemPrompt != nil && *req.SystemPrompt != "" {
		s.cfg.Inference.LLM.APISystemPrompt = *req.SystemPrompt
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "llm_config:update",
		Resource:  "llm_api",
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]any{
		"status":       "updated",
		"api_enabled":  s.cfg.Inference.LLM.APIEnabled,
		"api_base_url": s.cfg.Inference.LLM.APIBaseURL,
		"api_model":    s.cfg.Inference.LLM.APIModel,
		"has_api_key":  s.cfg.Inference.LLM.APIKey != "",
	})
}

// maskAPIKey 掩码 API Key
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// ── 模型管理 ──

func (s *Server) handleListModels(c echo.Context) error {
	modelList, err := s.store.ListModels()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if modelList == nil {
		modelList = []models.ModelInfo{}
	}
	return c.JSON(http.StatusOK, map[string]any{"models": modelList})
}

func (s *Server) handleLoadModel(c echo.Context) error {
	var req struct {
		ModelID  string `json:"model_id"`
		FilePath string `json:"file_path"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// 先写入数据库
	now := time.Now()
	modelType := "onnx"
	if strings.HasSuffix(req.FilePath, ".gguf") {
		modelType = "gguf"
	}
	m := &models.ModelInfo{
		ID:        req.ModelID,
		Name:      req.ModelID,
		Version:   "1.0.0",
		Type:      modelType,
		FilePath:  req.FilePath,
		Partition: "A",
		Active:    false,
		LoadedAt:  &now,
	}
	if err := s.store.SaveModel(m); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to save model: %v", err)})
	}

	// 尝试通过 NATS 通知 sidecar 加载，失败不阻塞（sidecar 可离线）
	subject := fmt.Sprintf("agent.onnx.load")
	payload, _ := json.Marshal(map[string]string{"model_id": req.ModelID, "file_path": req.FilePath})
	_, natsErr := s.bridge.RequestNATS(subject, payload, 5*time.Second)
	if natsErr != nil {
		// sidecar 离线，数据库层面已加载，返回部分成功
		s.store.SaveAudit(&models.AuditEntry{
			Timestamp: time.Now(),
			Actor:     getUsername(c),
			Action:    "model:load",
			Resource:  req.ModelID,
			IP:        c.RealIP(),
		})
		return c.JSON(http.StatusOK, map[string]string{"status": "loaded", "warning": "sidecar offline, model registered in database"})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "model:load",
		Resource:  req.ModelID,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "loaded"})
}

func (s *Server) handleUnloadModel(c echo.Context) error {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// 尝试通过 NATS 通知 sidecar 卸载，失败不阻塞
	subject := fmt.Sprintf("agent.onnx.unload")
	payload, _ := json.Marshal(map[string]string{"model_id": req.ModelID})
	_, natsErr := s.bridge.RequestNATS(subject, payload, 5*time.Second)

	// 无论 sidecar 是否在线，都从数据库中删除模型记录
	if err := s.store.DeleteModel(req.ModelID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to delete model: %v", err)})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "model:unload",
		Resource:  req.ModelID,
		IP:        c.RealIP(),
	})

	if natsErr != nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "unloaded", "warning": "sidecar offline, model removed from database"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "unloaded"})
}

func (s *Server) handleSwitchModel(c echo.Context) error {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	activated, err := s.store.ToggleModel(req.ModelID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "model:switch",
		Resource:  req.ModelID,
		IP:        c.RealIP(),
	})

	action := "deactivated"
	if activated {
		action = "activated"
	}
	return c.JSON(http.StatusOK, map[string]string{"status": action})
}

func (s *Server) handleRollbackModel(c echo.Context) error {
	if err := s.otaManager.Rollback(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "rolled_back"})
}

// ── 编排管理 ──

func (s *Server) handleListWorkflows(c echo.Context) error {
	wfList := s.orchestrator.ListWorkflows()
	if wfList == nil {
		wfList = []*models.Workflow{}
	}
	return c.JSON(http.StatusOK, map[string]any{"workflows": wfList})
}

func (s *Server) handleCreateWorkflow(c echo.Context) error {
	var wf models.Workflow
	if err := c.Bind(&wf); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	s.orchestrator.AddWorkflow(&wf)

	// 持久化
	data, _ := json.Marshal(wf)
	s.store.SaveWorkflow(wf.Name, data)

	return c.JSON(http.StatusCreated, wf)
}

func (s *Server) handleUpdateWorkflow(c echo.Context) error {
	name := c.Param("id")
	var wf models.Workflow
	if err := c.Bind(&wf); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	wf.Name = name
	s.orchestrator.AddWorkflow(&wf)

	data, _ := json.Marshal(wf)
	s.store.SaveWorkflow(name, data)

	return c.JSON(http.StatusOK, wf)
}

func (s *Server) handleTriggerWorkflow(c echo.Context) error {
	name := c.Param("id")
	var triggerData map[string]any
	if err := c.Bind(&triggerData); err != nil {
		triggerData = make(map[string]any)
	}

	execution, err := s.orchestrator.Trigger(name, triggerData)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusAccepted, execution)
}

func (s *Server) handleListExecutions(c echo.Context) error {
	execList := s.orchestrator.ListExecutions()
	if execList == nil {
		execList = []*models.WorkflowExecution{}
	}
	return c.JSON(http.StatusOK, map[string]any{"executions": execList})
}

func (s *Server) handleGetExecution(c echo.Context) error {
	id := c.Param("id")
	exec, ok := s.orchestrator.GetExecution(id)
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "execution not found"})
	}
	return c.JSON(http.StatusOK, exec)
}

// ── 设备管理 ──

func (s *Server) handleListDevices(c echo.Context) error {
	agentList := s.registry.List()
	if agentList == nil {
		agentList = []models.AgentInfo{}
	}
	// 将智能体映射为设备视角
	devices := make([]map[string]any, 0, len(agentList))
	for _, a := range agentList {
		deviceType := string(a.Type)
		if a.Endpoint == "internal" {
			deviceType = "internal"
		}
		devices = append(devices, map[string]any{
			"id":           a.ID,
			"type":         deviceType,
			"endpoint":     a.Endpoint,
			"capabilities": a.Capabilities,
			"status":       a.Status,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"devices": devices, "agents": agentList})
}

// ── 告警管理 ──

func (s *Server) handleListAlerts(c echo.Context) error {
	status := c.QueryParam("status")
	limit := 50
	offset := 0

	alertList, err := s.store.GetAlerts(status, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if alertList == nil {
		alertList = []models.Alert{}
	}
	return c.JSON(http.StatusOK, map[string]any{"alerts": alertList})
}

func (s *Server) handleAckAlert(c echo.Context) error {
	id := c.Param("id")
	ackedBy := getUsername(c)

	if err := s.store.AckAlert(id, ackedBy); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     ackedBy,
		Action:    "alert:ack",
		Resource:  id,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "acked"})
}

// ── 知识管理 ──

func (s *Server) handleKnowledgeUpload(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no file provided"})
	}

	// 保存到知识库目录（防止路径穿越）
	knowledgeDir := "data/knowledge"
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// 清理文件名，防止目录穿越攻击
	safeName := filepath.Base(file.Filename)
	dst := filepath.Join(knowledgeDir, safeName)
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// 通过 NATS 通知 RAG 服务索引文档
	subject := "agent.rag.index"
	payload, _ := json.Marshal(map[string]string{"file_path": dst})
	s.bridge.PublishNATS(subject, payload)

	return c.JSON(http.StatusCreated, map[string]string{"status": "uploaded", "file": filepath.Base(file.Filename)})
}

func (s *Server) handleKnowledgeSearch(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing query parameter 'q'"})
	}

	// 通过 NATS 请求 RAG 检索
	subject := "agent.rag.search"
	payload, _ := json.Marshal(map[string]string{"query": query})

	msg, err := s.bridge.RequestNATS(subject, payload, 10*time.Second)
	if err != nil {
		// RAG Agent 离线时返回空结果而非报错
		return c.JSON(http.StatusOK, map[string]any{"results": []any{}, "message": "RAG 检索服务当前不可用，知识库引擎未启动或已离线"})
	}

	var result any
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return c.JSON(http.StatusOK, map[string]any{"results": []any{}, "message": "RAG 检索服务返回异常"})
	}
	return c.JSON(http.StatusOK, result)
}

// ── OTA 管理 ──

func (s *Server) handleOTACheck(c echo.Context) error {
	var req struct {
		ModelName string `json:"model_name"`
		Version   string `json:"version"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	task, err := s.otaManager.CheckUpdate(context.Background(), req.ModelName, req.Version)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, task)
}

func (s *Server) handleOTAApply(c echo.Context) error {
	var req struct {
		TaskID       string `json:"task_id"`
		DownloadURL  string `json:"download_url"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// 异步执行 OTA
	go func() {
		tasks, _ := s.otaManager.ListTasks(100)
		for _, t := range tasks {
			if t.ID == req.TaskID {
				s.otaManager.ApplyUpdate(context.Background(), &t, req.DownloadURL)
				break
			}
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "applying"})
}

func (s *Server) handleListOTATasks(c echo.Context) error {
	taskList, err := s.otaManager.ListTasks(50)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if taskList == nil {
		taskList = []models.OTATask{}
	}
	return c.JSON(http.StatusOK, map[string]any{"tasks": taskList})
}

// ── 系统监控 ──

func (s *Server) handleSystemHealth(c echo.Context) error {
	health := models.HealthStatus{
		Status:        "healthy",
		NATSConnected: s.bridge.IsNATSConnected(),
		MQTTConnected: s.bridge.IsMQTTConnected(),
		OfflineMode:   s.offlineMgr.IsOfflineMode(),
		Components:    make(map[string]string),
		Timestamp:     time.Now(),
	}

	if s.bridge.IsNATSConnected() {
		health.Components["nats"] = "connected"
	} else {
		health.Components["nats"] = "disconnected"
		health.Status = "degraded"
	}

	if s.bridge.IsMQTTConnected() {
		health.Components["mqtt"] = "connected"
	} else {
		health.Components["mqtt"] = "disconnected"
		health.Status = "degraded"
	}

	// ONNX 推理引擎：配置启用即视为可用
	if s.cfg.Inference.ONNX.Enabled {
		health.Components["onnx"] = "available"
	} else {
		health.Components["onnx"] = "unavailable"
		health.Status = "degraded"
	}
	// LLM：配置了远程 API 则可用，否则降级
	if s.cfg.Inference.LLM.APIEnabled && s.cfg.Inference.LLM.APIBaseURL != "" {
		health.Components["llm"] = "available"
	} else {
		health.Components["llm"] = "degraded"
	}
	// RAG：内置知识检索引擎始终可用
	health.Components["rag"] = "available"

	return c.JSON(http.StatusOK, health)
}

func (s *Server) handleSystemMetrics(c echo.Context) error {
	// 返回关键指标摘要
	queueLen, _ := s.store.OfflineQueueLength()
	return c.JSON(http.StatusOK, map[string]any{
		"messages_in_total":  s.bridge.MessagesIn(),
		"messages_out_total": s.bridge.MessagesOut(),
		"agents_online":      s.registry.OnlineCount(),
		"active_workflows":   s.orchestrator.ActiveExecutionCount(),
		"offline_queue":      queueLen,
		"offline_mode":       s.offlineMgr.IsOfflineMode(),
		"nats_connected":     s.bridge.IsNATSConnected(),
		"mqtt_connected":     s.bridge.IsMQTTConnected(),
	})
}

// handleSetOfflineMode 手动切换离线模式
func (s *Server) handleSetOfflineMode(c echo.Context) error {
	var req struct {
		Offline bool `json:"offline"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	s.offlineMgr.SetOfflineMode(req.Offline)

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "system:offline_mode",
		Resource:  fmt.Sprintf("offline=%v", req.Offline),
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]any{
		"offline_mode": req.Offline,
		"message":     ternary(req.Offline, "已进入离线自治模式，消息将缓存到本地队列", "已退出离线模式，恢复正常通信"),
	})
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// ── 对话交互 ──

func (s *Server) handleChat(c echo.Context) error {
	var req models.ChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// 通过 NATS 发送到 LLM Agent
	subject := "agent.llm.chat"
	payload, _ := json.Marshal(req)

	msg, err := s.bridge.RequestNATS(subject, payload, 30*time.Second)
	if err != nil {
		// LLM Agent 离线时返回友好的降级响应
		return c.JSON(http.StatusOK, models.ChatResponse{
			Response: "智能对话服务当前不可用。请前往「模型管理」页面激活 GGUF 大语言模型（如 edge_tiny_llm），并确保 AI Sidecar 服务正在运行。",
			Anchored: false,
		})
	}

	var resp models.ChatResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return c.JSON(http.StatusOK, models.ChatResponse{
			Response: "智能对话服务返回异常，请稍后重试。",
			Anchored: false,
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// ── 审计日志 ──

func (s *Server) handleAuditLogs(c echo.Context) error {
	limit := 100
	offset := 0

	logList, err := s.store.GetAuditLogs(limit, offset, nil, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if logList == nil {
		logList = []models.AuditEntry{}
	}
	return c.JSON(http.StatusOK, map[string]any{"logs": logList})
}

// ── 安全管理 ──

func (s *Server) handleListUsers(c echo.Context) error {
	userList, err := s.store.ListUsers()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if userList == nil {
		userList = []models.User{}
	}
	return c.JSON(http.StatusOK, map[string]any{"users": userList})
}

func (s *Server) handleCreateUser(c echo.Context) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	user := &models.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         models.Role(req.Role),
		CreatedAt:    time.Now(),
	}

	if err := s.store.SaveUser(user); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "user:create",
		Resource:  req.Username,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleDeleteUser(c echo.Context) error {
	username := c.Param("username")
	if username == "admin" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "不能删除管理员账户"})
	}

	if err := s.store.DeleteUser(username); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "user:delete",
		Resource:  username,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// ── 智能体管理 ──

func (s *Server) handleListAgents(c echo.Context) error {
	agentList := s.registry.List()
	if agentList == nil {
		agentList = []models.AgentInfo{}
	}
	return c.JSON(http.StatusOK, map[string]any{"agents": agentList})
}

// handleChangePassword 修改当前用户密码
func (s *Server) handleChangePassword(c echo.Context) error {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	username := getUsername(c)
	if username == "unknown" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}

	if len(req.NewPassword) < 6 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "new password must be at least 6 characters"})
	}

	user, err := s.store.GetUser(username)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}

	if !security.CheckPassword(req.OldPassword, user.PasswordHash) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "old password incorrect"})
	}

	newHash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
	}

	user.PasswordHash = newHash
	if err := s.store.SaveUser(user); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save password"})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     username,
		Action:    "user:change_password",
		Resource:  username,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "message": "password changed successfully"})
}

// handleResetUserPassword 管理员重置指定用户密码
func (s *Server) handleResetUserPassword(c echo.Context) error {
	username := c.Param("username")
	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if len(req.Password) < 6 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "password must be at least 6 characters"})
	}

	user, err := s.store.GetUser(username)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
	}

	user.PasswordHash = hash
	if err := s.store.SaveUser(user); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save password"})
	}

	s.store.SaveAudit(&models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     getUsername(c),
		Action:    "user:reset_password",
		Resource:  username,
		IP:        c.RealIP(),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "message": "password reset successfully"})
}

func (s *Server) handleRegisterAgent(c echo.Context) error {
	var info models.AgentInfo
	if err := c.Bind(&info); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	s.registry.Register(&info)
	return c.JSON(http.StatusCreated, info)
}

// ── 辅助函数 ──

// getUsername 从上下文安全获取用户名
func getUsername(c echo.Context) string {
	if v, ok := c.Get("username").(string); ok {
		return v
	}
	return "unknown"
}

// ensureAdminUser 确保默认管理员用户存在
func (s *Server) EnsureAdminUser(password string) error {
	if _, err := s.store.GetUser(s.cfg.Security.Admin.Username); err == nil {
		return nil // 用户已存在
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}

	user := &models.User{
		Username:     s.cfg.Security.Admin.Username,
		PasswordHash: hash,
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	}
	return s.store.SaveUser(user)
}
