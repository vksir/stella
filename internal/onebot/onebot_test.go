package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/hub"
	"github.com/vksir/stella/internal/storex"
)

type replyLLM struct {
	calls   chan string
	release chan struct{}
	err     error
}

func (l *replyLLM) Chat(ctx context.Context, request agent.StateReadOnly, emit agent.OnEvent) error {
	text := request.Messages[len(request.Messages)-1].Content
	select {
	case l.calls <- text:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-l.release:
		if l.err != nil {
			return l.err
		}
		emit(ctx, agent.Event{Type: agent.EventTextDelta, TextDelta: "answer:" + text})
		emit(ctx, agent.Event{Type: agent.EventFinish, Finish: agent.FinishStop})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func next[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reply")
		var zero T
		return zero
	}
}

func TestRepliesMergeSteerAndKeepSendOrder(t *testing.T) {
	llm := &replyLLM{calls: make(chan string, 4), release: make(chan struct{}, 4)}
	h := hub.NewSessionHub(t.Context(), llm, storex.NewMemory(), 1)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	ob := New(h)
	sent := make(chan string, 4)
	releaseSend := make(chan struct{})
	var sendCount, staleCount atomic.Int32
	var firstReleased atomic.Bool
	send := func(ctx context.Context, payload []byte) error {
		var request struct {
			Params struct {
				Message string `json:"message"`
			} `json:"params"`
		}
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		count := sendCount.Add(1)
		if count > 1 && !firstReleased.Load() {
			t.Error("later run overtook an unfinished reply")
		}
		sent <- request.Params.Message
		if count == 1 {
			select {
			case <-releaseSend:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	handle := func(ctx context.Context, text string, sender SendFunc) {
		t.Helper()
		payload := []byte(fmt.Sprintf(`{"post_type":"message","message_type":"private","user_id":100,"raw_message":%q}`, text))
		if err := ob.HandleMessage(ctx, payload, sender); err != nil {
			t.Fatal(err)
		}
	}
	oldCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	handle(oldCtx, "first", func(context.Context, []byte) error {
		staleCount.Add(1)
		return nil
	})
	if got := next(t, llm.calls); got != "first" {
		t.Fatalf("first input: %q", got)
	}
	cancel()
	handle(t.Context(), "second", send)
	llm.release <- struct{}{}
	if got := next(t, llm.calls); got != "second" {
		t.Fatalf("steer input: %q", got)
	}
	llm.release <- struct{}{}
	if got := next(t, sent); got != "answer:first\n\nanswer:second" {
		t.Fatalf("combined reply: %q", got)
	}

	handle(t.Context(), "third", send)
	if got := next(t, llm.calls); got != "third" {
		t.Fatalf("next run input: %q", got)
	}
	llm.release <- struct{}{}
	firstReleased.Store(true)
	close(releaseSend)
	if got := next(t, sent); got != "answer:third" {
		t.Fatalf("next reply: %q", got)
	}
	if staleCount.Load() != 0 || sendCount.Load() != 2 {
		t.Fatalf("stale or duplicate replies: stale=%d, sent=%d", staleCount.Load(), sendCount.Load())
	}
}

func TestReplyContinuesAfterRunAndSendFailures(t *testing.T) {
	llm := &replyLLM{calls: make(chan string, 2), release: make(chan struct{}, 2), err: errors.New("injected model failure")}
	h := hub.NewSessionHub(t.Context(), llm, storex.NewMemory(), 1)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	ob := New(h)
	sent := make(chan string, 2)
	releaseSend := make(chan struct{})
	var sendCount atomic.Int32
	send := func(ctx context.Context, payload []byte) error {
		var request struct {
			Params sendMsgParams `json:"params"`
		}
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		count := sendCount.Add(1)
		sent <- request.Params.Message
		if count == 1 {
			select {
			case <-releaseSend:
				return errors.New("injected send failure")
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	for _, text := range []string{"first", "second"} {
		payload := []byte(fmt.Sprintf(`{"post_type":"message","message_type":"private","user_id":100,"raw_message":%q}`, text))
		if err := ob.HandleMessage(t.Context(), payload, send); err != nil {
			t.Fatal(err)
		}
		if got := next(t, llm.calls); got != text {
			t.Fatalf("input: %q, want %q", got, text)
		}
		llm.release <- struct{}{}
		if text == "second" {
			close(releaseSend)
		}
		if got := next(t, sent); got != errorMessage {
			t.Fatalf("error reply: %q", got)
		}
	}
	if sendCount.Load() != 2 {
		t.Fatalf("unexpected retries: sent=%d", sendCount.Load())
	}
}
