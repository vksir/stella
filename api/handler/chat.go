// api/handler/chat.go
package handler

import (
	"net/http"
	"stella/entity"
	"stella/internal/agent"

	"github.com/gin-gonic/gin"
	"github.com/vksir/vkiss-lib/pkg/util/apiutil"
)

var Agent *agent.Agent

// HandleChat
// @Summary 发送聊天消息
// @Description 通过 HTTP API 发送聊天消息并返回 AI 对话结果
// @Tags Chat
// @Accept json
// @Produce json
// @Param request body entity.Event true "聊天请求参数"
// @Success 200 {object} apiutil.Response
// @Router /api/chat [post]
func HandleChat(c *gin.Context) {
	var evt entity.Event
	if err := c.ShouldBindJSON(&evt); err != nil {
		c.JSON(http.StatusBadRequest, apiutil.Response{Message: err.Error()})
		return
	}

	// Default user for HTTP API
	userID := evt.UserID
	if userID == "" {
		userID = "api_user"
	}
	if evt.SessionID == "" {
		evt.SessionID = userID
	}

	err := Agent.Chat(c.Request.Context(), userID, &evt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiutil.Response{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, apiutil.Response{Data: evt.Ans})
}
