package consumer

import (
	"log"
	"qq-bot-go/confs"
	"qq-bot-go/pkg/messagequeue"
	"qq-bot-go/third_party/qq"
)

type Consumer struct {
	MQ   *messagequeue.MsgQueue
	QQ   qq.QQ
	Conf *confs.Conf
}

func NewConsumer(mq *messagequeue.MsgQueue, q qq.QQ, conf *confs.Conf) *Consumer {
	return &Consumer{mq, q, conf}
}

func (c *Consumer) Run() {
	log.Println("consumer run")
	go c.consumeMsg()
	go c.consumeTask()
}

func (c *Consumer) consumeMsg() {
	for {
		select {
		case msg := <-c.MQ.Msg:
			log.Printf("consume msg: %+v", msg)
			c.handleMsg(&msg)
		}
	}
}

func (c *Consumer) consumeTask() {
	for {
		select {
		case task := <-c.MQ.Task:
			log.Printf("consume task: %+v", task)
			c.handleTask(&task)
		}
	}
}
