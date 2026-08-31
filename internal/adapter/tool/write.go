package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/vksir/stella/internal/core/agent"
)

const writeDescription = "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories."

var writeSchema = objectSchema(map[string]any{
	"path":    strProp("Path to the file to write (relative or absolute)"),
	"content": strProp("Content to write to the file"),
}, "path", "content")

// writeTool 实现 write 工具。
type writeTool struct{}

func (t *writeTool) Name() string { return "write" }

func (t *writeTool) Description() string { return writeDescription }

func (t *writeTool) Schema() map[string]any { return writeSchema }

func (t *writeTool) Execute(ctx context.Context, args string) (out agent.Content, err error) {
	defer func() {
		if err != nil {
			slog.Error("tool failed", "tool", "write", "error", err)
		}
	}()
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return nil, err
	}
	absolutePath, err := resolveToolPath(in.Path)
	if err != nil {
		return nil, err
	}

	err = withMutationQueue(absolutePath, func() error {
		if ctx.Err() != nil {
			return errors.New("Operation aborted")
		}
		if err := writeFile(absolutePath, in.Content); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return errors.New("Operation aborted")
		}
		slog.Debug("write tool executed", "path", in.Path, "bytes", len(in.Content))
		out = agent.TextContent(fmt.Sprintf("Successfully wrote to %s", in.Path))
		return nil
	})
	return out, err
}

// NewWrite 创建 write 工具。
func NewWrite() agent.Tool {
	return &writeTool{}
}

// writeFile 创建文件，必要时自动创建父目录。
func writeFile(absPath, content string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}
