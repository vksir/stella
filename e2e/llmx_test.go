// Package e2e 包含依赖真实 API 的端到端用例，模型与密钥从配置文件读取，未配置时自动跳过。
package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/config"
	"github.com/vksir/stella/internal/llmx"
)

// chatResult 汇总一次流式调用的关键事件与产物。
type chatResult struct {
	content   strings.Builder
	reasoning strings.Builder
	lastEvent agent.EventType
	finish    agent.Event
	hasFinish bool
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	// go test 的工作目录为包目录，仓库根目录即上级
	cfg, err := config.Load("../stella.toml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// providerModel 返回指定 provider 的模型，优先选择默认模型，否则取首个模型。
func providerModel(cfg *config.Config, provider string) (config.ModelEntry, bool) {
	p, found := cfg.Model.Provider[provider]
	if !found || len(p.Models) == 0 {
		return config.ModelEntry{}, false
	}
	if cfg.Model.DefaultProvider == provider {
		for _, m := range p.Models {
			if m.Model == cfg.Model.DefaultModel {
				return m, true
			}
		}
	}
	return p.Models[0], true
}

// cacheSystemPrompt 足够长的系统提示词，跨轮相同前缀可触发 provider 的上下文缓存
var cacheSystemPrompt = strings.Repeat(
	"You are a meticulous senior software engineer specializing in distributed systems, database internals, and compiler optimization. ", 4)

func runChat(t *testing.T, llm agent.LLM, model, prompt string) *chatResult {
	t.Helper()

	res := &chatResult{}
	request := agent.StateReadOnly{
		Model:        model,
		SystemPrompt: cacheSystemPrompt,
		Messages:     []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	err := llm.Chat(ctx, request, func(_ context.Context, event agent.Event) {
		switch event.Type {
		case agent.EventTextDelta:
			res.content.WriteString(event.TextDelta)
		case agent.EventReasoningDelta:
			res.reasoning.WriteString(event.ReasoningDelta)
		case agent.EventFinish:
			res.finish = event
			res.hasFinish = true
		}
		res.lastEvent = event.Type
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	return res
}

func assertChatResult(t *testing.T, res *chatResult, requireCacheHit bool) {
	t.Helper()

	if res.content.Len() == 0 {
		t.Error("empty text content")
	}
	if !res.hasFinish {
		t.Fatal("missing finish event")
	}
	if res.lastEvent != agent.EventFinish {
		t.Errorf("finish should be the last event, got %s", res.lastEvent)
	}
	if res.finish.Finish != agent.FinishStop {
		t.Errorf("finish reason = %s, want stop", res.finish.Finish)
	}
	if res.finish.Usage.InputTokens <= 0 || res.finish.Usage.OutputTokens <= 0 {
		t.Errorf("unexpected usage: %+v", res.finish.Usage)
	}
	if cache := res.finish.Usage.CacheReadTokens; requireCacheHit && cache <= 0 {
		t.Errorf("expected cache hit, usage: %+v", res.finish.Usage)
	} else {
		t.Logf("cache read tokens: %d", cache)
	}
	t.Logf("content: %s", res.content.String())
	if res.reasoning.Len() > 0 {
		t.Logf("reasoning: %d chars", res.reasoning.Len())
	}
}

func TestOpenRouterChat(t *testing.T) {
	cfg := loadConfig(t)
	entry, ok := providerModel(cfg, "openrouter")
	if !ok {
		t.Skip("no openrouter model configured")
	}
	model := "openrouter/" + entry.Model
	t.Logf("model: %s", model)

	llm, err := llmx.NewLLM(cfg.Model)
	if err != nil {
		t.Fatalf("new llm: %v", err)
	}
	// 两轮相同请求，校验 usage 及 cached_tokens 字段可解析；上游是否命中缓存取决于模型
	prompt := "What is 2+2? Reply with the number only."
	res := runChat(t, llm, model, prompt)
	assertChatResult(t, res, false)
	res = runChat(t, llm, model, prompt)
	assertChatResult(t, res, false)
}

func TestDeepSeekChat(t *testing.T) {
	cfg := loadConfig(t)
	entry, ok := providerModel(cfg, "deepseek")
	if !ok {
		t.Skip("no deepseek model configured")
	}
	model := "deepseek/" + entry.Model
	t.Logf("model: %s", model)

	llm, err := llmx.NewLLM(cfg.Model)
	if err != nil {
		t.Fatalf("new llm: %v", err)
	}
	// 两轮相同请求，第二轮应命中上下文缓存，校验 prompt_cache_hit_tokens 解析
	prompt := "What is 2+2? Reply with the number only."
	res := runChat(t, llm, model, prompt)
	assertChatResult(t, res, false)
	res = runChat(t, llm, model, prompt)
	assertChatResult(t, res, true)
}
