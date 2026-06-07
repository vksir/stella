package entity

import (
	"context"
	"strings"
)

const (
	EvtTypePrivate = "private"
	EvtTypeGroup   = "group"
)

type Callback func(ctx context.Context, evt *Event) error

type Event struct {
	Type      string  `json:"type"`
	UserID    string  `json:"user_id"`
	UserName  string  `json:"user_name"`
	SessionID string  `json:"session_id"`
	Ask       []Chain `json:"ask"`
	Ans       []Chain `json:"ans"`
	Callback  Callback
}

func (e *Event) AskText() string {
	return e.text(e.Ask)
}

func (e *Event) AnsText() string {
	return e.text(e.Ans)
}

func (e *Event) text(chain []Chain) string {
	text := strings.Builder{}
	for _, c := range chain {
		if c.Type == ChainTypeText {
			text.WriteString(c.Text)
		}
	}
	return text.String()
}
