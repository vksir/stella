package agent

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

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

type Message struct {
	Role       Role         `json:"role"`
	Name       string       `json:"name,omitempty"`
	Content    string       `json:"content"`
	Reasoning  string       `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Finish     FinishReason `json:"finish,omitempty"`
}

func MessagesClone(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	cloned := make([]Message, len(messages))
	for i, message := range messages {
		cloned[i] = message
		cloned[i].ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
	}
	return cloned
}
