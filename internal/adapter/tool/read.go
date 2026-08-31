package tool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/vksir/stella/internal/core/agent"
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

func (t *readTool) Execute(ctx context.Context, args string) (out agent.Content, err error) {
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
		return nil, err
	}
	if in.Limit != nil && *in.Limit < 0 {
		return nil, fmt.Errorf("Invalid limit: must be a non-negative line count")
	}
	absolutePath, err := resolveReadToolPath(in.Path)
	if err != nil {
		return nil, err
	}
	bytes, err := os.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", in.Path, err)
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
		return nil, fmt.Errorf("Offset %d is beyond end of file (%d lines total)", *in.Offset, len(allLines))
	}

	var selectedContent string
	var userLimitedLines int
	hasLimit := in.Limit != nil
	if in.Limit != nil {
		endLine := startLine + min(*in.Limit, len(allLines)-startLine)
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
	return agent.TextContent(outputText), nil
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

// readImage 将图片处理为文本说明和独立的图片内容块。
func readImage(opts *ReadOptions, autoResize bool, bytes []byte, mimeType string) agent.Content {
	processor := opts.ImageProcessor
	if processor == nil {
		processor = processImage
	}
	processed := processor(bytes, mimeType, autoResize)
	if !processed.OK {
		return agent.TextContent(fmt.Sprintf("Read image file [%s]\n%s", mimeType, processed.Message))
	}
	if !supportedInlineImageType(processed.MimeType) || processed.Data == "" ||
		(autoResize && len(processed.Data) >= imageMaxBytes) {
		return agent.TextContent(fmt.Sprintf("Read image file [%s]\n[Image omitted: invalid inline image data.]", mimeType))
	}
	if _, err := base64.StdEncoding.Strict().DecodeString(processed.Data); err != nil {
		return agent.TextContent(fmt.Sprintf("Read image file [%s]\n[Image omitted: invalid inline image data.]", mimeType))
	}
	note := fmt.Sprintf("Read image file [%s]", processed.MimeType)
	if len(processed.Hints) > 0 {
		note += "\n" + strings.Join(processed.Hints, "\n")
	}
	return agent.Content{
		{Type: "text", Text: note},
		{Type: "image", Data: processed.Data, MimeType: processed.MimeType},
	}
}

var pngSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// detectSupportedImageMimeType 检测支持的图片 MIME 类型，不支持返回空串。
func detectSupportedImageMimeType(buffer []byte) string {
	if startsWith(buffer, []byte{0xff, 0xd8, 0xff}) {
		if len(buffer) > 3 && buffer[3] == 0xf7 {
			return ""
		}
		return "image/jpeg"
	}
	if startsWith(buffer, pngSignature) {
		if isPng(buffer) && !isAnimatedPng(buffer) {
			return "image/png"
		}
		return ""
	}
	if startsWithAscii(buffer, 0, "GIF87a") || startsWithAscii(buffer, 0, "GIF89a") {
		return "image/gif"
	}
	if startsWithAscii(buffer, 0, "RIFF") && startsWithAscii(buffer, 8, "WEBP") {
		return "image/webp"
	}
	if startsWithAscii(buffer, 0, "BM") && isBmp(buffer) {
		return "image/bmp"
	}
	return ""
}

func isPng(buffer []byte) bool {
	return len(buffer) >= 16 &&
		readUint32BE(buffer, len(pngSignature)) == 13 &&
		startsWithAscii(buffer, 12, "IHDR")
}

// isAnimatedPng 判断是否为 APNG：IHDR 之后、IDAT 之前出现 acTL 块。
func isAnimatedPng(buffer []byte) bool {
	offset := len(pngSignature)
	for offset+8 <= len(buffer) {
		chunkLength := readUint32BE(buffer, offset)
		chunkTypeOffset := offset + 4
		if startsWithAscii(buffer, chunkTypeOffset, "acTL") {
			return true
		}
		if startsWithAscii(buffer, chunkTypeOffset, "IDAT") {
			return false
		}
		nextOffset := offset + 8 + int(chunkLength) + 4
		if nextOffset <= offset || nextOffset > len(buffer) {
			return false
		}
		offset = nextOffset
	}
	return false
}

func isBmp(buffer []byte) bool {
	if len(buffer) < 26 {
		return false
	}
	declaredFileSize := readUint32LE(buffer, 2)
	pixelDataOffset := readUint32LE(buffer, 10)
	dibHeaderSize := readUint32LE(buffer, 14)
	if declaredFileSize != 0 && declaredFileSize < 26 {
		return false
	}
	if pixelDataOffset < 14+dibHeaderSize {
		return false
	}
	if declaredFileSize != 0 && pixelDataOffset >= declaredFileSize {
		return false
	}

	var colorPlanes, bitsPerPixel uint16
	if dibHeaderSize == 12 {
		colorPlanes = readUint16LE(buffer, 22)
		bitsPerPixel = readUint16LE(buffer, 24)
	} else if dibHeaderSize >= 40 && dibHeaderSize <= 124 {
		if len(buffer) < 30 {
			return false
		}
		colorPlanes = readUint16LE(buffer, 26)
		bitsPerPixel = readUint16LE(buffer, 28)
	} else {
		return false
	}
	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
		return colorPlanes == 1
	}
	return false
}

func readUint16LE(buffer []byte, offset int) uint16 {
	return uint16(buffer[offset]) | uint16(buffer[offset+1])<<8
}

func readUint32BE(buffer []byte, offset int) uint32 {
	return uint32(buffer[offset])<<24 | uint32(buffer[offset+1])<<16 |
		uint32(buffer[offset+2])<<8 | uint32(buffer[offset+3])
}

func readUint32LE(buffer []byte, offset int) uint32 {
	return uint32(buffer[offset]) | uint32(buffer[offset+1])<<8 |
		uint32(buffer[offset+2])<<16 | uint32(buffer[offset+3])<<24
}

func startsWith(buffer, prefix []byte) bool {
	if len(buffer) < len(prefix) {
		return false
	}
	for i := range prefix {
		if buffer[i] != prefix[i] {
			return false
		}
	}
	return true
}

func startsWithAscii(buffer []byte, offset int, text string) bool {
	if len(buffer) < offset+len(text) {
		return false
	}
	for i := 0; i < len(text); i++ {
		if buffer[offset+i] != text[i] {
			return false
		}
	}
	return true
}
