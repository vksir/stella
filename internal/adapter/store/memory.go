// Package store 提供内存与数据库会话存储实现。
package store

import (
	"context"
	"log/slog"
	"sync"
	"uuid"

	"github.com/vksir/stella/internal/core/agent"
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
	return cloneSession(sess), nil
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
			return cloneSession(sess), nil
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

	s.sessions[sess.ID] = cloneSession(sess)
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
	sess.Messages = append(sess.Messages, cloneMessages(messages)...)
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

// cloneSession 返回会话深拷贝，隔离存储与调用方。
func cloneSession(sess *agent.Session) *agent.Session {
	cloned := *sess
	cloned.Messages = cloneMessages(sess.Messages)
	if sess.Temperature != nil {
		cloned.Temperature = new(*sess.Temperature)
	}
	return &cloned
}

// cloneMessages 返回消息切片深拷贝。
func cloneMessages(messages []agent.Message) []agent.Message {
	if messages == nil {
		return nil
	}
	cloned := make([]agent.Message, len(messages))
	for i, message := range messages {
		cloned[i] = message
		cloned[i].Content = append(agent.Content(nil), message.Content...)
		cloned[i].ToolCalls = append([]agent.ToolCall(nil), message.ToolCalls...)
	}
	return cloned
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
