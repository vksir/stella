// Package hub 按名称管理 Agent 缓存和运行生命周期。
package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/pkg/collection"
)

const defaultCapacity = 64

var ErrClosed = errors.New("session hub is closed")

// SessionHub 的锁只保护缓存和 worker 生命周期，不覆盖执行与存储。
// 忙碌 Agent 不可淘汰，全部忙碌时允许暂时超过缓存容量。
type SessionHub struct {
	ctx      context.Context
	cancel   context.CancelFunc
	llm      agent.LLM
	store    agent.Store
	opts     []agent.Option
	log      *slog.Logger
	mu       sync.Mutex
	lru      *collection.OrderedMap[string, *sessionEntry]
	capacity int
	closed   bool
	wg       sync.WaitGroup
	done     chan struct{}
}

type sessionEntry struct {
	agent   *agent.Agent
	run     *Run
	pending []agent.Message // 执行交接期间暂存的输入。
}

func NewSessionHub(ctx context.Context, llm agent.LLM, store agent.Store, capacity int, opts ...agent.Option) *SessionHub {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	ctx, cancel := context.WithCancel(ctx)
	h := &SessionHub{
		ctx: ctx, cancel: cancel, llm: llm, store: store,
		opts: append([]agent.Option(nil), opts...), log: slog.Default(),
		lru: collection.NewOrderedMap[string, *sessionEntry](), capacity: capacity,
		done: make(chan struct{}),
	}
	h.log.Debug("session hub created", "capacity", capacity)
	return h
}

// Submit 定位 Agent 并提交输入；ctx 仅约束本次提交。
// 成功表示进程内接收，运行及存储错误由回执返回。
func (h *SessionHub) Submit(ctx context.Context, name string, message agent.Message, policy BusyPolicy) (Submission, error) {
	log := h.log.With("session_name", name)
	if name == "" {
		err := fmt.Errorf("session name is required")
		log.Warn("session submission rejected", "error", err)
		return Submission{}, err
	}
	if policy != RejectIfBusy && policy != SteerIfBusy {
		err := fmt.Errorf("invalid busy policy: %d", policy)
		log.Warn("session submission rejected", "error", err)
		return Submission{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		log.Warn("session submission rejected", "error", err)
		return Submission{}, err
	}
	if h.closed || h.ctx.Err() != nil {
		log.Warn("session submission rejected", "error", ErrClosed)
		return Submission{}, ErrClosed
	}
	entry, ok := h.lru.Get(name)
	if !ok {
		entry = &sessionEntry{agent: agent.New(name, h.llm, h.store, h.opts...)}
		h.lru.Set(name, entry)
	}
	if entry.run != nil && policy == RejectIfBusy {
		log.Warn("session submission rejected", "error", agent.ErrBusy)
		return Submission{}, agent.ErrBusy
	}
	// 已暂存输入时继续入队，避免新消息越过交接期间的旧消息。
	if entry.run == nil || len(entry.pending) > 0 {
		entry.pending = append(entry.pending, agent.MessagesClone([]agent.Message{message})[0])
	} else if err := entry.agent.Steer(ctx, message); err != nil {
		if !errors.Is(err, agent.ErrNotRunning) {
			return Submission{}, err
		}
		entry.pending = append(entry.pending, agent.MessagesClone([]agent.Message{message})[0])
	}
	submission := Submission{Started: entry.run == nil}
	if submission.Started {
		entry.run = &Run{id: uuid.New(), done: make(chan struct{})}
		h.wg.Add(1)
		go h.execute(name, entry, entry.run)
	}
	submission.Run = entry.run
	h.lru.MoveToEnd(name)
	h.evictLocked()
	log.Debug("session message accepted", "run_id", entry.run.id, "started", submission.Started)
	return submission, nil
}

func (h *SessionHub) execute(name string, entry *sessionEntry, run *Run) {
	defer h.wg.Done()
	log := h.log.With("session_name", name, "run_id", run.id)
	log.Info("session run started")
	for {
		h.mu.Lock()
		messages := entry.pending
		entry.pending = nil
		h.mu.Unlock()

		messages, err := entry.agent.Run(h.ctx, messages)
		h.mu.Lock()
		if sessionID := entry.agent.SessionID(); sessionID != uuid.Nil {
			run.result.SessionID = sessionID
		}
		run.result.Messages = append(run.result.Messages, messages...)
		if err == nil {
			err = h.ctx.Err()
		}
		if err == nil && len(entry.pending) > 0 {
			h.mu.Unlock()
			log.Debug("session run continuing")
			continue
		}
		if err != nil {
			log.Error("session run failed", "pending_count", len(entry.pending), "error", err)
			entry.pending = nil
		} else {
			log.Info("session run completed")
		}
		run.err = err
		entry.run = nil
		close(run.done)
		h.evictLocked()
		h.mu.Unlock()
		return
	}
}

func (h *SessionHub) evictLocked() {
	if h.lru.Len() <= h.capacity {
		return
	}
	for _, name := range h.lru.Keys() {
		if h.lru.Len() <= h.capacity {
			break
		}
		entry, _ := h.lru.Get(name)
		if entry.run == nil {
			h.lru.Delete(name)
			h.log.Debug("idle agent evicted", "session_name", name)
		}
	}
}

// Shutdown 拒绝新提交，取消运行并等待全部 worker 退出，可重复调用。
func (h *SessionHub) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		h.cancel()
		go func() {
			h.wg.Wait()
			close(h.done)
		}()
	}
	h.mu.Unlock()
	select {
	case <-h.done:
		h.log.Info("session hub stopped")
		return nil
	case <-ctx.Done():
		h.log.Error("session hub shutdown failed", "error", ctx.Err())
		return ctx.Err()
	}
}
