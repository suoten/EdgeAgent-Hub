package learning

import (
	"math"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestMetricLearnerBasic(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	learner := NewMetricLearner(0.1, 3.0, logger)

	// 学习正常数据
	for i := 0; i < 50; i++ {
		learner.Learn(50.0 + float64(i%5-2)) // 48~52 范围
	}

	// 正常值不应被标记为异常
	if learner.IsAnomaly(51.0) {
		t.Error("51.0 should not be anomaly")
	}

	// 极端值应被标记为异常
	if !learner.IsAnomaly(100.0) {
		t.Error("100.0 should be anomaly")
	}
}

func TestMetricLearnerMinSamples(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	learner := NewMetricLearner(0.1, 3.0, logger)

	// 样本不足时不应判断异常
	learner.Learn(50.0)
	if learner.IsAnomaly(1000.0) {
		t.Error("should not detect anomaly with insufficient samples")
	}
}

func TestMetricLearnerBounds(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	learner := NewMetricLearner(0.5, 3.0, logger)
	learner.SetBounds(40, 60, 10) // 均值限制在 40~60, 标准差限制在 10

	// 输入极端值，均值不应超出边界
	for i := 0; i < 100; i++ {
		learner.Learn(1000.0)
	}

	ewma, _ := learner.GetState()
	if ewma.Mean > 60 {
		t.Errorf("mean should be bounded at 60, got %f", ewma.Mean)
	}
}

func TestDeviceLearner(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	learner := NewDeviceLearner(0.1, 3.0, logger)

	// 学习多个指标
	for i := 0; i < 50; i++ {
		learner.Learn("temperature", 60.0+float64(i%3-1))
		learner.Learn("vibration", 3.5+float64(i%3-1)*0.1)
	}

	// 检查阈值
	thresholds := learner.GetThresholds()
	if len(thresholds) != 2 {
		t.Errorf("expected 2 thresholds, got %d", len(thresholds))
	}

	// 正常值不异常
	if learner.CheckAnomaly("temperature", 61.0) {
		t.Error("61.0 temperature should not be anomaly")
	}

	// 异常值
	if !learner.CheckAnomaly("temperature", 200.0) {
		t.Error("200.0 temperature should be anomaly")
	}
}

func TestSelfLearningManager(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	mgr := NewSelfLearningManager(0.1, 3.0, logger)

	// 学习多个设备
	for i := 0; i < 50; i++ {
		mgr.Learn("device1", "temp", 50.0+float64(i%3-1))
		mgr.Learn("device2", "pressure", 100.0+float64(i%3-1))
	}

	// 列出设备
	devices := mgr.ListDevices()
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}

	// 检查异常
	if !mgr.CheckAnomaly("device1", "temp", 500.0) {
		t.Error("500.0 should be anomaly for device1")
	}
	if mgr.CheckAnomaly("device2", "pressure", 101.0) {
		t.Error("101.0 should not be anomaly for device2")
	}

	// 获取阈值
	thresholds := mgr.GetDeviceThresholds("device1")
	if len(thresholds) != 1 {
		t.Errorf("expected 1 threshold, got %d", len(thresholds))
	}
}

func TestEWMAConvergence(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	learner := NewMetricLearner(0.05, 3.0, logger)

	// 模拟正态分布数据，均值=100，标准差≈5
	for i := 0; i < 1000; i++ {
		val := 100.0 + 5.0*math.Sin(float64(i)*0.1) // 简化的正态模拟
		learner.Learn(val)
	}

	ewma, threshold := learner.GetState()
	// 均值应接近 100
	if math.Abs(ewma.Mean-100.0) > 5.0 {
		t.Errorf("mean should be near 100, got %f", ewma.Mean)
	}
	// 阈值上限应大于均值
	if threshold.Upper <= ewma.Mean {
		t.Error("upper threshold should be > mean")
	}
	// 阈值下限应小于均值
	if threshold.Lower >= ewma.Mean {
		t.Error("lower threshold should be < mean")
	}
}
