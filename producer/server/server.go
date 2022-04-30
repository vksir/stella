package server

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"qq-bot-go/common"
	"qq-bot-go/messageQueue"
)

func Run(mq *messageQueue.MsgQueue) {
	//gin.SetMode(gin.ReleaseMode)
	routes := gin.Default()
	//err := routes.SetTrustedProxies([]string{"127.0.0.1"})
	//if err != nil {
	//	log.Println("gin set_trusted_proxies failed: ", err)
	//	return
	//}
	routes.POST("/", func(c *gin.Context) {
		var data Event
		err := c.ShouldBindJSON(&data)
		if err != nil {
			return
		}
		j, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(j))
		c.JSON(http.StatusOK, data)
	})
	routes.POST("/:component", func(c *gin.Context) {
		component := c.Param("component")
		var msg messageQueue.Msg
		err := c.ShouldBindJSON(&msg)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Ret:    common.RetFail,
				Detail: fmt.Sprintln("invalid json body: ", err),
			})
			return
		}
		msg.Component = component
		mq.Msg <- msg
		c.JSON(http.StatusOK, Response{})

	})
	err := routes.Run("0.0.0.0:5701")
	if err != nil {
		log.Println("gin run exit: ", err)
		return
	}
}
