// api/router.go
package api

import (
	"net/http"
	"stella/api/handler"
	"stella/docs"
	_ "stella/docs" // Import swagger docs
	"stella/internal/platform/qq"
	"stella/pkg/cfg"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/vksir/vkiss-lib/pkg/log"
)

func SetupRouter(qqAdapter *qq.Adapter) *gin.Engine {
	r := gin.Default()

	host := cfg.G.Host
	if host != "" {
		docs.SwaggerInfo.Host = host
	}
	r.GET("/api/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/api/docs/index.html")
	})
	r.GET("/api/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	apiGroup := r.Group("/api")
	{
		apiGroup.POST("/chat", handler.HandleChat)
	}

	// QQ WebSocket endpoint
	if qqAdapter != nil {
		r.GET("/ws/qq", func(c *gin.Context) {
			qqAdapter.HandleWS(c.Writer, c.Request)
		})
	}

	log.InfoF("listen: http://%s/api/docs", host)
	log.InfoF("doc: http://%s/api/docs", host)
	return r
}
