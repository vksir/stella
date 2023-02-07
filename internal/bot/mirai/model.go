package mirai

import (
	"encoding/json"
	"qq-bot-go/internal/event"
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

type Receive struct {
	SyncId string `json:"syncId"`
	Data   Data   `json:"data"`
}

type Send struct {
	SyncId     string      `json:"syncId"`
	Command    string      `json:"command"`
	SubCommand interface{} `json:"subCommand,omitempty"`
	Content    Content     `json:"content"`
}

type Data struct {
	Type          string         `json:"type"`
	Sender        Sender         `json:"sender"`
	MessageChains []MessageChain `json:"messageChain"`
	Code          int            `json:"code"`
	Msg           string         `json:"msg"`
	MessageId     int            `json:"messageId"`
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

type Content struct {
	Target        int            `json:"target"`
	MessageChains []MessageChain `json:"messageChain"`
}

func NewReceive(e *event.Event) *Receive {
	r := Receive{
		Data: Data{
			MessageChains: eventChainsToMessageChains(e.Chains),
		},
	}
	return &r
}

func NewSend(e *event.Event) *Send {
	s := Send{
		Content: Content{
			MessageChains: eventChainsToMessageChains(e.Chains),
		},
	}
	return &s
}

func (r *Receive) ToEvent() *event.Event {
	return &event.Event{
		Chains: messageChainsToEventChains(r.Data.MessageChains),
	}
}

func (s *Send) ToEvent() *event.Event {
	return &event.Event{
		Chains: messageChainsToEventChains(s.Content.MessageChains),
	}
}

func (s *Send) String() string {
	copySend := *s
	for i := range copySend.Content.MessageChains {
		copySend.Content.MessageChains[i].Base64 = "..."
	}
	bytes, err := json.Marshal(copySend)
	if err != nil {
		log.Error("To string failed: ", err)
		return ""
	}
	return string(bytes)
}

func messageChainsToEventChains(messageChains []MessageChain) []event.Chain {
	var chains []event.Chain
	for _, messageChain := range messageChains {
		chains = append(chains, event.Chain{
			Type:   messageChain.Type,
			Text:   messageChain.Text,
			Url:    messageChain.Url,
			Base64: messageChain.Base64,
		})
	}
	return chains
}

func eventChainsToMessageChains(chains []event.Chain) []MessageChain {
	var messageChains []MessageChain
	for _, chain := range chains {
		messageChains = append(messageChains, MessageChain{
			Type:   chain.Type,
			Text:   chain.Text,
			Url:    chain.Url,
			Base64: chain.Base64,
		})
	}
	return messageChains
}
