package agent

type EventType string

const (
	EventTextStart      EventType = "text_start"
	EventTextDelta      EventType = "text_delta"
	EventTextEnd        EventType = "text_end"
	EventReasoningStart EventType = "reasoning_start"
	EventReasoningDelta EventType = "reasoning_delta"
	EventReasoningEnd   EventType = "reasoning_end"
	EventToolCallStart  EventType = "tool_call_start"
	EventToolCallDelta  EventType = "tool_call_delta"
	EventToolCallEnd    EventType = "tool_call_end"
	EventFinish         EventType = "finish"
	EventDone           EventType = "done"
	EventError          EventType = "error"
)

type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishLength        FinishReason = "length"
	FinishToolCalls     FinishReason = "tool_calls"
	FinishContentFilter FinishReason = "content_filter"
)

type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

type Usage struct {
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	CacheReadTokens int
}

type Event struct {
	Type           EventType
	TextDelta      string
	ReasoningDelta string
	ToolCallDelta  ToolCallDelta
	Finish         FinishReason
	Usage          Usage
	Error          string
}
