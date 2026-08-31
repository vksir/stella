// Package storex 提供会话存储实现，当前为内存实现。
package storex

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/vksir/stella/internal/agent"
)

// Memory 保存会话副本，并发安全。
type Memory struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*agent.Session
	log      *slog.Logger
}

func NewMemory() *Memory {
	return &Memory{sessions: make(map[uuid.UUID]*agent.Session), log: slog.Default()}
}

func (s *Memory) Find(ctx context.Context, id uuid.UUID) (_ *agent.Session, err error) {
	defer func() { s.record("find session", err, "session_id", id) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, agent.ErrSessionNotFound
	}
	return sess.Clone(), nil
}

func (s *Memory) FindByName(ctx context.Context, name string) (_ *agent.Session, err error) {
	defer func() { s.record("find session by name", err, "session_name", name) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, sess := range s.sessions {
		if sess.Name == name {
			return sess.Clone(), nil
		}
	}
	return nil, agent.ErrSessionNotFound
}

func (s *Memory) Save(ctx context.Context, sess *agent.Session) (err error) {
	defer func() { s.record("save session", err, "session_id", sess.ID) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sess.ID] = sess.Clone()
	return nil
}

func (s *Memory) AppendMessage(ctx context.Context, id uuid.UUID, messages ...agent.Message) (err error) {
	defer func() { s.record("append session messages", err, "session_id", id, "count", len(messages)) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return agent.ErrSessionNotFound
	}
	sess.Messages = append(sess.Messages, agent.MessagesClone(messages)...)
	return nil
}

func (s *Memory) Delete(ctx context.Context, id uuid.UUID) (err error) {
	defer func() { s.record("delete session", err, "session_id", id) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)
	return nil
}

func (s *Memory) record(operation string, err error, fields ...any) {
	if err != nil {
		fields = append(fields, "error", err)
		if err == agent.ErrSessionNotFound {
			s.log.Debug(operation, fields...)
		} else {
			s.log.Error(operation, fields...)
		}
		return
	}
	s.log.Debug(operation, fields...)
}
