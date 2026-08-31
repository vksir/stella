package agent

import (
	"context"
	"uuid"
)

type OnEvent func(ctx context.Context, event Event)

type LLM interface {
	Chat(ctx context.Context, request ChatRequest, onEvent OnEvent) error
}

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args string) (Content, error)
}

type DynTool interface {
	Tool
	Summary() string
}

type Store interface {
	Find(ctx context.Context, id uuid.UUID) (*Session, error)
	FindByName(ctx context.Context, name string) (*Session, error)
	Save(ctx context.Context, s *Session) error
	AppendMessage(ctx context.Context, id uuid.UUID, messages ...Message) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type Session struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	ReasoningEffort string    `json:"reasoning_effort"`
	Temperature     *float64  `json:"temperature,omitempty"`
}

type ToolRequest struct {
	Name        string
	Description string
	Schema      map[string]any
}

type ChatRequest struct {
	Model           string // Model 使用 provider/model
	SystemPrompt    string
	Messages        []Message
	Tools           []ToolRequest
	ReasoningEffort string
	Temperature     *float64
}
