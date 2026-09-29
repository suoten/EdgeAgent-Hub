package resource

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
)

// Tier 资源层级
type Tier string

const (
	TierMinimal  Tier = "minimal"  // < 1GB: ONNX only, no LLM
	TierBasic    Tier = "basic"    // 1-4GB: ONNX + small LLM
	TierStandard Tier = "standard" // 4-8GB: ONNX + LLM + vision
	TierFull     Tier = "full"     // > 8GB: all models + multi-LLM
)

// AdapterConfig 资源适配配置
type AdapterConfig struct {
	AutoDetect         bool
	MinMemoryForLLM    int64 // MB
	MinMemoryForVision int64 // MB
	RAMTiers           RAMTierConfig
}

// RAMTierConfig RAM 分层配置
type RAMTierConfig struct {
	Minimal  int64 // MB
	Basic    int64
	Standard int64
	Full     int64
}

// DetectionResult 资源检测结果
type DetectionResult struct {
	Tier              Tier            `json:"tier"`
	TotalRAMMB        int64           `json:"total_ram_mb"`
	AvailableRAMMB    int64           `json:"available_ram_mb"`
	CPUCores          int             `json:"cpu_cores"`
	Arch              string          `json:"arch"`
	HasGPU            bool            `json:"has_gpu"`
	GPUType           string          `json:"gpu_type,omitempty"`
	RecommendedEP     string          `json:"recommended_ep"`     // ONNX Execution Provider
	RecommendedLLM    bool            `json:"recommended_llm"`    // 是否启用 LLM
	RecommendedVision bool            `json:"recommended_vision"` // 是否启用视觉
	RecommendedModel  string          `json:"recommended_model"`  // 推荐的 LLM 模型
	ModelPrecision    string          `json:"model_precision"`    // FP32/FP16/INT8/INT4
}

// Adapter 资源适配器
type Adapter struct {
	config AdapterConfig
	logger *logrus.Entry
}

// NewAdapter 创建资源适配器
func NewAdapter(config AdapterConfig, logger *logrus.Entry) *Adapter {
	return &Adapter{
		config: config,
		logger: logger,
	}
}

// Detect 检测硬件能力并返回推荐配置
func (a *Adapter) Detect() (*DetectionResult, error) {
	result := &DetectionResult{
		CPUCores: runtime.NumCPU(),
		Arch:     runtime.GOARCH,
	}

	// 检测内存
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to detect memory: %w", err)
	}
	result.TotalRAMMB = int64(vmStat.Total) / 1024 / 1024
	result.AvailableRAMMB = int64(vmStat.Available) / 1024 / 1024

	// 检测 GPU 类型
	a.detectGPU(result)

	// 确定资源层级
	result.Tier = a.determineTier(result.TotalRAMMB)

	// 根据层级推荐配置
	a.applyRecommendations(result)

	a.logger.Infof("Resource detection: tier=%s, RAM=%dMB, CPU=%d cores, arch=%s, GPU=%v (%s), EP=%s, LLM=%v, precision=%s",
		result.Tier, result.TotalRAMMB, result.CPUCores, result.Arch,
		result.HasGPU, result.GPUType, result.RecommendedEP,
		result.RecommendedLLM, result.ModelPrecision)

	return result, nil
}

// detectGPU 检测 GPU 类型
func (a *Adapter) detectGPU(result *DetectionResult) {
	// 基于 arch 和平台检测 GPU
	arch := runtime.GOARCH
	if arch == "arm64" || arch == "arm" {
		// ARM 平台: 可能是 Jetson / RK3588
		// 检查 /proc/device-tree/model 或 /etc/nv_tegra_release
		if a.fileExists("/proc/device-tree/model") {
			model := a.readFile("/proc/device-tree/model")
			if strings.Contains(strings.ToLower(model), "jetson") {
				result.HasGPU = true
				result.GPUType = "nvidia-jetson"
				return
			}
			if strings.Contains(strings.ToLower(model), "rk3588") || strings.Contains(strings.ToLower(model), "rockchip") {
				result.HasGPU = true
				result.GPUType = "rockchip-npu"
				return
			}
		}
	}
	// x86 平台: 检查 /dev/dri (Intel iGPU) 或 nvidia-smi
	if a.fileExists("/dev/dri/card0") || a.fileExists("/dev/dri/renderD128") {
		result.HasGPU = true
		result.GPUType = "intel-igpu"
		return
	}
	if a.commandExists("nvidia-smi") {
		result.HasGPU = true
		result.GPUType = "nvidia-cuda"
		return
	}
}

// determineTier 根据 RAM 确定层级
func (a *Adapter) determineTier(ramMB int64) Tier {
	tiers := a.config.RAMTiers
	if ramMB < tiers.Basic {
		return TierMinimal
	}
	if ramMB < tiers.Standard {
		return TierBasic
	}
	if ramMB < tiers.Full {
		return TierStandard
	}
	return TierFull
}

// applyRecommendations 根据层级应用推荐配置
func (a *Adapter) applyRecommendations(result *DetectionResult) {
	switch result.Tier {
	case TierMinimal:
		// < 1GB: ONNX only, no LLM, template alerts
		result.RecommendedEP = "cpu"
		result.RecommendedLLM = false
		result.RecommendedVision = false
		result.RecommendedModel = ""
		result.ModelPrecision = "INT8"

	case TierBasic:
		// 1-4GB: ONNX + Qwen3-0.6B Q4
		result.RecommendedEP = a.selectEP(result)
		result.RecommendedLLM = true
		result.RecommendedVision = false
		result.RecommendedModel = "qwen3-0.6b-q4_k_m.gguf"
		result.ModelPrecision = "INT4"

	case TierStandard:
		// 4-8GB: ONNX + Gemma-3-1B Q4 + vision
		result.RecommendedEP = a.selectEP(result)
		result.RecommendedLLM = true
		result.RecommendedVision = true
		result.RecommendedModel = "gemma-3-1b-q4_k_m.gguf"
		result.ModelPrecision = "INT4"

	case TierFull:
		// > 8GB: all models + multi-LLM
		result.RecommendedEP = a.selectEP(result)
		result.RecommendedLLM = true
		result.RecommendedVision = true
		result.RecommendedModel = "gemma-3-1b-q4_k_m.gguf"
		result.ModelPrecision = "Q4_K_M"
	}
}

// selectEP 根据硬件选择 Execution Provider
func (a *Adapter) selectEP(result *DetectionResult) string {
	if result.GPUType == "nvidia-jetson" || result.GPUType == "nvidia-cuda" {
		return "cuda"
	}
	if result.GPUType == "rockchip-npu" {
		return "rknn"
	}
	if result.GPUType == "intel-igpu" {
		return "openvino"
	}
	return "cpu"
}

// fileExists 检查文件是否存在
func (a *Adapter) fileExists(path string) bool {
	_, err := osStat(path)
	return err == nil
}

// readFile 读取文件内容
func (a *Adapter) readFile(path string) string {
	data, err := osReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// commandExists 检查命令是否存在
func (a *Adapter) commandExists(cmd string) bool {
	_, err := execLookPath(cmd)
	return err == nil
}
