package qq

import "qq-bot-go/messageQueue"

type QQ interface {
	SendFriendMsg(userId int, msg string)
	SendGroupMsg(groupId int, msg string)
	SendMsg(userId int, groupId int, targetType string, msg string)
	Listen(mq *messageQueue.MsgQueue)
}
