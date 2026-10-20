package bookkeeping

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// reportAgg 区间账单聚合。
type reportAgg struct {
	entries  []Entry
	expense  int64
	income   int64
	expByCat map[string]int64
}

func aggregateEntries(entries []Entry) reportAgg {
	agg := reportAgg{entries: entries, expByCat: map[string]int64{}}
	for _, e := range entries {
		switch e.Type {
		case "expense":
			agg.expense += e.Amount
			agg.expByCat[e.Category] += e.Amount
		case "income":
			agg.income += e.Amount
		}
	}
	return agg
}

// reportRange 计算锚点所在周期的起止日期与标题。
func reportRange(anchor time.Time, period string) (from, to, label, kind string) {
	switch period {
	case "day":
		d := anchor.Format(dateLayout)
		return d, d, anchor.Format("2006-01-02") + " 日报", "day"
	case "week":
		weekday := int(anchor.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := anchor.AddDate(0, 0, 1-weekday)
		end := start.AddDate(0, 0, 6)
		label = anchor.Format("2006-01") + fmt.Sprintf(" 第%d周 周报", (anchor.Day()-1)/7+1)
		return start.Format(dateLayout), end.Format(dateLayout), label, "week"
	case "year":
		start := time.Date(anchor.Year(), 1, 1, 0, 0, 0, 0, time.Local)
		end := start.AddDate(1, 0, 0).AddDate(0, 0, -1)
		return start.Format(dateLayout), end.Format(dateLayout), anchor.Format("2006") + " 年报", "year"
	default:
		start := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.Local)
		end := start.AddDate(0, 1, 0).AddDate(0, 0, -1)
		return start.Format(dateLayout), end.Format(dateLayout), anchor.Format("2006-01") + " 月报", "month"
	}
}

// renderReport 组装报表并渲染 PNG；无账单时 png 为 nil。
func (e *Extension) renderReport(ctx context.Context, period string, anchor time.Time) ([]byte, string, error) {
	from, to, label, kind := reportRange(anchor, period)
	entries, err := e.store.ListEntries(ctx, e.userID, from, to, 100000)
	if err != nil {
		return nil, "", err
	}
	if len(entries) == 0 {
		return nil, fmt.Sprintf("%s（%s ~ %s）：暂无账单", label, shortDate(from), shortDate(to)), nil
	}
	agg := aggregateEntries(entries)

	prevAnchor := anchor
	switch kind {
	case "day":
		prevAnchor = anchor.AddDate(0, 0, -1)
	case "week":
		prevAnchor = anchor.AddDate(0, 0, -7)
	case "month":
		prevAnchor = anchor.AddDate(0, -1, 0)
	case "year":
		prevAnchor = anchor.AddDate(-1, 0, 0)
	}
	prevFrom, prevTo, _, _ := reportRange(prevAnchor, period)
	prevEntries, err := e.store.ListEntries(ctx, e.userID, prevFrom, prevTo, 100000)
	if err != nil {
		return nil, "", err
	}
	prev := aggregateEntries(prevEntries)

	var month reportAgg
	if kind == "day" {
		mf, mt, _, _ := reportRange(anchor, "month")
		me, err := e.store.ListEntries(ctx, e.userID, mf, mt, 100000)
		if err != nil {
			return nil, "", err
		}
		month = aggregateEntries(me)
	}

	m := buildDrawReport(kind, label, from, to, anchor, agg, prev, month)
	png, err := DrawReportPNG(m)
	if err != nil {
		return nil, "", err
	}
	return png, buildSummaryText(kind, label, from, to, agg), nil
}

var weekdayNames = []string{"日", "一", "二", "三", "四", "五", "六"}

// buildDrawReport 将聚合数据组装为绘制模型。
func buildDrawReport(kind, label, from, to string, anchor time.Time, agg, prev, month reportAgg) *DrawReport {
	m := &DrawReport{Title: label}
	if kind == "day" {
		m.Subtitle = fmt.Sprintf("%s · 星期%s · %d 笔", from, weekdayNames[anchor.Weekday()], len(agg.entries))
	} else {
		m.Subtitle = fmt.Sprintf("%s ~ %s · %d 笔", shortDate(from), shortDate(to), len(agg.entries))
	}
	m.ShowPie = kind != "day"

	prefix := map[string]string{"day": "当日", "week": "本周", "month": "本月", "year": "年度"}[kind]
	incomeCount := 0
	for _, e := range agg.entries {
		if e.Type == "income" {
			incomeCount++
		}
	}
	incomeCaption := "暂无入账"
	if incomeCount > 0 {
		incomeCaption = fmt.Sprintf("%d 笔入账", incomeCount)
	}
	balanceCaption := "收入为 0"
	if agg.income > 0 {
		balanceCaption = fmt.Sprintf("结余率 %.0f%%", float64(agg.income-agg.expense)/float64(agg.income)*100)
	}

	avgLabel, avgValue, avgCaption := avgCard(kind, from, to, anchor, agg, month)
	m.Cards = [4]DrawCard{
		{Label: prefix + "支出", Value: yuanStr(agg.expense), Caption: diffCaption(kind, agg.expense, prev.expense), Color: colExpense},
		{Label: prefix + "收入", Value: yuanStr(agg.income), Caption: incomeCaption, Color: colIncome},
		{Label: prefix + "结余", Value: signedYuan(agg.income - agg.expense), Caption: balanceCaption, Color: colBalance},
		{Label: avgLabel, Value: avgValue, Caption: avgCaption, Color: colAccent},
	}

	// 分类明细（降序）
	cats := sortedCats(agg)
	m.Cats = make([]DrawCat, len(cats))
	var maxCat int64
	if len(cats) > 0 {
		maxCat = cats[0].amount
	}
	for i, c := range cats {
		ratio := 0.0
		if maxCat > 0 {
			ratio = float64(c.amount) / float64(maxCat)
		}
		m.Cats[i] = DrawCat{Name: c.name, Amount: yuanStr(c.amount), Pct: pctStr(c.amount, agg.expense), Ratio: ratio}
	}

	buildBars(m, kind, from, to, anchor, agg, cats)
	buildTops(m, kind, agg)
	return m
}

func avgCard(kind, from, to string, anchor time.Time, agg, month reportAgg) (label, value, caption string) {
	switch kind {
	case "day":
		days := periodDays(from, to, time.Now())
		return "本月日均", yuanStr(month.expense / int64(days)), fmt.Sprintf("按 %d 天平均", days)
	case "week":
		return "本周日均", yuanStr(agg.expense / 7), "按 7 天平均"
	case "month":
		days := periodDays(from, to, time.Now())
		return "本月日均", yuanStr(agg.expense / int64(days)), fmt.Sprintf("按 %d 天平均", days)
	default:
		months := int(anchor.Month())
		if to < time.Now().Format(dateLayout) {
			months = 12
		}
		return "年度月均", yuanStr(agg.expense / int64(months)), fmt.Sprintf("按 %d 月平均", months)
	}
}

// periodDays 完整过去的周期返回总天数，进行中的返回截至今天的天数。
func periodDays(from, to string, now time.Time) int {
	f, _ := time.ParseInLocation(dateLayout, from, time.Local)
	t, _ := time.ParseInLocation(dateLayout, to, time.Local)
	if t.Before(now) {
		return int(t.Sub(f).Hours()/24) + 1
	}
	today, _ := time.ParseInLocation(dateLayout, now.Format(dateLayout), time.Local)
	return int(today.Sub(f).Hours()/24) + 1
}

func diffCaption(kind string, cur, prev int64) string {
	base := "上期"
	if kind == "day" {
		base = "昨日"
	}
	if prev == 0 {
		return base + "无支出"
	}
	pct := float64(cur-prev) / float64(prev) * 100
	return fmt.Sprintf("较%s %+.0f%%", base, pct)
}

type catAmount struct {
	name   string
	amount int64
}

func sortedCats(agg reportAgg) []catAmount {
	cats := make([]catAmount, 0, len(agg.expByCat))
	for name, amount := range agg.expByCat {
		cats = append(cats, catAmount{name, amount})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].amount > cats[j].amount })
	return cats
}

// buildBars 按周期组装条形图数据。
func buildBars(m *DrawReport, kind, from, to string, anchor time.Time, agg reportAgg, cats []catAmount) {
	switch kind {
	case "day":
		m.BarTitle = "当日支出分类"
		for _, c := range cats {
			m.BarLabels = append(m.BarLabels, c.name)
			m.BarValues = append(m.BarValues, float64(c.amount)/100)
			m.BarValueLabels = append(m.BarValueLabels, "¥"+commaInt(c.amount/100))
		}
	case "week":
		m.BarTitle = "每日支出"
		daySums := map[string]int64{}
		for _, e := range agg.entries {
			if e.Type == "expense" {
				daySums[e.Date] += e.Amount
			}
		}
		f, _ := time.ParseInLocation(dateLayout, from, time.Local)
		for i := 0; i < 7; i++ {
			d := f.AddDate(0, 0, i)
			sum := daySums[d.Format(dateLayout)]
			m.BarLabels = append(m.BarLabels, weekdayNames[d.Weekday()])
			m.BarValues = append(m.BarValues, float64(sum)/100)
			m.BarValueLabels = append(m.BarValueLabels, commaInt(sum/100))
		}
		m.Avg = float64(agg.expense) / 100 / 7
		m.AvgLabel = "日均 " + commaInt(agg.expense/7/100)
	case "month":
		m.BarTitle = "每周支出"
		toT, _ := time.ParseInLocation(dateLayout, to, time.Local)
		weekCount := (toT.Day()-1)/7 + 1
		weekSums := make([]int64, weekCount)
		for _, e := range agg.entries {
			if e.Type != "expense" {
				continue
			}
			day, _ := time.ParseInLocation(dateLayout, e.Date, time.Local)
			weekSums[(day.Day()-1)/7] += e.Amount
		}
		for i, sum := range weekSums {
			m.BarLabels = append(m.BarLabels, fmt.Sprintf("第%d周", i+1))
			m.BarValues = append(m.BarValues, float64(sum)/100)
			m.BarValueLabels = append(m.BarValueLabels, commaInt(sum/100))
		}
		m.Avg = float64(agg.expense) / 100 / float64(weekCount)
		m.AvgLabel = "周均 " + commaInt(agg.expense/int64(weekCount)/100)
	default:
		m.BarTitle = "月度支出"
		monthSums := make([]int64, 12)
		for _, e := range agg.entries {
			if e.Type != "expense" {
				continue
			}
			d, _ := time.ParseInLocation(dateLayout, e.Date, time.Local)
			monthSums[int(d.Month())-1] += e.Amount
		}
		var maxV int64
		for i, sum := range monthSums {
			m.BarLabels = append(m.BarLabels, fmt.Sprintf("%d月", i+1))
			m.BarValues = append(m.BarValues, float64(sum)/100)
			if sum > maxV {
				maxV = sum
			}
		}
		if maxV > 0 {
			m.BarMaxLabel = commaInt(maxV / 100)
		}
		months := int(anchor.Month())
		if to < time.Now().Format(dateLayout) {
			months = 12
		}
		m.Avg = float64(agg.expense) / 100 / float64(months)
		m.AvgLabel = "月均 " + commaInt(agg.expense/int64(months)/100)
	}
}

// buildTops 组装最高消费列表。
func buildTops(m *DrawReport, kind string, agg reportAgg) {
	limit := map[string]int{"day": 20, "week": 5, "month": 10, "year": 10}[kind]
	expenses := make([]Entry, 0, len(agg.entries))
	for _, e := range agg.entries {
		if e.Type == "expense" {
			expenses = append(expenses, e)
		}
	}
	sort.Slice(expenses, func(i, j int) bool { return expenses[i].Amount > expenses[j].Amount })
	if len(expenses) > limit {
		expenses = expenses[:limit]
	}
	if kind == "day" {
		m.TopTitle = "当日账单"
	} else {
		m.TopTitle = fmt.Sprintf("最高消费 Top %d", limit)
	}
	for _, e := range expenses {
		note := e.Note
		if note == "" {
			note = "—"
		}
		m.Tops = append(m.Tops, DrawEntry{Date: shortDate(e.Date), Category: e.Category, Note: note, Amount: yuanStr(e.Amount)})
	}
}

// buildSummaryText 生成给模型的文字摘要。
func buildSummaryText(kind, label, from, to string, agg reportAgg) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s（%s ~ %s，%d 笔）\n", label, shortDate(from), shortDate(to), len(agg.entries)))
	b.WriteString(fmt.Sprintf("支出 %s | 收入 %s | 结余 %s\n", yuanStr(agg.expense), yuanStr(agg.income), signedYuan(agg.income-agg.expense)))
	for i, c := range sortedCats(agg) {
		if i >= 3 {
			break
		}
		if i > 0 {
			b.WriteString("、")
		}
		b.WriteString(fmt.Sprintf("%s %s（%s）", c.name, yuanStr(c.amount), pctStr(c.amount, agg.expense)))
	}
	return b.String()
}

func shortDate(date string) string {
	if len(date) >= 10 {
		return date[5:10]
	}
	return date
}

func pctStr(part, total int64) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(part)/float64(total)*100)
}

func signedYuan(cents int64) string {
	if cents >= 0 {
		return "+" + yuanStr(cents)
	}
	return "-" + yuanStr(-cents)
}

func yuanStr(cents int64) string {
	return fmt.Sprintf("¥%s.%02d", commaInt(cents/100), cents%100)
}

// commaInt 千分位整数。
func commaInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		out = "-" + out
	}
	return out
}
