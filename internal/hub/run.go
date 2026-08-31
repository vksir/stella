package hub

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/vksir/stella/internal/agent"
)

// BusyPolicy 决定忙碌会话如何接收新输入。
type BusyPolicy int

const (
	RejectIfBusy BusyPolicy = iota
	SteerIfBusy
)

// Run 是可重复等待的运行回执，可覆盖多次 Agent.Run 调用。
type Run struct {
	id     uuid.UUID
	done   chan struct{}
	result agent.Result
	err    error
}

func (r *Run) ID() uuid.UUID { return r.id }

// Wait 返回独立结果副本；取消等待不会取消共享运行。
func (r *Run) Wait(ctx context.Context) (agent.Result, error) {
	select {
	case <-ctx.Done():
		slog.Debug("run wait cancelled", "run_id", r.id, "error", ctx.Err())
		return agent.Result{}, ctx.Err()
	case <-r.done:
		slog.Debug("run wait completed", "run_id", r.id, "error", r.err)
		return agent.Result{SessionID: r.result.SessionID, Messages: agent.MessagesClone(r.result.Messages)}, r.err
	}
}

// Submission 标识消息所属运行，Started 仅对创建该运行的提交为 true。
type Submission struct {
	Run     *Run
	Started bool
}
