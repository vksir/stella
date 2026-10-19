package agent

import "log/slog"

type Option func(*Config)

func WithModel(model string) Option {
	return func(c *Config) { c.model = model }
}

func WithReasoningEffort(effort string) Option {
	return func(c *Config) { c.reasoningEffort = effort }
}

func WithTemperature(temperature float64) Option {
	return func(c *Config) { c.temperature = &temperature }
}

func WithTools(tools ...Tool) Option {
	tools = append([]Tool(nil), tools...)
	return func(c *Config) {
		for _, tool := range tools {
			c.tools.Set(tool.Name(), tool)
		}
	}
}

func WithDynTools(tools ...Tool) Option {
	tools = append([]Tool(nil), tools...)
	return func(c *Config) {
		for _, tool := range tools {
			c.dynTools.Set(tool.Name(), tool)
		}
	}
}

func WithExtensions(exts ...Extension) Option {
	exts = append([]Extension(nil), exts...)
	return func(c *Config) { c.extensions = append(c.extensions, exts...) }
}

func WithScenarioPrompt(prompt string) Option {
	return func(c *Config) { c.scenarioPrompt = prompt }
}

func WithLLM(llm LLM) Option {
	return func(c *Config) { c.llm = llm }
}

func WithStore(store Store) Option {
	return func(c *Config) { c.store = store }
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

func WithNonblock(nonblock bool) Option {
	return func(c *Config) { c.nonblock = nonblock }
}
