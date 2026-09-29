package resource

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// PrecisionAdaptor 精度自适应管理器
// 根据设备内存自动选择模型精度: FP32 → FP16 → INT8 → INT4
type PrecisionAdaptor struct {
	logger *logrus.Entry
}

// Precision 模型精度级别
type Precision string

const (
	PrecisionFP32 Precision = "FP32"
	PrecisionFP16 Precision = "FP16"
	PrecisionINT8 Precision = "INT8"
	PrecisionINT4 Precision = "INT4"
	PrecisionQ4   Precision = "Q4_K_M" // llama.cpp GGUF 量化格式
	PrecisionQ8   Precision = "Q8_0"
)

// NewPrecisionAdaptor 创建精度自适应管理器
func NewPrecisionAdaptor(logger *logrus.Entry) *PrecisionAdaptor {
	return &PrecisionAdaptor{logger: logger}
}

// SelectPrecision 根据可用 RAM 和模型类型选择最佳精度
func (p *PrecisionAdaptor) SelectPrecision(availableRAMMB int64, modelType string) (Precision, error) {
	switch modelType {
	case "llm", "gguf":
		return p.selectLLMPrecision(availableRAMMB)
	case "onnx", "vision":
		return p.selectONNXPrecision(availableRAMMB)
	default:
		return PrecisionFP32, nil
	}
}

// selectLLMPrecision 为 LLM 模型选择精度
// llama.cpp GGUF 量化格式:
//   Q4_K_M: ~4.5 bits/weight, 质量损失最小, 内存节省 ~75%
//   Q8_0:   ~8.5 bits/weight, 近乎无损, 内存节省 ~50%
//   FP16:   16 bits/weight, 无损, 内存占用大
func (p *PrecisionAdaptor) selectLLMPrecision(ramMB int64) (Precision, error) {
	switch {
	case ramMB >= 8192:
		// > 8GB: 可用 Q8（近乎无损）
		p.logger.Info("LLM precision: Q8_0 (8GB+ RAM)")
		return PrecisionQ8, nil
	case ramMB >= 4096:
		// 4-8GB: 使用 Q4_K_M（推荐平衡点）
		p.logger.Info("LLM precision: Q4_K_M (4-8GB RAM)")
		return PrecisionQ4, nil
	case ramMB >= 1024:
		// 1-4GB: 强制 Q4_K_M（可能略慢但可用）
		p.logger.Info("LLM precision: Q4_K_M (1-4GB RAM, minimal)")
		return PrecisionQ4, nil
	default:
		// < 1GB: 不建议运行 LLM
		return "", fmt.Errorf("insufficient RAM (%dMB) for LLM inference, minimum 1024MB required", ramMB)
	}
}

// selectONNXPrecision 为 ONNX 模型选择精度
// ONNX Runtime 支持 INT8 量化推理
func (p *PrecisionAdaptor) selectONNXPrecision(ramMB int64) (Precision, error) {
	switch {
	case ramMB >= 4096:
		// > 4GB: FP32 全精度
		p.logger.Info("ONNX precision: FP32 (4GB+ RAM)")
		return PrecisionFP32, nil
	case ramMB >= 2048:
		// 2-4GB: FP16 半精度
		p.logger.Info("ONNX precision: FP16 (2-4GB RAM)")
		return PrecisionFP16, nil
	case ramMB >= 512:
		// 512MB-2GB: INT8 量化
		p.logger.Info("ONNX precision: INT8 (512MB-2GB RAM)")
		return PrecisionINT8, nil
	default:
		// < 512MB: INT4 (需要特殊量化模型)
		p.logger.Info("ONNX precision: INT4 (<512MB RAM, minimal)")
		return PrecisionINT4, nil
	}
}

// GetModelSuffix 根据精度返回模型文件后缀
func GetModelSuffix(precision Precision, modelType string) string {
	switch modelType {
	case "llm", "gguf":
		switch precision {
		case PrecisionQ4:
			return "-q4_k_m.gguf"
		case PrecisionQ8:
			return "-q8_0.gguf"
		case PrecisionFP16:
			return "-fp16.gguf"
		default:
			return ".gguf"
		}
	case "onnx":
		switch precision {
		case PrecisionINT8:
			return "-int8.onnx"
		case PrecisionFP16:
			return "-fp16.onnx"
		case PrecisionINT4:
			return "-int4.onnx"
		default:
			return ".onnx"
		}
	default:
		return ""
	}
}

// GetEstimatedMemoryMB 估算模型在指定精度下的内存占用
func GetEstimatedMemoryMB(modelSizeMB float64, precision Precision) float64 {
	switch precision {
	case PrecisionFP32:
		return modelSizeMB
	case PrecisionFP16:
		return modelSizeMB * 0.5
	case PrecisionINT8:
		return modelSizeMB * 0.25
	case PrecisionINT4:
		return modelSizeMB * 0.125
	case PrecisionQ4:
		return modelSizeMB * 0.28 // Q4_K_M ~4.5 bits/weight
	case PrecisionQ8:
		return modelSizeMB * 0.53 // Q8_0 ~8.5 bits/weight
	default:
		return modelSizeMB
	}
}
