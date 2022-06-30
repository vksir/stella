package server

import (
	"github.com/gin-gonic/gin"
	"qq-bot-go/internal/server/router"
)

func Run() {
	e := gin.Default()
	router.LoadRouters(e)
	e.Run("0.0.0.0:5701")
}
