// internal/llm/openai.go
package llm

import (
	"context"
	"encoding/json"
	"stella/entity"
	"stella/pkg/cfg"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/vksir/vkiss-lib/pkg/log"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
)

type openAI struct {
	client *openai.Client
	model  string
	logger *log.Logger
}

func newOpenAILLM(config cfg.ConfigModel, logger *log.Logger) (LLM, error) {
	c := openai.NewClient(
		option.WithBaseURL(config.BaseUrl),
		option.WithAPIKey(config.ApiKey),
	)
	return &openAI{
		client: &c,
		model:  config.Name,
		logger: logger.With("tag", "openai_llm"),
	}, nil
}

func (m *openAI) Start(ctx context.Context) error {
	return nil
}

func (m *openAI) Chat(ctx context.Context, chains []entity.Chain, tools []mcp.Tool) ([]entity.Chain, error) {
	messages, err := m.convertMessages(chains)
	if err != nil {
		return nil, errutil.Wrap(err)
	}
	cvtTools, err := m.convertTool(tools)
	if err != nil {
		return nil, errutil.Wrap(err)
	}

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    m.model,
		Tools:    cvtTools,
	}
	resp, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, errutil.Wrap(err)
	}
	return m.convertResponse(resp)
}

func (m *openAI) Embedding(ctx context.Context, text string) ([]float32, error) {
	resp, err := m.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: m.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
	})
	if err != nil {
		return nil, errutil.Wrap(err)
	}
	if len(resp.Data) == 0 {
		return nil, errutil.WrapF("no embedding returned from model %s", m.model)
	}
	// Convert float64 to float32
	vec := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}

func (m *openAI) convertMessages(chains []entity.Chain) ([]openai.ChatCompletionMessageParamUnion, error) {
	var messages []openai.ChatCompletionMessageParamUnion
	for _, chain := range chains {
		switch chain.Role {
		case entity.ChainRoleSystem:
			messages = append(messages, openai.SystemMessage(chain.Text))
		case entity.ChainRoleUser:
			messages = append(messages, openai.UserMessage(chain.Text))
		case entity.ChainRoleAgent:
			if len(chain.ToolCall) > 0 {
				// Build assistant message with tool calls
				var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
				for _, tc := range chain.ToolCall {
					argsJSON, _ := json.Marshal(tc.Args)
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Func,
								Arguments: string(argsJSON),
							},
						},
					})
				}
				assistant := &openai.ChatCompletionAssistantMessageParam{
					ToolCalls: toolCalls,
				}
				if chain.Text != "" {
					assistant.Content.OfString = openai.String(chain.Text)
				}
				messages = append(messages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: assistant,
				})
			} else {
				messages = append(messages, openai.AssistantMessage(chain.Text))
			}
		case entity.ChainRoleToolResult:
			messages = append(messages, openai.ToolMessage(chain.ToolID, chain.Text))
		}
	}
	return messages, nil
}

func (m *openAI) convertTool(tools []mcp.Tool) ([]openai.ChatCompletionToolUnionParam, error) {
	var result []openai.ChatCompletionToolUnionParam
	for _, tool := range tools {
		var parameters map[string]any
		if tool.InputSchema != nil {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				return nil, errutil.WrapErrType(tool.InputSchema)
			}
			parameters = schema
		}
		result = append(result, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
			Parameters:  parameters,
		}))
	}
	return result, nil
}

func (m *openAI) convertResponse(resp *openai.ChatCompletion) ([]entity.Chain, error) {
	var result []entity.Chain
	for _, choice := range resp.Choices {
		c := entity.Chain{
			Role: entity.ChainRoleAgent,
			Type: entity.ChainTypeText,
			Text: choice.Message.Content,
		}

		if len(choice.Message.ToolCalls) > 0 {
			c.Type = entity.ChainTypeToolCall
			for _, call := range choice.Message.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
					return nil, errutil.Wrap(err)
				}
				c.ToolCall = append(c.ToolCall, entity.ToolCall{
					ID:   call.ID,
					Func: call.Function.Name,
					Args: args,
				})
			}
		}
		result = append(result, c)
	}
	return result, nil
}
