package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/agent"
	"github.com/edgelite/edgeagent-hub/internal/messaging"
	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Engine 是编排 DSL 引擎，负责解析和执行 YAML 工作流
type Engine struct {
	registry      *agent.Registry
	bridge        *messaging.Bridge
	jsManager     *messaging.JetStreamManager // JetStream 持久化管理器（可选）
	logger        *logrus.Entry
	workflowDir   string
	maxConcurrent int
	defaultTimeout time.Duration

	// 已加载的工作流
	workflows     map[string]*models.Workflow
	mu            sync.RWMutex

	// 活跃执行
	executions    sync.Map // workflowID → *models.WorkflowExecution

	// 模板渲染器
	templateRe    *regexp.Regexp
}

// NewEngine 创建编排引擎
func NewEngine(registry *agent.Registry, bridge *messaging.Bridge, workflowDir string,
	maxConcurrent int, defaultTimeout time.Duration, logger *logrus.Entry) *Engine {
	return &Engine{
		registry:       registry,
		bridge:         bridge,
		logger:         logger,
		workflowDir:    workflowDir,
		maxConcurrent:  maxConcurrent,
		defaultTimeout: defaultTimeout,
		workflows:      make(map[string]*models.Workflow),
		templateRe:     regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`),
	}
}

// SetJetStreamManager 设置 JetStream 持久化管理器（启用断点续行）
func (e *Engine) SetJetStreamManager(jsm *messaging.JetStreamManager) {
	e.jsManager = jsm
}

// RecoverExecutions 从 JetStream 恢复未完成的编排执行（断点续行）
func (e *Engine) RecoverExecutions() {
	if e.jsManager == nil {
		e.logger.Warn("JetStream manager not set, skipping execution recovery")
		return
	}

	e.logger.Info("Recovering incomplete workflow executions from JetStream...")
	err := e.jsManager.RecoverExecutions(func(exec *models.WorkflowExecution) {
		wf, ok := e.GetWorkflow(exec.WorkflowName)
		if !ok {
			e.logger.Warnf("Cannot recover execution %s: workflow %s not found", exec.WorkflowID, exec.WorkflowName)
			return
		}

		e.logger.Infof("Resuming execution %s (workflow: %s, completed steps: %d)",
			exec.WorkflowID, exec.WorkflowName, len(exec.Steps))

		// 恢复执行：从上次断点继续
		go e.executeRecovered(exec, wf)
	})
	if err != nil {
		e.logger.Errorf("Failed to recover executions: %v", err)
	}
}

// executeRecovered 恢复执行已持久化的编排
func (e *Engine) executeRecovered(execution *models.WorkflowExecution, wf *models.Workflow) {
	// 找到上次执行到哪一步
	completedSteps := make(map[string]bool)
	for stepID, result := range execution.Steps {
		if result.Status == "completed" {
			completedSteps[stepID] = true
		}
	}

	// 构建上下文，恢复步骤输出
	ctx := &WorkflowContext{
		Execution:   execution,
		TriggerData: execution.TriggerData,
		StepOutputs: make(map[string]any),
	}

	// 执行剩余步骤
	for _, step := range wf.Steps {
		if completedSteps[step.ID] {
			continue // 跳过已完成的步骤
		}

		// 检查条件
		if step.Condition != "" {
			result, err := e.evaluateCondition(step.Condition, ctx)
			if err != nil || !result {
				ctx.setStepResult(step.ID, models.StepResult{
					ID: step.ID, Status: "skipped", StartTime: time.Now(),
				})
				continue
			}
		}

		stepCopy := step
		e.executeStep(ctx, &stepCopy)

		// 每步执行后持久化状态到 JetStream
		if e.jsManager != nil {
			if err := e.jsManager.PublishExecutionState(execution); err != nil {
				e.logger.Warnf("Failed to persist execution state: %v", err)
			}
		}
	}

	// 判断最终状态
	hasFailed := false
	ctx.stepsMu.Lock()
	for _, stepResult := range execution.Steps {
		if stepResult.Status == "failed" {
			hasFailed = true
			break
		}
	}
	ctx.stepsMu.Unlock()
	if hasFailed {
		execution.Status = "failed"
	} else {
		execution.Status = "completed"
	}
	now := time.Now()
	execution.EndTime = &now
	e.executions.Store(execution.WorkflowID, execution)

	// 持久化最终状态
	if e.jsManager != nil {
		e.jsManager.PublishExecutionState(execution)
	}

	e.logger.Infof("Recovered execution %s completed (status: %s)", execution.WorkflowID, execution.Status)
}

// LoadWorkflows 从目录加载所有 YAML 工作流
func (e *Engine) LoadWorkflows() error {
	entries, err := os.ReadDir(e.workflowDir)
	if err != nil {
		return fmt.Errorf("failed to read workflow directory %s: %w", e.workflowDir, err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(e.workflowDir, entry.Name())
		if err := e.loadWorkflowFile(path); err != nil {
			e.logger.Errorf("Failed to load workflow %s: %v", path, err)
		}
	}

	e.logger.Infof("Loaded %d workflows from %s", len(e.workflows), e.workflowDir)
	return nil
}

// loadWorkflowFile 加载单个工作流文件
func (e *Engine) loadWorkflowFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var wf models.Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	if wf.Name == "" {
		wf.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	e.workflows[wf.Name] = &wf
	e.logger.Infof("Loaded workflow: %s (%d steps, trigger: %s/%s)",
		wf.Name, len(wf.Steps), wf.Trigger.Source, wf.Trigger.Topic)
	return nil
}

// AddWorkflow 添加一个工作流定义
func (e *Engine) AddWorkflow(wf *models.Workflow) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.workflows[wf.Name] = wf
	e.logger.Infof("Added workflow: %s (%d steps)", wf.Name, len(wf.Steps))
}

// RemoveWorkflow 移除一个工作流
func (e *Engine) RemoveWorkflow(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.workflows, name)
}

// ListWorkflows 列出所有工作流
func (e *Engine) ListWorkflows() []*models.Workflow {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*models.Workflow, 0, len(e.workflows))
	for _, wf := range e.workflows {
		result = append(result, wf)
	}
	return result
}

// GetWorkflow 获取指定工作流
func (e *Engine) GetWorkflow(name string) (*models.Workflow, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	wf, ok := e.workflows[name]
	return wf, ok
}

// Trigger 手动触发工作流
func (e *Engine) Trigger(workflowName string, triggerData map[string]any) (*models.WorkflowExecution, error) {
	wf, ok := e.GetWorkflow(workflowName)
	if !ok {
		return nil, fmt.Errorf("workflow not found: %s", workflowName)
	}

	execution := &models.WorkflowExecution{
		WorkflowID:   fmt.Sprintf("wf-%s-%s", time.Now().Format("20060102-150405"), uuid.New().String()[:8]),
		WorkflowName: workflowName,
		Status:       "running",
		StartTime:    time.Now(),
		Steps:        make(map[string]models.StepResult),
		TriggerData:  triggerData,
	}

	e.executions.Store(execution.WorkflowID, execution)

	// 持久化初始状态到 JetStream
	if e.jsManager != nil {
		if err := e.jsManager.PublishExecutionState(execution); err != nil {
			e.logger.Warnf("Failed to persist initial execution state: %v", err)
		}
	}

	// 异步执行
	go e.execute(execution, wf)

	return execution, nil
}

// execute 执行工作流
func (e *Engine) execute(execution *models.WorkflowExecution, wf *models.Workflow) {
	defer func() {
		now := time.Now()
		execution.EndTime = &now
		e.executions.Store(execution.WorkflowID, execution)
	}()

	e.logger.Infof("Workflow execution started: %s (workflow: %s)", execution.WorkflowID, wf.Name)

	// 构建上下文
	ctx := &WorkflowContext{
		Execution:   execution,
		TriggerData: execution.TriggerData,
		StepOutputs: make(map[string]any),
	}

	// 执行步骤
	for i, step := range wf.Steps {
		// 检查条件
		if step.Condition != "" {
			result, err := e.evaluateCondition(step.Condition, ctx)
			if err != nil {
			e.logger.Warnf("Workflow %s step %s condition error: %v, skipping", execution.WorkflowID, step.ID, err)
			ctx.setStepResult(step.ID, models.StepResult{
				ID: step.ID, Status: "skipped", Error: err.Error(),
				StartTime: time.Now(),
			})
			continue
			}
			if !result {
			e.logger.Infof("Workflow %s step %s condition not met, skipping", execution.WorkflowID, step.ID)
			ctx.setStepResult(step.ID, models.StepResult{
				ID: step.ID, Status: "skipped",
				StartTime: time.Now(),
			})
			continue
			}
		}

		// 处理并行步骤
		if step.Parallel && i < len(wf.Steps)-1 {
			// 收集连续的并行步骤
			var parallelSteps []models.StepDef
			parallelSteps = append(parallelSteps, step)
			for j := i + 1; j < len(wf.Steps); j++ {
				if wf.Steps[j].Parallel {
					parallelSteps = append(parallelSteps, wf.Steps[j])
				} else {
					break
				}
			}
			e.executeParallel(ctx, parallelSteps)
			continue
		}

		// 串行执行
		e.executeStep(ctx, &step)

		// 每步执行后持久化状态到 JetStream（断点续行）
		if e.jsManager != nil {
			if err := e.jsManager.PublishExecutionState(execution); err != nil {
				e.logger.Warnf("Failed to persist execution state after step %s: %v", step.ID, err)
			}
		}
	}

	// 判断最终状态
	hasFailed := false
	ctx.stepsMu.Lock()
	for _, stepResult := range execution.Steps {
		if stepResult.Status == "failed" {
			hasFailed = true
			break
		}
	}
	ctx.stepsMu.Unlock()
	if hasFailed {
		execution.Status = "failed"
	} else {
		execution.Status = "completed"
	}

	e.logger.Infof("Workflow execution completed: %s (status: %s)", execution.WorkflowID, execution.Status)

	// 持久化最终状态到 JetStream
	if e.jsManager != nil {
		if err := e.jsManager.PublishExecutionState(execution); err != nil {
			e.logger.Warnf("Failed to persist final execution state: %v", err)
		}
	}
}

// setStepOutput 安全写入步骤输出
func (ctx *WorkflowContext) setStepOutput(key string, value any) {
	ctx.stepMu.Lock()
	defer ctx.stepMu.Unlock()
	ctx.StepOutputs[key] = value
}

// getStepOutput 安全读取步骤输出
func (ctx *WorkflowContext) getStepOutput(key string) (any, bool) {
	ctx.stepMu.RLock()
	defer ctx.stepMu.RUnlock()
	val, ok := ctx.StepOutputs[key]
	return val, ok
}

// setStepResult 安全写入步骤结果
func (ctx *WorkflowContext) setStepResult(stepID string, result models.StepResult) {
	ctx.stepsMu.Lock()
	defer ctx.stepsMu.Unlock()
	ctx.Execution.Steps[stepID] = result
}

// executeStep 执行单个步骤
func (e *Engine) executeStep(ctx *WorkflowContext, step *models.StepDef) {
	startTime := time.Now()
	result := models.StepResult{
		ID:        step.ID,
		Status:    "running",
		StartTime: startTime,
	}
	ctx.setStepResult(step.ID, result)

	// 解析超时
	timeout := e.defaultTimeout
	if step.Timeout != "" {
		if d, err := time.ParseDuration(step.Timeout); err == nil {
			timeout = d
		}
	}

	// 渲染输入
	renderedInput := e.renderTemplate(step.Input, ctx)

	// 创建带超时的 context
	stepCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 调用智能体
	output, err := e.callAgent(stepCtx, step.Agent, renderedInput)

	now := time.Now()
	result.EndTime = &now
	result.DurationMS = float64(now.Sub(startTime).Microseconds()) / 1000.0

	if err != nil {
		e.logger.Errorf("Workflow %s step %s failed: %v", ctx.Execution.WorkflowID, step.ID, err)
		result.Status = "failed"
		result.Error = err.Error()

		// 处理失败策略
		e.handleFailure(ctx, step, err)
	} else {
		result.Status = "completed"
		result.Output = output
		ctx.setStepOutput(step.ID, output)
		if step.Output != "" {
			ctx.setStepOutput(step.Output, output)
		}
		e.logger.Infof("Workflow %s step %s completed (%.2fms)", ctx.Execution.WorkflowID, step.ID, result.DurationMS)
	}

	ctx.setStepResult(step.ID, result)
}

// executeParallel 并行执行多个步骤
func (e *Engine) executeParallel(ctx *WorkflowContext, steps []models.StepDef) {
	var wg sync.WaitGroup
	for _, step := range steps {
		wg.Add(1)
		go func(s models.StepDef) {
			defer wg.Done()
			e.executeStep(ctx, &s)
		}(step)
	}
	wg.Wait()
}

// callAgent 调用智能体执行推理
func (e *Engine) callAgent(ctx context.Context, agentID string, input any) (any, error) {
	// 通过 NATS 发送请求（内置智能体有 NATS 订阅处理器，即使 registry 中状态为 offline 也能响应）
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	subject := fmt.Sprintf("agent.%s.invoke", agentID)
	msg, err := e.bridge.RequestNATS(subject, inputBytes, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("agent %s invoke failed: %w", agentID, err)
	}

	var output any
	if err := json.Unmarshal(msg.Data, &output); err != nil {
		// 尝试作为原始字符串
		return string(msg.Data), nil
	}
	return output, nil
}

// handleFailure 处理步骤失败
func (e *Engine) handleFailure(ctx *WorkflowContext, step *models.StepDef, err error) {
	if step.OnFailure == nil {
		return
	}

	switch v := step.OnFailure.(type) {
	case string:
		switch v {
		case "continue":
			e.logger.Infof("Step %s on_failure=continue, proceeding", step.ID)
		case "abort":
			e.logger.Errorf("Step %s on_failure=abort, stopping workflow", step.ID)
			ctx.Execution.Status = "failed"
			ctx.Execution.Error = fmt.Sprintf("step %s aborted: %v", step.ID, err)
		}
	case map[string]any:
		// {agent: "template.fallback_alert"} → 调用回退智能体
		if fallbackAgent, ok := v["agent"].(string); ok {
			e.logger.Infof("Step %s on_failure=fallback, calling %s", step.ID, fallbackAgent)
			fallbackInput := map[string]any{
				"failed_step": step.ID,
				"error":       err.Error(),
				"trigger":     ctx.TriggerData,
			}
			output, fallbackErr := e.callAgent(context.Background(), fallbackAgent, fallbackInput)
			if fallbackErr != nil {
				e.logger.Errorf("Fallback agent %s also failed: %v", fallbackAgent, fallbackErr)
			} else {
				ctx.setStepOutput(step.ID+"_fallback", output)
			}
		}
	}
}

// evaluateCondition 评估条件表达式
// 支持简单的表达式如: "anomaly_result.confidence > 0.9"
// "alert_message.urgency >= HIGH"
func (e *Engine) evaluateCondition(condition string, ctx *WorkflowContext) (bool, error) {
	// 渲染模板变量
	rendered := e.renderString(condition, ctx)

	// 简单条件解析: field operator value
	operators := []string{">=", "<=", "!=", "==", ">", "<"}
	for _, op := range operators {
		if idx := strings.Index(rendered, op); idx > 0 {
			left := strings.TrimSpace(rendered[:idx])
			right := strings.TrimSpace(rendered[idx+len(op):])

			leftVal := e.resolveValue(left, ctx)
			rightVal := e.resolveValue(right, ctx)

			return e.compareValues(leftVal, op, rightVal)
		}
	}

	// 布尔表达式: AND / OR
	upperRendered := strings.ToUpper(rendered)
	if strings.Contains(upperRendered, " AND ") {
		parts := strings.SplitN(upperRendered, " AND ", 2)
		r1, _ := e.evaluateCondition(parts[0], ctx)
		r2, _ := e.evaluateCondition(parts[1], ctx)
		return r1 && r2, nil
	}
	if strings.Contains(upperRendered, " OR ") {
		parts := strings.SplitN(upperRendered, " OR ", 2)
		r1, _ := e.evaluateCondition(parts[0], ctx)
		r2, _ := e.evaluateCondition(parts[1], ctx)
		return r1 || r2, nil
	}

	// 简单布尔值
	switch strings.ToUpper(rendered) {
	case "TRUE", "1":
		return true, nil
	case "FALSE", "0":
		return false, nil
	}

	return false, fmt.Errorf("unable to evaluate condition: %s", condition)
}

// resolveValue 解析值
func (e *Engine) resolveValue(expr string, ctx *WorkflowContext) any {
	expr = strings.TrimSpace(expr)

	// 引用字符串
	if (strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"")) ||
		(strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'")) {
		return expr[1 : len(expr)-1]
	}

	// 数字
	if f, err := strconv.ParseFloat(expr, 64); err == nil {
		return f
	}

	// 枚举值
	switch strings.ToUpper(expr) {
	case "HIGH":
		return float64(3)
	case "MEDIUM":
		return float64(2)
	case "LOW":
		return float64(1)
	case "CRITICAL":
		return float64(4)
	}

	// 从步骤输出解析字段
	parts := strings.Split(expr, ".")
	if len(parts) >= 2 {
		stepName := parts[0]
		fieldPath := parts[1:]

		val, ok := ctx.getStepOutput(stepName)
		if !ok {
			return nil
		}
		return e.navigateMap(val, fieldPath)
	}

	return nil
}

// navigateMap 在嵌套 map 中导航
func (e *Engine) navigateMap(val any, path []string) any {
	for _, key := range path {
		switch v := val.(type) {
		case map[string]any:
			val = v[key]
		default:
			// 尝试 JSON 序列化后重新解析
			bytes, err := json.Marshal(val)
			if err != nil {
				return nil
			}
			var m map[string]any
			if err := json.Unmarshal(bytes, &m); err != nil {
				return nil
			}
			val = m[key]
		}
		if val == nil {
			return nil
		}
	}
	return val
}

// compareValues 比较两个值
func (e *Engine) compareValues(left any, op string, right any) (bool, error) {
	leftF := toFloat64(left)
	rightF := toFloat64(right)

	switch op {
	case ">":
		return leftF > rightF, nil
	case "<":
		return leftF < rightF, nil
	case ">=":
		return leftF >= rightF, nil
	case "<=":
		return leftF <= rightF, nil
	case "==":
		return leftF == rightF, nil
	case "!=":
		return leftF != rightF, nil
	}
	return false, fmt.Errorf("unknown operator: %s", op)
}

// renderTemplate 渲染模板变量
func (e *Engine) renderTemplate(input any, ctx *WorkflowContext) any {
	switch v := input.(type) {
	case string:
		return e.renderString(v, ctx)
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			result[k] = e.renderTemplate(val, ctx)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = e.renderTemplate(val, ctx)
		}
		return result
	default:
		return input
	}
}

// renderString 渲染字符串中的 {{ }} 模板
func (e *Engine) renderString(s string, ctx *WorkflowContext) string {
	return e.templateRe.ReplaceAllStringFunc(s, func(match string) string {
		// 提取变量名
		varName := strings.TrimSpace(match[2 : len(match)-2])

		// trigger.payload → ctx.TriggerData
		parts := strings.SplitN(varName, ".", 2)
		if parts[0] == "trigger" {
			if len(parts) == 1 {
				// 返回整个 trigger data
				if bytes, err := json.Marshal(ctx.TriggerData); err == nil {
					return string(bytes)
				}
				return ""
			}
			val := e.navigateMap(ctx.TriggerData, strings.Split(parts[1], "."))
			if val == nil {
				return ""
			}
			return fmt.Sprintf("%v", val)
		}

		// 从步骤输出解析
		val := e.resolveValue(varName, ctx)
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	})
}

// GetExecution 获取执行状态
func (e *Engine) GetExecution(workflowID string) (*models.WorkflowExecution, bool) {
	val, ok := e.executions.Load(workflowID)
	if !ok {
		return nil, false
	}
	exec := val.(*models.WorkflowExecution)
	return exec, true
}

// ListExecutions 列出所有执行（按开始时间倒序）
func (e *Engine) ListExecutions() []*models.WorkflowExecution {
	var result []*models.WorkflowExecution
	e.executions.Range(func(key, value any) bool {
		result = append(result, value.(*models.WorkflowExecution))
		return true
	})
	// 按开始时间倒序排列
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.After(result[j].StartTime)
	})
	return result
}

// ActiveExecutionCount 返回活跃执行数
func (e *Engine) ActiveExecutionCount() int {
	count := 0
	e.executions.Range(func(key, value any) bool {
		exec := value.(*models.WorkflowExecution)
		if exec.Status == "running" {
			count++
		}
		return true
	})
	return count
}

// WorkflowContext 是工作流执行上下文
type WorkflowContext struct {
	Execution   *models.WorkflowExecution
	TriggerData map[string]any
	StepOutputs map[string]any
	stepMu      sync.RWMutex // 保护 StepOutputs 并发访问
	stepsMu     sync.Mutex   // 保护 Execution.Steps 并发写入
}

// toFloat64 将任意值转为 float64
func toFloat64(val any) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return 0
	}
}
