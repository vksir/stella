package hub_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/hub"
	"github.com/vksir/stella/internal/storex"
)

type chatResponse struct {
	message agent.Message
	err     error
}

type chatCall struct {
	request  agent.StateReadOnly
	response chan chatResponse
}

func (c *chatCall) reply(text string) {
	c.response <- chatResponse{message: agent.Message{Content: text, Finish: agent.FinishStop}}
}

type controlledLLM struct {
	calls chan *chatCall
}

func newLLM() *controlledLLM { return &controlledLLM{calls: make(chan *chatCall, 8)} }

func (l *controlledLLM) Chat(ctx context.Context, request agent.StateReadOnly, onEvent agent.OnEvent) error {
	call := &chatCall{request: request, response: make(chan chatResponse, 1)}
	select {
	case l.calls <- call:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case response := <-call.response:
		if response.err != nil {
			return response.err
		}
		message := response.message
		onEvent(ctx, agent.Event{Type: agent.EventTextDelta, TextDelta: message.Content})
		for i, tool := range message.ToolCalls {
			onEvent(ctx, agent.Event{Type: agent.EventToolCallDelta, ToolCallDelta: agent.ToolCallDelta{
				Index: i, ID: tool.ID, Name: tool.Name, Arguments: tool.Args,
			}})
		}
		onEvent(ctx, agent.Event{Type: agent.EventFinish, Finish: message.Finish})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker")
		var zero T
		return zero
	}
}

func newHub(t *testing.T, llm agent.LLM, store agent.Store, capacity int, tools ...agent.Tool) *hub.SessionHub {
	t.Helper()
	h := hub.NewSessionHub(t.Context(), llm, store, capacity, agent.WithSystemPrompt("default prompt"), agent.WithTools(tools...))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return h
}

func submit(t *testing.T, h *hub.SessionHub, name, text string) hub.Submission {
	t.Helper()
	submission, err := h.Submit(t.Context(), name, agent.Message{Role: agent.RoleUser, Content: text}, hub.SteerIfBusy)
	if err != nil {
		t.Fatal(err)
	}
	return submission
}

func wait(t *testing.T, run *hub.Run) agent.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := run.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type gatedTool struct {
	started chan struct{}
	release chan struct{}
}

func (g *gatedTool) Name() string           { return "gate" }
func (g *gatedTool) Description() string    { return "wait for release" }
func (g *gatedTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (g *gatedTool) Execute(ctx context.Context, _ string) (string, error) {
	close(g.started)
	select {
	case <-g.release:
		return "tool result", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestSubmitSteersAtToolBoundary(t *testing.T) {
	llm, store := newLLM(), storex.NewMemory()
	tool := &gatedTool{started: make(chan struct{}), release: make(chan struct{})}
	h := newHub(t, llm, store, 1, tool)
	ctx, cancel := context.WithCancel(t.Context())
	first, err := h.Submit(ctx, "chat", agent.Message{Role: agent.RoleUser, Content: "first"}, hub.SteerIfBusy)
	cancel()
	if err != nil || !first.Started {
		t.Fatalf("first submission: %+v, %v", first, err)
	}
	call := receive(t, llm.calls)
	if call.request.SystemPrompt != "default prompt" || len(call.request.Messages) != 1 {
		t.Fatalf("initial request: %+v", call.request)
	}
	if _, err := h.Submit(t.Context(), "chat", agent.Message{Content: "rejected"}, hub.RejectIfBusy); !errors.Is(err, agent.ErrBusy) {
		t.Fatalf("busy submission: %v", err)
	}
	second := submit(t, h, "chat", "second")
	call.response <- chatResponse{message: agent.Message{
		ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "gate", Args: "{}"}}, Finish: agent.FinishToolCalls,
	}}
	receive(t, tool.started)
	third := submit(t, h, "chat", "third")
	if second.Started || third.Started || second.Run != first.Run || third.Run != first.Run {
		t.Fatal("steering must share the active run")
	}
	close(tool.release)
	call = receive(t, llm.calls)
	want := []agent.Message{
		{Role: agent.RoleUser, Content: "first"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "gate", Args: "{}"}}, Finish: agent.FinishToolCalls},
		{Role: agent.RoleTool, Content: "tool result", ToolCallID: "call-1"},
		{Role: agent.RoleUser, Content: "second"},
		{Role: agent.RoleUser, Content: "third"},
	}
	if !reflect.DeepEqual(call.request.Messages, want) {
		t.Fatalf("steer ordering: %+v", call.request.Messages)
	}
	call.reply("answer")
	result := wait(t, first.Run)
	other := wait(t, second.Run)
	if !reflect.DeepEqual(result, other) {
		t.Fatal("waiters must receive the same complete result")
	}
	result.Messages[1].ToolCalls[0].ID = "changed"
	saved, err := store.Find(t.Context(), other.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Messages, other.Messages) || !reflect.DeepEqual(wait(t, third.Run), other) {
		t.Fatal("result mutation leaked into history or another waiter")
	}
}

type gatedStore struct {
	agent.Store
	started chan struct{}
	release chan struct{}
	blocked atomic.Bool
}

func (s *gatedStore) AppendMessage(ctx context.Context, id uuid.UUID, messages ...agent.Message) error {
	if s.blocked.CompareAndSwap(false, true) {
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.Store.AppendMessage(ctx, id, messages...)
}

func TestSubmitDuringAndAfterCompletion(t *testing.T) {
	llm := newLLM()
	store := &gatedStore{Store: storex.NewMemory(), started: make(chan struct{}), release: make(chan struct{})}
	h := newHub(t, llm, store, 1)
	first := submit(t, h, "chat", "first")
	receive(t, llm.calls).reply("first answer")
	receive(t, store.started)
	second := submit(t, h, "chat", "second")
	if second.Started || first.Run != second.Run {
		t.Fatal("input during commit must join the current run")
	}
	close(store.release)
	call := receive(t, llm.calls)
	if len(call.request.Messages) != 3 || call.request.Messages[2].Content != "second" {
		t.Fatalf("pending input was not resumed: %+v", call.request.Messages)
	}
	call.reply("second answer")
	if result := wait(t, first.Run); len(result.Messages) != 4 {
		t.Fatalf("combined result: %+v", result)
	}

	third := submit(t, h, "chat", "third")
	if !third.Started || third.Run == first.Run {
		t.Fatal("input after completion must start a new run")
	}
	call = receive(t, llm.calls)
	if len(call.request.Messages) != 5 || call.request.Messages[4].Content != "third" {
		t.Fatalf("new run history: %+v", call.request.Messages)
	}
	call.reply("third answer")
	if result := wait(t, third.Run); len(result.Messages) != 2 {
		t.Fatalf("new run contains old output: %+v", result)
	}
}

func TestActiveSessionsArePinnedAndEvictedHistoryIsRestored(t *testing.T) {
	llm := newLLM()
	h := newHub(t, llm, storex.NewMemory(), 1)
	first := submit(t, h, "a", "a1")
	blocked := receive(t, llm.calls)
	second := submit(t, h, "b", "b1")
	receive(t, llm.calls).reply("b answer")
	secondResult := wait(t, second.Run)

	steer := submit(t, h, "a", "a2")
	if steer.Started || steer.Run != first.Run {
		t.Fatal("active session was evicted")
	}
	blocked.reply("a answer")
	receive(t, llm.calls).reply("a continued")
	wait(t, first.Run)

	restored := submit(t, h, "b", "b2")
	call := receive(t, llm.calls)
	if len(call.request.Messages) != 3 || call.request.Messages[0].Content != "b1" || call.request.Messages[1].Content != "b answer" {
		t.Fatalf("history was not restored: %+v", call.request.Messages)
	}
	call.reply("b continued")
	if result := wait(t, restored.Run); result.SessionID != secondResult.SessionID {
		t.Fatal("cache miss created a duplicate session")
	}
}

func TestConcurrentSubmissionsAreConsumedOnce(t *testing.T) {
	llm, store := newLLM(), storex.NewMemory()
	h := newHub(t, llm, store, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	go func() {
		for {
			select {
			case call := <-llm.calls:
				call.reply("answer")
			case <-ctx.Done():
				return
			}
		}
	}()
	const count = 100
	done := make(chan error, count)
	for i := range count {
		go func() {
			submission, err := h.Submit(ctx, "chat", agent.Message{Role: agent.RoleUser, Content: fmt.Sprintf("input-%d", i)}, hub.SteerIfBusy)
			if err == nil {
				_, err = submission.Run.Wait(ctx)
			}
			done <- err
		}()
	}
	for range count {
		if err := receive(t, done); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := store.FindByName(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, message := range saved.Messages {
		if message.Role == agent.RoleUser {
			seen[message.Content]++
		}
	}
	if len(seen) != count {
		t.Fatalf("accepted inputs missing from history: got=%d want=%d", len(seen), count)
	}
	for message, occurrences := range seen {
		if occurrences != 1 {
			t.Fatalf("input %q was consumed %d times", message, occurrences)
		}
	}
}

type failingStore struct {
	agent.Store
	operation string
	failure   error
	failed    atomic.Bool
}

func (s *failingStore) fail(operation string) error {
	if operation == s.operation && s.failed.CompareAndSwap(false, true) {
		return s.failure
	}
	return nil
}
func (s *failingStore) FindByName(ctx context.Context, name string) (*agent.Session, error) {
	if err := s.fail("load"); err != nil {
		return nil, err
	}
	return s.Store.FindByName(ctx, name)
}
func (s *failingStore) Save(ctx context.Context, session *agent.Session) error {
	if err := s.fail("create"); err != nil {
		return err
	}
	return s.Store.Save(ctx, session)
}
func (s *failingStore) AppendMessage(ctx context.Context, id uuid.UUID, messages ...agent.Message) error {
	if err := s.fail("append"); err != nil {
		return err
	}
	return s.Store.AppendMessage(ctx, id, messages...)
}

func TestFailuresCompleteReceiptsAndAllowNewRuns(t *testing.T) {
	for _, operation := range []string{"load", "create", "append", "llm"} {
		t.Run(operation, func(t *testing.T) {
			failure := fmt.Errorf("injected %s failure", operation)
			llm := newLLM()
			store := &failingStore{Store: storex.NewMemory(), operation: operation, failure: failure}
			h := newHub(t, llm, store, 1)
			first := submit(t, h, "chat", "first")
			var joined hub.Submission
			if operation == "append" || operation == "llm" {
				call := receive(t, llm.calls)
				joined = submit(t, h, "chat", "pending")
				if operation == "llm" {
					call.response <- chatResponse{err: failure}
				} else {
					call.reply("answer")
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if _, err := first.Run.Wait(ctx); !errors.Is(err, failure) {
				t.Fatalf("run failure: %v", err)
			}
			if joined.Run != nil {
				if _, err := joined.Run.Wait(ctx); !errors.Is(err, failure) {
					t.Fatalf("pending input must fail with its run: %v", err)
				}
			}
			retry := submit(t, h, "chat", "retry")
			if !retry.Started || retry.Run == first.Run {
				t.Fatal("failed run remained active")
			}
			call := receive(t, llm.calls)
			for _, message := range call.request.Messages {
				if message.Content == "pending" {
					t.Fatal("failed pending input was silently retried")
				}
			}
			call.reply("recovered")
			wait(t, retry.Run)
		})
	}
}

func TestWaitCancellationAndShutdown(t *testing.T) {
	llm := newLLM()
	h := newHub(t, llm, storex.NewMemory(), 1)
	first := submit(t, h, "chat", "first")
	receive(t, llm.calls)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := first.Run.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
	second := submit(t, h, "chat", "second")
	if second.Run != first.Run || second.Started {
		t.Fatal("cancelling a waiter cancelled the shared run")
	}
	shutdownCtx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	if err := h.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Run.Wait(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown must fail active and pending inputs: %v", err)
	}
	if _, err := h.Submit(t.Context(), "chat", agent.Message{}, hub.SteerIfBusy); !errors.Is(err, hub.ErrClosed) {
		t.Fatalf("closed hub accepted input: %v", err)
	}
}
