package agent

import "github.com/vksir/stella/pkg/collection"

const agentPrompt = "You are a versatile assistant named Stella. You help users with a wide range of questions and tasks, using available tools when needed to provide accurate, clear, and useful results."

type SystemPrompt struct {
}

func newSystemPrompt() *SystemPrompt {
	return &SystemPrompt{}
}

func (p *SystemPrompt) Build(tools *collection.OrderedMap[string, Tool], dynTools *collection.OrderedMap[string, DynTool]) string {
	return ""
}
