package agent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/vksir/stella/pkg/collection"
)

var (
	ErrUndefined = errors.New("undefined")
)

type LLM interface {
	Chat(ctx context.Context, session *Session, onEvent OnEvent) error
}

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args string) (string, error)
}

type Store interface {
	Load(ctx context.Context, id uuid.UUID) (*Session, error)
	Save(ctx context.Context, s *Session) error
	AppendMessage(ctx context.Context, id uuid.UUID, m ...Message) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type OnEvent func(ctx context.Context, event Event)

type Session struct {
	ID             uuid.UUID                           `json:"id"`
	Model          string                              `json:"model"`
	SystemPrompt   string                              `json:"system_prompt"`
	Messages       []Message                           `json:"messages"`
	ThinkingEffort string                              `json:"-"`
	AppendMessages []Message                           `json:"-"`
	Tools          collection.OrderedMap[string, Tool] `json:"-"`
}

type Agent struct {
	llm   LLM
	store Store
	log   *slog.Logger
}

func New(llm LLM, store Store) *Agent {
	return &Agent{
		llm:   llm,
		store: store,
		log:   slog.Default(),
	}
}

func (a *Agent) Run(ctx context.Context, session *Session, onEvent OnEvent) error {
	var err error

	defer clear(session.AppendMessages)

	for {
		mb := newMessageBuilder()
		combineOnEvent := a.genOnEvent(mb, onEvent)

		err = a.llm.Chat(ctx, session, combineOnEvent)
		if err != nil {
			return err
		}

		m := mb.toMessage()
		session.AppendMessages = append(session.AppendMessages, m)

		if len(m.ToolCalls) > 0 {
			if m.Finish == FinishLength {
				return ErrUndefined
			}

			toolResults := make(map[string]Message, len(m.ToolCalls))

			var wg sync.WaitGroup
			wg.Add(len(m.ToolCalls))
			for _, toolCall := range m.ToolCalls {
				tool, ok := session.Tools.Get(toolCall.Name)
				if !ok {
					return ErrUndefined
				}

				go func() {
					defer wg.Done()
					res, err := tool.Execute(ctx, toolCall.Args)
					if err != nil {
						res = err.Error()
					}
					toolResults[toolCall.Name] = Message{
						Role:       RoleTool,
						Content:    res,
						ToolCallID: toolCall.ID,
					}
				}()
			}
			wg.Wait()

			for _, toolCall := range m.ToolCalls {
				session.AppendMessages = append(session.AppendMessages, toolResults[toolCall.Name])
			}
		}

		if len(m.ToolCalls) == 0 {
			session.Messages = append(session.Messages, session.AppendMessages...)
			return a.store.AppendMessage(ctx, session.ID, session.AppendMessages...)
		}
	}
}

func (a *Agent) genOnEvent(mb *messageBuilder, onEvent OnEvent) OnEvent {
	return func(ctx context.Context, event Event) {
		onEvent(ctx, event)

		switch event.Type {
		case EventTextDelta:
			mb.ContentBuilder.WriteString(event.TextDelta)
		case EventThinkingDelta:
			mb.ThinkingBuilder.WriteString(event.ThinkingDelta)
		case EventToolCallDelta:
			delta := event.ToolCallDelta
			tcb, ok := mb.ToolCallsBuilder.Get(delta.Index)
			if !ok {
				tcb = &toolCallBuilder{ID: delta.ID, Name: delta.Name}
				mb.ToolCallsBuilder.Set(delta.Index, tcb)
			}
			tcb.Args.WriteString(delta.Arguments)
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
	ThinkingBuilder  strings.Builder
	ToolCallsBuilder *collection.OrderedMap[int, *toolCallBuilder]
}

func newMessageBuilder() *messageBuilder {
	return &messageBuilder{
		Message:          Message{Role: RoleAssistant},
		ToolCallsBuilder: collection.New[int, *toolCallBuilder](),
	}
}

func (m *messageBuilder) toMessage() Message {
	msg := m.Message
	msg.Content = m.ContentBuilder.String()
	msg.Thinking = m.ThinkingBuilder.String()
	for _, tcb := range m.ToolCallsBuilder.All() {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:   tcb.ID,
			Name: tcb.Name,
			Args: tcb.Args.String(),
		})
	}
	return msg
}
