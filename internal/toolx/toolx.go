// Package toolx 提供 read/write/edit/bash 工具。
//
// read 按 2000 行/50KB 截断、支持 offset/limit 与常见图片格式检测；
// edit 支持多块精确替换、模糊匹配、BOM 与行尾保留；
// bash 按尾部 2000 行/50KB 截断并将完整输出落临时文件。
package toolx

import "strings"

// strProp / numProp 构造与 typebox Type.String/Type.Number 等价的属性定义。
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

// objectSchema 构造与 typebox Type.Object 等价的属性对象定义。
func objectSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// splitLines 按行拆分，末尾换行符不产生空行。
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}
