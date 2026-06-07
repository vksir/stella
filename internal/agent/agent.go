// internal/agent/agent.go
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"stella/ent"
	"stella/entity"
	"stella/internal/llm"
	"stella/internal/memory"
	"stella/internal/tool"
	"stella/pkg/cfg"
	"stella/pkg/constant"
	"stella/pkg/database"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vksir/vkiss-lib/pkg/log"
	"github.com/vksir/vkiss-lib/pkg/registry"
	"github.com/vksir/vkiss-lib/pkg/util"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
)

const (
	MaxToolCallIterations = 10
	MaxMessagesPerSession = 20
	MaxHistoryMessages    = 20
	MaxMemoryResults      = 5
)

type Agent struct {
	rt     *util.Runtime
	logger *log.Logger
	chat   llm.LLM
}

func New(logger *log.Logger) (*Agent, error) {
	var err error
	var chatLLM llm.LLM
	for _, m := range cfg.G.Model {
		switch m.Use {
		case cfg.ModelUseChat:
			chatLLM, err = llm.New(m, logger)
			if err != nil {
				return nil, errutil.Wrap(err)
			}
		}
	}
	if chatLLM == nil {
		return nil, errors.New("no chat model found")
	}

	return &Agent{
		chat:   chatLLM,
		logger: logger.With("tag", "agent"),
	}, nil
}

func (a *Agent) Start(ctx context.Context) error {
	a.rt = util.NewRuntime(ctx)
	registry.Subscribe(constant.TopicAgentChat, constant.SubscriberAgent, a.registryCallback())
	return nil
}

func (a *Agent) Stop() {
	registry.Unsubscribe(constant.TopicAgentChat, constant.SubscriberAgent)
	a.rt.Cancel()
}

// Chat is the core conversation method.
// It receives user messages, runs LLM dialog (with tool calling loop), fills evt.Ans.
func (a *Agent) Chat(ctx context.Context, userID string, evt *entity.Event) error {
	// 1. GetOrCreateSession
	session, err := a.getOrCreateSession(ctx, userID, "api", evt.SessionID)
	if err != nil {
		return errutil.Wrap(err)
	}

	// 2. Retrieve context
	longTermMemories, err := a.retrieveMemories(ctx, userID, evt.AskText())
	if err != nil {
		a.logger.WarnC(ctx, "retrieve memories failed", "err", err)
	}

	shortTermHistory, err := a.getRecentMessages(ctx, session.ID)
	if err != nil {
		a.logger.WarnC(ctx, "get recent messages failed", "err", err)
	}

	// 3. Build messages for LLM
	messages := a.buildMessages(longTermMemories, shortTermHistory, evt.Ask)

	// 4. LLM tool calling loop
	responseChains, allChains, err := a.runToolCallingLoop(ctx, messages)
	if err != nil {
		return errutil.Wrap(err)
	}

	// 5. Save messages to DB
	err = a.saveMessages(ctx, session.ID, userID, evt.Ask, allChains)
	if err != nil {
		a.logger.ErrorC(ctx, "save messages failed", "err", err)
	}

	// 6. Clean old messages (keep max 20)
	err = a.cleanOldMessages(ctx, session.ID)
	if err != nil {
		a.logger.WarnC(ctx, "clean old messages failed", "err", err)
	}

	// 7. Async extract memories
	go a.asyncExtractMemories(userID, evt.AskText(), responseChains)

	// 8. Fill response
	evt.Ans = responseChains

	// 9. Update session updated_at
	database.G.Session.UpdateOneID(session.ID).
		SetUpdatedAt(time.Now()).
		Exec(ctx)

	return nil
}

func (a *Agent) registryCallback() registry.Callback {
	return func(ctx context.Context, msgAny any) error {
		msg, ok := msgAny.(*entity.Event)
		if !ok {
			return errutil.WrapErrType(msg)
		}
		return a.Chat(ctx, msg.UserID, msg)
	}
}

// getOrCreateSession gets or creates a session
func (a *Agent) getOrCreateSession(ctx context.Context, userID, platform, sessionID string) (*ent.Session, error) {
	s, err := database.G.Session.Get(ctx, sessionID)
	if err == nil {
		return s, nil
	}
	if !ent.IsNotFound(err) {
		return nil, errutil.Wrap(err)
	}

	create := database.G.Session.Create().
		SetID(sessionID).
		SetPlatform(platform).
		SetTitle("")

	if userID != "" {
		create.SetNillableUserID(&userID)
	}

	return create.Save(ctx)
}

// retrieveMemories retrieves relevant long-term memories
func (a *Agent) retrieveMemories(ctx context.Context, userID, query string) ([]*ent.Memory, error) {
	// Placeholder: use empty vector (L3 will implement actual embedding)
	queryVector := make([]float32, 768)
	return memory.RetrieveTopK(ctx, userID, queryVector, MaxMemoryResults)
}

// getRecentMessages gets recent conversation history
func (a *Agent) getRecentMessages(ctx context.Context, sessionID string) ([]*ent.Message, error) {
	return memory.GetRecentHistory(ctx, sessionID, MaxHistoryMessages)
}

// buildMessages builds the message list to send to LLM
func (a *Agent) buildMessages(memories []*ent.Memory, history []*ent.Message, userChains []entity.Chain) []entity.Chain {
	var messages []entity.Chain

	// System prompt + memories
	systemPrompt := "你是一个智能 AI 助手，能够记住对话上下文、调用工具提供帮助。\n请使用中文回答。"
	if len(memories) > 0 {
		systemPrompt += "\n\n关于用户的一些长期记忆："
		for _, m := range memories {
			systemPrompt += "\n- " + m.Content
		}
	}

	messages = append(messages, entity.Chain{
		Role: entity.ChainRoleSystem,
		Type: entity.ChainTypeText,
		Text: systemPrompt,
	})

	// History messages (reverse: oldest first)
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		c := entity.Chain{
			Role: msg.Role,
			Type: msg.Type,
			Text: msg.Content,
		}

		// Restore tool_call info
		if msg.Type == entity.ChainTypeToolCall && msg.ToolCalls != "" {
			var toolCalls []entity.ToolCall
			if err := json.Unmarshal([]byte(msg.ToolCalls), &toolCalls); err == nil {
				c.ToolCall = toolCalls
			}
		}

		// Restore tool_id
		if msg.Role == entity.ChainRoleToolResult {
			c.ToolID = msg.ToolID
		}

		messages = append(messages, c)
	}

	// User messages
	messages = append(messages, userChains...)

	return messages
}

// runToolCallingLoop executes the LLM tool calling loop
func (a *Agent) runToolCallingLoop(ctx context.Context, messages []entity.Chain) ([]entity.Chain, []entity.Chain, error) {
	var responseChains []entity.Chain
	var allChains []entity.Chain

	// Collect tools
	tools, err := a.collectTools(ctx)
	if err != nil {
		a.logger.WarnC(ctx, "collect tools failed", "err", err)
	}

	for i := 0; i < MaxToolCallIterations; i++ {
		resp, err := a.chat.Chat(ctx, messages, tools)
		if err != nil {
			return nil, nil, errutil.Wrap(err)
		}

		messages = append(messages, resp...)
		allChains = append(allChains, resp...)

		hasToolCall := false
		for _, c := range resp {
			if c.Type == entity.ChainTypeToolCall {
				hasToolCall = true

				toolResults, err := a.executeToolCalls(ctx, c.ToolCall)
				if err != nil {
					a.logger.ErrorC(ctx, "execute tool calls failed", "err", err)
				}

				for _, tr := range toolResults {
					messages = append(messages, tr)
					allChains = append(allChains, tr)
				}
			} else {
				responseChains = append(responseChains, c)
			}
		}

		if !hasToolCall {
			break
		}
	}

	if len(responseChains) == 0 {
		return nil, nil, fmt.Errorf("no text response from agent after %d iterations", MaxToolCallIterations)
	}

	return responseChains, allChains, nil
}

// collectTools collects MCP and local tools
func (a *Agent) collectTools(ctx context.Context) ([]mcp.Tool, error) {
	var allTools []mcp.Tool

	// MCP tools (not connected yet, placeholder)
	for _, mcpCfg := range cfg.G.Mcp {
		_ = mcpCfg
		// TODO: Connect to MCP servers and list tools
	}

	// Local tools
	for _, t := range tool.GetLocalTools() {
		allTools = append(allTools, mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		})
	}

	return allTools, nil
}

// executeToolCalls executes tool calls (local first, then MCP)
func (a *Agent) executeToolCalls(ctx context.Context, toolCalls []entity.ToolCall) ([]entity.Chain, error) {
	var results []entity.Chain
	for _, tc := range toolCalls {
		result, err := tool.Execute(tc.Func, tc.Args)
		if err != nil {
			result = fmt.Sprintf("tool '%s' execution error: %s", tc.Func, err.Error())
		}
		results = append(results, entity.Chain{
			Role:   entity.ChainRoleToolResult,
			Type:   entity.ChainTypeText,
			Text:   result,
			ToolID: tc.ID,
		})
	}
	return results, nil
}

// saveMessages saves messages to the database
func (a *Agent) saveMessages(ctx context.Context, sessionID, userID string, ask, allChains []entity.Chain) error {
	// Save user messages
	for _, c := range ask {
		_, err := database.G.Message.Create().
			SetSessionID(sessionID).
			SetNillableUserID(&userID).
			SetRole(entity.ChainRoleUser).
			SetType(c.Type).
			SetContent(c.Text).
			SetToolCalls("").
			SetToolID("").
			SetData(c.Data).
			Save(ctx)
		if err != nil {
			return errutil.Wrap(err)
		}
	}

	// Save agent/tool messages
	for _, c := range allChains {
		toolCallsJSON := ""
		if len(c.ToolCall) > 0 {
			data, _ := json.Marshal(c.ToolCall)
			toolCallsJSON = string(data)
		}

		_, err := database.G.Message.Create().
			SetSessionID(sessionID).
			SetNillableUserID(nil).
			SetRole(c.Role).
			SetType(c.Type).
			SetContent(c.Text).
			SetToolCalls(toolCallsJSON).
			SetToolID(c.ToolID).
			SetData(c.Data).
			Save(ctx)
		if err != nil {
			return errutil.Wrap(err)
		}
	}
	return nil
}

// cleanOldMessages deletes messages exceeding the limit
func (a *Agent) cleanOldMessages(ctx context.Context, sessionID string) error {
	count, err := database.G.Message.Query().Count(ctx)
	if err != nil {
		return errutil.Wrap(err)
	}

	if count <= MaxMessagesPerSession {
		return nil
	}

	toDelete := count - MaxMessagesPerSession
	oldMessages, err := database.G.Message.Query().
		Order(ent.Asc("created_at")).
		Limit(toDelete).
		All(ctx)
	if err != nil {
		return errutil.Wrap(err)
	}

	for _, msg := range oldMessages {
		err := database.G.Message.DeleteOne(msg).Exec(ctx)
		if err != nil {
			a.logger.WarnC(ctx, "delete old message failed", "id", msg.ID, "err", err)
		}
	}

	return nil
}

// asyncExtractMemories asynchronously extracts and stores memories
func (a *Agent) asyncExtractMemories(userID, userMsg string, responseChains []entity.Chain) {
	responseText := ""
	for _, c := range responseChains {
		if c.Type == entity.ChainTypeText {
			responseText += c.Text
		}
	}
	memory.AsyncExtractAndStore(userID, userMsg, responseText)
}
