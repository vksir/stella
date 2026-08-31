package llmx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/config"
	"github.com/vksir/stella/internal/llmx"
)

var _ agent.LLM = (*llmx.LLM)(nil)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func modelConfig() config.ModelConfig {
	return config.ModelConfig{
		DefaultProvider: "deepseek", DefaultModel: "shared", Temperature: new(0.6), ReasoningEffort: "low",
		Provider: map[string]config.ProviderConfig{
			"deepseek": {
				APIKey: "deepseek-test-key",
				Models: []config.ModelEntry{{Model: "shared", Temperature: new(0.8), ReasoningEffort: "medium"}},
			},
			"openrouter": {
				APIKey: "openrouter-test-key",
				Models: []config.ModelEntry{
					{Model: "shared", Temperature: new(0.2), ReasoningEffort: "high"},
					{Model: "anthropic/claude"},
				},
			},
		},
	}
}

func streamResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("data: " +
			`{"choices":[{"delta":{"content":"ok","reasoning":"openrouter","reasoning_content":"deepseek"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_cache_hit_tokens":4,"prompt_tokens_details":{"cached_tokens":6}}}` +
			"\n\ndata: [DONE]\n\n")),
	}
}

func TestLLMRefreshesOnlyWhenModelChanges(t *testing.T) {
	var builds atomic.Int32
	var host, authorization string
	var body struct {
		Model           string   `json:"model"`
		Temperature     *float64 `json:"temperature"`
		ReasoningEffort string   `json:"reasoning_effort"`
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		host, authorization = r.URL.Host, r.Header.Get("Authorization")
		body.Temperature = nil
		body.ReasoningEffort = ""
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		return streamResponse(), nil
	})}
	llm, err := llmx.NewLLM(modelConfig(), llmx.WithHTTPClient(client), llmx.WithModel("ignored"), func(*llmx.OpenAI) { builds.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		state       agent.StateReadOnly
		provider    string
		model       string
		temperature float64
		effort      string
		builds      int32
	}{
		{"default", agent.StateReadOnly{}, "deepseek", "shared", 0.8, "medium", 1},
		{"overrides", agent.StateReadOnly{Model: "deepseek/shared", Temperature: new(0.0), ReasoningEffort: "high"}, "deepseek", "shared", 0, "high", 1},
		{"reset_overrides", agent.StateReadOnly{Model: "deepseek/shared"}, "deepseek", "shared", 0.8, "medium", 1},
		{"switch_provider", agent.StateReadOnly{Model: "openrouter/shared"}, "openrouter", "shared", 0.2, "high", 2},
		{"nested_model", agent.StateReadOnly{Model: "openrouter/anthropic/claude"}, "openrouter", "anthropic/claude", 0.6, "low", 3},
		{"return_to_default", agent.StateReadOnly{}, "deepseek", "shared", 0.8, "medium", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reasoning string
			var finish agent.Event
			if err := llm.Chat(t.Context(), tt.state, func(_ context.Context, event agent.Event) {
				reasoning += event.ReasoningDelta
				if event.Type == agent.EventFinish {
					finish = event
				}
			}); err != nil {
				t.Fatal(err)
			}
			wantHost, cached := "api.deepseek.com", 4
			if tt.provider == "openrouter" {
				wantHost, cached = "openrouter.ai", 6
			}
			if host != wantHost || authorization != "Bearer "+tt.provider+"-test-key" {
				t.Fatalf("wrong provider connection: host=%q", host)
			}
			if body.Model != tt.model || body.Temperature == nil || *body.Temperature != tt.temperature || body.ReasoningEffort != tt.effort {
				t.Fatalf("wrong model parameters: %+v", body)
			}
			if reasoning != tt.provider || finish.Finish != agent.FinishStop || finish.Usage.CacheReadTokens != cached {
				t.Fatalf("wrong provider stream parsing: reasoning=%q finish=%+v", reasoning, finish)
			}
			if got := builds.Load(); got != tt.builds {
				t.Fatalf("client builds = %d, want %d", got, tt.builds)
			}
		})
	}
}

func TestLLMInvalidModelKeepsCachedClient(t *testing.T) {
	cfg := modelConfig()
	cfg.Provider["openai"] = config.ProviderConfig{Models: []config.ModelEntry{{Model: "shared"}}}
	cfg.Provider["unsupported"] = config.ProviderConfig{APIKey: "test-key", Models: []config.ModelEntry{{Model: "shared"}}}
	var builds, calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.Body.Close()
		calls.Add(1)
		return streamResponse(), nil
	})}
	llm, err := llmx.NewLLM(cfg, llmx.WithHTTPClient(client), func(*llmx.OpenAI) { builds.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"shared", "/shared", "deepseek/", "missing/shared", "deepseek/missing", "unsupported/shared", "openai/shared"} {
		t.Run(model, func(t *testing.T) {
			before := calls.Load()
			if err := llm.Chat(t.Context(), agent.StateReadOnly{Model: model}, nil); err == nil {
				t.Fatal("invalid model succeeded")
			}
			if calls.Load() != before {
				t.Fatal("invalid model reached the provider")
			}
			if err := llm.Chat(t.Context(), agent.StateReadOnly{}, nil); err != nil {
				t.Fatal(err)
			}
			if builds.Load() != 1 {
				t.Fatal("invalid model replaced the cached client")
			}
		})
	}
	if _, err := llmx.NewLLM(config.ModelConfig{}); err == nil {
		t.Fatal("missing default model succeeded")
	}
}

func TestLLMConcurrentModelSwitchesDoNotBlockRequests(t *testing.T) {
	const count = 16
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, count)
	release := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		var body struct {
			Model    string                     `json:"model"`
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		provider, model, _ := strings.Cut(body.Messages[0].Content, "/")
		wantHost := "api.deepseek.com"
		if provider == "openrouter" {
			wantHost = "openrouter.ai"
		}
		if body.Model != model || r.URL.Host != wantHost || r.Header.Get("Authorization") != "Bearer "+provider+"-test-key" {
			return nil, fmt.Errorf("concurrent request used another model or provider")
		}
		return streamResponse(), nil
	})}
	llm, err := llmx.NewLLM(modelConfig(), llmx.WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, count)
	for i := range count {
		go func() {
			<-start
			model := "deepseek/shared"
			if i%2 != 0 {
				model = "openrouter/anthropic/claude"
			}
			var reasoning string
			err := llm.Chat(ctx, agent.StateReadOnly{Model: model, Messages: []agent.Message{{Role: agent.RoleUser, Content: model}}}, func(_ context.Context, event agent.Event) {
				reasoning += event.ReasoningDelta
			})
			provider, _, _ := strings.Cut(model, "/")
			if err == nil && reasoning != provider {
				err = fmt.Errorf("concurrent stream used another provider parser: %q", reasoning)
			}
			results <- err
		}()
	}
	close(start)
	for range count {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("model cache lock blocked concurrent requests")
		}
	}
	close(release)
	for range count {
		select {
		case err := <-results:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
