package agent

import (
	"strings"

	"github.com/vksir/stella/pkg/collection"
)

const agentPrompt = "You are a versatile assistant named Stella. You help users with a wide range of questions and tasks, using available tools when needed to provide accurate, clear, and useful results."

// SystemPrompt 按固定骨架组装系统提示词。
type SystemPrompt struct {
	identity string
	rules    string
	scenario string
	tools    *collection.OrderedMap[string, Tool]
	dynTools *collection.OrderedMap[string, Tool]
	sections []PromptSection
}

// Build 渲染系统提示词：identity 开头段，随后依次为 tools、dyn_tools、rules、scenario_context 与扩展段；空段省略。
func (p *SystemPrompt) Build() string {
	var blocks []string

	if p.identity != "" {
		blocks = append(blocks, p.identity)
	}
	if p.tools.Len() > 0 {
		var b strings.Builder
		b.WriteString("<tools>\n")
		for _, tool := range p.tools.All() {
			b.WriteString("- ")
			b.WriteString(tool.Name())
			b.WriteString(": ")
			b.WriteString(tool.Summary())
			b.WriteByte('\n')
		}
		b.WriteString("</tools>")
		blocks = append(blocks, b.String())
	}
	if p.dynTools.Len() > 0 {
		var b strings.Builder
		b.WriteString("<dyn_tools>\n")
		b.WriteString("Dynamic tools are accessed through dyn. Available dynamic tools:\n")
		for _, tool := range p.dynTools.All() {
			b.WriteString("- ")
			b.WriteString(tool.Name())
			b.WriteString(": ")
			b.WriteString(tool.Summary())
			b.WriteByte('\n')
		}
		b.WriteString("</dyn_tools>")
		blocks = append(blocks, b.String())
	}
	if p.rules != "" {
		blocks = append(blocks, "<rules>\n"+p.rules+"\n</rules>")
	}
	if p.scenario != "" {
		blocks = append(blocks, "<scenario_context>\n"+p.scenario+"\n</scenario_context>")
	}
	for _, s := range p.sections {
		blocks = append(blocks, "<"+s.Title+">\n"+s.Body+"\n</"+s.Title+">")
	}

	return strings.Join(blocks, "\n\n")
}
