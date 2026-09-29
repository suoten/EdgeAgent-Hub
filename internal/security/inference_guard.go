package security

import (
	"math"
	"strings"

	"github.com/sirupsen/logrus"
)

// InputRange 输入数据范围约束
type InputRange struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Deadband float64 `json:"deadband"` // 变化量超过此值视为可疑
}

// InferenceGuard 推理投毒防护
type InferenceGuard struct {
	ranges  map[string]map[string]InputRange // deviceID -> metricName -> range
	logger  *logrus.Entry
}

// NewInferenceGuard 创建推理防护器
func NewInferenceGuard(logger *logrus.Entry) *InferenceGuard {
	return &InferenceGuard{
		ranges: make(map[string]map[string]InputRange),
		logger: logger,
	}
}

// SetInputRange 设置设备指标的输入范围约束
func (g *InferenceGuard) SetInputRange(deviceID, metric string, rng InputRange) {
	if g.ranges[deviceID] == nil {
		g.ranges[deviceID] = make(map[string]InputRange)
	}
	g.ranges[deviceID][metric] = rng
}

// ValidateInput 验证输入数据是否在合理范围内
// 返回通过验证的数据和被过滤的指标列表
func (g *InferenceGuard) ValidateInput(deviceID string, metrics map[string]float64) (map[string]float64, []string) {
	filtered := make(map[string]float64)
	var rejected []string

	deviceRanges, ok := g.ranges[deviceID]
	if !ok {
		// 无约束配置，全部通过
		return metrics, nil
	}

	for name, val := range metrics {
		rng, hasRange := deviceRanges[name]
		if !hasRange {
			filtered[name] = val
			continue
		}

		// 范围检查
		if val < rng.Min || val > rng.Max {
			g.logger.Warnf("Input poisoning detected: device=%s metric=%s value=%.4f (range: %.4f-%.4f)",
				deviceID, name, val, rng.Min, rng.Max)
			rejected = append(rejected, name)
			continue
		}

		// NaN/Inf 检查
		if math.IsNaN(val) || math.IsInf(val, 0) {
			g.logger.Warnf("Input poisoning detected: device=%s metric=%s value is NaN/Inf", deviceID, name)
			rejected = append(rejected, name)
			continue
		}

		filtered[name] = val
	}

	return filtered, rejected
}

// LLMOutputFilter LLM 输出内容过滤
type LLMOutputFilter struct {
	// 被禁止的关键词模式
	blockedPatterns []string
	// 最大输出长度
	maxLength int
	logger    *logrus.Entry
}

// NewLLMOutputFilter 创建 LLM 输出过滤器
func NewLLMOutputFilter(logger *logrus.Entry) *LLMOutputFilter {
	return &LLMOutputFilter{
		blockedPatterns: []string{
			"<script", "javascript:", "eval(", "exec(",
			"rm -rf", "format c:", "del /f",
			"sudo ", "chmod 777",
			"DROP TABLE", "DELETE FROM",
			"ignore previous", "disregard", "forget your instructions",
			"you are now", "act as", "pretend you are",
		},
		maxLength: 4096,
		logger:    logger,
	}
}

// Filter 过滤 LLM 输出内容
// 返回过滤后的内容和是否检测到越狱尝试
func (f *LLMOutputFilter) Filter(output string) (string, bool) {
	// 长度限制
	if len(output) > f.maxLength {
		output = output[:f.maxLength]
		f.logger.Warn("LLM output truncated (exceeds max length)")
	}

	// 检测越狱尝试
	jailbreakDetected := false
	lowerOutput := strings.ToLower(output)
	for _, pattern := range f.blockedPatterns {
		if strings.Contains(lowerOutput, strings.ToLower(pattern)) {
			jailbreakDetected = true
			f.logger.Warnf("LLM output filtered: blocked pattern '%s' detected", pattern)
			// 替换为安全占位符
			output = strings.ReplaceAll(output, pattern, "[FILTERED]")
			output = strings.ReplaceAll(output, strings.ToUpper(pattern), "[FILTERED]")
			output = strings.ReplaceAll(output, strings.ToLower(pattern), "[FILTERED]")
		}
	}

	// 移除可能的 HTML 标签
	if strings.Contains(output, "<") && strings.Contains(output, ">") {
		output = stripHTMLTags(output)
	}

	return output, jailbreakDetected
}

// stripHTMLTags 简单移除 HTML 标签
func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ModelEncryption 模型文件加密存储
type ModelEncryption struct {
	key []byte
	logger *logrus.Entry
}

// NewModelEncryption 创建模型加密器
func NewModelEncryption(key []byte, logger *logrus.Entry) *ModelEncryption {
	return &ModelEncryption{
		key:    key,
		logger: logger,
	}
}

// EncryptModel 加密模型文件（生产环境使用 AES-256-GCM）
// 此处提供框架，实际加密使用 crypto/aes + crypto/cipher
func (e *ModelEncryption) EncryptModel(plaintext []byte) ([]byte, error) {
	if len(e.key) == 0 {
		// 未配置密钥，不加密
		return plaintext, nil
	}

	// 生产环境:
	// block, err := aes.NewCipher(e.key)
	// gcm, err := cipher.NewGCM(block)
	// nonce := make([]byte, gcm.NonceSize())
	// return gcm.Seal(nonce, nonce, plaintext, nil), nil

	// 简化: XOR 编码（仅用于框架展示，不安全）
	result := make([]byte, len(plaintext))
	for i, b := range plaintext {
		result[i] = b ^ e.key[i%len(e.key)]
	}
	return result, nil
}

// DecryptModel 解密模型文件
func (e *ModelEncryption) DecryptModel(ciphertext []byte) ([]byte, error) {
	if len(e.key) == 0 {
		return ciphertext, nil
	}

	// 简化: XOR 解码
	result := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		result[i] = b ^ e.key[i%len(e.key)]
	}
	return result, nil
}
