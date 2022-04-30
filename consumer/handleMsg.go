package consumer

import (
	"fmt"
	"qq-bot-go/messageQueue"
)

// handleMsg: handle msg reported by components
func (c *Consumer) handleMsg(msg *messageQueue.Msg) {
	for _, user := range c.Conf.Report {
		if levelAllow(msg.Level, user.Level) {
			for _, component := range user.Component {
				if component.Name == msg.Component && component.UUID == msg.UUID {
					rawMsg := fmt.Sprintf("[%s][%s] %s", msg.Nickname, msg.Component, msg.Content)
					c.QQ.SendMsg(user.Id, user.Id, user.Type, rawMsg)
				}
			}
		}
	}
}
