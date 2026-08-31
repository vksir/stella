package llmx

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/config"
)

// LLM 按 provider/model 选择客户端，只缓存当前模型，支持并发调用。
type LLM struct {
	config config.ModelConfig
	extra  []Option
	log    *slog.Logger
	mu     sync.Mutex
	model  string
	client agent.LLM
}

// NewLLM 创建默认模型客户端；cfg 在对象生命周期内只读，extra 应用于每个新客户端。
func NewLLM(cfg config.ModelConfig, extra ...Option) (*LLM, error) {
	l := &LLM{config: cfg, extra: append([]Option(nil), extra...), log: slog.Default()}
	if _, err := l.clientFor(cfg.DefaultProvider + "/" + cfg.DefaultModel); err != nil {
		l.log.Error("llm creation failed", "error", err)
		return nil, err
	}
	l.log.Debug("llm created", "model", l.model)
	return l, nil
}

// Chat 使用本次状态的模型与生成参数，空模型继承配置中的默认模型。
func (l *LLM) Chat(ctx context.Context, state agent.StateReadOnly, onEvent agent.OnEvent) error {
	if err := ctx.Err(); err != nil {
		l.log.Debug("llm chat cancelled", "error", err)
		return err
	}
	model := state.Model
	if model == "" {
		model = l.config.DefaultProvider + "/" + l.config.DefaultModel
	}
	client, err := l.clientFor(model)
	if err != nil {
		l.log.Error("llm client selection failed", "model", model, "error", err)
		return err
	}
	_, state.Model, _ = strings.Cut(model, "/")
	return client.Chat(ctx, state, onEvent)
}

// clientFor 的锁只保护客户端缓存，不覆盖流式请求。
func (l *LLM) clientFor(model string) (agent.LLM, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.client != nil && l.model == model {
		l.log.Debug("llm client reused", "model", model)
		return l.client, nil
	}

	provider, name, ok := strings.Cut(model, "/")
	if !ok || provider == "" || name == "" {
		return nil, fmt.Errorf("model %q must use provider/model format", model)
	}
	p, ok := l.config.Provider[provider]
	if !ok {
		return nil, fmt.Errorf("provider %q is not configured", provider)
	}
	var entry *config.ModelEntry
	for i := range p.Models {
		if p.Models[i].Model == name {
			entry = &p.Models[i]
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("model %q is not configured", model)
	}

	temperature := l.config.Temperature
	if entry.Temperature != nil {
		temperature = entry.Temperature
	}
	effort := l.config.ReasoningEffort
	if entry.ReasoningEffort != "" {
		effort = entry.ReasoningEffort
	}
	opts := make([]Option, 0, len(l.extra)+2)
	if temperature != nil {
		opts = append(opts, WithTemperature(*temperature))
	}
	if effort != "" {
		opts = append(opts, WithReasoningEffort(effort))
	}
	opts = append(opts, l.extra...)
	client, err := newClient(p.APIKey, provider, name, opts...)
	if err != nil {
		return nil, err
	}
	l.log.Info("llm client refreshed", "previous_model", l.model, "model", model)
	l.model, l.client = model, client
	return client, nil
}
