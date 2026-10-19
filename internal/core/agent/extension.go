package agent

import "context"

// Extension 扩展入口，仅 Name 必需；其余能力按需实现对应接口，实现即生效。
type Extension interface {
	Name() string
}

// ToolProvider 提供直发工具。
type ToolProvider interface {
	Tools() []Tool
}

// DynToolProvider 提供经 dyn 网关懒暴露的工具。
type DynToolProvider interface {
	DynTools() []Tool
}

// PromptSectionProvider 提供系统提示词自定义段。
type PromptSectionProvider interface {
	PromptSections() []PromptSection
}

// EventListener 监听事件。
type EventListener interface {
	OnEvent(ctx context.Context, evt Event)
}

// PromptSection 系统提示词的自定义段。
type PromptSection struct {
	Title string
	Body  string
}
