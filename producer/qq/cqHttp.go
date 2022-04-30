package qq

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"log"
	"net/http"
	"strconv"
)

type cqHttp struct {
	url string
}

func NewCqHttp(url string) *cqHttp {
	return &cqHttp{
		url: url,
	}
}

func (q *cqHttp) SendFriendMsg(userId int, msg string) {
	c := resty.New()
	resp, err := c.R().
		SetQueryParam("user_id", strconv.Itoa(userId)).
		SetQueryParam("Message", msg).
		Get(fmt.Sprintf("%s/send_private_msg", q.url))
	if err != nil {
		log.Println("request failed: ", err)
		return
	}
	if resp.StatusCode() != http.StatusOK {
		log.Printf("SendFriendMsg failed: status=%s, body=%s", resp.Status(), string(resp.Body()))
	}
	log.Printf("SendFriendMsg succeed: body=%s", string(resp.Body()))
}

func (q *cqHttp) SendGroupMsg(groupId int, msg string) {
	c := resty.New()
	resp, err := c.R().
		SetQueryParam("group_id", strconv.Itoa(groupId)).
		SetPathParam("Message", msg).
		Get(fmt.Sprintf("%s/send_group_msg", q.url))
	if err != nil {
		log.Println("request failed: ", err)
		return
	}
	if resp.StatusCode() != http.StatusOK {
		log.Printf("SendGroupMsg failed: status=%s, body=%s", resp.Status(), string(resp.Body()))
	}
	log.Printf("SendGroupMsg succeed: body=%s", string(resp.Body()))
}
