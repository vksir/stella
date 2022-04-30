package qq

// Message use to send qq msg
type Message struct {
	SyncId     int         `json:"syncId"`
	Command    string      `json:"command"`
	SubCommand interface{} `json:"subCommand"`
	Content    Content     `json:"content"`
}

type Content struct {
	Target       int          `json:"target"`
	MessageChain MessageChain `json:"messageChain"`
}

// Event use to recv qq msg
type Event struct {
	SyncId string `json:"syncId"`
	Data   Data   `json:"data"`
}

type Data struct {
	Type         string       `json:"type"`
	MessageChain MessageChain `json:"messageChain"`
	Sender       Sender       `json:"sender"`
}

type MessageChain []struct {
	Type string `json:"type"`
	Id   int    `json:"id"`
	Time int    `json:"time"`
	Text string `json:"text"`
}

type Sender struct {
	// friend and group
	Id int `json:"id"`

	// friend only
	Nickname string `json:"nickname"`
	Remark   string `json:"remark"`

	// group only
	MemberName         string `json:"memberName"`
	SpecialTitle       string `json:"specialTitle"`
	Permission         string `json:"permission"` // 发送消息者群权限
	JoinTimestamp      int    `json:"joinTimestamp"`
	LastSpeakTimestamp int    `json:"lastSpeakTimestamp"`
	MuteTimeRemaining  int    `json:"muteTimeRemaining"`
	Group              Group  `json:"group"`
}

type Group struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Permission string `json:"permission"` // 自身群权限
}

func (mc *MessageChain) getContent() (content string) {
	for _, v := range *mc {
		if v.Type == "Plain" {
			content += v.Text
		}
	}
	return
}
