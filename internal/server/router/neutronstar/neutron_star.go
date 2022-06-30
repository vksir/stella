package neutronstar

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func LoadHandlers(e *gin.Engine) {
	e.POST("/:component", componentEvent)
}

func componentEvent(c *gin.Context) {
	//component := c.Param("component")
	//var msg messagequeue.Msg
	//if err := c.ShouldBindJSON(&msg); err != nil {
	//	c.JSON(http.StatusBadRequest, gin.H{
	//		"detail": fmt.Sprintln("invalid json body: ", err),
	//	})
	//	return
	//}
	//msg.Component = component
	//messagequeue.MQList.Msg <- msg
	c.JSON(http.StatusOK, gin.H{})
}
