package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vksir/stella/pkg/collection"
)

type dynGateway struct {
	tools *collection.OrderedMap[string, DynTool]
}

func newDynGateway(tools *collection.OrderedMap[string, DynTool]) *dynGateway {
	return &dynGateway{tools: tools}
}

func (t *dynGateway) Name() string {
	return "dyn"
}

func (t *dynGateway) Description() string {
	return "Inspect and execute extension tools listed in the system prompt catalog. Use tool_show to retrieve a tool's full description and parameter schema; omit arguments. Use tool_exec to execute a tool with arguments matching its schema; use {} for tools with no parameters. If the definition is already available in context, you may execute the tool directly. Do not guess parameters."
}

func (t *dynGateway) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"tool_show", "tool_exec"},
				"description": "Show a tool definition or execute a tool.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "The exact extension tool name from the system prompt catalog.",
			},
			"arguments": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"description":          "Tool arguments. Required for tool_exec.",
			},
		},
		"required":             []string{"action", "name"},
		"additionalProperties": false,
	}
}

func (t *dynGateway) Execute(ctx context.Context, args string) (Content, error) {
	var input struct {
		Action    string          `json:"action"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return nil, err
	}

	switch input.Action {
	case "tool_show":
		return t.toolShow(input.Name)
	case "tool_exec":
		return t.toolExec(ctx, input.Name, input.Arguments)
	default:
		return nil, fmt.Errorf("unknown action %q", input.Action)
	}
}

func (t *dynGateway) toolShow(name string) (Content, error) {
	tool, err := t.toolLookup(name)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	}{
		Name: name, 
		Description: tool.Description(), 
		Parameters: tool.Schema(),
	})
	if err != nil {
		return nil, fmt.Errorf("extension tool %q encode definition failed: %w", name, err)
	}
	return TextContent(string(result)), nil
}

func (t *dynGateway) toolExec(ctx context.Context, name string, arguments json.RawMessage) (Content, error) {
	tool, err := t.toolLookup(name)
	if err != nil {
		return nil, err
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}
	result, err := tool.Execute(ctx, string(arguments))
	if err != nil {
		return nil, fmt.Errorf("extension tool %q exec failed: %w", name, err)
	}
	return result, nil
}

func (t *dynGateway) toolLookup(name string) (DynTool, error) {
	tool, ok := t.tools.Get(name)
	if !ok {
		return nil, fmt.Errorf("extension tool %q not found", name)
	}
	return tool, nil
}
