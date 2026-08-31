package toolx

import (
	"fmt"
	"strings"
)

// 工具输出截断常量。
const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024
)

// TruncationResult 描述一次截断的结果。
type TruncationResult struct {
	Content               string
	Truncated             bool
	TruncatedBy           string // "" 表示未截断，否则为 "lines" 或 "bytes"
	TotalLines            int
	TotalBytes            int
	OutputLines           int
	OutputBytes           int
	LastLinePartial       bool
	FirstLineExceedsLimit bool
	MaxLines              int
	MaxBytes              int
}

// formatSize 将字节数格式化为可读大小。
func formatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

// truncateHead 从头截断，保留前若干完整行。
func truncateHead(content string, maxLines, maxBytes int) TruncationResult {
	totalBytes := len(content)
	lines := splitLines(content)
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content: content, TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: totalLines, OutputBytes: totalBytes, MaxLines: maxLines, MaxBytes: maxBytes,
		}
	}

	firstLineBytes := len(lines[0])
	if firstLineBytes > maxBytes {
		return TruncationResult{
			Truncated: true, TruncatedBy: "bytes", TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: 0, OutputBytes: 0, FirstLineExceedsLimit: true, MaxLines: maxLines, MaxBytes: maxBytes,
		}
	}

	output := make([]string, 0, maxLines)
	outputBytes := 0
	truncatedBy := "lines"
	for i := 0; i < len(lines) && i < maxLines; i++ {
		lineBytes := len(lines[i])
		if i > 0 {
			lineBytes++ // 非首行 +1 换行符
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}
		output = append(output, lines[i])
		outputBytes += lineBytes
	}
	if len(output) >= maxLines && outputBytes <= maxBytes {
		truncatedBy = "lines"
	}

	outputContent := strings.Join(output, "\n")
	return TruncationResult{
		Content: outputContent, Truncated: true, TruncatedBy: truncatedBy,
		TotalLines: totalLines, TotalBytes: totalBytes,
		OutputLines: len(output), OutputBytes: len(outputContent), MaxLines: maxLines, MaxBytes: maxBytes,
	}
}

// truncateTail 从尾截断，保留最后若干完整行。
// 最后一行超过字节限制时允许部分保留（LastLinePartial）。
func truncateTail(content string, maxLines, maxBytes int) TruncationResult {
	totalBytes := len(content)
	lines := splitLines(content)
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content: content, TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: totalLines, OutputBytes: totalBytes, MaxLines: maxLines, MaxBytes: maxBytes,
		}
	}

	output := make([]string, 0, maxLines)
	outputBytes := 0
	truncatedBy := "lines"
	lastLinePartial := false
	for i := len(lines) - 1; i >= 0 && len(output) < maxLines; i-- {
		lineBytes := len(lines[i])
		if len(output) > 0 {
			lineBytes++ // 非末行 +1 换行符
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			if len(output) == 0 {
				truncatedLine := truncateStringFromEnd(lines[i], maxBytes)
				output = append([]string{truncatedLine}, output...)
				outputBytes = len(truncatedLine)
				lastLinePartial = true
			}
			break
		}
		output = append([]string{lines[i]}, output...)
		outputBytes += lineBytes
	}
	if len(output) >= maxLines && outputBytes <= maxBytes {
		truncatedBy = "lines"
	}

	outputContent := strings.Join(output, "\n")
	return TruncationResult{
		Content: outputContent, Truncated: true, TruncatedBy: truncatedBy,
		TotalLines: totalLines, TotalBytes: totalBytes,
		OutputLines: len(output), OutputBytes: len(outputContent),
		LastLinePartial: lastLinePartial, MaxLines: maxLines, MaxBytes: maxBytes,
	}
}

// truncateStringFromEnd 保留字符串末尾的完整 UTF-8 字符至多 maxBytes 字节。
func truncateStringFromEnd(s string, maxBytes int) string {
	b := []byte(s)
	if len(b) <= maxBytes {
		return s
	}
	start := len(b) - maxBytes
	for start < len(b) && b[start]&0xC0 == 0x80 {
		start++
	}
	return string(b[start:])
}

// trimToLastUTF8Bytes 保留字符串末尾最多 maxBytes 字节，并按 UTF-8 边界对齐。
func trimToLastUTF8Bytes(text string, maxBytes int) string {
	b := []byte(text)
	if len(b) <= maxBytes {
		return text
	}
	start := len(b) - maxBytes
	for start < len(b) && b[start]&0xC0 == 0x80 {
		start++
	}
	return string(b[start:])
}
