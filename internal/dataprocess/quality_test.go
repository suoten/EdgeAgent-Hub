package dataprocess

import (
	"math"
	"testing"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

func TestDataQualityMonitor(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	monitor := NewDataQualityMonitor(logger)

	// 设置预期指标
	monitor.SetExpectedMetrics("device1", []string{"temperature", "vibration", "current"})

	// 模拟数据流入
	for i := 0; i < 20; i++ {
		envelope := &models.SensorEnvelope{
			DeviceID:  "device1",
			Protocol:  "modbus_tcp",
			Timestamp: 0,
			Metrics: map[string]float64{
				"temperature": 60.0 + float64(i)*0.5,
				"vibration":   3.5 + float64(i)*0.1,
				"current":     12.0,
			},
		}
		score := monitor.Evaluate("device1", envelope)
		if score.Overall < 0 || score.Overall > 1 {
			t.Errorf("overall score out of range: %f", score.Overall)
		}
	}

	// 获取最终评分
	score := monitor.GetScore("device1")
	if score.Integrity != 1.0 {
		t.Errorf("expected integrity 1.0, got %f", score.Integrity)
	}
}

func TestPreprocessorScale(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	p := NewPreprocessor(logger)
	p.SetConfig("device1", map[string]PreprocessConfig{
		"temperature": {Scale: 0.1, Offset: -10},
	})

	envelope := &models.SensorEnvelope{
		DeviceID: "device1",
		Metrics:  map[string]float64{"temperature": 700},
	}

	result := p.Process(envelope)
	// 700 * 0.1 + (-10) = 60
	if math.Abs(result.Metrics["temperature"]-60.0) > 0.001 {
		t.Errorf("expected 60.0, got %f", result.Metrics["temperature"])
	}
}

func TestPreprocessorClamp(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	p := NewPreprocessor(logger)
	p.SetConfig("device1", map[string]PreprocessConfig{
		"pressure": {MinLimit: 0, MaxLimit: 100},
	})

	envelope := &models.SensorEnvelope{
		DeviceID: "device1",
		Metrics:  map[string]float64{"pressure": 150},
	}

	result := p.Process(envelope)
	if result.Metrics["pressure"] != 100 {
		t.Errorf("expected 100 (clamped), got %f", result.Metrics["pressure"])
	}

	envelope2 := &models.SensorEnvelope{
		DeviceID: "device1",
		Metrics:  map[string]float64{"pressure": -10},
	}

	result2 := p.Process(envelope2)
	if result2.Metrics["pressure"] != 0 {
		t.Errorf("expected 0 (clamped), got %f", result2.Metrics["pressure"])
	}
}

func TestPreprocessorMovingAvg(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	p := NewPreprocessor(logger)
	p.SetConfig("device1", map[string]PreprocessConfig{
		"temp": {Filter: "moving_avg", FilterN: 3},
	})

	// 依次输入 10, 20, 30 → 移动平均应为 20
	p.Process(&models.SensorEnvelope{DeviceID: "device1", Metrics: map[string]float64{"temp": 10}})
	p.Process(&models.SensorEnvelope{DeviceID: "device1", Metrics: map[string]float64{"temp": 20}})
	result := p.Process(&models.SensorEnvelope{DeviceID: "device1", Metrics: map[string]float64{"temp": 30}})

	// (10+20+30)/3 = 20
	if math.Abs(result.Metrics["temp"]-20.0) > 0.001 {
		t.Errorf("expected 20.0, got %f", result.Metrics["temp"])
	}
}

func TestComputeStats(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5}
	mean, stdDev := computeStats(values)
	if mean != 3.0 {
		t.Errorf("expected mean 3.0, got %f", mean)
	}
	// 方差 = (4+1+0+1+4)/5 = 2, 标准差 = sqrt(2) ≈ 1.414
	if math.Abs(stdDev-math.Sqrt(2)) > 0.001 {
		t.Errorf("expected stdDev %f, got %f", math.Sqrt(2), stdDev)
	}
}
