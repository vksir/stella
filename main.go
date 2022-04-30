package main

import (
	"qq-bot-go/common"
	"qq-bot-go/confs"
	"qq-bot-go/consumer"
	"qq-bot-go/messageQueue"
	"qq-bot-go/producer/qq"
	"qq-bot-go/producer/server"
)

func main() {
	fp := common.NewFilePath()
	fp.InitPath()
	conf := confs.NewConf(fp)
	mq := messageQueue.NewMQ()
	q := qq.NewMiraiWebsocket(conf)
	q.Listen(mq)
	c := consumer.NewConsumer(mq, q, conf)
	c.Run()
	server.Run(mq)
}
