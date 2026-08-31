package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/vksir/stella/internal/core/agent"
	"golang.org/x/text/unicode/norm"
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

func (t *editTool) Execute(ctx context.Context, args string) (out agent.Content, err error) {
	defer func() {
		if err != nil {
			slog.Error("tool failed", "tool", "edit", "error", err)
		}
	}()
	in, err := parseEditArgs(args)
	if err != nil {
		return nil, err
	}
	absolutePath, err := resolveToolPath(in.path)
	if err != nil {
		return nil, err
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
		out = agent.TextContent(fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(in.edits), in.path))
		return nil
	})
	return out, err
}

// NewEdit 创建 edit 工具。
func NewEdit() agent.Tool {
	return &editTool{}
}

// 编辑匹配与替换逻辑。

func detectLineEnding(content string) string {
	crlfIdx := strings.Index(content, "\r\n")
	lfIdx := strings.Index(content, "\n")
	switch {
	case lfIdx == -1:
		return "\n"
	case crlfIdx == -1:
		return "\n"
	case crlfIdx < lfIdx:
		return "\r\n"
	default:
		return "\n"
	}
}

func normalizeToLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func restoreLineEndings(text, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// normalizeForFuzzyMatch 归一化文本用于模糊匹配。
func normalizeForFuzzyMatch(text string) string {
	lines := strings.Split(norm.NFKC.String(text), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	text = strings.Join(lines, "\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u2018', '\u2019', '\u201A', '\u201B':
			return '\''
		case '\u201C', '\u201D', '\u201E', '\u201F':
			return '"'
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006',
			'\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			return ' '
		default:
			return r
		}
	}, text)
}

// splitLinesWithEndings 按行拆分并保留行尾换行符。
func splitLinesWithEndings(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i+1])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// LineSpan 是内容中一行的字节区间。
type LineSpan struct{ start, end int }

func getLineSpans(content string) []LineSpan {
	lines := splitLinesWithEndings(content)
	spans := make([]LineSpan, 0, len(lines))
	offset := 0
	for _, line := range lines {
		spans = append(spans, LineSpan{start: offset, end: offset + len(line)})
		offset += len(line)
	}
	return spans
}

// TextReplacement 是一次文本替换的定位信息。
type TextReplacement struct {
	matchIndex  int
	matchLength int
	newText     string
}

func getReplacementLineRange(lines []LineSpan, replacement TextReplacement) (int, int, error) {
	replacementStart := replacement.matchIndex
	replacementEnd := replacement.matchIndex + replacement.matchLength

	startLine := -1
	for i, line := range lines {
		if replacementStart >= line.start && replacementStart < line.end {
			startLine = i
			break
		}
	}
	if startLine == -1 {
		return 0, 0, fmt.Errorf("Replacement range is outside the base content.")
	}

	endLine := startLine
	for endLine < len(lines) && lines[endLine].end < replacementEnd {
		endLine++
	}
	if endLine >= len(lines) {
		return 0, 0, fmt.Errorf("Replacement range is outside the base content.")
	}

	return startLine, endLine + 1, nil
}

// applyReplacements 逆序应用替换，保证各替换的索引基于同一份原始内容。
func applyReplacements(content string, replacements []TextReplacement, offset int) string {
	result := []byte(content)
	for i := len(replacements) - 1; i >= 0; i-- {
		replacement := replacements[i]
		matchIndex := replacement.matchIndex - offset
		buf := make([]byte, 0, len(result)-replacement.matchLength+len(replacement.newText))
		buf = append(buf, result[:matchIndex]...)
		buf = append(buf, replacement.newText...)
		buf = append(buf, result[matchIndex+replacement.matchLength:]...)
		result = buf
	}
	return string(result)
}

// replacementGroup 是一组落在同一批相邻行的替换。
type replacementGroup struct {
	startLine    int
	endLine      int
	replacements []TextReplacement
}

// applyReplacementsPreservingUnchangedLines 将替换应用到规范化后的内容，同时保留未修改行的原始字节。
func applyReplacementsPreservingUnchangedLines(
	originalContent, baseContent string,
	replacements []TextReplacement,
) (string, error) {
	originalLines := splitLinesWithEndings(originalContent)
	baseLines := getLineSpans(baseContent)
	if len(originalLines) != len(baseLines) {
		return "", fmt.Errorf("Cannot preserve unchanged lines because the base content has a different line count.")
	}

	var groups []*replacementGroup
	sorted := make([]TextReplacement, len(replacements))
	copy(sorted, replacements)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].matchIndex < sorted[j].matchIndex })
	for _, replacement := range sorted {
		startLine, endLine, err := getReplacementLineRange(baseLines, replacement)
		if err != nil {
			return "", err
		}
		current := lastGroup(groups)
		if current != nil && startLine < current.endLine {
			if endLine > current.endLine {
				current.endLine = endLine
			}
			current.replacements = append(current.replacements, replacement)
			continue
		}
		groups = append(groups, &replacementGroup{
			startLine:    startLine,
			endLine:      endLine,
			replacements: []TextReplacement{replacement},
		})
	}

	var sb strings.Builder
	originalLineIndex := 0
	for _, g := range groups {
		for _, line := range originalLines[originalLineIndex:g.startLine] {
			sb.WriteString(line)
		}
		groupStartOffset := baseLines[g.startLine].start
		groupEndOffset := baseLines[g.endLine-1].end
		sb.WriteString(applyReplacements(
			baseContent[groupStartOffset:groupEndOffset],
			g.replacements,
			groupStartOffset,
		))
		originalLineIndex = g.endLine
	}
	for _, line := range originalLines[originalLineIndex:] {
		sb.WriteString(line)
	}
	return sb.String(), nil
}

func lastGroup(groups []*replacementGroup) *replacementGroup {
	if len(groups) == 0 {
		return nil
	}
	return groups[len(groups)-1]
}

// fuzzyMatch 是一次模糊查找的结果。
type fuzzyMatch struct {
	found          bool
	index          int
	matchLength    int
	usedFuzzyMatch bool
}

// fuzzyFindText 先在原内容中精确查找，失败后在归一化空间中模糊查找。
func fuzzyFindText(content, oldText string) fuzzyMatch {
	exactIndex := strings.Index(content, oldText)
	if exactIndex != -1 {
		return fuzzyMatch{found: true, index: exactIndex, matchLength: len(oldText)}
	}

	fuzzyContent := normalizeForFuzzyMatch(content)
	fuzzyOldText := normalizeForFuzzyMatch(oldText)
	fuzzyIndex := strings.Index(fuzzyContent, fuzzyOldText)
	if fuzzyIndex == -1 {
		return fuzzyMatch{found: false}
	}
	return fuzzyMatch{
		found: true, index: fuzzyIndex, matchLength: len(fuzzyOldText),
		usedFuzzyMatch: true,
	}
}

// stripBom 剥离 UTF-8 BOM，返回 BOM 与剩余文本。
func stripBom(content string) (string, string) {
	if strings.HasPrefix(content, "\uFEFF") {
		return "\uFEFF", content[3:]
	}
	return "", content
}

func countOccurrences(content, oldText string) int {
	return strings.Count(normalizeForFuzzyMatch(content), normalizeForFuzzyMatch(oldText))
}

// Edit 是一次精确文本替换。
type Edit struct {
	oldText string
	newText string
}

// appliedEdits 是一次编辑操作前后的内容。
type appliedEdits struct {
	newContent string
}

func notFoundError(path string, editIndex, totalEdits int) error {
	if totalEdits == 1 {
		return fmt.Errorf("Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", path)
	}
	return fmt.Errorf("Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.", editIndex, path)
}

func duplicateError(path string, editIndex, totalEdits, occurrences int) error {
	if totalEdits == 1 {
		return fmt.Errorf("Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.", occurrences, path)
	}
	return fmt.Errorf("Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.", occurrences, editIndex, path)
}

func emptyOldTextError(path string, editIndex, totalEdits int) error {
	if totalEdits == 1 {
		return fmt.Errorf("oldText must not be empty in %s.", path)
	}
	return fmt.Errorf("edits[%d].oldText must not be empty in %s.", editIndex, path)
}

func noChangeError(path string, totalEdits int) error {
	if totalEdits == 1 {
		return fmt.Errorf("No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.", path)
	}
	return fmt.Errorf("No changes made to %s. The replacements produced identical content.", path)
}

// matchedEdit 是一次已定位的编辑。
type matchedEdit struct {
	editIndex   int
	matchIndex  int
	matchLength int
	newText     string
}

// applyEditsToNormalizedContent 对 LF 归一化内容应用一个或多个精确文本替换。
func applyEditsToNormalizedContent(normalizedContent string, edits []Edit, path string) (appliedEdits, error) {
	normalizedEdits := make([]Edit, len(edits))
	for i, edit := range edits {
		normalizedEdits[i] = Edit{oldText: normalizeToLF(edit.oldText), newText: normalizeToLF(edit.newText)}
	}

	for i := range normalizedEdits {
		if normalizedEdits[i].oldText == "" {
			return appliedEdits{}, emptyOldTextError(path, i, len(normalizedEdits))
		}
	}

	usedFuzzyMatch := false
	for _, edit := range normalizedEdits {
		if fuzzyFindText(normalizedContent, edit.oldText).usedFuzzyMatch {
			usedFuzzyMatch = true
			break
		}
	}
	replacementBaseContent := normalizedContent
	if usedFuzzyMatch {
		replacementBaseContent = normalizeForFuzzyMatch(normalizedContent)
	}

	matched := make([]matchedEdit, 0, len(normalizedEdits))
	for i, edit := range normalizedEdits {
		matchResult := fuzzyFindText(replacementBaseContent, edit.oldText)
		if !matchResult.found {
			return appliedEdits{}, notFoundError(path, i, len(normalizedEdits))
		}
		occurrences := countOccurrences(replacementBaseContent, edit.oldText)
		if occurrences > 1 {
			return appliedEdits{}, duplicateError(path, i, len(normalizedEdits), occurrences)
		}
		matched = append(matched, matchedEdit{
			editIndex:   i,
			matchIndex:  matchResult.index,
			matchLength: matchResult.matchLength,
			newText:     edit.newText,
		})
	}

	sort.SliceStable(matched, func(i, j int) bool { return matched[i].matchIndex < matched[j].matchIndex })
	for i := 1; i < len(matched); i++ {
		previous := matched[i-1]
		current := matched[i]
		if previous.matchIndex+previous.matchLength > current.matchIndex {
			return appliedEdits{}, fmt.Errorf(
				"edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.",
				previous.editIndex, current.editIndex, path)
		}
	}

	baseContent := normalizedContent
	replacements := make([]TextReplacement, len(matched))
	for i, m := range matched {
		replacements[i] = TextReplacement{matchIndex: m.matchIndex, matchLength: m.matchLength, newText: m.newText}
	}
	var newContent string
	var err error
	if usedFuzzyMatch {
		newContent, err = applyReplacementsPreservingUnchangedLines(normalizedContent, replacementBaseContent, replacements)
	} else {
		newContent = applyReplacements(replacementBaseContent, replacements, 0)
	}
	if err != nil {
		return appliedEdits{}, err
	}

	if baseContent == newContent {
		return appliedEdits{}, noChangeError(path, len(normalizedEdits))
	}
	return appliedEdits{newContent: newContent}, nil
}
