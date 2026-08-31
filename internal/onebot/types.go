package onebot

// incoming 用于区分入站帧类型：事件（post_type）或 API 响应（retcode）。
// Status 为 any：API 响应是字符串（如 "ok"），heartbeat 元事件是对象。
type incoming struct {
	PostType string `json:"post_type"`
	Status   any    `json:"status"`
	RetCode  *int   `json:"retcode"`
	Echo     string `json:"echo"`
}

// Event 是 OneBot 11 事件。
type Event struct {
	PostType    string    `json:"post_type"`
	MessageType string    `json:"message_type"`
	SubType     string    `json:"sub_type"`
	SelfID      int64     `json:"self_id"`
	UserID      int64     `json:"user_id"`
	GroupID     int64     `json:"group_id"`
	MessageID   int64     `json:"message_id"`
	RawMessage  string    `json:"raw_message"`
	Message     []Segment `json:"message"`
}

// Segment 是消息段。
type Segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// apiRequest 是 OneBot 11 API 调用。
type apiRequest struct {
	Action string `json:"action"`
	Params any    `json:"params"`
	Echo   string `json:"echo"`
}

// sendMsgParams 是 send_msg 的参数。
type sendMsgParams struct {
	MessageType string `json:"message_type"`
	UserID      int64  `json:"user_id,omitempty"`
	GroupID     int64  `json:"group_id,omitempty"`
	Message     string `json:"message"`
}
