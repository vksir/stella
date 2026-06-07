package entity

const (
	ChainRoleSystem     = "system"
	ChainRoleUser       = "user"
	ChainRoleAgent      = "agent"
	ChainRoleToolResult = "tool_result"

	ChainTypeText     = "text"
	ChainTypeImage    = "image"
	ChainTypeVoice    = "voice"
	ChainTypeVideo    = "video"
	ChainTypeToolCall = "tool_call"
)

type Chain struct {
	Role     string     `json:"role"`
	Type     string     `json:"type"`
	Text     string     `json:"text"`
	Data     []byte     `json:"data"`
	ToolID   string     `json:"tool_id"`
	ToolCall []ToolCall `json:"tool_call"`
}

type ToolCall struct {
	ID   string         `json:"id"`
	Func string         `json:"func"`
	Args map[string]any `json:"args"`
}
