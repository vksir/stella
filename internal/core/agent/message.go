package agent

import "strings"

type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

type Content []ContentPart

func TextContent(text string) Content {
	return Content{{Type: "text", Text: text}}
}

func (c Content) Text() string {
	var parts []string
	for _, part := range c {
		if part.Type == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type Message struct {
	Role       Role         `json:"role"`
	Name       string       `json:"name,omitempty"`
	Content    Content      `json:"content"`
	Reasoning  string       `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Finish     FinishReason `json:"finish,omitempty"`
}
