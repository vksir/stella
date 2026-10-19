package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"uuid"

	"github.com/vksir/stella/pkg/collection"
)

var (
	defaultMaxTurn = 32

	ErrAgentBusy       = errors.New("agent busy")
	ErrMessageSteered  = errors.New("message steered")
	ErrSessionNotFound = errors.New("session not found")
)

type Config struct {
	name            string
	prompt          *SystemPrompt
	scenarioPrompt  string
	extensions      []Extension
	model           string
	reasoningEffort string
	temperature     *float64
	tools           *collection.OrderedMap[string, Tool]
	dynTools        *collection.OrderedMap[string, Tool]

	llm      LLM
	store    Store
	log      *slog.Logger
	onEvent  OnEvent
	maxTurn  int
	nonblock bool
}

type Agent struct {
	Config

	dyn       *dynGateway
	session   *Session
	appendant []Message

	pending []Message
	mu      sync.Mutex
	running bool
}

func New(name string, opts ...Option) (*Agent, error) {
	cfg := Config{
		name:     name,
		tools:    collection.NewOrderedMap[string, Tool](),
		dynTools: collection.NewOrderedMap[string, Tool](),
		log:      slog.Default(),
		onEvent:  func(ctx context.Context, event Event) {},
		maxTurn:  defaultMaxTurn,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// 探测扩展能力接口，收集贡献：工具、dyn 工具、提示词段与事件监听
	var sections []PromptSection
	var listeners []EventListener
	for _, ext := range cfg.extensions {
		if p, ok := ext.(ToolProvider); ok {
			for _, t := range p.Tools() {
				cfg.tools.Set(t.Name(), t)
			}
		}
		if p, ok := ext.(DynToolProvider); ok {
			for _, t := range p.DynTools() {
				cfg.dynTools.Set(t.Name(), t)
			}
		}
		if p, ok := ext.(PromptSectionProvider); ok {
			sections = append(sections, p.PromptSections()...)
		}
		if l, ok := ext.(EventListener); ok {
			listeners = append(listeners, l)
		}
		cfg.log.Debug("extension registered", "extension", ext.Name())
	}
	// 事件扇出：先用户回调，后扩展监听器，按注册顺序同步派发
	if len(listeners) > 0 {
		userOnEvent := cfg.onEvent
		cfg.onEvent = func(ctx context.Context, event Event) {
			userOnEvent(ctx, event)
			for _, l := range listeners {
				l.OnEvent(ctx, event)
			}
		}
	}

	dyn := newDynGateway(cfg.dynTools)
	if cfg.dynTools.Len() > 0 {
		cfg.tools.Set(dyn.Name(), dyn)
	}

	cfg.prompt = &SystemPrompt{
		identity: agentPrompt,
		scenario: cfg.scenarioPrompt,
		tools:    cfg.tools,
		dynTools: cfg.dynTools,
		sections: sections,
	}

	cfg.log = cfg.log.With("agent", name)
	return &Agent{Config: cfg, dyn: dyn}, nil
}

func (a *Agent) Name() string {
	return a.name
}

func (a *Agent) Run(ctx context.Context, messages []Message) (*Message, error) {
	a.mu.Lock()
	if a.running {
		if !a.nonblock {
			a.mu.Unlock()
			return nil, ErrAgentBusy
		}
		a.pending = append(a.pending, messages...)
		a.mu.Unlock()
		return nil, ErrMessageSteered
	}
	a.pending = append(a.pending, messages...)
	a.running = true
	a.mu.Unlock()

	if err := a.load(ctx); err != nil {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		a.log.Error("agent load failed", "err", err)
		return nil, err
	}

	for {
		err := a.loop(ctx)
		if len(a.appendant) > 0 {
			if saveErr := a.store.AppendMessage(ctx, a.session.ID, a.appendant...); saveErr != nil {
				err = errors.Join(err, fmt.Errorf("append session history: %w", saveErr))
			} else {
				a.session.Messages = append(a.session.Messages, a.appendant...)
				a.appendant = nil
			}
		}

		if err != nil {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			a.log.Error("agent loop failed", "err", err)
			return nil, err
		}

		a.mu.Lock()
		if len(a.pending) == 0 {
			a.running = false
			a.mu.Unlock()
			return &a.session.Messages[len(a.session.Messages)-1], nil
		}
		a.mu.Unlock()
	}
}

func (a *Agent) load(ctx context.Context) error {
	if a.session != nil {
		return nil
	}

	session, err := a.store.FindByName(ctx, a.name)
	if err == nil {
		a.session = session
		return nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		return fmt.Errorf("load session: %w", err)
	}

	session = &Session{
		ID:              uuid.New(),
		Name:            a.name,
		Model:           a.model,
		ReasoningEffort: a.reasoningEffort,
		Temperature:     a.temperature,
	}
	if err := a.store.Save(ctx, session); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	a.session = session
	a.log.Info("session created", "session_id", session.ID)
	return nil
}

func (a *Agent) loop(ctx context.Context) error {
	var turn int

	for turn < a.maxTurn {
		turn++
		if err := ctx.Err(); err != nil {
			return err
		}

		a.mu.Lock()
		messages := a.pending
		a.pending = nil
		a.mu.Unlock()
		if len(messages) > 0 {
			a.appendant = append(a.appendant, messages...)
		}

		mb := newMessageBuilder(a.onEvent)
		if err := a.llm.Chat(ctx, a.toChatRequest(), mb.onEvent); err != nil {
			return err
		}
		m := mb.toMessage()

		if len(m.ToolCalls) == 0 {
			a.appendant = append(a.appendant, m)
			return nil
		}
		if m.Finish == FinishLength {
			return fmt.Errorf("model tool calls were truncated")
		}
		a.appendant = append(a.appendant, m)
		a.appendant = append(a.appendant, a.executeTools(ctx, m.ToolCalls)...)
	}

	return fmt.Errorf("agent loop exceeds max turn %d", a.maxTurn)
}

func (a *Agent) executeTools(ctx context.Context, toolCalls []ToolCall) []Message {
	results := make([]Message, len(toolCalls))

	var wg sync.WaitGroup
	for i, call := range toolCalls {
		wg.Go(func() {
			result := Message{Role: RoleTool, ToolCallID: call.ID}

			var err error
			tool, ok := a.tools.Get(call.Name)
			if !ok {
				err = fmt.Errorf("tool %q is not available", call.Name)
			} else {
				result.Content, err = tool.Execute(ctx, call.Args)
			}
			if err != nil {
				result.Content = TextContent(err.Error())
			}

			results[i] = result
			a.onEvent(ctx, Event{
				Type: EventToolExecution,
				ToolCallResult: ToolCallResult{
					Index:   i,
					ID:      call.ID,
					Name:    call.Name,
					Success: err == nil,
					Content: result.Content,
				},
			})
		})
	}
	wg.Wait()

	return results
}

func (a *Agent) toChatRequest() ChatRequest {
	request := ChatRequest{
		Model:           a.model,
		SystemPrompt:    a.prompt.Build(),
		Messages:        append(a.session.Messages, a.appendant...),
		ReasoningEffort: a.session.ReasoningEffort,
	}
	if a.session.Temperature != nil {
		request.Temperature = new(*a.session.Temperature)
	}
	if a.tools != nil {
		for _, tool := range a.tools.All() {
			request.Tools = append(request.Tools, ToolRequest{
				Name:        tool.Name(),
				Description: tool.Description(),
				Schema:      tool.Schema(),
			})
		}
	}
	return request
}


type toolCallBuilder struct {
	ID   string
	Name string
	Args strings.Builder
}

type messageBuilder struct {
	Message
	ContentBuilder   strings.Builder
	ReasoningBuilder strings.Builder
	ToolCallsBuilder *collection.OrderedMap[int, *toolCallBuilder]
	innerOnEvent     OnEvent
}

func newMessageBuilder(onEvent OnEvent) *messageBuilder {
	return &messageBuilder{
		Message:          Message{Role: RoleAssistant},
		ToolCallsBuilder: collection.NewOrderedMap[int, *toolCallBuilder](),
		innerOnEvent:     onEvent,
	}
}

func (m *messageBuilder) onEvent(ctx context.Context, event Event) {
	m.innerOnEvent(ctx, event)

	switch event.Type {
	case EventTextDelta:
		m.ContentBuilder.WriteString(event.TextDelta)
	case EventReasoningDelta:
		m.ReasoningBuilder.WriteString(event.ReasoningDelta)
	case EventToolCallDelta:
		delta := event.ToolCallDelta
		tcb, ok := m.ToolCallsBuilder.Get(delta.Index)
		if !ok {
			tcb = &toolCallBuilder{}
			m.ToolCallsBuilder.Set(delta.Index, tcb)
		}
		if delta.ID != "" && tcb.ID == "" {
			tcb.ID = delta.ID
		}
		if delta.Name != "" && tcb.Name == "" {
			tcb.Name = delta.Name
		}
		tcb.Args.WriteString(delta.Arguments)
	case EventFinish:
		m.Finish = event.Finish
	}
}

func (m *messageBuilder) toMessage() Message {
	msg := m.Message
	msg.Content = Content{}
	if m.ContentBuilder.Len() > 0 {
		msg.Content = TextContent(m.ContentBuilder.String())
	}
	msg.Reasoning = m.ReasoningBuilder.String()
	for _, tcb := range m.ToolCallsBuilder.All() {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:   tcb.ID,
			Name: tcb.Name,
			Args: tcb.Args.String(),
		})
	}
	return msg
}
