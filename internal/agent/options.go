package agent

import "log/slog"

type Option func(*Config)

func WithSystemPrompt(prompt string) Option {
	return func(c *Config) { c.systemPrompt = prompt }
}

func WithTools(tools ...Tool) Option {
	tools = append([]Tool(nil), tools...)
	return func(c *Config) {
		for _, tool := range tools {
			c.tools.Set(tool.Name(), tool)
		}
	}
}

func WithLogger(log *slog.Logger) Option {
	return func(c *Config) { c.log = log }
}

func WithOnEvent(onEvent OnEvent) Option {
	return func(c *Config) { c.onEvent = onEvent }
}

func WithMaxTurn(maxTurn int) Option {
	return func(c *Config) { c.maxTurn = maxTurn }
}
