package router

import (
	"github.com/gin-gonic/gin"
	"qq-bot-go/internal/server/router/neutronstar"
)

func LoadRouters(e *gin.Engine) {
	neutronstar.LoadHandlers(e)
}
