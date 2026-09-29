package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMApiClient 是 OpenAI 兼容 API 的调用器
// 支持: OpenAI / 通义千问(DashScope) / DeepSeek / Ollama / 任何 OpenAI 兼容端点
type LLMApiClient struct {
	baseURL      string
	apiKey       string
	model        string
	systemPrompt string
	temperature  float64
	maxTokens    int
	httpClient   *http.Client
}

// NewLLMApiClient 创建 LLM API 客户端
func NewLLMApiClient(baseURL, apiKey, model, systemPrompt string, temperature float64, maxTokens int, timeout time.Duration) *LLMApiClient {
	return &LLMApiClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		temperature:  temperature,
		maxTokens:    maxTokens,
		httpClient:   &http.Client{Timeout: timeout},
	}
}

// chatMessage 是 OpenAI 兼容的消息格式
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest 是 OpenAI 兼容的请求体
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// chatResponse 是 OpenAI 兼容的响应体
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Chat 调用远程 LLM API 进行对话
// userMessage: 用户消息
// context: 可选的上下文（如 RAG 检索结果）
// 返回: LLM 回复内容, 是否成功, 错误信息
func (c *LLMApiClient) Chat(userMessage, context string) (string, error) {
	if c.baseURL == "" || c.model == "" {
		return "", fmt.Errorf("LLM API 未配置: 需要 api_base_url 和 api_model")
	}

	// 构建消息列表
	messages := []chatMessage{
		{Role: "system", Content: c.systemPrompt},
	}

	// 如果有上下文（RAG 检索结果），作为 system 消息的一部分
	if context != "" {
		messages[0].Content += "\n\n相关知识库内容:\n" + context
	}

	messages = append(messages, chatMessage{Role: "user", Content: userMessage})

	reqBody := chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	// 构建请求 URL (兼容 OpenAI /v1/chat/completions 格式)
	url := c.baseURL + "/v1/chat/completions"
	// 如果 baseURL 已包含 /v1，不重复添加
	if strings.HasSuffix(c.baseURL, "/v1") {
		url = c.baseURL + "/chat/completions"
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 尝试解析错误消息
		var errResp chatResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("LLM API 返回错误 (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("LLM API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM API 返回空响应")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("LLM API 返回空内容")
	}

	return content, nil
}

// IsConfigured 检查 LLM API 是否已配置
func (c *LLMApiClient) IsConfigured() bool {
	return c.baseURL != "" && c.model != ""
}

// ModelName 返回当前使用的模型名称
func (c *LLMApiClient) ModelName() string {
	return c.model
}
