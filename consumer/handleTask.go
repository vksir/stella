package consumer

import (
	"fmt"
	"log"
	"qq-bot-go/pkg/messagequeue"
	"regexp"
	"strings"
)

// handleTask: handle task send from qq
func (c *Consumer) handleTask(task *messagequeue.Task) {
	for _, user := range c.Conf.Task {
		if strings.HasPrefix(task.Content, "/") {
			if task.Sender.Id == user.Id {
				re := regexp.MustCompile(`/(\S*)\s*(\S*)`)
				res := re.FindStringSubmatch(task.Content)
				if len(res) < 2 {
					c.QQ.SendMsg(task.Sender.Id, task.Group.Id, task.Type, "命令格式不规范")
				} else {
					short, cmd := res[1], res[2]
					log.Println("short: ", short, ", cmd: ", cmd)
					c.QQ.SendMsg(task.Sender.Id, task.Group.Id, task.Type, fmt.Sprintf("short: %s, cmd: %s", short, cmd))
				}
			}
		} else {
			//c.QQ.SendMsg(task.Sender.Id, task.Group.Id, task.Type, task.Content)
		}
	}
}
