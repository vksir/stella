package agent

import "context"

type OnEvent func(ctx context.Context, event Event)

type LLM interface {
	Chat(ctx context.Context, request StateReadOnly, onEvent OnEvent) error
}

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args string) (string, error)
}
