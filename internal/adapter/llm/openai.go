// Package llm 提供各模型提供商的流式调用实现。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/vksir/stella/internal/core/agent"
)

// Option 配置 OpenAI 客户端。
type Option func(*OpenAI)

// OpenAI 封装 OpenAI Chat Completions 流式调用，兼容常见 OpenAI 风格提供商。
type OpenAI struct {
	apiKey          string
	baseURL         string
	model           string
	client          *http.Client
	log             *slog.Logger
	headers         map[string]string
	reasoningField  string
	reasoningEffort string
	temperature     *float64
	usageParser     UsageParser
	supportsImages  bool
}

// NewOpenAI 创建 OpenAI 客户端，baseURL、reasoningField、usageParser 由 provider 配置注入。
func NewOpenAI(apiKey string, opts ...Option) *OpenAI {
	o := &OpenAI{
		apiKey: apiKey,
		client: &http.Client{},
		log:    slog.Default(),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithBaseURL 设置 API 基地址，如 https://api.deepseek.com/v1。
func WithBaseURL(url string) Option {
	return func(o *OpenAI) { o.baseURL = strings.TrimRight(url, "/") }
}

// WithModel 设置 provider 原生模型名，如 anthropic/claude-sonnet-4。
func WithModel(model string) Option {
	return func(o *OpenAI) { o.model = model }
}

// WithHTTPClient 设置自定义 HTTP 客户端。
func WithHTTPClient(c *http.Client) Option {
	return func(o *OpenAI) { o.client = c }
}

// WithLogger 设置日志器。
func WithLogger(l *slog.Logger) Option {
	return func(o *OpenAI) { o.log = l }
}

// WithHeader 添加自定义请求头。
func WithHeader(key, value string) Option {
	return func(o *OpenAI) {
		if o.headers == nil {
			o.headers = make(map[string]string)
		}
		o.headers[key] = value
	}
}

// WithReasoningField 设置推理内容字段名，如 reasoning_content；置空表示不使用推理内容。
func WithReasoningField(field string) Option {
	return func(o *OpenAI) { o.reasoningField = field }
}

// WithReasoningEffort 设置推理力度，如 low、medium、high，未设置时不下发该字段。
func WithReasoningEffort(effort string) Option {
	return func(o *OpenAI) { o.reasoningEffort = effort }
}

// WithTemperature 设置采样温度，未设置时不下发该字段。
func WithTemperature(t float64) Option {
	return func(o *OpenAI) { o.temperature = &t }
}

// WithUsageParser 设置流末尾原始 usage 的解析函数。
func WithUsageParser(p UsageParser) Option {
	return func(o *OpenAI) { o.usageParser = p }
}

// WithSupportsImages 设置模型是否接受图片输入。
func WithSupportsImages(supported bool) Option {
	return func(o *OpenAI) { o.supportsImages = supported }
}

// Chat 发起流式请求，request.Model 为 provider 原生模型名，增量通过 onEvent 回调。
func (o *OpenAI) Chat(ctx context.Context, request agent.ChatRequest, onEvent agent.OnEvent) error {
	model := request.Model
	if model == "" {
		model = o.model
	}
	o.log.Debug("openai chat started", "model", model)
	if o.baseURL == "" {
		err := fmt.Errorf("baseURL is not set")
		o.log.Error("openai chat failed", "error", err)
		return err
	}

	body, err := o.buildRequest(request)
	if err != nil {
		o.log.Error("openai request encoding failed", "error", err)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		o.log.Error("openai request creation failed", "error", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		o.log.Error("openai chat request failed", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		err := fmt.Errorf("openai chat http %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
		o.log.Error("openai chat failed", "error", err)
		return err
	}

	// 部分 provider 异常时可能返回 200 加 JSON 错误体而非 SSE 流
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		err := fmt.Errorf("openai chat unexpected content type %q: %s", ct, strings.TrimSpace(string(detail)))
		o.log.Error("openai chat failed", "error", err)
		return err
	}

	if err := o.consumeStream(ctx, resp.Body, onEvent); err != nil {
		o.log.Error("openai stream failed", "error", err)
		return err
	}
	o.log.Debug("openai chat stream completed", "model", model)
	return nil
}

func (o *OpenAI) buildRequest(request agent.ChatRequest) ([]byte, error) {
	messages, err := o.buildMessages(request)
	if err != nil {
		return nil, err
	}
	req := chatRequest{
		Model:           o.model,
		Stream:          true,
		Messages:        messages,
		Temperature:     o.temperature,
		ReasoningEffort: o.reasoningEffort,
	}
	if request.Model != "" {
		req.Model = request.Model
	}
	if request.Temperature != nil {
		req.Temperature = request.Temperature
	}
	if request.ReasoningEffort != "" {
		req.ReasoningEffort = request.ReasoningEffort
	}

	for _, tool := range request.Tools {
		req.Tools = append(req.Tools, chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Schema,
			},
		})
	}
	return json.Marshal(req)
}

func (o *OpenAI) buildMessages(request agent.ChatRequest) ([]map[string]any, error) {
	msgs := make([]map[string]any, 0, 1+len(request.Messages))
	if request.SystemPrompt != "" {
		msgs = append(msgs, map[string]any{"role": agent.RoleSystem, "content": request.SystemPrompt})
	}
	for i := 0; i < len(request.Messages); i++ {
		m := request.Messages[i]
		if m.Role != agent.RoleTool {
			msg, err := o.toChatMessage(m)
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, msg)
			continue
		}

		// 连续工具结果先逐条发送，再将其图片放进请求专用的 user 消息。
		var images []map[string]any
		for ; i < len(request.Messages) && request.Messages[i].Role == agent.RoleTool; i++ {
			toolMessage := request.Messages[i]
			msg, err := o.toChatMessage(toolMessage)
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, msg)
			if !o.supportsImages {
				continue
			}
			for _, part := range toolMessage.Content {
				if part.Type != "image" {
					continue
				}
				url, err := imageURL(part)
				if err != nil {
					return nil, err
				}
				images = append(images,
					map[string]any{"type": "text", "text": "Image from tool call " + toolMessage.ToolCallID + ":"},
					map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}},
				)
			}
		}
		i--
		if len(images) > 0 {
			msgs = append(msgs, map[string]any{"role": agent.RoleUser, "content": images})
		}
	}
	return msgs, nil
}

func (o *OpenAI) toChatMessage(m agent.Message) (map[string]any, error) {
	text := m.Content.Text()
	msg := map[string]any{"role": m.Role, "content": text}
	if m.Role == agent.RoleTool {
		for _, part := range m.Content {
			if part.Type == "image" {
				if o.supportsImages {
					if text == "" {
						msg["content"] = "(see attached image)"
					}
				} else {
					note := "[Tool image omitted: model does not support images.]"
					if text != "" {
						note = text + "\n" + note
					}
					msg["content"] = note
				}
				break
			}
		}
		if msg["content"] == "" {
			msg["content"] = "(no tool output)"
		}
	}
	if m.Role != agent.RoleUser && m.Role != agent.RoleTool {
		for _, part := range m.Content {
			if part.Type != "text" {
				return nil, fmt.Errorf("unsupported %s content type %q", m.Role, part.Type)
			}
		}
	}
	if m.Role == agent.RoleUser {
		var blocks []map[string]any
		for _, part := range m.Content {
			switch part.Type {
			case "text":
				blocks = append(blocks, map[string]any{"type": "text", "text": part.Text})
			case "image":
				if !o.supportsImages {
					blocks = append(blocks, map[string]any{"type": "text", "text": "(image omitted: model does not support images)"})
					continue
				}
				url, err := imageURL(part)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}})
			default:
				return nil, fmt.Errorf("unsupported content type %q", part.Type)
			}
		}
		if len(blocks) > 0 && (len(blocks) > 1 || blocks[0]["type"] == "image_url") {
			msg["content"] = blocks
		}
	}
	if m.Name != "" {
		msg["name"] = m.Name
	}
	if m.ToolCallID != "" {
		msg["tool_call_id"] = m.ToolCallID
	}
	if m.Reasoning != "" && o.reasoningField != "" {
		msg[o.reasoningField] = m.Reasoning
	}
	if len(m.ToolCalls) > 0 {
		toolCalls := make([]map[string]any, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			toolCalls = append(toolCalls, map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]string{
					"name":      tc.Name,
					"arguments": tc.Args,
				},
			})
		}
		msg["tool_calls"] = toolCalls
	}
	return msg, nil
}

func imageURL(part agent.ContentPart) (string, error) {
	switch part.MimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", fmt.Errorf("unsupported image MIME type %q", part.MimeType)
	}
	if part.Data == "" {
		return "", fmt.Errorf("image data is empty")
	}
	return "data:" + part.MimeType + ";base64," + part.Data, nil
}

// consumeStream 解析 SSE 流并发送事件，finish 事件在流结束时发送。
func (o *OpenAI) consumeStream(ctx context.Context, r io.Reader, onEvent agent.OnEvent) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)

	emit := func(event agent.Event) {
		if onEvent != nil {
			onEvent(ctx, event)
		}
	}

	var (
		textStart   bool
		reasonStart bool
		activeCall  = make(map[int]struct{})
		finish      agent.FinishReason
		hasFinish   bool
		usage       agent.Usage
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("decode chunk: %w", err)
		}
		// 流中错误以 SSE 事件形式下发（如 OpenRouter mid-stream error）
		if chunk.Error != nil {
			err := fmt.Errorf("provider error: %s", chunk.Error.Message)
			o.log.Error("openai stream provider error", "error", err)
			return err
		}
		// usage 可能附在无 choices 的独立块（OpenAI），也可能附在复述 finish_reason 的块上（OpenRouter）
		if len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
			if o.usageParser == nil {
				return fmt.Errorf("usage parser not configured")
			}
			u, err := o.usageParser(chunk.Usage)
			if err != nil {
				return fmt.Errorf("parse usage: %w", err)
			}
			usage = u
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if hasFinish {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		if s := rawString(delta, o.reasoningField); s != "" {
			if !reasonStart {
				emit(agent.Event{Type: agent.EventReasoningStart})
				reasonStart = true
			}
			emit(agent.Event{Type: agent.EventReasoningDelta, ReasoningDelta: s})
		}

		if s := rawString(delta, "content"); s != "" {
			if reasonStart {
				emit(agent.Event{Type: agent.EventReasoningEnd})
				reasonStart = false
			}
			if !textStart {
				emit(agent.Event{Type: agent.EventTextStart})
				textStart = true
			}
			emit(agent.Event{Type: agent.EventTextDelta, TextDelta: s})
		}

		var toolCalls []chatToolCallChunk
		if raw, ok := delta["tool_calls"]; ok {
			if err := json.Unmarshal(raw, &toolCalls); err != nil {
				return fmt.Errorf("decode tool_calls: %w", err)
			}
		}
		for _, tc := range toolCalls {
			if _, ok := activeCall[tc.Index]; !ok {
				if textStart {
					emit(agent.Event{Type: agent.EventTextEnd})
					textStart = false
				}
				if reasonStart {
					emit(agent.Event{Type: agent.EventReasoningEnd})
					reasonStart = false
				}
				emit(agent.Event{Type: agent.EventToolCallStart, ToolCallDelta: agent.ToolCallDelta{Index: tc.Index, ID: tc.ID, Name: tc.Function.Name}})
				activeCall[tc.Index] = struct{}{}
			}
			emit(agent.Event{Type: agent.EventToolCallDelta, ToolCallDelta: agent.ToolCallDelta{
				Index:     tc.Index,
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}})
		}

		if fr := choice.FinishReason; fr != nil && *fr != "" {
			finish = mapFinishReason(*fr)
			hasFinish = true
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	if textStart {
		emit(agent.Event{Type: agent.EventTextEnd})
	}
	if reasonStart {
		emit(agent.Event{Type: agent.EventReasoningEnd})
	}
	for index := range activeCall {
		emit(agent.Event{Type: agent.EventToolCallEnd, ToolCallDelta: agent.ToolCallDelta{Index: index}})
	}
	if hasFinish {
		emit(agent.Event{Type: agent.EventFinish, Finish: finish, Usage: usage})
	}
	return nil
}

func rawString(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func mapFinishReason(s string) agent.FinishReason {
	switch s {
	case "length":
		return agent.FinishLength
	case "tool_calls":
		return agent.FinishToolCalls
	case "content_filter":
		return agent.FinishContentFilter
	default:
		return agent.FinishStop
	}
}

type chatRequest struct {
	Model           string           `json:"model"`
	Messages        []map[string]any `json:"messages"`
	Tools           []chatTool       `json:"tools,omitempty"`
	Stream          bool             `json:"stream"`
	Temperature     *float64         `json:"temperature,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatChunk struct {
	Choices []struct {
		Delta        map[string]json.RawMessage `json:"delta"`
		FinishReason *string                    `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
	Error *chatError      `json:"error"`
}

type chatError struct {
	Message string `json:"message"`
}

type chatToolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
