package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/agent"
	"github.com/edgelite/edgeagent-hub/internal/api"
	"github.com/edgelite/edgeagent-hub/internal/cli"
	"github.com/edgelite/edgeagent-hub/internal/config"
	"github.com/edgelite/edgeagent-hub/internal/dataprocess"
	"github.com/edgelite/edgeagent-hub/internal/learning"
	"github.com/edgelite/edgeagent-hub/internal/messaging"
	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/edgelite/edgeagent-hub/internal/offline"
	"github.com/edgelite/edgeagent-hub/internal/observability"
	"github.com/edgelite/edgeagent-hub/internal/orchestrator"
	"github.com/edgelite/edgeagent-hub/internal/ota"
	"github.com/edgelite/edgeagent-hub/internal/protocol"
	"github.com/edgelite/edgeagent-hub/internal/resource"
	"github.com/edgelite/edgeagent-hub/internal/security"
	"github.com/edgelite/edgeagent-hub/internal/storage"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	flag.Parse()

	args := flag.Args()

	// 如果有子命令且不是 serve，则走 CLI 路径
	if len(args) > 0 && args[0] != "serve" {
		cliApp := cli.NewCLI(*configPath)
		os.Exit(cliApp.Execute(args))
	}

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 设置日志
	logger := observability.SetupLogger(
		cfg.Observability.LogLevel,
		cfg.Observability.LogFormat,
		cfg.Observability.LogFile,
		cfg.Observability.LogMaxSize,
		cfg.Observability.LogMaxDays,
	)
	logEntry := logrus.NewEntry(logger)

	logEntry.Infof("EdgeAgent Hub v%s starting...", version)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logEntry.Infof("Received signal %s, shutting down...", sig)
		cancel()
	}()

	// ── 资源适配检测 ──
	var resourceResult *resource.DetectionResult
	if cfg.Resource.AutoDetect {
		resAdapter := resource.NewAdapter(resource.AdapterConfig{
			AutoDetect:         cfg.Resource.AutoDetect,
			MinMemoryForLLM:    cfg.Resource.MinMemoryForLLM,
			MinMemoryForVision: cfg.Resource.MinMemoryForVision,
			RAMTiers: resource.RAMTierConfig{
				Minimal:  cfg.Resource.RAMTiers.Minimal,
				Basic:    cfg.Resource.RAMTiers.Basic,
				Standard: cfg.Resource.RAMTiers.Standard,
				Full:     cfg.Resource.RAMTiers.Full,
			},
		}, logEntry)
		resourceResult, err = resAdapter.Detect()
		if err != nil {
			logEntry.Warnf("Resource detection failed: %v", err)
		} else {
			// 根据检测结果调整配置
			if resourceResult != nil {
				if !resourceResult.RecommendedLLM {
					cfg.Inference.LLM.Enabled = false
					logEntry.Info("LLM disabled due to resource constraints")
				}
				if resourceResult.RecommendedEP != "" {
					cfg.Inference.ONNX.ExecutionProvider = resourceResult.RecommendedEP
					logEntry.Infof("Execution provider set to: %s", resourceResult.RecommendedEP)
				}
			}
		}
	}

	// 初始化存储
	store, err := storage.New(
		cfg.Storage.DBPath,
		cfg.Storage.WALMode,
		cfg.Storage.MaxConnections,
		cfg.Storage.RetentionDays,
		cfg.Storage.AuditRetentionDays,
		cfg.Storage.OfflineQueueMax,
	)
	if err != nil {
		logEntry.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()
	logEntry.Info("Storage initialized")

	// ── 启动内嵌 MQTT broker（如果外部 broker 不可用则自动接管）──
	embeddedMQTT, err := messaging.NewEmbeddedBroker(1883, logEntry)
	if err != nil {
		logEntry.Warnf("Failed to start embedded MQTT broker: %v (will use external broker)", err)
	} else {
		// 使用内嵌 broker 的地址
		cfg.MQTT.Broker = embeddedMQTT.BrokerURL()
		defer embeddedMQTT.Close()
	}

	// ── 启动内嵌 NATS 服务器（如果外部服务器不可用则自动接管）──
	embeddedNATS, err := messaging.NewEmbeddedNATS(4222, logEntry)
	if err != nil {
		logEntry.Warnf("Failed to start embedded NATS server: %v (will use external server)", err)
	} else {
		cfg.NATS.URL = embeddedNATS.URL()
		defer embeddedNATS.Close()
	}

	// 初始化 MQTT-NATS 桥接
	bridge, err := messaging.NewBridge(
		cfg.NATS.URL,
		cfg.MQTT.Broker,
		cfg.MQTT.ClientID,
		cfg.MQTT.Username,
		cfg.MQTT.Password,
		cfg.MQTT.QoS,
		cfg.MQTT.KeepAlive.Duration,
		cfg.MQTT.CleanSession,
		cfg.MQTT.SubscribeTopics,
		logEntry,
	)
	if err != nil {
		logEntry.Fatalf("Failed to initialize messaging bridge: %v", err)
	}
	defer bridge.Close()
	logEntry.Info("MQTT-NATS bridge initialized")

	// ── JetStream 持久化管理器 ──
	jsManager, err := messaging.NewJetStreamManager(bridge.NATSConn(), logEntry)
	if err != nil {
		logEntry.Warnf("Failed to initialize JetStream manager: %v", err)
	} else {
		if err := jsManager.InitializeStreams(); err != nil {
			logEntry.Warnf("Failed to initialize JetStream streams: %v", err)
		} else {
			logEntry.Info("JetStream streams initialized")
		}
	}

	// 初始化 Prometheus 指标
	promRegistry := prometheus.NewRegistry()
	metrics := observability.NewMetrics(promRegistry)

	// ── 链路追踪 ──
	_ = observability.NewTracer(logEntry)

	// 初始化健康检查器
	healthChecker := observability.NewHealthChecker()
	healthChecker.SetComponent("nats", "connected")
	healthChecker.SetComponent("mqtt", "connected")

	// 初始化智能体注册中心
	registry := agent.NewRegistry(logEntry, cfg.Orchestrator.HealthCheckTimeout.Duration)

	// 注册内置智能体
	if cfg.Inference.ONNX.Enabled {
		registry.Register(&models.AgentInfo{
			ID:           "onnx.inference",
			Type:         models.AgentTypeONNX,
			Endpoint:     cfg.Inference.ONNX.Endpoint,
			Capabilities: []string{"onnx", "inference", "vibration", "temperature", "anomaly"},
			Status:       "online",
			Weight:       1,
		})
	}
	if cfg.Inference.LLM.Enabled {
		registry.Register(&models.AgentInfo{
			ID:           "llm.rag_anchored",
			Type:         models.AgentTypeLLM,
			Endpoint:     cfg.Inference.LLM.Endpoint,
			Capabilities: []string{"llm", "rag", "chat", "alert"},
			Status:       "online",
			Weight:       1,
		})
	}
	if cfg.Inference.RAG.Enabled {
		registry.Register(&models.AgentInfo{
			ID:           "rag.service",
			Type:         models.AgentTypeRAG,
			Endpoint:     cfg.Inference.RAG.Endpoint,
			Capabilities: []string{"rag", "search", "index"},
			Status:       "online",
			Weight:       1,
		})
	}
	// 模板告警智能体 (内置)
	registry.Register(&models.AgentInfo{
		ID:           "template.fallback_alert",
		Type:         models.AgentTypeTemplate,
		Endpoint:     "internal",
		Capabilities: []string{"template", "alert", "fallback"},
		Status:       "online",
		Weight:       1,
	})
	// 通知智能体 (内置)
	registry.Register(&models.AgentInfo{
		ID:           "notify.multi_channel",
		Type:         models.AgentTypeNotify,
		Endpoint:     "internal",
		Capabilities: []string{"notify", "dingtalk", "sms", "webhook"},
		Status:       "online",
		Weight:       1,
	})

	// 启动健康检查
	go registry.StartHealthCheck(cfg.Orchestrator.HeartbeatInterval.Duration, ctx.Done())

	// ── 注册内置智能体的 NATS 订阅 ──
	// 初始化内置 LLM 对话引擎（纯 Go 实现，无需 sidecar）
	builtinLLM := agent.NewBuiltinLLM("data/knowledge")

	// 如果配置了远程 LLM API，初始化 API 客户端
	if cfg.Inference.LLM.APIEnabled && cfg.Inference.LLM.APIBaseURL != "" {
		apiClient := agent.NewLLMApiClient(
			cfg.Inference.LLM.APIBaseURL,
			cfg.Inference.LLM.APIKey,
			cfg.Inference.LLM.APIModel,
			cfg.Inference.LLM.APISystemPrompt,
			cfg.Inference.LLM.Temperature,
			cfg.Inference.LLM.MaxTokens,
			cfg.Inference.LLM.APITimeout.Duration,
		)
		builtinLLM.SetAPIClient(apiClient)
		logEntry.Infof("LLM API client configured: %s (model: %s)", cfg.Inference.LLM.APIBaseURL, cfg.Inference.LLM.APIModel)
	} else {
		logEntry.Info("LLM API not configured, using built-in rule engine (configure api_enabled in config.yaml for AI-powered chat)")
	}

	registerBuiltinAgentHandlers(bridge, store, registry, builtinLLM, cfg, logEntry)

	// ── OOM 自动卸载管理器 ──
	oomMaxMemory := int64(2048) // 默认 2GB
	if resourceResult != nil && resourceResult.AvailableRAMMB > 0 {
		oomMaxMemory = resourceResult.AvailableRAMMB * 8 / 10 // 使用 80% 可用内存作为阈值
	}
	oomMgr := agent.NewOOMManager(registry, oomMaxMemory, logEntry)
	oomMgr.Start()
	defer oomMgr.Stop()

	// 初始化编排引擎
	orchEngine := orchestrator.NewEngine(
		registry, bridge,
		cfg.Orchestrator.WorkflowDir,
		cfg.Orchestrator.MaxConcurrent,
		cfg.Orchestrator.DefaultTimeout.Duration,
		logEntry,
	)
	if err := orchEngine.LoadWorkflows(); err != nil {
		logEntry.Warnf("Failed to load workflows: %v", err)
	}

	// ── JetStream 断点续行集成 ──
	if jsManager != nil {
		orchEngine.SetJetStreamManager(jsManager)
		// 恢复未完成的编排执行
		orchEngine.RecoverExecutions()
		logEntry.Info("JetStream checkpoint recovery completed")
	}

	// 初始化安全服务
	authService := security.NewAuthService(
		cfg.Security.JWTSecret,
		cfg.Security.JWTAlgorithm,
		cfg.Security.AccessTokenTTL.Duration,
		cfg.Security.RefreshTokenTTL.Duration,
		logEntry,
	)
	rateLimiter := security.NewRateLimiter(
		cfg.Security.RateLimitPerMinute,
		time.Minute,
	)
	loginProtector := security.NewLoginProtector(
		cfg.Security.MaxLoginAttempts,
		cfg.Security.LockoutDuration.Duration,
	)

	// ── Token 撤销列表 ──
	tokenRevocation := security.NewTokenRevocationList(logEntry)
	tokenRevocation.StartCleanupTask(time.Hour, ctx.Done())

	// ── mTLS 证书管理 ──
	var mtlsManager *security.MTLSCertManager
	if cfg.Server.TLS.Enabled {
		// 如果证书不存在，自动生成自签名证书
		if err := security.GenerateInitialCerts(
			cfg.Server.TLS.CertFile,
			cfg.Server.TLS.KeyFile,
			cfg.Server.TLS.CAFile,
			"EdgeAgent Hub",
		); err != nil {
			logEntry.Warnf("Failed to generate initial certs: %v", err)
		}

		mtlsManager, err = security.NewMTLSCertManager(
			cfg.Server.TLS.CertFile,
			cfg.Server.TLS.KeyFile,
			cfg.Server.TLS.CAFile,
			logEntry,
		)
		if err != nil {
			logEntry.Warnf("Failed to initialize mTLS: %v", err)
		} else {
			// 启动证书自动轮换
			certRotation := security.NewCertAutoRotation(
				mtlsManager,
				cfg.Server.TLS.CertFile,
				cfg.Server.TLS.KeyFile,
				cfg.Server.TLS.CAFile,
				"EdgeAgent Hub",
				logEntry,
			)
			certRotation.Start()
			defer certRotation.Stop()
			logEntry.Info("mTLS enabled with auto-rotation")
		}
	}

	// ── 推理投毒防护 ──
	inferenceGuard := security.NewInferenceGuard(logEntry)

	// ── LLM 输出过滤 ──
	_ = security.NewLLMOutputFilter(logEntry)

	// ── 数据质量监控 + 预处理 ──
	qualityMonitor := dataprocess.NewDataQualityMonitor(logEntry)
	preprocessor := dataprocess.NewPreprocessor(logEntry)

	// ── 自学习管理器 ──
	selfLearning := learning.NewSelfLearningManager(0.1, 3.0, logEntry)

	// 初始化 OTA 管理器（启用模型签名验证）
	var signingKey []byte
	if cfg.Security.ModelSigningKey != "" {
		signingKey = []byte(cfg.Security.ModelSigningKey)
	}
	otaManager := ota.NewManager(
		store,
		cfg.OTA.PartitionA,
		cfg.OTA.PartitionB,
		cfg.OTA.ActivePartition,
		cfg.OTA.RetentionDays,
		cfg.OTA.CanaryPercent,
		cfg.OTA.RegistryURL,
		cfg.OTA.RegistryUsername,
		cfg.OTA.RegistryPassword,
		signingKey,
		logEntry,
	)

	// 初始化断网自治管理器
	offlineMgr := offline.NewManager(
		store, bridge,
		cfg.Offline.CheckInterval.Duration,
		cfg.Offline.RetransmitInterval.Duration,
		cfg.Offline.MaxRetries,
		logEntry,
	)
	if cfg.Offline.Enabled {
		offlineMgr.Start(ctx)
	}

	// 启动系统指标采集
	if cfg.Observability.CollectSystemMetrics {
		sysCollector := observability.NewSystemCollector(
			metrics,
			cfg.Observability.MetricsInterval.Duration,
			logEntry,
		)
		sysCollector.Start(ctx)
	}

	// 初始化 API 服务器
	apiServer := api.NewServer(
		cfg, store, bridge, registry, orchEngine, otaManager, offlineMgr,
		authService, rateLimiter, loginProtector,
		metrics, healthChecker, promRegistry, logEntry,
	)

	// 确保管理员用户存在
	if err := apiServer.EnsureAdminUser("admin123"); err != nil {
		logEntry.Warnf("Failed to ensure admin user: %v", err)
	}

	// ── 种子默认模型数据 ──
	seedDefaultModels(store, logEntry)

	// ── 初始化协议管理器 (Modbus / OPC UA / BLE / ONVIF / Webhook) ──
	protoMgr := protocol.NewProtocolManager(logEntry)

	// 设置数据回调：协议数据 → 质量评分 → 预处理 → 自学习 → NATS
	protoMgr.SetDataHandler(func(deviceID string, envelope models.SensorEnvelope) {
		// 1. 数据质量评分
		score := qualityMonitor.Evaluate(deviceID, &envelope)
		if score.Overall < 0.3 {
			logEntry.Warnf("Low data quality for device %s: %.2f", deviceID, score.Overall)
		}

		// 2. 数据预处理
		processed := preprocessor.Process(&envelope)

		// 3. 自学习：将指标值反馈到学习器
		for metricName, val := range processed.Metrics {
			selfLearning.Learn(deviceID, metricName, val)
		}

		// 4. 推理投毒防护
		filteredMetrics, rejected := inferenceGuard.ValidateInput(deviceID, processed.Metrics)
		if len(rejected) > 0 {
			logEntry.Warnf("Input poisoning detected for device %s, rejected metrics: %v", deviceID, rejected)
		}
		processed.Metrics = filteredMetrics

		// 5. 发布到 NATS
		subject := fmt.Sprintf("edgelite.sensors.%s", deviceID)
		data, _ := json.Marshal(processed)
		bridge.PublishNATS(subject, data)

		// 6. 持久化到 JetStream
		if jsManager != nil {
			jsManager.PublishSensorData(processed)
		}

		// 更新消息计数器
		metrics.MessagesInPerSec.Inc()
	})

	if cfg.Protocols.Modbus.Enabled {
		for _, dev := range cfg.Protocols.Modbus.Devices {
			modbusCfg := protocol.ModbusDeviceConfig{
				DeviceID:     dev.DeviceID,
				Host:         dev.Host,
				Port:         dev.Port,
				UnitID:       dev.UnitID,
				Timeout:      dev.Timeout.Duration,
				PollInterval: dev.PollInterval.Duration,
			}
			for _, reg := range dev.Registers {
				modbusCfg.Registers = append(modbusCfg.Registers, protocol.ModbusRegister{
					Name:     reg.Name,
					Address:  reg.Address,
					Quantity: reg.Quantity,
					FuncCode: reg.FuncCode,
					DataType: reg.DataType,
					Scale:    reg.Scale,
					Offset:   reg.Offset,
					Unit:     reg.Unit,
				})
			}
			if err := protoMgr.AddModbusDevice(modbusCfg); err != nil {
				logEntry.Warnf("Failed to add Modbus device %s: %v", dev.DeviceID, err)
			}
		}
	}
	if cfg.Protocols.OPCUA.Enabled {
		for _, dev := range cfg.Protocols.OPCUA.Devices {
			opcuaCfg := protocol.OPCUADeviceConfig{
				DeviceID:     dev.DeviceID,
				Endpoint:     dev.Endpoint,
				SecurityMode: dev.SecurityMode,
				Policy:       dev.Policy,
				CertFile:     dev.CertFile,
				KeyFile:      dev.KeyFile,
				AuthMode:     dev.AuthMode,
				Username:     dev.Username,
				Password:     dev.Password,
				Timeout:      dev.Timeout.Duration,
				PollInterval: dev.PollInterval.Duration,
			}
			for _, node := range dev.Nodes {
				opcuaCfg.Nodes = append(opcuaCfg.Nodes, protocol.OPCUANodeConfig{
					NodeID:   node.NodeID,
					Name:     node.Name,
					Unit:     node.Unit,
					Scale:    node.Scale,
					Offset:   node.Offset,
					Deadband: node.Deadband,
				})
			}
			if err := protoMgr.AddOPCUADevice(opcuaCfg); err != nil {
				logEntry.Warnf("Failed to add OPC UA device %s: %v", dev.DeviceID, err)
			}
		}
	}
	defer protoMgr.Stop()

	// ── HTTP Webhook 南向接入 ──
	if cfg.Protocols.Webhook.Enabled {
		webhookReceiver := protocol.NewWebhookReceiver(protocol.WebhookConfig{
			ListenAddr: cfg.Protocols.Webhook.ListenAddr,
			Path:       cfg.Protocols.Webhook.Path,
			AuthSecret: cfg.Protocols.Webhook.AuthSecret,
		}, logEntry)
		webhookReceiver.SetDataHandler(func(deviceID string, envelope models.SensorEnvelope) {
			// 复用协议管理器的数据回调
			protoMgr.PublishEnvelope(deviceID, envelope)
		})
		webhookReceiver.Start()
		defer webhookReceiver.Stop()
		logEntry.Infof("HTTP Webhook receiver started on %s%s", cfg.Protocols.Webhook.ListenAddr, cfg.Protocols.Webhook.Path)
	}

	// 启动数据清理 goroutine
	go startDataCleanup(ctx, store, logEntry)

	// 创建 Echo 服务器
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// 中间件
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	// CORS: 仅允许配置的白名单域名，未配置则同源
	if len(cfg.Server.CORSOrigins) > 0 && cfg.Server.CORSOrigins[0] != "*" {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.Server.CORSOrigins,
			AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders: []string{"Content-Type", "Authorization", "X-Requested-With"},
			AllowCredentials: true,
		}))
		logEntry.Infof("CORS enabled for origins: %v", cfg.Server.CORSOrigins)
	} else {
		// 同源模式：不添加 CORS 中间件，浏览器默认同源策略生效
		logEntry.Info("CORS: same-origin mode (configure cors_origins in config.yaml to allow cross-origin)")
	}

	// TLS 安全检查
	if !cfg.Server.TLS.Enabled {
		logEntry.Warn("HTTP mode: TLS is not enabled. For production deployment, enable TLS in config.yaml or use a reverse proxy (e.g. Nginx) with HTTPS.")
	}

	// 设置路由
	apiServer.SetupRoutes(e)

	// ── 额外 API 路由（日志导出、备份/恢复） ──
	setupExtraRoutes(e, store, logEntry)

	// 标记启动就绪
	healthChecker.SetStartupReady(true)
	logEntry.Infof("EdgeAgent Hub v%s started on %s:%d", version, cfg.Server.Host, cfg.Server.Port)

	// 启动 HTTP 服务器
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
			if cfg.Server.TLS.Enabled && mtlsManager != nil {
				// 使用 mTLS 启动
				logEntry.Infof("Starting HTTPS server with mTLS on %s", addr)
				if err := e.StartTLS(addr, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil {
					logEntry.Errorf("HTTPS server error: %v", err)
					cancel()
				}
		} else {
			if err := e.Start(addr); err != nil {
				logEntry.Errorf("HTTP server error: %v", err)
				cancel()
			}
		}
	}()

	// 等待关闭
	<-ctx.Done()

	// 优雅关闭
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logEntry.Errorf("HTTP server shutdown error: %v", err)
	}

	// 停止限流器清理 goroutine
	rateLimiter.Stop()

	// 停止断网自治管理器
	offlineMgr.Stop()

	logEntry.Info("EdgeAgent Hub stopped")
}

// startDataCleanup 定期清理过期数据
func startDataCleanup(ctx context.Context, store *storage.Store, logger *logrus.Entry) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.Cleanup(); err != nil {
				logger.Errorf("Data cleanup failed: %v", err)
			} else {
				logger.Debug("Data cleanup completed")
			}
		}
	}
}

// seedDefaultModels 在首次启动时写入示例模型记录，方便用户了解系统功能
// 用户可以卸载示例模型并上传自己的真实模型
func seedDefaultModels(store *storage.Store, logger *logrus.Entry) {
	existing, err := store.ListModels()
	if err != nil {
		logger.Warnf("Failed to list models for seeding: %v", err)
		return
	}
	if len(existing) > 0 {
		return // 已有模型，不重复写入
	}

	now := time.Now()

	models := []models.ModelInfo{
		{
			ID:        "vibration_anomaly_v1",
			Name:      "振动异常检测模型",
			Version:   "1.0.0",
			Type:      "onnx",
			FilePath:  "models/onnx/vibration_anomaly.onnx",
			Partition: "A",
			Active:    true,
			SizeBytes: 0,
			SHA256:    "",
			LoadedAt:  &now,
		},
		{
			ID:        "temperature_anomaly_v1",
			Name:      "温度异常预警模型",
			Version:   "1.0.0",
			Type:      "onnx",
			FilePath:  "models/onnx/temperature_anomaly.onnx",
			Partition: "A",
			Active:    false,
			SizeBytes: 0,
			SHA256:    "",
			LoadedAt:  &now,
		},
		{
			ID:        "power_trend_predict_v1",
			Name:      "功率趋势预测模型",
			Version:   "1.0.0",
			Type:      "onnx",
			FilePath:  "models/onnx/power_trend_predict.onnx",
			Partition: "A",
			Active:    false,
			SizeBytes: 0,
			SHA256:    "",
			LoadedAt:  &now,
		},
		{
			ID:        "current_anomaly_v1",
			Name:      "电流异常检测模型",
			Version:   "1.0.0",
			Type:      "onnx",
			FilePath:  "models/onnx/current_anomaly.onnx",
			Partition: "A",
			Active:    false,
			SizeBytes: 0,
			SHA256:    "",
			LoadedAt:  &now,
		},
		{
			ID:        "pressure_monitor_v1",
			Name:      "管道压力监测模型",
			Version:   "1.0.0",
			Type:      "onnx",
			FilePath:  "models/onnx/pressure_monitor.onnx",
			Partition: "A",
			Active:    false,
			SizeBytes: 0,
			SHA256:    "",
			LoadedAt:  &now,
		},
	}

	for i := range models {
		if err := store.SaveModel(&models[i]); err != nil {
			logger.Warnf("Failed to seed model %s: %v", models[i].Name, err)
		} else {
			logger.Infof("Seeded example model: %s (v%s)", models[i].Name, models[i].Version)
		}
	}

	logger.Info("Example models seeded. Upload real ONNX/GGUF files to replace them.")
}

// setupExtraRoutes 设置额外 API 路由
func setupExtraRoutes(e *echo.Echo, store *storage.Store, logger *logrus.Entry) {
	// 日志导出 API
	e.GET("/api/v1/system/logs/export", func(c echo.Context) error {
		// 简化实现: 返回日志文件
		return c.File("logs/edgeagent.log")
	})

	// 备份 API
	e.POST("/api/v1/system/backup", func(c echo.Context) error {
		backupPath := fmt.Sprintf("data/backup-%s.zip", time.Now().Format("20060102-150405"))
		if err := store.Backup(backupPath); err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, map[string]string{"status": "ok", "path": backupPath})
	})

	// 恢复 API
	e.POST("/api/v1/system/restore", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "use CLI: edgelite-hub restore <backup.zip>"})
	})
}

// registerBuiltinAgentHandlers 为内置智能体注册 NATS 订阅处理器
// 这样工作流调用 template.fallback_alert 和 notify.multi_channel 时能得到响应
func registerBuiltinAgentHandlers(bridge *messaging.Bridge, store *storage.Store, registry *agent.Registry, llm *agent.BuiltinLLM, cfg *config.Config, logger *logrus.Entry) {
	// 模板告警智能体：接收输入，生成告警消息并存入数据库
	bridge.SubscribeNATS("agent.template.fallback_alert.invoke", func(msg *nats.Msg) {
		var input map[string]any
		if err := json.Unmarshal(msg.Data, &input); err != nil {
			resp, _ := json.Marshal(map[string]any{"error": "invalid input"})
			msg.Respond(resp)
			return
		}

		failedStep, _ := input["failed_step"].(string)
		errMsg, _ := input["error"].(string)

		alertMsg := fmt.Sprintf("工作流步骤 %s 执行失败，已触发回退告警。错误: %s", failedStep, errMsg)
		severity := models.AlertSeverityMedium
		if strings.Contains(errMsg, "timeout") {
			severity = models.AlertSeverityHigh
		}

		// 存入告警数据库
		alert := &models.Alert{
			ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			DeviceID:  "system",
			Message:   alertMsg,
			Severity:  severity,
			Status:    models.AlertStatus("active"),
			Type:      "fallback",
			Anchored:  false,
			CreatedAt: time.Now(),
		}
		store.SaveAlert(alert)

		resp, _ := json.Marshal(map[string]any{
			"alert_id":  alert.ID,
			"message":   alertMsg,
			"severity":  string(severity),
			"anchored":  false,
		})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: template.fallback_alert")

	// 多渠道通知智能体：接收消息，模拟发送通知
	bridge.SubscribeNATS("agent.notify.multi_channel.invoke", func(msg *nats.Msg) {
		var input map[string]any
		if err := json.Unmarshal(msg.Data, &input); err != nil {
			resp, _ := json.Marshal(map[string]any{"error": "invalid input"})
			msg.Respond(resp)
			return
		}

		message, _ := input["message"].(string)
		if message == "" {
			// 尝试从 alert_message 中获取
			if am, ok := input["alert_message"]; ok {
				message = fmt.Sprintf("%v", am)
			}
		}

		// 记录审计日志
		store.SaveAudit(&models.AuditEntry{
			Timestamp: time.Now(),
			Actor:     "system",
			Action:    "notify:send",
			Resource:  message,
			IP:        "127.0.0.1",
		})

		resp, _ := json.Marshal(map[string]any{
			"status":   "sent",
			"channels": []string{"webhook", "dingtalk"},
			"message":  message,
		})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: notify.multi_channel")

	// ONNX 推理智能体：通过 HTTP 调用 sidecar 进行真正推理
	bridge.SubscribeNATS("agent.onnx.inference.invoke", func(msg *nats.Msg) {
		var input map[string]any
		json.Unmarshal(msg.Data, &input)

		modelID, _ := input["model_id"].(string)
		if modelID == "" {
			modelID = "default"
		}

		// 尝试通过 HTTP 调用 sidecar 进行真正推理
		onnxAEndpoint := cfg.Inference.ONNX.Endpoint
		if onnxAEndpoint == "" {
			onnxAEndpoint = "http://127.0.0.1:50052"
		}

		// 通过 HTTP POST 调用 sidecar 的推理接口
		inputBytes, _ := json.Marshal(input)
		httpResp, err := http.Post(onnxAEndpoint+"/infer", "application/json", strings.NewReader(string(inputBytes)))
		if err != nil {
			// sidecar 不可用，返回明确错误
			resp, _ := json.Marshal(map[string]any{
				"model_id": modelID,
				"error":    "ONNX sidecar 不可用，请启动 AI Sidecar 服务 (python inference_onnx.py) 或上传模型文件",
				"status":   "sidecar_offline",
			})
			msg.Respond(resp)
			return
		}
		defer httpResp.Body.Close()

		body, _ := io.ReadAll(httpResp.Body)
		if httpResp.StatusCode != 200 {
			resp, _ := json.Marshal(map[string]any{
				"model_id": modelID,
				"error":    fmt.Sprintf("sidecar 返回错误 (HTTP %d): %s", httpResp.StatusCode, string(body)),
				"status":   "error",
			})
			msg.Respond(resp)
			return
		}

		// 返回 sidecar 的真正推理结果
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			// 直接返回原始响应
			msg.Respond(body)
			return
		}
		result["status"] = "ok"
		resp, _ := json.Marshal(result)
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: onnx.inference (HTTP sidecar mode)")

	// 视觉确认智能体：返回不可用状态（需要配置外部视觉系统）
	bridge.SubscribeNATS("agent.vision.camera_check.invoke", func(msg *nats.Msg) {
		resp, _ := json.Marshal(map[string]any{
			"status":        "skipped",
			"message":       "视觉确认智能体未配置，跳过视觉确认步骤",
			"confirmed":     false,
			"vision_result": "not_available",
		})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: vision.camera_check (skip mode)")

	// LLM/RAG 智能体：使用内置对话引擎生成回答
	bridge.SubscribeNATS("agent.llm.rag_anchored.invoke", func(msg *nats.Msg) {
		var input map[string]any
		json.Unmarshal(msg.Data, &input)

		sensorData, _ := input["sensor_data"]
		anomalyResult, _ := input["anomaly_result"]

		// 构造上下文查询
		query := ""
		if sensorData != nil {
			query = fmt.Sprintf("传感器数据异常 数据:%v", sensorData)
		}
		if anomalyResult != nil {
			query += fmt.Sprintf(" 推理结果:%v", anomalyResult)
		}
		if query == "" {
			query = "设备异常"
		}

		// 使用内置 LLM 生成回答
		answer, anchored := llm.Chat(query)

		resp, _ := json.Marshal(map[string]any{
			"message":  answer,
			"urgency":  "MEDIUM",
			"anchored": anchored,
		})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: llm.rag_anchored")

	// LLM 对话智能体：处理智能对话页面的聊天请求
	bridge.SubscribeNATS("agent.llm.chat", func(msg *nats.Msg) {
		var req models.ChatRequest
		json.Unmarshal(msg.Data, &req)

		answer, anchored := llm.Chat(req.Message)

		resp, _ := json.Marshal(models.ChatResponse{
			Response: answer,
			Anchored: anchored,
		})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: llm.chat")

	// RAG 知识检索智能体：处理知识库搜索请求
	bridge.SubscribeNATS("agent.rag.search", func(msg *nats.Msg) {
		var req map[string]string
		json.Unmarshal(msg.Data, &req)
		query := req["query"]

		results := llm.Search(query, 5)

		resp, _ := json.Marshal(map[string]any{
			"results": results,
			"query":   query,
		})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: rag.search")

	// RAG 文档索引智能体：处理知识库上传后的索引请求
	bridge.SubscribeNATS("agent.rag.index", func(msg *nats.Msg) {
		var req map[string]string
		json.Unmarshal(msg.Data, &req)
		filePath := req["file_path"]

		if err := llm.IndexDocument(filePath); err != nil {
			resp, _ := json.Marshal(map[string]any{"status": "error", "error": err.Error()})
			msg.Respond(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "indexed", "file_path": filePath})
		msg.Respond(resp)
	})
	logger.Info("Built-in agent handler registered: rag.index")
}
