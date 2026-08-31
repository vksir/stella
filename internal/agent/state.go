package agent

type ToolDefinition struct {
	Name        string
	Description string
	Schema      map[string]any
}

type StateReadOnly struct {
	Model           string // Model 使用 provider/model，空值继承默认模型
	SystemPrompt    string
	Messages        []Message
	Tools           []ToolDefinition
	ReasoningEffort string
	Temperature     *float64
}

func (a *Agent) readOnly() StateReadOnly {
	request := StateReadOnly{
		Model:           a.session.Model,
		SystemPrompt:    a.systemPrompt,
		Messages:        append(MessagesClone(a.session.Messages), MessagesClone(a.appendant)...),
		ReasoningEffort: a.session.ReasoningEffort,
	}
	if a.session.Temperature != nil {
		request.Temperature = new(*a.session.Temperature)
	}
	if a.tools != nil {
		for _, tool := range a.tools.All() {
			request.Tools = append(request.Tools, ToolDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Schema:      tool.Schema(),
			})
		}
	}
	return request
}
