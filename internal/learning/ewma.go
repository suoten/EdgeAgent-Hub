package learning

import (
	"math"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// EWMAState 指数加权移动平均状态
type EWMAState struct {
	Mean   float64 `json:"mean"`
	Var    float64 `json:"var"`     // EWMA 方差
	Count  int64   `json:"count"`   // 样本数
	Alpha  float64 `json:"alpha"`   // 平滑因子 (0 < alpha <= 1)
}

// ThresholdState 动态阈值状态
type ThresholdState struct {
	Upper float64 `json:"upper"` // 上限阈值
	Lower float64 `json:"lower"` // 下限阈值
	ZScore float64 `json:"z_score"` // Z-Score 倍数 (通常 3.0)
}

// MetricLearner 单指标的在线学习器
type MetricLearner struct {
	mu sync.Mutex
	ewma       EWMAState
	threshold  ThresholdState
	minSamples int     // 最少样本数才生效
	maxMean    float64 // 均值上限约束
	minMean    float64 // 均值下限约束
	maxStdDev  float64 // 标准差上限约束
	logger     *logrus.Entry
}

// NewMetricLearner 创建指标学习器
func NewMetricLearner(alpha, zScore float64, logger *logrus.Entry) *MetricLearner {
	return &MetricLearner{
		ewma: EWMAState{
			Alpha: alpha,
		},
		threshold: ThresholdState{
			ZScore: zScore,
		},
		minSamples: 30,
		maxMean:    math.MaxFloat64,
		minMean:    -math.MaxFloat64,
		maxStdDev:  math.MaxFloat64,
		logger:     logger,
	}
}

// SetBounds 设置安全边界（防止漂移失控）
func (l *MetricLearner) SetBounds(minMean, maxMean, maxStdDev float64) {
	l.minMean = minMean
	l.maxMean = maxMean
	l.maxStdDev = maxStdDev
}

// Learn 学习一个新样本
func (l *MetricLearner) Learn(value float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.ewma.Count == 0 {
		// 第一个样本
		l.ewma.Mean = value
		l.ewma.Var = 0
	} else {
		// EWMA 更新
		alpha := l.ewma.Alpha
		delta := value - l.ewma.Mean
		l.ewma.Mean = l.ewma.Mean + alpha*delta
		l.ewma.Var = (1-alpha)*(l.ewma.Var+alpha*delta*delta)

		// 安全边界约束
		if l.ewma.Mean > l.maxMean {
			l.ewma.Mean = l.maxMean
		}
		if l.ewma.Mean < l.minMean {
			l.ewma.Mean = l.minMean
		}
		stdDev := math.Sqrt(l.ewma.Var)
		if stdDev > l.maxStdDev {
			l.ewma.Var = l.maxStdDev * l.maxStdDev
		}
	}
	l.ewma.Count++

	// 更新动态阈值
	stdDev := math.Sqrt(l.ewma.Var)
	l.threshold.Upper = l.ewma.Mean + l.threshold.ZScore*stdDev
	l.threshold.Lower = l.ewma.Mean - l.threshold.ZScore*stdDev
}

// IsAnomaly 检测是否为异常
func (l *MetricLearner) IsAnomaly(value float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.ewma.Count < int64(l.minSamples) {
		return false // 样本不足，不判断
	}

	if l.ewma.Var == 0 {
		return false
	}

	zScore := math.Abs(value - l.ewma.Mean) / math.Sqrt(l.ewma.Var)
	return zScore > l.threshold.ZScore
}

// GetState 获取学习状态
func (l *MetricLearner) GetState() (EWMAState, ThresholdState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ewma, l.threshold
}

// ── 设备级学习器 ──

// DeviceLearner 设备级自学习闭环
type DeviceLearner struct {
	mu       sync.RWMutex
	metrics  map[string]*MetricLearner // metricName -> learner
	alpha    float64
	zScore   float64
	logger   *logrus.Entry
	lastUpdate time.Time
}

// NewDeviceLearner 创建设备学习器
func NewDeviceLearner(alpha, zScore float64, logger *logrus.Entry) *DeviceLearner {
	return &DeviceLearner{
		metrics: make(map[string]*MetricLearner),
		alpha:   alpha,
		zScore:  zScore,
		logger:  logger,
	}
}

// Learn 学习设备数据
func (d *DeviceLearner) Learn(metricName string, value float64) {
	d.mu.RLock()
	learner, ok := d.metrics[metricName]
	d.mu.RUnlock()

	if !ok {
		d.mu.Lock()
		// 双重检查
		if learner, ok = d.metrics[metricName]; !ok {
			learner = NewMetricLearner(d.alpha, d.zScore, d.logger)
			d.metrics[metricName] = learner
		}
		d.mu.Unlock()
	}

	learner.Learn(value)
	d.lastUpdate = time.Now()
}

// CheckAnomaly 检测异常
func (d *DeviceLearner) CheckAnomaly(metricName string, value float64) bool {
	d.mu.RLock()
	learner, ok := d.metrics[metricName]
	d.mu.RUnlock()
	if !ok {
		return false
	}
	return learner.IsAnomaly(value)
}

// GetThresholds 获取所有指标的动态阈值
func (d *DeviceLearner) GetThresholds() map[string]ThresholdState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]ThresholdState)
	for name, learner := range d.metrics {
		_, threshold := learner.GetState()
		result[name] = threshold
	}
	return result
}

// GetMetricState 获取指标学习状态
func (d *DeviceLearner) GetMetricState(metricName string) (EWMAState, ThresholdState, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	learner, ok := d.metrics[metricName]
	if !ok {
		return EWMAState{}, ThresholdState{}, false
	}
	ewma, threshold := learner.GetState()
	return ewma, threshold, true
}

// ── 全局自学习管理器 ──

// SelfLearningManager 自学习闭环管理器
type SelfLearningManager struct {
	mu      sync.RWMutex
	devices map[string]*DeviceLearner // deviceID -> learner
	alpha   float64
	zScore  float64
	logger  *logrus.Entry
}

// NewSelfLearningManager 创建自学习管理器
func NewSelfLearningManager(alpha, zScore float64, logger *logrus.Entry) *SelfLearningManager {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.1 // 默认平滑因子
	}
	if zScore <= 0 {
		zScore = 3.0 // 默认3倍标准差
	}
	return &SelfLearningManager{
		devices: make(map[string]*DeviceLearner),
		alpha:   alpha,
		zScore:  zScore,
		logger:  logger,
	}
}

// Learn 学习设备数据
func (m *SelfLearningManager) Learn(deviceID, metricName string, value float64) {
	m.mu.RLock()
	learner, ok := m.devices[deviceID]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		if learner, ok = m.devices[deviceID]; !ok {
			learner = NewDeviceLearner(m.alpha, m.zScore, m.logger)
			m.devices[deviceID] = learner
		}
		m.mu.Unlock()
	}

	learner.Learn(metricName, value)
}

// CheckAnomaly 检测异常
func (m *SelfLearningManager) CheckAnomaly(deviceID, metricName string, value float64) bool {
	m.mu.RLock()
	learner, ok := m.devices[deviceID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return learner.CheckAnomaly(metricName, value)
}

// GetDeviceThresholds 获取设备所有指标的动态阈值
func (m *SelfLearningManager) GetDeviceThresholds(deviceID string) map[string]ThresholdState {
	m.mu.RLock()
	learner, ok := m.devices[deviceID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return learner.GetThresholds()
}

// GetDeviceLearner 获取设备学习器
func (m *SelfLearningManager) GetDeviceLearner(deviceID string) *DeviceLearner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.devices[deviceID]
}

// ListDevices 列出所有正在学习的设备
func (m *SelfLearningManager) ListDevices() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var devices []string
	for deviceID := range m.devices {
		devices = append(devices, deviceID)
	}
	return devices
}
