package toolx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/vksir/stella/internal/agent"
)

const editDescription = "Edit a single file using exact text replacement. Every edits[].oldText must match a unique, non-overlapping region of the original file. If two changes affect the same block or nearby lines, merge them into one edit instead of emitting overlapping edits. Do not include large unchanged regions just to connect distant changes."

var editItemSchema = objectSchema(map[string]any{
	"oldText": strProp("Exact text for one targeted replacement. It must be unique in the original file and must not overlap with any other edits[].oldText in the same call."),
	"newText": strProp("Replacement text for this targeted edit."),
}, "oldText", "newText")

var editSchema = objectSchema(map[string]any{
	"path": strProp("Path to the file to edit (relative or absolute)"),
	"edits": map[string]any{
		"type":        "array",
		"description": "One or more targeted replacements. Each edit is matched against the original file, not incrementally. Do not include overlapping or nested edits. If two changes touch the same block or nearby lines, merge them into one edit instead.",
		"items":       editItemSchema,
	},
}, "path", "edits")

// editArgs 是解析并归一化后的 edit 工具参数。
type editArgs struct {
	path  string
	edits []Edit
}

// isSingleEdit 判断值是否为 {oldText, newText} 均为字符串的单条编辑。
func isSingleEdit(value any) bool {
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, ok1 := m["oldText"].(string)
	_, ok2 := m["newText"].(string)
	return ok1 && ok2
}

func toEditItem(value any) (Edit, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return Edit{}, false
	}
	oldText, ok1 := m["oldText"].(string)
	newText, ok2 := m["newText"].(string)
	if !ok1 || !ok2 {
		return Edit{}, false
	}
	return Edit{oldText: oldText, newText: newText}, true
}

func appendEditItem(edits *[]Edit, value any) {
	if item, ok := toEditItem(value); ok {
		*edits = append(*edits, item)
	}
}

// parseEditArgs 解析参数：
// edits 可为 JSON 字符串或单条编辑对象，顶层 oldText/newText 合并进 edits。
func parseEditArgs(raw string) (*editArgs, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	out := &editArgs{}
	if path, ok := m["path"].(string); ok {
		out.path = path
	}

	var edits []Edit
	switch e := m["edits"].(type) {
	case string: // edits 为字符串时按 JSON 解析
		var parsed any
		if err := json.Unmarshal([]byte(e), &parsed); err == nil {
			switch v := parsed.(type) {
			case []any:
				for _, item := range v {
					appendEditItem(&edits, item)
				}
			case map[string]any:
				if isSingleEdit(v) {
					appendEditItem(&edits, v)
				}
			}
		}
	case []any:
		for _, item := range e {
			appendEditItem(&edits, item)
		}
	case map[string]any:
		if isSingleEdit(e) {
			appendEditItem(&edits, e)
		}
	}

	// 顶层 oldText/newText 作为追加的编辑（旧模型输出兼容）。
	if oldText, ok := m["oldText"].(string); ok {
		if newText, ok2 := m["newText"].(string); ok2 {
			edits = append(edits, Edit{oldText: oldText, newText: newText})
		}
	}

	if len(edits) == 0 {
		return nil, errors.New("Edit tool input is invalid. edits must contain at least one replacement.")
	}
	out.edits = edits
	return out, nil
}

// fileErrorCode 将文件系统错误归一化为错误码。
func fileErrorCode(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "not_found"
	case errors.Is(err, fs.ErrPermission):
		return "permission_denied"
	default:
		return "unknown"
	}
}

// editTool 实现 edit 工具。
type editTool struct{}

func (t *editTool) Name() string { return "edit" }

func (t *editTool) Description() string { return editDescription }

func (t *editTool) Schema() map[string]any { return editSchema }

func (t *editTool) Execute(ctx context.Context, args string) (out string, err error) {
	defer func() {
		if err != nil {
			slog.Error("tool failed", "tool", "edit", "error", err)
		}
	}()
	in, err := parseEditArgs(args)
	if err != nil {
		return "", err
	}
	absolutePath, err := resolveToolPath(in.path)
	if err != nil {
		return "", err
	}

	err = withMutationQueue(absolutePath, func() error {
		if ctx.Err() != nil {
			return errors.New("Operation aborted")
		}
		info, err := os.Lstat(absolutePath)
		if err != nil {
			return fmt.Errorf("Could not edit file: %s. Error code: %s.", in.path, fileErrorCode(err))
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("Could not edit file: %s. Path is not a file.", in.path)
		}

		content, err := os.ReadFile(absolutePath)
		if err != nil {
			return fmt.Errorf("Could not edit file: %s. Error code: %s.", in.path, fileErrorCode(err))
		}
		if ctx.Err() != nil {
			return errors.New("Operation aborted")
		}

		bom, text := stripBom(string(content))
		originalEnding := detectLineEnding(text)
		normalizedContent := normalizeToLF(text)
		result, err := applyEditsToNormalizedContent(normalizedContent, in.edits, in.path)
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return errors.New("Operation aborted")
		}

		finalContent := bom + restoreLineEndings(result.newContent, originalEnding)
		if err := os.WriteFile(absolutePath, []byte(finalContent), 0o644); err != nil {
			return fmt.Errorf("Could not edit file: %s. Error code: %s.", in.path, fileErrorCode(err))
		}
		if ctx.Err() != nil {
			return errors.New("Operation aborted")
		}
		slog.Debug("edit tool executed", "path", in.path, "edits", len(in.edits))
		out = fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(in.edits), in.path)
		return nil
	})
	return out, err
}

// NewEdit 创建 edit 工具。
func NewEdit() agent.Tool {
	return &editTool{}
}
