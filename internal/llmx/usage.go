package llmx

import (
	"encoding/json"

	"github.com/vksir/stella/internal/agent"
)

// 各 provider 的 usage 解析实现，经 WithUsageParser 注入。
// 字段差异较大：缓存命中 token 的上报位置各不相同。

// UsageParser 将流末尾的原始 usage JSON 解析为 agent.Usage。
type UsageParser func(raw []byte) (agent.Usage, error)

// parseOpenAIUsage 解析 OpenAI 标准用量，缓存命中位于 prompt_tokens_details.cached_tokens。
func parseOpenAIUsage(raw []byte) (agent.Usage, error) {
	var u struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return agent.Usage{}, err
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	return agent.Usage{
		InputTokens:     u.PromptTokens,
		OutputTokens:    u.CompletionTokens,
		TotalTokens:     u.TotalTokens,
		CacheReadTokens: cached,
	}, nil
}

// parseOpenRouterUsage 解析 OpenRouter 用量，归一化为 OpenAI 风格并附带 cached_tokens。
func parseOpenRouterUsage(raw []byte) (agent.Usage, error) {
	return parseOpenAIUsage(raw)
}

// parseDeepSeekUsage 解析 DeepSeek 用量，缓存命中位于 prompt_cache_hit_tokens。
func parseDeepSeekUsage(raw []byte) (agent.Usage, error) {
	var u struct {
		PromptTokens         int `json:"prompt_tokens"`
		CompletionTokens     int `json:"completion_tokens"`
		TotalTokens          int `json:"total_tokens"`
		PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return agent.Usage{}, err
	}
	return agent.Usage{
		InputTokens:     u.PromptTokens,
		OutputTokens:    u.CompletionTokens,
		TotalTokens:     u.TotalTokens,
		CacheReadTokens: u.PromptCacheHitTokens,
	}, nil
}
