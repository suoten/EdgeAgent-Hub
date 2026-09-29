package dataprocess

import (
	"math"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// QualityScore 数据质量评分
type QualityScore struct {
	Completeness float64 `json:"completeness"` // 完整性: 采集率
	Latency      float64 `json:"latency"`      // 延迟评分
	Integrity    float64 `json:"integrity"`    // 完整性: 无缺失字段
	AnomalyRate  float64 `json:"anomaly_rate"` // 异常率评分
	Continuity   float64 `json:"continuity"`   // 连续性评分
	Overall      float64 `json:"overall"`      // 综合评分
}

// DataQualityMonitor 数据质量监控器
type DataQualityMonitor struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// 每个设备的统计
	deviceStats map[string]*deviceQualityStats
}

type deviceQualityStats struct {
	totalReceived   int64
	totalExpected   int64
	lastReceived    time.Time
	lastLatency     float64
	missingFields   int64
	anomalyCount    int64
	totalCount      int64
	expectedMetrics map[string]bool
	metricValues    map[string][]float64 // 保留最近N个值用于异常检测
}

// NewDataQualityMonitor 创建数据质量监控器
func NewDataQualityMonitor(logger *logrus.Entry) *DataQualityMonitor {
	return &DataQualityMonitor{
		logger:      logger,
		deviceStats: make(map[string]*deviceQualityStats),
	}
}

// SetExpectedMetrics 设置设备预期指标列表
func (m *DataQualityMonitor) SetExpectedMetrics(deviceID string, metrics []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, ok := m.deviceStats[deviceID]
	if !ok {
		stats = &deviceQualityStats{
			expectedMetrics: make(map[string]bool),
			metricValues:    make(map[string][]float64),
		}
		m.deviceStats[deviceID] = stats
	}
	stats.expectedMetrics = make(map[string]bool)
	for _, metric := range metrics {
		stats.expectedMetrics[metric] = true
	}
}

// Evaluate 评估数据质量
func (m *DataQualityMonitor) Evaluate(deviceID string, envelope *models.SensorEnvelope) QualityScore {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, ok := m.deviceStats[deviceID]
	if !ok {
		stats = &deviceQualityStats{
			expectedMetrics: make(map[string]bool),
			metricValues:    make(map[string][]float64),
		}
		m.deviceStats[deviceID] = stats
	}

	stats.totalReceived++
	stats.lastReceived = time.Now()
	stats.totalCount++

	// 延迟评分
	now := time.Now().UnixMilli()
	latency := float64(now - envelope.Timestamp)
	if latency < 0 {
		latency = 0
	}
	stats.lastLatency = latency

	// 完整性: 检查预期指标是否都有
	missing := 0
	if len(stats.expectedMetrics) > 0 {
		for expected := range stats.expectedMetrics {
			if _, ok := envelope.Metrics[expected]; !ok {
				missing++
			}
		}
	}
	stats.missingFields += int64(missing)

	// 更新指标值历史 (保留最近100个)
	for name, val := range envelope.Metrics {
		values := stats.metricValues[name]
		if len(values) >= 100 {
			values = values[1:]
		}
		values = append(values, val)
		stats.metricValues[name] = values
	}

	// 异常检测: Z-Score
	anomalies := 0
	for name, val := range envelope.Metrics {
		values := stats.metricValues[name]
		if len(values) < 10 {
			continue
		}
		mean, stdDev := computeStats(values[:len(values)-1]) // 排除当前值
		if stdDev > 0 {
			zScore := math.Abs(val - mean) / stdDev
			if zScore > 3.0 {
				anomalies++
			}
		}
	}
	if anomalies > 0 {
		stats.anomalyCount++
	}

	// 计算综合评分
	return m.computeScore(stats)
}

func (m *DataQualityMonitor) computeScore(stats *deviceQualityStats) QualityScore {
	// 完整性 (采集率)
	completeness := 1.0
	if stats.totalExpected > 0 {
		completeness = float64(stats.totalReceived) / float64(stats.totalExpected)
		if completeness > 1.0 {
			completeness = 1.0
		}
	}

	// 延迟评分: <100ms=1.0, >5s=0.0
	latencyScore := 1.0
	if stats.lastLatency > 100 {
		latencyScore = 1.0 - (stats.lastLatency-100)/4900.0
		if latencyScore < 0 {
			latencyScore = 0
		}
	}

	// 完整性 (无缺失字段)
	integrity := 1.0
	if stats.totalCount > 0 {
		totalExpected := stats.totalCount * int64(len(stats.expectedMetrics))
		if totalExpected > 0 {
			integrity = 1.0 - float64(stats.missingFields)/float64(totalExpected)
		}
	}

	// 异常率评分
	anomalyRateScore := 1.0
	if stats.totalCount > 0 {
		anomalyRateScore = 1.0 - float64(stats.anomalyCount)/float64(stats.totalCount)
	}

	// 连续性: 检查时间间隔
	continuity := 1.0 // 简化实现

	overall := (completeness*0.25 + latencyScore*0.20 + integrity*0.25 + anomalyRateScore*0.20 + continuity*0.10)

	return QualityScore{
		Completeness: round(completeness, 4),
		Latency:      round(latencyScore, 4),
		Integrity:    round(integrity, 4),
		AnomalyRate:  round(anomalyRateScore, 4),
		Continuity:   round(continuity, 4),
		Overall:      round(overall, 4),
	}
}

// GetScore 获取设备当前质量评分
func (m *DataQualityMonitor) GetScore(deviceID string) QualityScore {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats, ok := m.deviceStats[deviceID]
	if !ok {
		return QualityScore{}
	}
	return m.computeScore(stats)
}

// ── 数据预处理管线 ──

// PreprocessConfig 预处理配置
type PreprocessConfig struct {
	Scale     float64 `yaml:"scale" json:"scale"`
	Offset    float64 `yaml:"offset" json:"offset"`
	Deadband  float64 `yaml:"deadband" json:"deadband"`
	MinLimit  float64 `yaml:"min_limit" json:"min_limit"`
	MaxLimit  float64 `yaml:"max_limit" json:"max_limit"`
	Filter    string  `yaml:"filter" json:"filter"` // none, moving_avg, exponential
	FilterN   int     `yaml:"filter_n" json:"filter_n"` // 滤波窗口大小
}

// Preprocessor 数据预处理器
type Preprocessor struct {
	configs  map[string]map[string]PreprocessConfig // deviceID -> metricName -> config
	history  map[string]map[string][]float64       // 滤波历史
	mu       sync.RWMutex
	logger   *logrus.Entry
}

// NewPreprocessor 创建预处理器
func NewPreprocessor(logger *logrus.Entry) *Preprocessor {
	return &Preprocessor{
		configs: make(map[string]map[string]PreprocessConfig),
		history: make(map[string]map[string][]float64),
		logger:  logger,
	}
}

// SetConfig 设置预处理配置
func (p *Preprocessor) SetConfig(deviceID string, configs map[string]PreprocessConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.configs[deviceID] = configs
}

// Process 处理传感器信封
func (p *Preprocessor) Process(envelope *models.SensorEnvelope) *models.SensorEnvelope {
	p.mu.Lock()
	defer p.mu.Unlock()

	deviceConfigs, ok := p.configs[envelope.DeviceID]
	if !ok {
		return envelope // 无配置，原样返回
	}

	processed := &models.SensorEnvelope{
		DeviceID:  envelope.DeviceID,
		Protocol:  envelope.Protocol,
		Timestamp: envelope.Timestamp,
		Metrics:   make(map[string]float64),
		Metadata:  envelope.Metadata,
	}

	for name, val := range envelope.Metrics {
		cfg, hasCfg := deviceConfigs[name]
		if !hasCfg {
			processed.Metrics[name] = val
			continue
		}

		// 1. 缩放和偏移
		if cfg.Scale != 0 {
			val = val*cfg.Scale + cfg.Offset
		}

		// 2. 限幅
		if cfg.MinLimit != 0 || cfg.MaxLimit != 0 {
			if val < cfg.MinLimit {
				val = cfg.MinLimit
			}
			if val > cfg.MaxLimit {
				val = cfg.MaxLimit
			}
		}

		// 3. 滤波
		switch cfg.Filter {
		case "moving_avg":
			val = p.movingAverage(envelope.DeviceID, name, val, cfg.FilterN)
		case "exponential":
			val = p.exponentialSmoothing(envelope.DeviceID, name, val, 0.3)
		}

		// 4. 死区过滤 (如果变化量小于死区，使用上一次的值)
		if cfg.Deadband > 0 {
			if hist, ok := p.history[envelope.DeviceID]; ok {
				if prevVals, ok := hist[name]; ok && len(prevVals) > 0 {
					prev := prevVals[len(prevVals)-1]
					diff := val - prev
					if diff < 0 {
						diff = -diff
					}
					if diff < cfg.Deadband {
						val = prev // 在死区内，保持不变
					}
				}
			}
		}

		processed.Metrics[name] = val
	}

	return processed
}

// movingAverage 移动平均滤波
func (p *Preprocessor) movingAverage(deviceID, metric string, val float64, n int) float64 {
	if n <= 1 {
		return val
	}

	if p.history[deviceID] == nil {
		p.history[deviceID] = make(map[string][]float64)
	}
	hist := p.history[deviceID][metric]
	if len(hist) >= n {
		hist = hist[1:]
	}
	hist = append(hist, val)
	p.history[deviceID][metric] = hist

	sum := 0.0
	for _, v := range hist {
		sum += v
	}
	return sum / float64(len(hist))
}

// exponentialSmoothing 指数平滑
func (p *Preprocessor) exponentialSmoothing(deviceID, metric string, val, alpha float64) float64 {
	if p.history[deviceID] == nil {
		p.history[deviceID] = make(map[string][]float64)
	}
	hist := p.history[deviceID][metric]
	if len(hist) == 0 {
		p.history[deviceID][metric] = []float64{val}
		return val
	}
	prev := hist[len(hist)-1]
	smoothed := alpha*val + (1-alpha)*prev
	p.history[deviceID][metric] = append(hist, smoothed)
	return smoothed
}

// ── 辅助函数 ──

func computeStats(values []float64) (mean, stdDev float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	stdDev = math.Sqrt(variance)
	return mean, stdDev
}

func round(val float64, precision int) float64 {
	factor := math.Pow(10, float64(precision))
	return math.Round(val*factor) / factor
}
