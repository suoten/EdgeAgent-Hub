package observability

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Metrics 包含所有 Prometheus 指标
type Metrics struct {
	// 推理性能
	InferenceLatency    *prometheus.HistogramVec
	InferenceThroughput prometheus.Counter
	InferenceConfidence *prometheus.HistogramVec

	// 模型管理
	ModelLoadedCount prometheus.Gauge
	ModelMemoryBytes *prometheus.GaugeVec

	// 编排引擎
	WorkflowActiveCount   prometheus.Gauge
	WorkflowDuration      *prometheus.HistogramVec
	WorkflowFailureRate   prometheus.Counter

	// 协议接入
	MessagesInPerSec  prometheus.Counter
	MessagesOutPerSec prometheus.Counter

	// 系统资源
	CPUUsagePercent   prometheus.Gauge
	MemoryUsageBytes  prometheus.Gauge
	DiskUsagePercent  prometheus.Gauge

	// 网络状态
	CloudConnected prometheus.Gauge

	// 断网自治
	OfflineQueueLength prometheus.Gauge

	// RAG
	RAGRetrievalMS      *prometheus.HistogramVec
	RAGAnchorPassRate   prometheus.Counter
	RAGAnchorTotalCount prometheus.Counter

	// API 请求
	APIRequestCount    *prometheus.CounterVec
	APIRequestDuration *prometheus.HistogramVec
}

// NewMetrics 创建并注册所有 Prometheus 指标
func NewMetrics(registry *prometheus.Registry) *Metrics {
	factory := promauto.With(registry)

	m := &Metrics{
		InferenceLatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "inference_latency_ms",
				Help:    "Inference latency in milliseconds",
				Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
			},
			[]string{"model_id", "agent_id"},
		),
		InferenceThroughput: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "inference_throughput_total",
				Help: "Total number of inferences",
			},
		),
		InferenceConfidence: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "inference_confidence",
				Help:    "Inference confidence distribution",
				Buckets: []float64{0.1, 0.3, 0.5, 0.7, 0.8, 0.85, 0.9, 0.95, 0.99, 1.0},
			},
			[]string{"model_id"},
		),
		ModelLoadedCount: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "model_loaded_count",
				Help: "Number of loaded models",
			},
		),
		ModelMemoryBytes: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "model_memory_bytes",
				Help: "Model memory usage in bytes",
			},
			[]string{"model_id"},
		),
		WorkflowActiveCount: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "workflow_active_count",
				Help: "Number of active workflow executions",
			},
		),
		WorkflowDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "workflow_duration_ms",
				Help:    "Workflow end-to-end duration in milliseconds",
				Buckets: []float64{10, 50, 100, 500, 1000, 2000, 3000, 5000, 10000, 30000},
			},
			[]string{"workflow_name", "status"},
		),
		WorkflowFailureRate: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "workflow_failure_total",
				Help: "Total number of failed workflows",
			},
		),
		MessagesInPerSec: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "messages_in_total",
				Help: "Total inbound messages",
			},
		),
		MessagesOutPerSec: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "messages_out_total",
				Help: "Total outbound messages",
			},
		),
		CPUUsagePercent: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "cpu_usage_percent",
				Help: "CPU usage percentage",
			},
		),
		MemoryUsageBytes: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "memory_usage_bytes",
				Help: "Memory usage in bytes",
			},
		),
		DiskUsagePercent: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "disk_usage_percent",
				Help: "Disk usage percentage",
			},
		),
		CloudConnected: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "cloud_connected",
				Help: "Cloud connection status (1=connected, 0=disconnected)",
			},
		),
		OfflineQueueLength: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "offline_queue_length",
				Help: "Offline queue length",
			},
		),
		RAGRetrievalMS: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rag_retrieval_ms",
				Help:    "RAG retrieval latency in milliseconds",
				Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
			},
			[]string{},
		),
		RAGAnchorPassRate: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "rag_anchor_pass_total",
				Help: "Total RAG anchor verification passes",
			},
		),
		RAGAnchorTotalCount: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "rag_anchor_total",
				Help: "Total RAG anchor verifications",
			},
		),
		APIRequestCount: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_request_total",
				Help: "Total API requests",
			},
			[]string{"method", "path", "status"},
		),
		APIRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_request_duration_ms",
				Help:    "API request duration in milliseconds",
				Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
			},
			[]string{"method", "path"},
		),
	}

	return m
}

// ── 健康检查 ──

// HealthChecker 健康检查器
type HealthChecker struct {
	mu           sync.RWMutex
	components   map[string]string
	startupReady atomic.Bool
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		components: make(map[string]string),
	}
}

// SetComponent 设置组件状态
func (h *HealthChecker) SetComponent(name, status string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.components[name] = status
}

// SetStartupReady 设置启动就绪状态
func (h *HealthChecker) SetStartupReady(ready bool) {
	h.startupReady.Store(ready)
}

// Liveness 存活探针检查
func (h *HealthChecker) Liveness() map[string]any {
	return map[string]any{
		"status":    "alive",
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

// Readiness 就绪探针检查
func (h *HealthChecker) Readiness() map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()

	allReady := true
	componentsCopy := make(map[string]string)
	for k, v := range h.components {
		componentsCopy[k] = v
		if v != "ok" && v != "ready" && v != "connected" {
			allReady = false
		}
	}

	status := "ready"
	if !allReady {
		status = "not_ready"
	}

	return map[string]any{
		"status":     status,
		"components": componentsCopy,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
}

// Startup 启动探针检查
func (h *HealthChecker) Startup() map[string]any {
	ready := h.startupReady.Load()
	status := "started"
	if !ready {
		status = "starting"
	}
	return map[string]any{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

// ── 系统指标采集 ──

// SystemCollector 系统指标采集器
type SystemCollector struct {
	metrics   *Metrics
	interval  time.Duration
	logger    *logrus.Entry
	cancel    context.CancelFunc
}

// NewSystemCollector 创建系统指标采集器
func NewSystemCollector(metrics *Metrics, interval time.Duration, logger *logrus.Entry) *SystemCollector {
	return &SystemCollector{
		metrics:  metrics,
		interval: interval,
		logger:   logger,
	}
}

// Start 启动系统指标采集
func (s *SystemCollector) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go s.collectLoop(ctx)
	s.logger.Info("System metrics collector started")
}

// Stop 停止采集
func (s *SystemCollector) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *SystemCollector) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collect()
		}
	}
}

func (s *SystemCollector) collect() {
	// CPU 使用率
	if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
		s.metrics.CPUUsagePercent.Set(cpuPercent[0])
	}

	// 内存使用
	if memInfo, err := mem.VirtualMemory(); err == nil {
		s.metrics.MemoryUsageBytes.Set(float64(memInfo.Used))
	}

	// 磁盘使用
	if diskStat, err := diskUsage(); err == nil {
		s.metrics.DiskUsagePercent.Set(diskStat)
	}
}

// diskUsage 获取磁盘使用率 (跨平台简化实现)
func diskUsage() (float64, error) {
	// 使用 gopsutil 获取磁盘使用率
	diskStat, err := disk.Usage("/")
	if err != nil {
		// 尝试 Windows 路径
		diskStat, err = disk.Usage("C:")
		if err != nil {
			return 0, err
		}
	}
	return diskStat.UsedPercent, nil
}

// ── 日志设置 ──

// SetupLogger 配置结构化日志
func SetupLogger(level, format, logFile string, maxSize, maxDays int) *logrus.Logger {
	logger := logrus.New()

	// 设置日志级别
	switch level {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logger.SetLevel(logrus.FatalLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	// 设置日志格式
	if format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}

	// 设置日志输出: 同时输出到文件和控制台
	if logFile != "" {
		// 确保目录存在
		dir := filepath.Dir(logFile)
		if dir != "" && dir != "." {
			os.MkdirAll(dir, 0755)
		}

		fileWriter := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    maxSize, // MB
			MaxBackups: 3,
			MaxAge:     maxDays,
			Compress:   true,
		}
		// 同时输出到文件和控制台
		logger.SetOutput(io.MultiWriter(os.Stdout, fileWriter))
	}

	return logger
}
