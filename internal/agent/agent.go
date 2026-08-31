package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/vksir/stella/pkg/collection"
)

var (
	defaultMaxTurn = 32
)

type Config struct {
	name         string
	systemPrompt string
	llm          LLM
	store        Store
	tools        *collection.OrderedMap[string, Tool]
	log          *slog.Logger
	onEvent      OnEvent
	maxTurn      int
	nonblock     bool
}

type Agent struct {
	Config

	session   *Session
	appendant []Message

	pending []Message
	mu      sync.Mutex
	running bool
}

func New(name string, llm LLM, store Store, opts ...Option) *Agent {
	cfg := Config{
		name:    name,
		llm:     llm,
		store:   store,
		tools:   collection.NewOrderedMap[string, Tool](),
		log:     slog.Default(),
		maxTurn: defaultMaxTurn,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.log = cfg.log.With("agent", name)
	return &Agent{Config: cfg}
}

func (a *Agent) Steer(ctx context.Context, messages ...Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return ErrNotRunning
	}
	a.pending = append(a.pending, MessagesClone(messages)...)
	return nil
}

// SessionID 返回当前会话标识。
func (a *Agent) SessionID() uuid.UUID {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session == nil {
		return uuid.Nil
	}
	return a.session.ID
}

func (a *Agent) Run(ctx context.Context, messages []Message) ([]Message, error) {
	a.mu.Lock()
	a.pending = append(a.pending, MessagesClone(messages)...)
	if a.running {
		a.mu.Unlock()
		return nil, nil
	}
	a.running = true
	a.mu.Unlock()

	if err := a.load(ctx); err != nil {
		a.mu.Lock()
		a.pending = nil
		a.appendant = nil
		a.session = nil
		a.running = false
		a.mu.Unlock()
		a.log.Error("agent run failed", "error", err)
		return nil, err
	}

	var result []Message
	for {
		err := a.loop(ctx)
		if len(a.appendant) > 0 {
			if saveErr := a.store.AppendMessage(ctx, a.session.ID, a.appendant...); saveErr != nil {
				err = errors.Join(err, fmt.Errorf("append session history: %w", saveErr))
			} else {
				a.session.Messages = append(a.session.Messages, a.appendant...)
				result = append(result, MessagesClone(a.appendant)...)
				a.appendant = nil
			}
		}
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			a.mu.Lock()
			pendingCount := len(a.pending)
			a.pending = nil
			a.appendant = nil
			a.session = nil
			a.running = false
			a.mu.Unlock()
			a.log.Error("agent run failed", "pending_count", pendingCount, "error", err)
			return result, err
		}

		a.mu.Lock()
		if len(a.pending) == 0 {
			a.running = false
			a.mu.Unlock()
			return result, nil
		}
		a.mu.Unlock()
	}
}

func (a *Agent) load(ctx context.Context) error {
	a.mu.Lock()
	if a.session != nil {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	session, err := a.store.FindByName(ctx, a.name)
	if err == nil {
		a.mu.Lock()
		a.session = session
		a.mu.Unlock()
		return nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		return fmt.Errorf("load session: %w", err)
	}
	session = &Session{ID: uuid.New(), Name: a.name}
	if err := a.store.Save(ctx, session); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	a.mu.Lock()
	a.session = session
	a.mu.Unlock()
	a.log.Info("session created", "session_id", session.ID)
	return nil
}

func (a *Agent) loop(ctx context.Context) error {
	var turn int

	for turn < a.maxTurn {
		turn++

		a.mu.Lock()
		messages := a.pending
		a.pending = nil
		a.mu.Unlock()
		if len(messages) > 0 {
			a.log.Debug("agent inputs consumed", "count", len(messages))
			a.appendant = append(a.appendant, messages...)
		}

		mb, combinedOnEvent := a.combineOnEvent(a.onEvent)
		if err := a.llm.Chat(ctx, a.readOnly(), combinedOnEvent); err != nil {
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

		if err := ctx.Err(); err != nil {
			return err
		}
	}

	return fmt.Errorf("agent loop exceeds max turn %d", a.maxTurn)
}

func (a *Agent) executeTools(ctx context.Context, toolCalls []ToolCall) []Message {
	results := make([]Message, len(toolCalls))

	var wg sync.WaitGroup
	for i, call := range toolCalls {
		wg.Go(func() {
			result := Message{Role: RoleTool, ToolCallID: call.ID}

			tool, ok := a.tools.Get(call.Name)
			if !ok {
				result.Content = fmt.Sprintf("tool %q is not available", call.Name)
			} else {
				a.log.Debug("agent tool started", "tool_name", call.Name, "tool_call_id", call.ID)
				content, err := tool.Execute(ctx, call.Args)
				if err != nil {
					content = err.Error()
				}
				result.Content = content
			}

			results[i] = result
		})
	}
	wg.Wait()

	return results
}

func (a *Agent) combineOnEvent(onEvent OnEvent) (*messageBuilder, OnEvent) {
	mb := &messageBuilder{
		Message:          Message{Role: RoleAssistant},
		ToolCallsBuilder: collection.NewOrderedMap[int, *toolCallBuilder](),
	}

	return mb, func(ctx context.Context, event Event) {
		if onEvent != nil {
			onEvent(ctx, event)
		}

		switch event.Type {
		case EventTextDelta:
			mb.ContentBuilder.WriteString(event.TextDelta)
		case EventReasoningDelta:
			mb.ReasoningBuilder.WriteString(event.ReasoningDelta)
		case EventToolCallDelta:
			delta := event.ToolCallDelta
			tcb, ok := mb.ToolCallsBuilder.Get(delta.Index)
			if !ok {
				tcb = &toolCallBuilder{}
				mb.ToolCallsBuilder.Set(delta.Index, tcb)
			}
			if delta.ID != "" && tcb.ID == "" {
				tcb.ID = delta.ID
			}
			if delta.Name != "" && tcb.Name == "" {
				tcb.Name = delta.Name
			}
			if event.Type == EventToolCallDelta {
				tcb.Args.WriteString(delta.Arguments)
			}
		case EventFinish:
			mb.Finish = event.Finish
		}
	}
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
}

func (m *messageBuilder) toMessage() Message {
	msg := m.Message
	msg.Content = m.ContentBuilder.String()
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
