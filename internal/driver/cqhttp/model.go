package cqhttp

type SendMsgBody struct {
	MessageType string `json:"message_type"`
	UserId      int64  `json:"user_id"`
	GroupId     int64  `json:"group_id"`
	Message     string `json:"message"`
}
