package server

type Sender struct {
	UserId   int    `json:"user_id"`
	NickName string `json:"nick_name"`
	Card     string `json:"card"`
}

type Event struct {
	MessageType string `json:"message_type"`
	SubType     string `json:"sub_type"`
	GroupId     int    `json:"group_id"`
	RawMessage  string `json:"raw_message"`
	Sender      Sender `json:"sender"`
}

type Report struct {
	Nickname string `json:"nickname"`
	Uuid     string `json:"uuid"`
	Msg      string `json:"msg"`
	Level    string `json:"level"`
}

type Response struct {
	Ret    int    `json:"ret"`
	Detail string `json:"detail"`
}
