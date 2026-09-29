package observability

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Span 链路追踪 span
type Span struct {
	TraceID   string                 `json:"trace_id"`
	SpanID    string                 `json:"span_id"`
	ParentID  string                 `json:"parent_id,omitempty"`
	Name      string                 `json:"name"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Tags      map[string]string      `json:"tags,omitempty"`
	Status    string                 `json:"status"` // ok / error
}

// Tracer 链路追踪器（轻量级实现）
// 生产环境可替换为 OpenTelemetry SDK
type Tracer struct {
	mu     sync.RWMutex
	spans  []Span
	logger *logrus.Entry
}

// NewTracer 创建链路追踪器
func NewTracer(logger *logrus.Entry) *Tracer {
	return &Tracer{
		spans:  make([]Span, 0, 1000),
		logger: logger,
	}
}

// StartSpan 开始一个新的 span
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		TraceID:   getTraceID(ctx),
		SpanID:    generateSpanID(),
		ParentID:  getParentSpanID(ctx),
		Name:      name,
		StartTime: time.Now(),
		Tags:      make(map[string]string),
		Status:    "ok",
	}

	// 将 span ID 存入 context
	return context.WithValue(ctx, spanKey{}, span.SpanID), span
}

// FinishSpan 结束 span
func (t *Tracer) FinishSpan(span *Span) {
	now := time.Now()
	span.EndTime = &now

	t.mu.Lock()
	t.spans = append(t.spans, *span)
	// 保持最近 1000 个 span
	if len(t.spans) > 1000 {
		t.spans = t.spans[len(t.spans)-1000:]
	}
	t.mu.Unlock()

	t.logger.Debugf("Span finished: %s (trace: %s, duration: %v, status: %s)",
		span.Name, span.TraceID, now.Sub(span.StartTime), span.Status)
}

// SetTag 设置 span 标签
func (s *Span) SetTag(key, value string) {
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

// SetError 标记 span 为错误状态
func (s *Span) SetError(err error) {
	s.Status = "error"
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags["error"] = err.Error()
}

// GetSpans 获取所有 span
func (t *Tracer) GetSpans(limit int) []Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limit <= 0 || limit > len(t.spans) {
		limit = len(t.spans)
	}
	start := len(t.spans) - limit
	if start < 0 {
		start = 0
	}
	result := make([]Span, limit)
	copy(result, t.spans[start:])
	return result
}

// spanKey 用于 context 存储 span ID
type spanKey struct{}

// traceKey 用于 context 存储 trace ID
type traceKey struct{}

// getTraceID 从 context 获取 trace ID
func getTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceKey{}).(string); ok {
		return v
	}
	// 生成新的 trace ID
	return generateSpanID()
}

// getParentSpanID 从 context 获取父 span ID
func getParentSpanID(ctx context.Context) string {
	if v, ok := ctx.Value(spanKey{}).(string); ok {
		return v
	}
	return ""
}

// generateSpanID 生成 span ID
func generateSpanID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().UnixMicro()%10000)
}

// WithTraceID 将 trace ID 存入 context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceID)
}
