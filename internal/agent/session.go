package agent

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrSessionNotFound = errors.New("session not found")

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

func (s *Session) Clone() *Session {
	cloned := *s
	cloned.Messages = MessagesClone(s.Messages)
	if s.Temperature != nil {
		cloned.Temperature = new(*s.Temperature)
	}
	return &cloned
}
