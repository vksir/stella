package router

import (
	"github.com/gin-gonic/gin"
	"stella/internal/server/router/neutronstar"
)

func LoadRouters(e *gin.Engine) {
	neutronstar.LoadHandlers(e)
}
