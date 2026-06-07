// internal/tool/tool.go
package tool

import (
	"context"
	"fmt"
	"sync"
)

type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

var (
	mu         sync.RWMutex
	localTools []Tool
)

func Register(t Tool) {
	mu.Lock()
	defer mu.Unlock()
	localTools = append(localTools, t)
}

func GetLocalTools() []Tool {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Tool, len(localTools))
	copy(result, localTools)
	return result
}

func Execute(name string, args map[string]any) (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	for _, t := range localTools {
		if t.Name == name {
			return t.Handler(context.Background(), args)
		}
	}
	return "", fmt.Errorf("tool not found: %s", name)
}
