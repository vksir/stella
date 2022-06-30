package agent

import (
	"qq-bot-go/internal/agent/mirai"
)

func LoadAgents() {
	mirai.Load()
}
