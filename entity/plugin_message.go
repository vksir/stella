// entity/plugin_message.go
package entity

// PluginMessage 是旧插件系统中使用的消息类型，桥接到 entity.Chain。
// 后续插件转为 MCP tool 后删除此文件。
type PluginMessage struct {
	Chains []Chain
}

func (m *PluginMessage) Text() string {
	sb := ""
	for _, c := range m.Chains {
		if c.Type == ChainTypeText {
			sb += c.Text
		}
	}
	return sb
}
