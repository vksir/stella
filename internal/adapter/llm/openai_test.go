package llm_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	llmpkg "github.com/vksir/stella/internal/adapter/llm"
	"github.com/vksir/stella/internal/core/agent"
)

func TestChatRequestOverridesDoNotChangeClientDefaults(t *testing.T) {
	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	llm := llmpkg.NewOpenAI("test-key", llmpkg.WithBaseURL(server.URL), llmpkg.WithModel("default-model"),
		llmpkg.WithTemperature(0.8), llmpkg.WithReasoningEffort("low"))
	temperature := 0.0
	request := agent.ChatRequest{
		Model: "session-model", Temperature: &temperature, ReasoningEffort: "high", SystemPrompt: "state prompt",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: agent.TextContent("input")}},
		Tools:    []agent.ToolRequest{{Name: "echo", Description: "echo input", Schema: map[string]any{"type": "object"}}},
	}
	if err := llm.Chat(t.Context(), request, nil); err != nil {
		t.Fatal(err)
	}
	got := <-requests
	if got["model"] != "session-model" || got["temperature"] != 0.0 || got["reasoning_effort"] != "high" {
		t.Fatalf("request overrides: %+v", got)
	}
	messages := got["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["content"] != "state prompt" || messages[1].(map[string]any)["content"] != "input" {
		t.Fatalf("request messages: %+v", messages)
	}
	tools := got["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["function"].(map[string]any)["name"] != "echo" {
		t.Fatalf("request tools: %+v", tools)
	}
	if err := llm.Chat(t.Context(), agent.ChatRequest{Messages: request.Messages}, nil); err != nil {
		t.Fatal(err)
	}
	got = <-requests
	if got["model"] != "default-model" || got["temperature"] != 0.8 || got["reasoning_effort"] != "low" || got["tools"] != nil {
		t.Fatalf("client defaults were changed: %+v", got)
	}
}
