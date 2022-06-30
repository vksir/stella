package mirai

import (
	"encoding/json"
	"qq-bot-go/pkg/event"
)

const (
	TypeFriendMessage = "FriendMessage"
	TypeGroupMessage  = "GroupMessage"

	ChainSource     = "Source"
	ChainAt         = "At"
	ChainAtAll      = "AtAll"
	ChainPlain      = "Plain"
	ChainImage      = "Image"
	ChainFlashImage = "FlashImage"
	ChainVoice      = "Voice"
	ChainForward    = "Forward"

	CommandSendFriendMessage = "sendFriendMessage"
	CommandSendGroupMessage  = "sendGroupMessage"
	CommandSendNudge         = "sendNudge"
)

type EventReceive struct {
	SyncId string `json:"syncId"`
	Data   Data   `json:"data"`
}

type Data struct {
	Type         string         `json:"type"`
	Sender       Sender         `json:"sender"`
	MessageChain []MessageChain `json:"messageChain"`
	Code         int            `json:"code"`
	Msg          string         `json:"msg"`
	MessageId    int            `json:"messageId"`
}

type Sender struct {
	Id                 int    `json:"id"`
	Nickname           string `json:"nickname"`
	Remark             string `json:"remark"`
	MemberName         string `json:"memberName"`
	SpecialTitle       string `json:"specialTitle"`
	Permission         string `json:"permission"`
	JoinTimestamp      int    `json:"joinTimestamp"`
	LastSpeakTimestamp int    `json:"lastSpeakTimestamp"`
	MuteTimeRemaining  int    `json:"muteTimeRemaining"`
	Group              Group  `json:"group"`
}

type Group struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Permission string `json:"permission"`
}

type MessageChain struct {
	Type     string        `json:"type,omitempty"`
	Id       int           `json:"id,omitempty"`
	Time     int           `json:"time,omitempty"`
	Target   int           `json:"target,omitempty"`
	Display  string        `json:"display,omitempty"`
	Text     string        `json:"text,omitempty"`
	ImageId  string        `json:"imageId,omitempty"`
	VoiceId  string        `json:"voiceId,omitempty"`
	Url      string        `json:"url,omitempty"`
	Path     interface{}   `json:"path,omitempty"`
	Base64   string        `json:"base64,omitempty"`
	NodeList []MessageNode `json:"nodeList,omitempty"`
}

type MessageNode struct {
	MessageId int `json:"messageId"`
}

type EventSend struct {
	SyncId     string      `json:"syncId"`
	Command    string      `json:"command"`
	SubCommand interface{} `json:"subCommand,omitempty"`
	Content    Content     `json:"content"`
}

func (e *EventSend) appendMessageChain(chainType, text, url, base64 string) {
	var chain MessageChain
	chain.Type = chainType
	if text != "" {
		chain.Text = text
	}
	if url != "" {
		chain.Url = url
	}
	if base64 != "" {
		chain.Base64 = base64
	}
	e.appendCustomMessageChain(chain)
}

func (e *EventSend) appendCustomMessageChain(chain MessageChain) {
	e.Content.MessageChain = append(e.Content.MessageChain, chain)
}

func (e EventSend) log() string {
	for i, chain := range e.Content.MessageChain {
		if chain.Base64 != "" {
			chain.Base64 = ""
		}
		e.Content.MessageChain[i] = chain
	}
	b, _ := json.Marshal(e)
	return string(b)
}

type Content struct {
	Target       int            `json:"target"`
	MessageChain []MessageChain `json:"messageChain"`
}

func (e EventReceive) transformToStandardReceive() event.Receive {
	var receive event.Receive
	for _, chain := range e.Data.MessageChain {
		switch chain.Type {
		case ChainPlain:
			receive.AppendChain(event.TypePlain, chain.Text, "", "")
		case ChainImage:
			receive.AppendChain(event.TypeImage, "", chain.Url, chain.Base64)
		case ChainVoice:
			receive.AppendChain(event.TypeVoice, "", chain.Url, chain.Base64)
		}
	}
	return receive
}

func newSendEvent(send *event.Send) *EventSend {
	sendEvent := EventSend{
		Content: Content{},
	}
	for _, c := range send.Send {
		switch c.Type {
		case event.TypePlain:
			sendEvent.appendMessageChain(ChainPlain, c.Text, "", "")
		case event.TypeImage:
			sendEvent.appendMessageChain(ChainImage, "", c.Url, c.Base64)
		case event.TypeVoice:
			sendEvent.appendMessageChain(ChainVoice, "", c.Url, c.Base64)
		}
	}
	return &sendEvent
}

func newForwardEvent(messageIds []int) *EventSend {
	forwardEvent := EventSend{Content: Content{}}
	chain := MessageChain{Type: ChainForward}
	for _, id := range messageIds {
		node := MessageNode{MessageId: id}
		chain.NodeList = append(chain.NodeList, node)
	}
	forwardEvent.appendCustomMessageChain(chain)
	return &forwardEvent
}
