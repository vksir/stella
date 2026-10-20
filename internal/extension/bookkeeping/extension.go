package bookkeeping

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/vksir/stella/internal/core/agent"
)

// Extension 记账扩展，绑定单用户。
type Extension struct {
	store     *Store
	userID    int64
	sendImage func([]byte)
}

// New 构造绑定用户的记账扩展；sendImage 直发图片给用户，不进 LLM 会话。
func New(store *Store, userID int64, sendImage func([]byte)) *Extension {
	return &Extension{store: store, userID: userID, sendImage: sendImage}
}

func (e *Extension) Name() string { return "bookkeeping" }

func (e *Extension) Tools() []agent.Tool {
	return []agent.Tool{&entryTool{ext: e}, &categoryTool{ext: e}, &reportTool{ext: e}}
}

func fmtYuan(cents int64) string {
	return fmt.Sprintf("¥%.2f", float64(cents)/100)
}

func today() string { return time.Now().Format(dateLayout) }

// entryTool 账单与周期规则增删改。
type entryTool struct {
	ext *Extension
}

const entryDescription = "Manage bookkeeping entries and recurring rules for the user. Actions: add (create an expense or income entry; amount in yuan; date defaults to today; refunds are income entries with category 退款, referencing the original entry id in the note), update (change fields of an entry by id), delete, add_recurring (periodic entry generated at query time; period: daily|weekly|monthly|yearly), list_recurring, delete_recurring."

const entrySummary = "Add, update, or delete bookkeeping entries and recurring rules."

func (t *entryTool) Name() string        { return "bookkeeping" }
func (t *entryTool) Description() string { return entryDescription }
func (t *entryTool) Summary() string     { return entrySummary }

func (t *entryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"add", "update", "delete", "add_recurring", "list_recurring", "delete_recurring"},
				"description": "Entry or recurring rule action.",
			},
			"id": map[string]any{"type": "number", "description": "Entry or recurring id, required for update/delete/delete_recurring."},
			"type": map[string]any{"type": "string", "enum": []string{"expense", "income"}, "description": "Entry type."},
			"category": map[string]any{"type": "string", "description": "Category name, must exist."},
			"amount": map[string]any{"type": "number", "description": "Amount in yuan, positive."},
			"note":    map[string]any{"type": "string", "description": "Optional note."},
			"date":    map[string]any{"type": "string", "description": "Entry date, YYYY-MM-DD, defaults to today."},
			"period": map[string]any{
				"type":        "string",
				"enum":        []string{"daily", "weekly", "monthly", "yearly"},
				"description": "Recurring period, required for add_recurring.",
			},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
}

type entryArgs struct {
	Action   string   `json:"action"`
	ID       int64    `json:"id"`
	Type     string   `json:"type"`
	Category string   `json:"category"`
	Amount   *float64 `json:"amount"`
	Note     *string  `json:"note"`
	Date     string   `json:"date"`
	Period   string   `json:"period"`
}

func (t *entryTool) Execute(ctx context.Context, args string) (agent.Content, error) {
	var a entryArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return nil, fmt.Errorf("invalid bookkeeping arguments: %w", err)
	}
	switch a.Action {
	case "add":
		return t.add(ctx, a)
	case "update":
		return t.update(ctx, a)
	case "delete":
		if err := t.ext.store.DeleteEntry(ctx, t.ext.userID, a.ID); err != nil {
			return nil, err
		}
		return agent.TextContent(fmt.Sprintf("已删除账单 %d", a.ID)), nil
	case "add_recurring":
		return t.addRecurring(ctx, a)
	case "list_recurring":
		return t.listRecurring(ctx)
	case "delete_recurring":
		if err := t.ext.store.DeleteRecurring(ctx, t.ext.userID, a.ID); err != nil {
			return nil, err
		}
		return agent.TextContent(fmt.Sprintf("已删除周期规则 %d", a.ID)), nil
	default:
		return nil, fmt.Errorf("unknown action %q", a.Action)
	}
}

func (t *entryTool) validateType(typ string) error {
	if typ != "expense" && typ != "income" {
		return fmt.Errorf("invalid type %q, want expense or income", typ)
	}
	return nil
}

func (t *entryTool) validateCategory(ctx context.Context, name string) error {
	exists, err := t.ext.store.CategoryExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("category %q does not exist, list categories with bookkeeping_category first", name)
	}
	return nil
}

func (t *entryTool) normalizeDate(date string) (string, error) {
	if date == "" {
		return today(), nil
	}
	if _, err := time.ParseInLocation(dateLayout, date, time.Local); err != nil {
		return "", fmt.Errorf("invalid date %q, want YYYY-MM-DD", date)
	}
	return date, nil
}

func (t *entryTool) cents(amount *float64) (int64, error) {
	if amount == nil || *amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	return int64(math.Round(*amount * 100)), nil
}

func (t *entryTool) add(ctx context.Context, a entryArgs) (agent.Content, error) {
	if err := t.validateType(a.Type); err != nil {
		return nil, err
	}
	if err := t.validateCategory(ctx, a.Category); err != nil {
		return nil, err
	}
	amount, err := t.cents(a.Amount)
	if err != nil {
		return nil, err
	}
	date, err := t.normalizeDate(a.Date)
	if err != nil {
		return nil, err
	}
	note := ""
	if a.Note != nil {
		note = *a.Note
	}
	id, err := t.ext.store.AddEntry(ctx, Entry{
		UserID: t.ext.userID, Type: a.Type, Category: a.Category, Amount: amount, Note: note, Date: date,
	})
	if err != nil {
		return nil, err
	}
	return agent.TextContent(fmt.Sprintf("已记录%s %s %s（%s，id=%d）", typeLabel(a.Type), a.Category, fmtYuan(amount), date, id)), nil
}

func (t *entryTool) update(ctx context.Context, a entryArgs) (agent.Content, error) {
	fields := map[string]any{}
	if a.Type != "" {
		if err := t.validateType(a.Type); err != nil {
			return nil, err
		}
		fields["type"] = a.Type
	}
	if a.Category != "" {
		if err := t.validateCategory(ctx, a.Category); err != nil {
			return nil, err
		}
		fields["category"] = a.Category
	}
	if a.Amount != nil {
		amount, err := t.cents(a.Amount)
		if err != nil {
			return nil, err
		}
		fields["amount"] = amount
	}
	if a.Note != nil {
		fields["note"] = *a.Note
	}
	if a.Date != "" {
		date, err := t.normalizeDate(a.Date)
		if err != nil {
			return nil, err
		}
		fields["date"] = date
	}
	if err := t.ext.store.UpdateEntry(ctx, t.ext.userID, a.ID, fields); err != nil {
		return nil, err
	}
	return agent.TextContent(fmt.Sprintf("已更新账单 %d", a.ID)), nil
}

func (t *entryTool) addRecurring(ctx context.Context, a entryArgs) (agent.Content, error) {
	if err := t.validateType(a.Type); err != nil {
		return nil, err
	}
	if err := t.validateCategory(ctx, a.Category); err != nil {
		return nil, err
	}
	amount, err := t.cents(a.Amount)
	if err != nil {
		return nil, err
	}
	switch a.Period {
	case "daily", "weekly", "monthly", "yearly":
	default:
		return nil, fmt.Errorf("invalid period %q", a.Period)
	}
	date, err := t.normalizeDate(a.Date)
	if err != nil {
		return nil, err
	}
	note := ""
	if a.Note != nil {
		note = *a.Note
	}
	id, err := t.ext.store.AddRecurring(ctx, Recurring{
		UserID: t.ext.userID, Type: a.Type, Category: a.Category, Amount: amount,
		Note: note, Period: a.Period, NextDate: date,
	})
	if err != nil {
		return nil, err
	}
	return agent.TextContent(fmt.Sprintf("已添加周期规则 %d：%s %s %s/%s（自 %s 起，查询时生效）",
		id, typeLabel(a.Type), a.Category, fmtYuan(amount), periodLabel(a.Period), date)), nil
}

func (t *entryTool) listRecurring(ctx context.Context) (agent.Content, error) {
	rules, err := t.ext.store.ListRecurring(ctx, t.ext.userID)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return agent.TextContent("暂无周期规则"), nil
	}
	lines := "周期规则："
	for _, r := range rules {
		lines += fmt.Sprintf("\n%d | %s | %s | %s/%s | 自 %s 起 | %s", r.ID, typeLabel(r.Type), r.Category, fmtYuan(r.Amount), periodLabel(r.Period), r.NextDate, r.Note)
	}
	return agent.TextContent(lines), nil
}

func typeLabel(typ string) string {
	if typ == "income" {
		return "收入"
	}
	return "支出"
}

func periodLabel(period string) string {
	switch period {
	case "daily":
		return "每天"
	case "weekly":
		return "每周"
	case "yearly":
		return "每年"
	default:
		return "每月"
	}
}

// categoryTool 类别增删改查。
type categoryTool struct {
	ext *Extension
}

const categoryDescription = "Manage bookkeeping categories: list, add, rename (cascades to entries and recurring rules), delete (only unused categories can be deleted)."

const categorySummary = "List, add, rename, or delete bookkeeping categories."

func (t *categoryTool) Name() string        { return "bookkeeping_category" }
func (t *categoryTool) Description() string { return categoryDescription }
func (t *categoryTool) Summary() string     { return categorySummary }

func (t *categoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":   map[string]any{"type": "string", "enum": []string{"list", "add", "rename", "delete"}},
			"name":     map[string]any{"type": "string", "description": "Category name."},
			"new_name": map[string]any{"type": "string", "description": "New name, required for rename."},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
}

type categoryArgs struct {
	Action  string `json:"action"`
	Name    string `json:"name"`
	NewName string `json:"new_name"`
}

func (t *categoryTool) Execute(ctx context.Context, args string) (agent.Content, error) {
	var a categoryArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return nil, fmt.Errorf("invalid bookkeeping_category arguments: %w", err)
	}
	switch a.Action {
	case "list":
		names, err := t.ext.store.Categories(ctx)
		if err != nil {
			return nil, err
		}
		return agent.TextContent("类别：" + joinNames(names)), nil
	case "add":
		if a.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if err := t.ext.store.AddCategory(ctx, a.Name); err != nil {
			return nil, err
		}
		return agent.TextContent(fmt.Sprintf("已添加类别 %s", a.Name)), nil
	case "rename":
		if a.Name == "" || a.NewName == "" {
			return nil, fmt.Errorf("name and new_name are required")
		}
		if err := t.ext.store.RenameCategory(ctx, a.Name, a.NewName); err != nil {
			return nil, err
		}
		return agent.TextContent(fmt.Sprintf("已将类别 %s 改名为 %s", a.Name, a.NewName)), nil
	case "delete":
		if a.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if err := t.ext.store.DeleteCategory(ctx, a.Name); err != nil {
			return nil, err
		}
		return agent.TextContent(fmt.Sprintf("已删除类别 %s", a.Name)), nil
	default:
		return nil, fmt.Errorf("unknown action %q", a.Action)
	}
}

func joinNames(names []string) string {
	out := ""
	for i, name := range names {
		if i > 0 {
			out += "、"
		}
		out += name
	}
	return out
}

// reportTool 报表与明细查询。
type reportTool struct {
	ext *Extension
}

const reportDescription = "Generate bookkeeping reports. Recurring rules are materialized first. Action report renders a chart image, sends it directly to the user, and returns a text summary; action detail lists entries in the range. period is day, week, month, or year around the anchor date (default today, period defaults to month)."

const reportSummary = "Daily, weekly, monthly, and yearly bookkeeping reports with a chart image."

func (t *reportTool) Name() string        { return "bookkeeping_report" }
func (t *reportTool) Description() string { return reportDescription }
func (t *reportTool) Summary() string     { return reportSummary }

func (t *reportTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"report", "detail"}, "description": "Report or entry detail."},
			"period": map[string]any{"type": "string", "enum": []string{"day", "week", "month", "year"}, "description": "Report period, default month."},
			"date":   map[string]any{"type": "string", "description": "Anchor date, YYYY-MM-DD, default today."},
			"limit":  map[string]any{"type": "number", "description": "Max entries for detail, default 20."},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
}

type reportArgs struct {
	Action string `json:"action"`
	Period string `json:"period"`
	Date   string `json:"date"`
	Limit  int    `json:"limit"`
}

func (t *reportTool) Execute(ctx context.Context, args string) (agent.Content, error) {
	var a reportArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return nil, fmt.Errorf("invalid bookkeeping_report arguments: %w", err)
	}
	if a.Period == "" {
		a.Period = "month"
	}
	anchor := time.Now()
	if a.Date != "" {
		parsed, err := time.ParseInLocation(dateLayout, a.Date, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q, want YYYY-MM-DD", a.Date)
		}
		anchor = parsed
	}
	if err := t.ext.store.MaterializeRecurring(ctx, t.ext.userID, time.Now()); err != nil {
		return nil, err
	}

	from, to, _, _ := reportRange(anchor, a.Period)
	switch a.Action {
	case "report":
		png, text, err := t.ext.renderReport(ctx, a.Period, anchor)
		if err != nil {
			return nil, err
		}
		if png != nil && t.ext.sendImage != nil {
			t.ext.sendImage(png)
			text += "\n图表已发送给用户。"
		}
		return agent.TextContent(text), nil
	case "detail":
		return t.detail(ctx, from, to, a.Limit)
	default:
		return nil, fmt.Errorf("unknown action %q", a.Action)
	}
}

func (t *reportTool) detail(ctx context.Context, from, to string, limit int) (agent.Content, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	entries, err := t.ext.store.ListEntries(ctx, t.ext.userID, from, to, limit)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return agent.TextContent(fmt.Sprintf("%s ~ %s：暂无账单", shortDate(from), shortDate(to))), nil
	}
	text := fmt.Sprintf("账单明细（%s ~ %s，最多 %d 条）：", shortDate(from), shortDate(to), limit)
	for _, e := range entries {
		text += fmt.Sprintf("\n%d | %s | %s | %s | %s | %s", e.ID, e.Date, typeLabel(e.Type), e.Category, yuanStr(e.Amount), e.Note)
	}
	return agent.TextContent(text), nil
}
