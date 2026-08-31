package toolx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/vksir/stella/internal/agent"
)

// ImageProcessorResult 是图片处理的结果。
type ImageProcessorResult struct {
	OK       bool
	Data     string
	MimeType string
	Hints    []string
	Message  string // OK 为 false 时的提示消息
}

// ImageProcessor 将图片字节转换为可返回的表示（如压缩后再 base64）。
type ImageProcessor func(bytes []byte, mimeType string, autoResizeImages bool) ImageProcessorResult

// ReadOptions 配置 read 工具的图片处理行为。
type ReadOptions struct {
	AutoResizeImages *bool
	ImageProcessor   ImageProcessor
}

const readDescription = "Read the contents of a file. Supports text files and images (jpg, png, gif, webp, bmp). Images are sent as attachments. For text files, output is truncated to 2000 lines or 50KB (whichever is hit first). Use offset/limit for large files. When you need the full file, continue with offset until complete."

var readSchema = objectSchema(map[string]any{
	"path":   strProp("Path to the file to read (relative or absolute)"),
	"offset": numProp("Line number to start reading from (1-indexed)"),
	"limit":  numProp("Maximum number of lines to read"),
}, "path")

// readTool 实现 read 工具。
type readTool struct {
	opts       ReadOptions
	autoResize bool
}

func (t *readTool) Name() string { return "read" }

func (t *readTool) Description() string { return readDescription }

func (t *readTool) Schema() map[string]any { return readSchema }

func (t *readTool) Execute(ctx context.Context, args string) (out string, err error) {
	defer func() {
		if err != nil {
			slog.Error("tool failed", "tool", "read", "error", err)
		}
	}()
	var in struct {
		Path   string `json:"path"`
		Offset *int   `json:"offset"`
		Limit  *int   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", err
	}
	absolutePath, err := resolveReadToolPath(in.Path)
	if err != nil {
		return "", err
	}
	bytes, err := os.ReadFile(absolutePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", in.Path, err)
	}
	slog.Debug("read tool executed", "path", in.Path, "offset", in.Offset, "limit", in.Limit)

	mimeType := detectSupportedImageMimeType(bytes)
	if mimeType != "" {
		return readImage(&t.opts, t.autoResize, bytes, mimeType), nil
	}

	// 非法 UTF-8 字节替换为 U+FFFD。
	textContent := strings.ToValidUTF8(string(bytes), "\uFFFD")
	allLines := strings.Split(textContent, "\n")
	totalFileLines := len(allLines)
	startLine := 0
	if in.Offset != nil {
		startLine = max(0, *in.Offset-1)
	}
	startLineDisplay := startLine + 1
	if startLine >= len(allLines) {
		return "", fmt.Errorf("Offset %d is beyond end of file (%d lines total)", *in.Offset, len(allLines))
	}

	var selectedContent string
	var userLimitedLines int
	hasLimit := in.Limit != nil
	if in.Limit != nil {
		endLine := min(startLine+*in.Limit, len(allLines))
		selectedContent = strings.Join(allLines[startLine:endLine], "\n")
		userLimitedLines = endLine - startLine
	} else {
		selectedContent = strings.Join(allLines[startLine:], "\n")
	}

	truncation := truncateHead(selectedContent, DefaultMaxLines, DefaultMaxBytes)
	var outputText string
	if truncation.FirstLineExceedsLimit {
		firstLineSize := formatSize(len(allLines[startLine]))
		outputText = fmt.Sprintf(
			"[Line %d is %s, exceeds %s limit. Use bash: sed -n '%dp' %s | head -c %d]",
			startLineDisplay, firstLineSize, formatSize(DefaultMaxBytes), startLineDisplay, in.Path, DefaultMaxBytes)
	} else if truncation.Truncated {
		endLineDisplay := startLineDisplay + truncation.OutputLines - 1
		nextOffset := endLineDisplay + 1
		outputText = truncation.Content
		if truncation.TruncatedBy == "lines" {
			outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
				startLineDisplay, endLineDisplay, totalFileLines, nextOffset)
		} else {
			outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				startLineDisplay, endLineDisplay, totalFileLines, formatSize(DefaultMaxBytes), nextOffset)
		}
	} else if hasLimit && startLine+userLimitedLines < len(allLines) {
		remaining := len(allLines) - (startLine + userLimitedLines)
		nextOffset := startLine + userLimitedLines + 1
		outputText = fmt.Sprintf("%s\n\n[%d more lines in file. Use offset=%d to continue.]",
			truncation.Content, remaining, nextOffset)
	} else {
		outputText = truncation.Content
	}
	return outputText, nil
}

// NewRead 创建 read 工具。
func NewRead(options ...ReadOptions) agent.Tool {
	var opts ReadOptions
	if len(options) > 0 {
		opts = options[0]
	}
	autoResize := true
	if opts.AutoResizeImages != nil {
		autoResize = *opts.AutoResizeImages
	}
	return &readTool{opts: opts, autoResize: autoResize}
}

// readImage 处理图片文件。Go 的 Tool 接口只能返回纯文本，图片附件以 base64 拼入文本返回；
// 可通过 ReadOptions.ImageProcessor 注入转换（如缩放、省略）来改变该行为。
func readImage(opts *ReadOptions, autoResize bool, bytes []byte, mimeType string) string {
	if opts.ImageProcessor != nil {
		processed := opts.ImageProcessor(bytes, mimeType, autoResize)
		if !processed.OK {
			return fmt.Sprintf("Read image file [%s]\n%s", mimeType, processed.Message)
		}
		hints := ""
		if len(processed.Hints) > 0 {
			hints = "\n" + strings.Join(processed.Hints, "\n")
		}
		return fmt.Sprintf("Read image file [%s]%s\n%s", processed.MimeType, hints, processed.Data)
	}
	if mimeType == "image/bmp" {
		return "Read image file [image/bmp]\n[Image omitted: configure an imageProcessor to convert BMP images.]"
	}
	return fmt.Sprintf("Read image file [%s]\n%s", mimeType, encodeBase64(bytes))
}

func encodeBase64(bytes []byte) string {
	return base64.StdEncoding.EncodeToString(bytes)
}
