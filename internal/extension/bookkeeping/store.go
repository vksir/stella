// Package bookkeeping 实现记账扩展：账单、类别、周期规则与报表。
package bookkeeping

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// defaultCategories 预置类别。
var defaultCategories = []string{"餐饮", "交通", "购物", "娱乐", "医疗", "居住", "教育", "工资", "退款", "其他"}

// Store 记账数据存储。
type Store struct {
	db *sql.DB
}

// Entry 账单。
type Entry struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Type     string `json:"type"` // expense | income
	Category string `json:"category"`
	Amount   int64  `json:"amount"` // 分
	Note     string `json:"note"`
	Date     string `json:"date"` // YYYY-MM-DD
}

// Recurring 周期规则。
type Recurring struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
	Note     string `json:"note"`
	Period   string `json:"period"`   // daily | weekly | monthly | yearly
	NextDate string `json:"next_date"` // YYYY-MM-DD
}

// CategoryTotal 类别汇总。
type CategoryTotal struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

// Report 区间报表。
type Report struct {
	From         string          `json:"from"`
	To           string          `json:"to"`
	ExpenseTotal int64           `json:"expense_total"`
	IncomeTotal  int64           `json:"income_total"`
	ExpenseByCat []CategoryTotal `json:"expense_by_cat"`
	IncomeByCat  []CategoryTotal `json:"income_by_cat"`
}

// NewStore 初始化记账表并预置类别。
func NewStore(ctx context.Context, db *sql.DB) (*Store, error) {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS bookkeeping_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			category TEXT NOT NULL,
			amount INTEGER NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			date TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS bookkeeping_entries_user_date ON bookkeeping_entries(user_id, date);
		CREATE TABLE IF NOT EXISTS bookkeeping_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);
		CREATE TABLE IF NOT EXISTS bookkeeping_recurring (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			category TEXT NOT NULL,
			amount INTEGER NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			period TEXT NOT NULL,
			next_date TEXT NOT NULL
		);
	`); err != nil {
		return nil, fmt.Errorf("initialize bookkeeping store: %w", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bookkeeping_categories").Scan(&n); err != nil {
		return nil, fmt.Errorf("count bookkeeping categories: %w", err)
	}
	if n == 0 {
		for _, name := range defaultCategories {
			if _, err := db.ExecContext(ctx, "INSERT INTO bookkeeping_categories(name) VALUES (?)", name); err != nil {
				return nil, fmt.Errorf("seed bookkeeping category: %w", err)
			}
		}
	}
	return &Store{db: db}, nil
}

// Categories 返回全部类别。
func (s *Store) Categories(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name FROM bookkeeping_categories ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list bookkeeping categories: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// CategoryExists 判断类别是否存在。
func (s *Store) CategoryExists(ctx context.Context, name string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM bookkeeping_categories WHERE name = ?", name).Scan(&one)
	if errorsIsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query bookkeeping category: %w", err)
	}
	return true, nil
}

// AddCategory 新增类别。
func (s *Store) AddCategory(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO bookkeeping_categories(name) VALUES (?)", name); err != nil {
		return fmt.Errorf("add bookkeeping category: %w", err)
	}
	return nil
}

// RenameCategory 改名并级联更新账单与周期规则。
func (s *Store) RenameCategory(ctx context.Context, oldName, newName string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE bookkeeping_categories SET name = ? WHERE name = ?", newName, oldName); err != nil {
		return fmt.Errorf("rename bookkeeping category: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE bookkeeping_entries SET category = ? WHERE category = ?", newName, oldName); err != nil {
		return fmt.Errorf("cascade rename entries: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE bookkeeping_recurring SET category = ? WHERE category = ?", newName, oldName); err != nil {
		return fmt.Errorf("cascade rename recurring: %w", err)
	}
	return nil
}

// DeleteCategory 删除未被账单与周期规则引用的类别。
func (s *Store) DeleteCategory(ctx context.Context, name string) error {
	var used int
	if err := s.db.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM bookkeeping_entries WHERE category = ?) + (SELECT COUNT(*) FROM bookkeeping_recurring WHERE category = ?)",
		name, name).Scan(&used); err != nil {
		return fmt.Errorf("count category usage: %w", err)
	}
	if used > 0 {
		return fmt.Errorf("category %q is used by %d entries or rules, rename it instead", name, used)
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM bookkeeping_categories WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete bookkeeping category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("category %q not found", name)
	}
	return nil
}

// AddEntry 新增账单，返回账单 ID。
func (s *Store) AddEntry(ctx context.Context, e Entry) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO bookkeeping_entries(user_id, type, category, amount, note, date) VALUES (?, ?, ?, ?, ?, ?)",
		e.UserID, e.Type, e.Category, e.Amount, e.Note, e.Date)
	if err != nil {
		return 0, fmt.Errorf("add bookkeeping entry: %w", err)
	}
	return res.LastInsertId()
}

// UpdateEntry 按白名单字段部分更新账单。
func (s *Store) UpdateEntry(ctx context.Context, userID, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return fmt.Errorf("no fields to update")
	}
	var sets []string
	var args []any
	for _, col := range []string{"type", "category", "amount", "note", "date"} {
		if v, ok := fields[col]; ok {
			sets = append(sets, col+" = ?")
			args = append(args, v)
		}
	}
	args = append(args, userID, id)
	res, err := s.db.ExecContext(ctx,
		"UPDATE bookkeeping_entries SET "+strings.Join(sets, ", ")+" WHERE user_id = ? AND id = ?", args...)
	if err != nil {
		return fmt.Errorf("update bookkeeping entry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("entry %d not found", id)
	}
	return nil
}

// DeleteEntry 删除账单。
func (s *Store) DeleteEntry(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM bookkeeping_entries WHERE user_id = ? AND id = ?", userID, id)
	if err != nil {
		return fmt.Errorf("delete bookkeeping entry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("entry %d not found", id)
	}
	return nil
}

// ListEntries 返回区间内账单，按日期倒序。
func (s *Store) ListEntries(ctx context.Context, userID int64, from, to string, limit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, type, category, amount, note, date FROM bookkeeping_entries WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date DESC, id DESC LIMIT ?",
		userID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("list bookkeeping entries: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Type, &e.Category, &e.Amount, &e.Note, &e.Date); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// AddRecurring 新增周期规则。
func (s *Store) AddRecurring(ctx context.Context, r Recurring) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO bookkeeping_recurring(user_id, type, category, amount, note, period, next_date) VALUES (?, ?, ?, ?, ?, ?, ?)",
		r.UserID, r.Type, r.Category, r.Amount, r.Note, r.Period, r.NextDate)
	if err != nil {
		return 0, fmt.Errorf("add bookkeeping recurring: %w", err)
	}
	return res.LastInsertId()
}

// ListRecurring 返回用户全部周期规则。
func (s *Store) ListRecurring(ctx context.Context, userID int64) ([]Recurring, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, type, category, amount, note, period, next_date FROM bookkeeping_recurring WHERE user_id = ? ORDER BY id",
		userID)
	if err != nil {
		return nil, fmt.Errorf("list bookkeeping recurring: %w", err)
	}
	defer rows.Close()
	var rules []Recurring
	for rows.Next() {
		var r Recurring
		if err := rows.Scan(&r.ID, &r.UserID, &r.Type, &r.Category, &r.Amount, &r.Note, &r.Period, &r.NextDate); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// DeleteRecurring 删除周期规则。
func (s *Store) DeleteRecurring(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM bookkeeping_recurring WHERE user_id = ? AND id = ?", userID, id)
	if err != nil {
		return fmt.Errorf("delete bookkeeping recurring: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("recurring %d not found", id)
	}
	return nil
}

// MaterializeRecurring 将到期周期规则物化为账单，每条规则最多补 500 期。
func (s *Store) MaterializeRecurring(ctx context.Context, userID int64, today time.Time) error {
	rules, err := s.ListRecurring(ctx, userID)
	if err != nil {
		return err
	}
	for _, r := range rules {
		next, err := time.ParseInLocation(dateLayout, r.NextDate, time.Local)
		if err != nil {
			continue
		}
		for i := 0; next.Before(today) || sameDay(next, today); i++ {
			if i >= 500 {
				break
			}
			if _, err := s.AddEntry(ctx, Entry{
				UserID: userID, Type: r.Type, Category: r.Category, Amount: r.Amount, Note: r.Note, Date: next.Format(dateLayout),
			}); err != nil {
				return err
			}
			next = advancePeriod(next, r.Period)
		}
		if _, err := s.db.ExecContext(ctx,
			"UPDATE bookkeeping_recurring SET next_date = ? WHERE id = ?", next.Format(dateLayout), r.ID); err != nil {
			return fmt.Errorf("advance bookkeeping recurring: %w", err)
		}
	}
	return nil
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func advancePeriod(t time.Time, period string) time.Time {
	switch period {
	case "daily":
		return t.AddDate(0, 0, 1)
	case "weekly":
		return t.AddDate(0, 0, 7)
	case "yearly":
		return t.AddDate(1, 0, 0)
	default: // monthly
		return t.AddDate(0, 1, 0)
	}
}

// Report 汇总区间报表。
func (s *Store) Report(ctx context.Context, userID int64, from, to string) (Report, error) {
	r := Report{From: from, To: to}
	rows, err := s.db.QueryContext(ctx,
		"SELECT type, category, SUM(amount) FROM bookkeeping_entries WHERE user_id = ? AND date >= ? AND date <= ? GROUP BY type, category ORDER BY SUM(amount) DESC",
		userID, from, to)
	if err != nil {
		return r, fmt.Errorf("aggregate bookkeeping report: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var typ, category string
		var sum int64
		if err := rows.Scan(&typ, &category, &sum); err != nil {
			return r, err
		}
		switch typ {
		case "expense":
			r.ExpenseTotal += sum
			r.ExpenseByCat = append(r.ExpenseByCat, CategoryTotal{Category: category, Amount: sum})
		case "income":
			r.IncomeTotal += sum
			r.IncomeByCat = append(r.IncomeByCat, CategoryTotal{Category: category, Amount: sum})
		}
	}
	return r, rows.Err()
}

func errorsIsNotFound(err error) bool {
	return err == sql.ErrNoRows
}
