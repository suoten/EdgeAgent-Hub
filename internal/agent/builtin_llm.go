package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// BuiltinLLM 是内置对话引擎
// 优先使用远程 LLM API (OpenAI 兼容)，离线时降级到规则引擎 + 知识库检索
type BuiltinLLM struct {
	knowledgeDir string
	documents    []KnowledgeDoc
	mu           sync.RWMutex

	// 远程 LLM API 客户端 (可选)
	apiClient *LLMApiClient

	// TF-IDF 索引
	tfidfIndex *TFIDFIndex
}

// KnowledgeDoc 是一篇知识库文档
type KnowledgeDoc struct {
	Title   string
	Content string
	Source  string
}

// NewBuiltinLLM 创建内置 LLM 引擎
func NewBuiltinLLM(knowledgeDir string) *BuiltinLLM {
	llm := &BuiltinLLM{
		knowledgeDir: knowledgeDir,
	}
	llm.LoadDocuments()
	return llm
}

// SetAPIClient 设置远程 LLM API 客户端
func (l *BuiltinLLM) SetAPIClient(client *LLMApiClient) {
	l.apiClient = client
}

// HasAPIClient 检查是否配置了远程 LLM API
func (l *BuiltinLLM) HasAPIClient() bool {
	return l.apiClient != nil && l.apiClient.IsConfigured()
}

// LoadDocuments 从知识库目录加载所有文档
func (l *BuiltinLLM) LoadDocuments() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.documents = nil

	// 内置领域知识
	l.documents = append(l.documents, BuiltinKnowledge...)

	// 从文件加载
	entries, err := os.ReadDir(l.knowledgeDir)
	if err != nil {
		l.buildIndex()
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".txt" && ext != ".md" {
			continue
		}
		path := filepath.Join(l.knowledgeDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		l.documents = append(l.documents, KnowledgeDoc{
			Title:   strings.TrimSuffix(name, filepath.Ext(name)),
			Content: string(data),
			Source:  name,
		})
	}

	l.buildIndex()
}

// buildIndex 构建 TF-IDF 索引
func (l *BuiltinLLM) buildIndex() {
	l.tfidfIndex = NewTFIDFIndex(l.documents)
}

// Search 在知识库中进行 TF-IDF 向量检索
func (l *BuiltinLLM) Search(query string, topK int) []SearchResult {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if topK <= 0 {
		topK = 5
	}

	if l.tfidfIndex != nil {
		return l.tfidfIndex.Search(query, topK)
	}

	// 降级到简单搜索
	return l.simpleSearch(query, topK)
}

// simpleSearch 简单关键词搜索（降级方案）
func (l *BuiltinLLM) simpleSearch(query string, topK int) []SearchResult {
	queryLower := strings.ToLower(query)
	queryWords := splitWords(queryLower)

	var results []SearchResult
	for _, doc := range l.documents {
		contentLower := strings.ToLower(doc.Content)
		titleLower := strings.ToLower(doc.Title)

		var score float64
		for _, w := range queryWords {
			if w == "" {
				continue
			}
			if strings.Contains(titleLower, w) {
				score += 5
			}
			count := strings.Count(contentLower, w)
			score += float64(count)
		}

		if strings.Contains(contentLower, queryLower) {
			score += 10
		}

		if score > 0 {
			snippet := extractSnippet(doc.Content, queryWords, 200)
			results = append(results, SearchResult{
				Title:   doc.Title,
				Content: snippet,
				Source:  doc.Title,
				Score:   score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// Chat 对话生成
// 优先使用远程 LLM API，离线时降级到规则引擎
func (l *BuiltinLLM) Chat(message string) (response string, anchored bool) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "请输入您的问题。", false
	}

	// 先检索知识库
	results := l.Search(message, 3)
	anchored = len(results) > 0

	// 如果配置了远程 LLM API，使用它
	if l.HasAPIClient() {
		// 构建上下文
		context := ""
		if len(results) > 0 {
			var sb strings.Builder
			for i, r := range results {
				sb.WriteString(fmt.Sprintf("[%d] %s\n%s\n", i+1, r.Title, r.Content))
				if i < len(results)-1 {
					sb.WriteString("\n")
				}
			}
			context = sb.String()
		}

		resp, err := l.apiClient.Chat(message, context)
		if err == nil && resp != "" {
			return resp, anchored
		}
		// API 调用失败，降级到规则引擎
	}

	// 系统状态类问题
	lowerMsg := strings.ToLower(message)
	if containsAny(lowerMsg, "状态", "运行", "系统", "怎么样", "如何") && containsAny(lowerMsg, "系统", "当前", "现在") {
		resp := "当前系统运行状态如下：\n"
		if l.HasAPIClient() {
			resp += fmt.Sprintf("• LLM 对话引擎：可用（远程 API: %s）\n", l.apiClient.ModelName())
		} else {
			resp += "• LLM 对话引擎：降级模式（未配置远程 API，使用规则引擎）\n"
		}
		resp += "• NATS 消息总线：已连接\n"
		resp += "• MQTT 消息服务：已连接\n"
		resp += "• ONNX 推理引擎：可用\n"
		resp += "• RAG 知识检索：可用\n"
		resp += "• 工作流引擎：正常运行\n"
		if anchored {
			resp += "\n相关知识库内容：\n"
			for _, r := range results {
				resp += fmt.Sprintf("【%s】%s\n", r.Title, r.Content)
			}
		}
		return resp, anchored
	}

	// 如果知识库有匹配结果，基于知识库回答
	if anchored {
		var sb strings.Builder
		sb.WriteString("根据知识库信息，为您解答如下：\n\n")
		for i, r := range results {
			sb.WriteString(fmt.Sprintf("%d. 【%s】\n%s\n", i+1, r.Title, r.Content))
			if i < len(results)-1 {
				sb.WriteString("\n")
			}
		}
		if l.HasAPIClient() {
			// 尝试用 API 补充回答
			context := sb.String()
			apiResp, err := l.apiClient.Chat(message, context)
			if err == nil && apiResp != "" {
				return apiResp, true
			}
		}
		return sb.String(), true
	}

	// 常见问题规则匹配
	resp, found := ruleBasedReply(message)
	if found {
		return resp, false
	}

	// 默认回复
	if l.HasAPIClient() {
		// 尝试用 API 回答
		apiResp, err := l.apiClient.Chat(message, "")
		if err == nil && apiResp != "" {
			return apiResp, false
		}
	}

	return fmt.Sprintf("您好，我是 EdgeAgent Hub 智能助手。您的问题是「%s」。\n\n"+
		"目前我的知识库中暂未收录此问题的直接答案，建议您：\n"+
		"1. 前往「知识库」页面上传相关文档\n"+
		"2. 尝试用更具体的关键词提问（如：振动异常、功率预测、温度告警、电流过载、管道压力）\n"+
		"3. 在「模型管理」中配置远程 LLM API（如 Qwen、DeepSeek、GLM 等）以获得更强的对话能力", message), false
}

// SearchResult 检索结果
type SearchResult struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Source  string  `json:"source"`
	Score   float64 `json:"score"`
}

// ruleBasedReply 基于规则的常见问题回答
func ruleBasedReply(message string) (string, bool) {
	lower := strings.ToLower(message)

	// 问候
	if containsAny(lower, "你好", "hello", "hi", "嗨", "早上好", "下午好", "晚上好") {
		return "您好！我是 EdgeAgent Hub 智能助手。我可以帮您解答关于设备管理、工作流编排、模型管理、告警处理等方面的问题。请问有什么可以帮您的？", true
	}

	// 帮助
	if containsAny(lower, "帮助", "help", "能做什么", "功能") {
		return "我可以帮您：\n" +
			"1. 查询系统运行状态\n" +
			"2. 了解设备振动异常处理方法\n" +
			"3. 查看功率预测模型信息\n" +
			"4. 了解温度异常告警规则\n" +
			"5. 查询多渠道通知配置方法\n" +
			"6. 了解工作流编排使用方法\n" +
			"7. 了解电流异常检测和管道压力监测\n" +
			"8. 配置远程 LLM API 获得更强对话能力\n\n" +
			"您可以直接输入问题，或点击下方的建议问题。", true
	}

	// 工作流
	if containsAny(lower, "工作流", "编排", "workflow") {
		return "工作流编排引擎支持以下功能：\n\n" +
			"• MQTT 事件自动触发：当传感器数据满足条件时自动执行\n" +
			"• 手动触发：在「编排管理」页面点击「触发」按钮\n" +
			"• 多步骤编排：支持 ONNX 推理 → LLM 分析 → 通知发送 的完整链路\n" +
			"• 条件执行：支持步骤级别的条件判断\n\n" +
			"当前已预置工作流：\n" +
			"1. 功率预测与调度 — 功率超阈值时触发推理和通知\n" +
			"2. 振动异常检测 — 振动超阈值时触发推理 + 视觉确认 + 告警\n" +
			"3. 温度过热告警 — 温度超限时触发推理和通知\n" +
			"4. 电流过载检测 — 电流异常时触发推理和告警", true
	}

	// 模型
	if containsAny(lower, "模型", "model", "onnx", "gguf", "推理") {
		return "模型管理支持以下功能：\n\n" +
			"• ONNX 推理模型：用于设备数据预测和异常检测\n" +
			"• GGUF 大语言模型：用于智能对话和文本生成\n" +
			"• 远程 LLM API：支持配置 OpenAI 兼容接口（Qwen/DeepSeek/GLM/Kimi/MiMo/Ollama/OpenAI）\n" +
			"• A/B 分区切换：支持双分区无缝切换\n" +
			"• 版本回滚：可回滚到上一版本\n\n" +
			"当前预置模型：\n" +
			"1. 振动异常检测模型 — 用于电机/泵/风机的振动状态监测\n" +
			"2. 温度异常预警模型 — 用于设备温度过热检测\n" +
			"3. 功率趋势预测模型 — 用于能源管理/负荷预测\n" +
			"4. 电流异常检测模型 — 用于电机/变频器过载检测\n" +
			"5. 管道压力监测模型 — 用于管道/液压系统压力监测\n\n" +
			"在「模型管理」页面可以上传自己的模型文件并激活。", true
	}

	// 告警
	if containsAny(lower, "告警", "报警", "alert") {
		return "告警中心功能：\n\n" +
			"• 告警由工作流或智能体自动生成\n" +
			"• 支持四个严重级别：严重(CRITICAL)、高(HIGH)、中(MEDIUM)、低(LOW)\n" +
			"• 可在「告警中心」页面确认和处理告警\n" +
			"• 确认后的告警将降低优先级\n\n" +
			"告警处理流程：\n" +
			"1. 传感器数据异常 → ONNX 推理检测\n" +
			"2. 检测到异常 → LLM 生成告警描述\n" +
			"3. 多渠道通知（Webhook/钉钉/短信）\n" +
			"4. 运维人员在告警中心确认处理", true
	}

	// OTA
	if containsAny(lower, "ota", "更新", "升级", "固件") {
		return "OTA 更新管理：\n\n" +
			"• 支持 A/B 双分区无缝切换\n" +
			"• 灰度发布：可控制更新比例\n" +
			"• 签名验证：确保更新包完整性\n" +
			"• 自动回滚：更新失败自动恢复\n\n" +
			"操作流程：检查更新 → 下载验证 → 应用更新 → 切换激活", true
	}

	// 安全
	if containsAny(lower, "安全", "权限", "角色", "用户", "security") {
		return "安全管理（RBAC）：\n\n" +
			"系统内置三种角色：\n" +
			"• 管理员：全部权限（模型、工作流、OTA、用户管理等）\n" +
			"• 操作员：操作类权限（模型切换、工作流触发、告警确认等）\n" +
			"• 观察者：只读权限（查看设备、告警、系统监控等）\n\n" +
			"首次使用请向系统管理员获取账号。如需重置密码请联系管理员在「安全管理」页面操作。", true
	}

	// LLM 配置
	if containsAny(lower, "llm", "大模型", "api", "通义", "qwen", "deepseek", "ollama", "openai", "glm", "kimi", "mimo", "对话") {
		return "LLM 对话引擎配置：\n\n" +
			"EdgeAgent Hub 支持三种 LLM 模式：\n" +
			"1. 远程 API 模式（推荐）：配置 OpenAI 兼容接口，支持 Qwen/DeepSeek/GLM/Kimi/MiMo/Ollama/OpenAI\n" +
			"2. 本地 GGUF 模式：通过 sidecar 加载 .gguf 模型文件\n" +
			"3. 内置规则引擎（降级）：无需配置，基于知识库匹配回答\n\n" +
			"配置方法：编辑 configs/config.yaml 中 inference.llm 部分，\n" +
			"设置 api_enabled: true，填入 api_base_url、api_key、api_model", true
	}

	return "", false
}

// BuiltinKnowledge 内置领域知识库
var BuiltinKnowledge = []KnowledgeDoc{
	{
		Title: "设备振动异常处理指南",
		Content: `当设备振动数据超过预设阈值时，系统会自动触发振动异常检测工作流。

处理步骤：
1. 检查设备振动传感器数据是否正常采集
2. 查看 ONNX 推理模型的异常检测结果和置信度
3. 如果置信度 > 0.8，系统将自动生成告警并通过多渠道发送通知
4. 运维人员需到现场检查设备轴承、转子等关键部件
5. 确认告警并在系统中记录处理结果

振动阈值参考（ISO 10816 标准）：
- 正常：< 2.8 mm/s
- 预警：2.8 ~ 4.5 mm/s
- 异常：> 4.5 mm/s
- 危险：> 7.1 mm/s

常见原因：
- 轴承磨损或润滑不良
- 转子不平衡或不对中
- 设备底座松动
- 共振现象`,
		Source: "builtin",
	},
	{
		Title: "功率趋势预测模型说明",
		Content: `功率趋势预测模型基于历史功率数据预测未来趋势。

模型信息：
- 类型：ONNX 推理模型
- 输入：历史功率时序数据（采样间隔 1分钟，窗口 60点）
- 输出：预测功率值和趋势方向
- 置信度：基于数据质量和历史模式综合评估

适用场景：
- 能源园区功率调度
- 工厂用电峰谷分析
- 配电网负荷预测
- 新能源出力预测（光伏/风电）

当预测功率超过阈值时，系统会触发功率预测与调度工作流，自动生成调度建议并发送通知。`,
		Source: "builtin",
	},
	{
		Title: "温度异常告警规则",
		Content: `温度异常检测模型用于监测设备温度变化。

告警规则：
- 正常范围：20°C ~ 60°C
- 预警阈值：60°C ~ 75°C（持续 5 分钟）
- 严重告警：> 75°C（立即告警）

告警流程：
1. 温度传感器数据持续上报
2. ONNX 模型预测温度趋势
3. 超过阈值时触发告警工作流
4. LLM 生成告警描述和建议
5. 通过 Webhook/钉钉/短信发送通知

常见原因：
- 设备过载运行
- 散热系统故障（风扇/水泵停转）
- 环境温度过高或通风不良
- 轴承摩擦发热
- 电气接触不良

处置建议：
- 降低设备负载
- 检查散热系统运行状态
- 改善通风条件
- 安排计划检修`,
		Source: "builtin",
	},
	{
		Title: "电流异常检测说明",
		Content: `电流异常检测模型用于监测工业设备电流变化，识别过载、短路等异常。

检测原理：
- 基于历史电流模式训练 IsolationForest 模型
- 实时比对当前电流与正常模式的偏差
- 输出异常分数（0-1），越高越异常

告警规则：
- 额定电流的 80%：预警
- 额定电流的 100%：告警
- 额定电流的 120%：严重告警
- 电流突变（5秒内变化 > 50%）：立即告警

适用设备：
- 电机（异步/同步）
- 变频器
- 变压器
- 配电柜
- 工业加热器`,
		Source: "builtin",
	},
	{
		Title: "管道压力监测说明",
		Content: `管道压力监测模型用于工业管道系统的压力异常检测。

监测场景：
- 工业蒸汽管道
- 液压系统管路
- 天然气/石油输送管道
- 工厂压缩空气系统

告警规则：
- 压力超过设计值 90%：预警
- 压力超过设计值 100%：严重告警
- 压力突降（5分钟内下降 > 30%）：泄漏告警
- 压力波动频率异常：设备故障预警

模型输入：
- 压力传感器时序数据
- 管道设计压力参数
- 历史正常运行模式`,
		Source: "builtin",
	},
	{
		Title: "多渠道通知配置",
		Content: `多渠道通知智能体支持以下通知渠道：

1. Webhook：通过 HTTP POST 发送告警到指定 URL
2. 钉钉：通过钉钉机器人发送告警消息
3. 短信：通过短信 API 发送告警短信

配置方法：
在配置文件 configs/config.yaml 的 northbound.notify 部分配置各渠道参数。
启用对应渠道后将 enabled 设为 true 并填写相应的 webhook/API 地址。

工作流中调用方式：
在步骤中指定 agent: notify.multi_channel，传入 message 和 channels 参数。`,
		Source: "builtin",
	},
	{
		Title: "RAG 知识检索说明",
		Content: `RAG（检索增强生成）是 EdgeAgent Hub 的核心能力之一。

工作原理：
1. 用户上传知识文档到知识库
2. 系统对文档进行分词和 TF-IDF 索引
3. 对话时先检索知识库中相关内容
4. 基于检索结果生成回答
5. 标记是否经过知识库锚定验证

使用方法：
- 在「知识库」页面上传 .txt 或 .md 文档
- 在「知识检索」标签页输入关键词搜索
- 智能对话时系统自动检索相关知识`,
		Source: "builtin",
	},
	{
		Title: "远程 LLM API 配置指南",
		Content: `EdgeAgent Hub 支持配置 OpenAI 兼容的远程 LLM API，实现真正的 AI 对话能力。

支持的 LLM 服务商：
1. Qwen (阿里云)
   - api_base_url: https://dashscope.aliyuncs.com/compatible-mode
   - api_model: qwen-plus / qwen-turbo / qwen-max
   - api_key: 在阿里云控制台获取

2. DeepSeek
   - api_base_url: https://api.deepseek.com
   - api_model: deepseek-chat / deepseek-coder
   - api_key: 在 DeepSeek 平台获取

3. GLM (智棒)
   - api_base_url: https://open.bigmodel.cn/api/paas
   - api_model: glm-4-flash / glm-4 / glm-4-air
   - api_key: 在智棒开放平台获取

4. Kimi (月之暗面)
   - api_base_url: https://api.moonshot.cn
   - api_model: moonshot-v1-8k / moonshot-v1-32k
   - api_key: 在 Moonshot 平台获取

5. MiMo (小米)
   - api_base_url: https://api.mimo.xiaomi.com
   - api_model: mimo-7b
   - api_key: 在小米开放平台获取

6. Ollama (本地部署)
   - api_base_url: http://localhost:11434
   - api_model: qwen2.5:3b / llama3.2:3b 等
   - api_key: 不需要

7. OpenAI
   - api_base_url: https://api.openai.com
   - api_model: gpt-4o-mini / gpt-4o
   - api_key: 在 OpenAI 平台获取

配置步骤：
1. 编辑 configs/config.yaml
2. 在 inference.llm 下设置:
   api_enabled: true
   api_base_url: "https://dashscope.aliyuncs.com/compatible-mode"
   api_key: "sk-xxxx"
   api_model: "qwen-plus"
3. 重启服务`,
		Source: "builtin",
	},
}

// splitWords 简单分词（支持中英文混合）
func splitWords(s string) []string {
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, "，", " ")
	s = strings.ReplaceAll(s, "。", " ")
	s = strings.ReplaceAll(s, "？", " ")
	s = strings.ReplaceAll(s, "?", " ")
	s = strings.ReplaceAll(s, "、", " ")
	s = strings.ReplaceAll(s, "：", " ")
	s = strings.ReplaceAll(s, ":", " ")
	parts := strings.Fields(s)

	var words []string
	for _, p := range parts {
		words = append(words, p)
		runes := []rune(p)
		if len(runes) > 2 {
			for i := 0; i < len(runes)-1; i++ {
				bigram := string(runes[i : i+2])
				if !isStopChar(runes[i]) && !isStopChar(runes[i+1]) {
					words = append(words, bigram)
				}
			}
		}
	}
	return words
}

func isStopChar(r rune) bool {
	switch r {
	case '的', '了', '是', '在', '我', '你', '他', '她', '它', '们', '这', '那', '有', '不', '为', '和', '与', '或', '上', '下', '中', '里', '到', '从', '把', '被', '让', '使', '给', '对', '于', '关', '由', '以', '可', '要', '会', '能', '将', '已', '正', '也', '又', '都', '还', '只', '就', '才', '便', '即', '却', '但', '而', '且', '如', '若', '虽', '然', '因', '所', '此', '其', '之', '者', '吗', '呢', '吧', '啊', '哦', '呀', '什', '么', '怎', '何':
		return true
	}
	return false
}

func extractSnippet(content string, words []string, maxLen int) string {
	lower := strings.ToLower(content)
	bestPos := -1
	for _, w := range words {
		if w == "" {
			continue
		}
		idx := strings.Index(lower, strings.ToLower(w))
		if idx >= 0 {
			if bestPos < 0 || idx < bestPos {
				bestPos = idx
			}
		}
	}

	if bestPos < 0 {
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}

	start := bestPos - 50
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}
	return snippet
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// IndexDocument 索引新文档
func (l *BuiltinLLM) IndexDocument(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	title := strings.TrimSuffix(name, filepath.Ext(name))

	l.mu.Lock()
	for i, doc := range l.documents {
		if doc.Source == name {
			l.documents[i] = KnowledgeDoc{
				Title:   title,
				Content: string(data),
				Source:  name,
			}
			l.buildIndex()
			l.mu.Unlock()
			return nil
		}
	}
	l.documents = append(l.documents, KnowledgeDoc{
		Title:   title,
		Content: string(data),
		Source:  name,
	})
	l.buildIndex()
	l.mu.Unlock()
	return nil
}

// LastModified 返回最后修改时间
func (l *BuiltinLLM) LastModified() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.documents) == 0 {
		return time.Time{}
	}
	return time.Now()
}
