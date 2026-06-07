// internal/llm/llm.go
package llm

import (
	"context"
	"stella/entity"
	"stella/pkg/cfg"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vksir/vkiss-lib/pkg/log"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
)

type LLM interface {
	Start(ctx context.Context) error
	Chat(ctx context.Context, chains []entity.Chain, tools []mcp.Tool) ([]entity.Chain, error)
	Embedding(ctx context.Context, text string) ([]float32, error)
}

func New(config cfg.ConfigModel, logger *log.Logger) (LLM, error) {
	switch config.Type {
	case cfg.ModelTypeOpenAI:
		return newOpenAILLM(config, logger)
	default:
		return nil, errutil.WrapErrType(config.Type)
	}
}
