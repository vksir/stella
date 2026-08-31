package toolx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/vksir/stella/internal/agent"
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

func (t *writeTool) Execute(ctx context.Context, args string) (out string, err error) {
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
		return "", err
	}
	absolutePath, err := resolveToolPath(in.Path)
	if err != nil {
		return "", err
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
		slog.Debug("write tool executed", "path", in.Path, "bytes", utf16Length(in.Content))
		// 字节数按 content 的 UTF-16 code unit 数量计算。
		out = fmt.Sprintf("Successfully wrote %d bytes to %s", utf16Length(in.Content), in.Path)
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

// utf16Length 返回字符串的 UTF-16 code unit 数量，与 JS 的 string.length 一致。
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}
