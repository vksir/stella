package agent

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrBusy       = errors.New("agent is busy")
	ErrNotRunning = errors.New("agent is not running")
)

// Result 包含本次同步执行已提交的消息，与会话历史独立。
type Result struct {
	SessionID uuid.UUID
	Messages  []Message
}
