package llmx

import (
	"fmt"

	"github.com/vksir/stella/internal/agent"
)

// providerOpts 定义各 provider 的连接选项，baseURL、reasoningField、usageParser 均显式配置。
var providerOpts = map[string][]Option{
	"openai": {
		WithBaseURL("https://api.openai.com/v1"),
		WithReasoningField(""),
		WithUsageParser(parseOpenAIUsage),
	},
	"deepseek": {
		WithBaseURL("https://api.deepseek.com/v1"),
		WithReasoningField("reasoning_content"),
		WithUsageParser(parseDeepSeekUsage),
	},
	"openrouter": {
		WithBaseURL("https://openrouter.ai/api/v1"),
		WithReasoningField("reasoning"),
		WithUsageParser(parseOpenRouterUsage),
	},
}

// newClient 按 provider 创建客户端，model 为 provider 原生模型名，不受 extra 覆盖。
func newClient(apiKey, provider, model string, extra ...Option) (agent.LLM, error) {
	base, ok := providerOpts[provider]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api key for provider %q is required", provider)
	}
	opts := append([]Option(nil), base...)
	opts = append(opts, extra...)
	opts = append(opts, WithModel(model))
	return NewOpenAI(apiKey, opts...), nil
}
