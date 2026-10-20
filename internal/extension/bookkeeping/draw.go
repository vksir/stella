// draw.go 使用 gg 纯 Go 绘制报表图片。
package bookkeeping

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

// 报表画布宽度（物理像素，2x）
const (
	drawWidth  = 1640
	drawLeft   = 60
	drawRight  = 1640 - 60
)

// 报表配色
var (
	palette = []color.RGBA{
		{0x54, 0x70, 0xc6, 0xff}, {0x91, 0xcc, 0x75, 0xff}, {0xfa, 0xc8, 0x58, 0xff},
		{0xee, 0x66, 0x66, 0xff}, {0x73, 0xc0, 0xde, 0xff}, {0x3b, 0xa2, 0x72, 0xff},
	}
	colText      = color.RGBA{0x33, 0x33, 0x33, 0xff}
	colSub       = color.RGBA{0x8a, 0x93, 0xa6, 0xff}
	colGray      = color.RGBA{0x6b, 0x72, 0x80, 0xff}
	colLightGray = color.RGBA{0x9a, 0xa3, 0xb2, 0xff}
	colCardBg    = color.RGBA{0xf5, 0xf7, 0xfa, 0xff}
	colTrack     = color.RGBA{0xee, 0xf1, 0xf6, 0xff}
	colZebra     = color.RGBA{0xfa, 0xfb, 0xfd, 0xff}
	colGrid      = color.RGBA{0xec, 0xef, 0xf3, 0xff}
	colExpense   = color.RGBA{0xee, 0x66, 0x66, 0xff}
	colIncome    = color.RGBA{0x91, 0xcc, 0x75, 0xff}
	colBalance   = color.RGBA{0x54, 0x70, 0xc6, 0xff}
	colAccent    = color.RGBA{0x18, 0x8d, 0xf0, 0xff}
	colLeader    = color.RGBA{0xc3, 0xc9, 0xd4, 0xff}
)

// fontCandidates CJK 字体候选（常规/粗体），按优先级尝试。
var (
	regularFontCandidates = []string{"Deng.ttf", "simhei.ttf", "msyh.ttf"}
	boldFontCandidates    = []string{"Dengb.ttf", "simhei.ttf"}
)

// faceCache 按字号缓存字体。
type faceCache struct {
	regular, bold string
	faces         map[string]font.Face
}

var faces = &faceCache{faces: map[string]font.Face{}}

func fontDir() string {
	if dir := os.Getenv("WINDIR"); dir != "" {
		return filepath.Join(dir, "Fonts")
	}
	return "/usr/share/fonts"
}

func (c *faceCache) load(candidates []string, size float64) (font.Face, error) {
	key := fmt.Sprintf("%v:%v", candidates[0], size)
	if f, ok := c.faces[key]; ok {
		return f, nil
	}
	for _, name := range candidates {
		path := filepath.Join(fontDir(), name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if f, err := gg.LoadFontFace(path, size); err == nil {
			c.faces[key] = f
			return f, nil
		}
	}
	return nil, fmt.Errorf("no usable CJK font found in %s", fontDir())
}

func (c *faceCache) regularFace(dc *gg.Context, size float64) error {
	if c.regular == "" {
		for _, name := range regularFontCandidates {
			if _, err := os.Stat(filepath.Join(fontDir(), name)); err == nil {
				c.regular = name
				break
			}
		}
	}
	if c.bold == "" {
		for _, name := range boldFontCandidates {
			if _, err := os.Stat(filepath.Join(fontDir(), name)); err == nil {
				c.bold = name
				break
			}
		}
	}
	f, err := c.load([]string{c.regular}, size)
	if err != nil {
		return err
	}
	dc.SetFontFace(f)
	return nil
}

func (c *faceCache) boldFace(dc *gg.Context, size float64) error {
	if c.bold == "" {
		_ = c.regularFace(dc, size)
	}
	f, err := c.load([]string{c.bold}, size)
	if err != nil {
		return c.regularFace(dc, size)
	}
	dc.SetFontFace(f)
	return nil
}

// DrawCard 总览卡片。
type DrawCard struct {
	Label, Value, Caption string
	Color                 color.RGBA
}

// DrawCat 分类明细。
type DrawCat struct {
	Name, Amount, Pct string
	Ratio             float64 // 相对最高类，0~1
}

// DrawEntry 最高消费条目。
type DrawEntry struct {
	Date, Category, Note, Amount string
}

// DrawReport 报表绘制模型。
type DrawReport struct {
	Title, Subtitle string
	Cards           [4]DrawCard
	ShowPie         bool
	BarTitle        string
	BarLabels       []string
	BarValues       []float64
	BarValueLabels  []string // 为空则不画数值标签
	BarMaxLabel     string   // 年报最高柱强调标签
	Avg             float64  // 均线值，0 不画
	AvgLabel        string
	Cats            []DrawCat
	TopTitle        string
	Tops            []DrawEntry
}

// DrawReportPNG 绘制报表并返回 PNG 字节。
func DrawReportPNG(m *DrawReport) ([]byte, error) {
	height := drawHeight(m)
	dc := gg.NewContext(drawWidth, height)
	dc.SetHexColor("#ffffff")
	dc.Clear()

	drawHeader(dc, m)
	drawCards(dc, m)
	if err := drawCharts(dc, m); err != nil {
		return nil, err
	}
	drawCats(dc, m)
	drawTops(dc, m)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("encode report png: %w", err)
	}
	return buf.Bytes(), nil
}

// drawHeight 计算画布总高度。
func drawHeight(m *DrawReport) int {
	h := 132 + 140 + 24 // 头部 + 卡片
	if m.ShowPie {
		h += 800 + 8
	} else {
		h += 660 + 8
	}
	h += 48 + len(m.Cats)*64 // 分类标题 + 行
	h += 16 + 48 + len(m.Tops)*72
	return h + 24
}

func drawHeader(dc *gg.Context, m *DrawReport) {
	if err := faces.boldFace(dc, 44); err != nil {
		return
	}
	dc.SetColor(colText)
	dc.DrawString(m.Title, drawLeft, 64)
	if err := faces.regularFace(dc, 24); err != nil {
		return
	}
	dc.SetColor(colSub)
	dc.DrawString(m.Subtitle, drawLeft, 110)
}

func drawCards(dc *gg.Context, m *DrawReport) {
	const gap = 28
	const cardH = 140
	w := float64(drawWidth-2*drawLeft-3*gap) / 4
	for i, card := range m.Cards {
		x := float64(drawLeft) + float64(i)*(w+gap)
		y := 132.0
		dc.SetColor(colCardBg)
		dc.DrawRoundedRectangle(x, y, w, cardH, 24)
		dc.Fill()

		if err := faces.regularFace(dc, 24); err != nil {
			return
		}
		dc.SetColor(colGray)
		dc.DrawString(card.Label, x+28, y+44)

		if err := faces.boldFace(dc, 48); err != nil {
			return
		}
		dc.SetColor(card.Color)
		dc.DrawString(card.Value, x+28, y+96)

		if err := faces.regularFace(dc, 20); err != nil {
			return
		}
		dc.SetColor(colLightGray)
		dc.DrawString(card.Caption, x+28, y+124)
	}
}

// chartsTop 图表区顶部 y。
func chartsTop(m *DrawReport) float64 {
	if m.ShowPie {
		return 296.0
	}
	return 296.0
}

// chartsBottom 图表区底部 y。
func chartsBottom(m *DrawReport) float64 {
	if m.ShowPie {
		return 296 + 800
	}
	return 296 + 660
}

func drawCharts(dc *gg.Context, m *DrawReport) error {
	if m.ShowPie {
		if err := drawPie(dc, m, 470, chartsTop(m)+400, 236); err != nil {
			return err
		}
		return drawBars(dc, m, 960, drawRight, chartsTop(m)+70, chartsBottom(m)-50)
	}
	return drawBars(dc, m, 180, drawRight, chartsTop(m)+70, chartsBottom(m)-50)
}

// drawPie 绘制环形图与外部标签。
func drawPie(dc *gg.Context, m *DrawReport, cx, cy, r float64) error {
	if err := faces.regularFace(dc, 30); err != nil {
		return err
	}
	dc.SetColor(colGray)
	dc.DrawStringAnchored("支出类别占比", 90, chartsTop(m)+40, 0, 0.5)

	var total float64
	values := make([]float64, len(m.Cats))
	for i, c := range m.Cats {
		values[i] = c.Ratio
		total += c.Ratio
	}
	// 无数据时跳过扇区
	if total <= 0 {
		return nil
	}

	inner := r * 0.62
	const gapAngle = 0.022
	start := -math.Pi / 2
	for i := range m.Cats {
		sweep := values[i] / total * 2 * math.Pi
		a0 := start + gapAngle/2
		a1 := start + sweep - gapAngle/2
		if a1 > a0 {
			fillAnnulusSegment(dc, cx, cy, r, inner, a0, a1, palette[i%len(palette)])
		}
		start += sweep
	}

	// 外部标签与引线
	if err := faces.regularFace(dc, 22); err != nil {
		return err
	}
	start = -math.Pi / 2
	for _, c := range m.Cats {
		sweep := c.Ratio / total * 2 * math.Pi
		am := start + sweep/2
		cos, sin := math.Cos(am), math.Sin(am)
		lx, ly := cx+cos*(r+34), cy+sin*(r+34)
		dc.SetColor(colLeader)
		dc.SetLineWidth(2)
		dc.DrawLine(cx+cos*(r+8), cy+sin*(r+8), cx+cos*(r+26), cy+sin*(r+26))
		dc.Stroke()
		dc.SetColor(colText)
		label := c.Name + " " + c.Pct
		if cos >= 0 {
			dc.DrawStringAnchored(label, lx, ly, 0, 0.5)
		} else {
			dc.DrawStringAnchored(label, lx, ly, 1, 0.5)
		}
		start += sweep
	}

	// 中心文字
	if err := faces.regularFace(dc, 28); err != nil {
		return err
	}
	dc.SetColor(colGray)
	dc.DrawStringAnchored("总支出", cx, cy-26, 0.5, 0.5)
	if err := faces.boldFace(dc, 42); err != nil {
		return err
	}
	dc.SetColor(colText)
	dc.DrawStringAnchored(m.Cards[0].Value, cx, cy+30, 0.5, 0.5)
	return nil
}

// fillAnnulusSegment 填充环形扇区。
func fillAnnulusSegment(dc *gg.Context, cx, cy, rOuter, rInner, a0, a1 float64, c color.Color) {
	ox0, oy0 := cx+rOuter*math.Cos(a0), cy+rOuter*math.Sin(a0)
	ix1, iy1 := cx+rInner*math.Cos(a1), cy+rInner*math.Sin(a1)

	dc.MoveTo(ox0, oy0)
	dc.DrawArc(cx, cy, rOuter, a0, a1)
	dc.LineTo(ix1, iy1)
	dc.DrawArc(cx, cy, rInner, a1, a0)
	dc.ClosePath()
	dc.SetColor(c)
	dc.Fill()
}

// drawBars 绘制纵向条形图。
func drawBars(dc *gg.Context, m *DrawReport, x0, x1, yTop, yBottom float64) error {
	titleX := x0 - 60
	if !m.ShowPie {
		titleX = drawLeft
	}
	if err := faces.regularFace(dc, 30); err != nil {
		return err
	}
	dc.SetColor(colAccent)
	dc.DrawRoundedRectangle(titleX, chartsTop(m)+24, 8, 32, 4)
	dc.Fill()
	dc.SetColor(color.RGBA{0x44, 0x44, 0x44, 0xff})
	dc.DrawString(m.BarTitle, titleX+22, chartsTop(m)+40)

	var max float64
	for _, v := range m.BarValues {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		return nil
	}
	nice := niceCeil(max)
	ticks := 4

	// 网格与 y 轴标签
	if err := faces.regularFace(dc, 22); err != nil {
		return err
	}
	for i := 0; i <= ticks; i++ {
		v := nice * float64(i) / float64(ticks)
		y := yBottom - (yBottom-yTop)*float64(i)/float64(ticks)
		dc.SetColor(colGrid)
		dc.SetLineWidth(2)
		dc.DrawLine(x0, y, x1, y)
		dc.Stroke()
		dc.SetColor(colLightGray)
		label := "¥" + compactNumber(v)
		dc.DrawStringAnchored(label, x0-12, y, 1, 0.5)
	}

	n := len(m.BarValues)
	if n == 0 {
		return nil
	}
	slot := (x1 - x0) / float64(n)
	barW := math.Min(slot*0.55, 100)
	showValues := len(m.BarValueLabels) == n && barW >= 56

	for i, v := range m.BarValues {
		cx := x0 + slot*(float64(i)+0.5)
		h := (yBottom - yTop) * (v / nice)
		if h < 1 {
			h = 1
		}
		fillRoundedTopBar(dc, cx-barW/2, yBottom-h, barW, h, 14)

		if err := faces.regularFace(dc, 24); err != nil {
			return err
		}
		dc.SetColor(colLightGray)
		dc.DrawStringAnchored(m.BarLabels[i], cx, yBottom+26, 0.5, 0)

		if showValues && v > 0 {
			dc.SetColor(colText)
			if err := faces.regularFace(dc, 22); err != nil {
				return err
			}
			dc.DrawStringAnchored(m.BarValueLabels[i], cx, yBottom-h-14, 0.5, 0.5)
		}
	}

	// 年报最高柱强调
	if m.BarMaxLabel != "" {
		maxI := 0
		for i, v := range m.BarValues {
			if v > m.BarValues[maxI] {
				maxI = i
			}
		}
		cx := x0 + slot*(float64(maxI)+0.5)
		h := (yBottom - yTop) * (m.BarValues[maxI] / nice)
		if err := faces.boldFace(dc, 24); err != nil {
			return err
		}
		dc.SetColor(colExpense)
		dc.DrawStringAnchored(m.BarMaxLabel, cx, yBottom-h-46, 0.5, 0.5)
		dc.SetColor(colExpense)
		dc.DrawCircle(cx, yBottom-h-22, 6)
		dc.Fill()
	}

	// 均线
	if m.Avg > 0 && m.Avg < nice {
		y := yBottom - (yBottom-yTop)*(m.Avg/nice)
		dc.SetColor(colExpense)
		dc.SetLineWidth(3)
		dc.SetDash(14, 10)
		dc.DrawLine(x0, y, x1, y)
		dc.Stroke()
		dc.SetDash()
		if err := faces.regularFace(dc, 22); err != nil {
			return err
		}
		dc.SetColor(colExpense)
		dc.DrawStringAnchored(m.AvgLabel, x1, y-16, 1, 0.5)
	}
	return nil
}

// fillRoundedTopBar 填充顶部圆角渐变条。
func fillRoundedTopBar(dc *gg.Context, x, y, w, h, r float64) {
	if h < r {
		r = h / 2
	}
	if w < 2*r {
		r = w / 2
	}
	k := 0.5523 * r
	dc.MoveTo(x, y+h)
	dc.LineTo(x, y+r)
	dc.CubicTo(x, y+r-k, x+r-k, y, x+r, y)
	dc.LineTo(x+w-r, y)
	dc.CubicTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
	dc.LineTo(x+w, y+h)
	dc.ClosePath()
	grad := gg.NewLinearGradient(0, y+h, 0, y)
	grad.AddColorStop(0, color.RGBA{0x83, 0xbf, 0xf6, 0xff})
	grad.AddColorStop(1, color.RGBA{0x18, 0x8d, 0xf0, 0xff})
	dc.SetFillStyle(grad)
	dc.Fill()
}

// drawCats 绘制分类明细条。
func drawCats(dc *gg.Context, m *DrawReport) {
	y := chartsBottom(m) + 8
	drawSectionTitle(dc, "支出分类", y)
	y += 48
	for _, c := range m.Cats {
		cy := y + 32
		if err := faces.regularFace(dc, 26); err != nil {
			return
		}
		dc.SetColor(colText)
		dc.DrawString(c.Name, drawLeft, cy)

		if err := faces.boldFace(dc, 28); err != nil {
			return
		}
		dc.SetColor(colText)
		w, _ := dc.MeasureString(c.Amount)
		dc.DrawString(c.Amount, 390-w, cy)

		trackX0, trackX1 := 420.0, 1420.0
		dc.SetColor(colTrack)
		dc.DrawRoundedRectangle(trackX0, cy-9, trackX1-trackX0, 18, 9)
		dc.Fill()
		if c.Ratio > 0 {
			grad := gg.NewLinearGradient(trackX0, 0, trackX1, 0)
			grad.AddColorStop(0, color.RGBA{0x83, 0xbf, 0xf6, 0xff})
			grad.AddColorStop(1, color.RGBA{0x18, 0x8d, 0xf0, 0xff})
			dc.SetFillStyle(grad)
			dc.DrawRoundedRectangle(trackX0, cy-9, math.Max(18, (trackX1-trackX0)*c.Ratio), 18, 9)
			dc.Fill()
		}

		if err := faces.regularFace(dc, 22); err != nil {
			return
		}
		dc.SetColor(colLightGray)
		w, _ = dc.MeasureString(c.Pct)
		dc.DrawString(c.Pct, drawRight-w, cy)
		y += 64
	}
}

// drawTops 绘制最高消费列表。
func drawTops(dc *gg.Context, m *DrawReport) {
	y := chartsBottom(m) + 8 + 48 + float64(len(m.Cats))*64 + 16
	drawSectionTitle(dc, m.TopTitle, y)
	y += 48
	for i, t := range m.Tops {
		cy := y + 36
		if i%2 == 1 {
			dc.SetColor(colZebra)
			dc.DrawRectangle(0, y, drawWidth, 72)
			dc.Fill()
		}
		if err := faces.regularFace(dc, 26); err != nil {
			return
		}
		dc.SetColor(colLightGray)
		dc.DrawString(fmt.Sprintf("%d", i+1), drawLeft+4, cy)
		dc.SetColor(colGray)
		dc.DrawString(t.Date, 140, cy)
		dc.SetColor(colText)
		dc.DrawString(t.Category, 300, cy)
		dc.SetColor(colGray)
		note := t.Note
		nw, _ := dc.MeasureString(note)
		for nw > 1080 && len(note) > 1 {
			note = note[:len(note)-2] + "…"
			nw, _ = dc.MeasureString(note)
		}
		dc.DrawString(note, 424, cy)
		if err := faces.boldFace(dc, 28); err != nil {
			return
		}
		dc.SetColor(colExpense)
		w, _ := dc.MeasureString(t.Amount)
		dc.DrawString(t.Amount, drawRight-w, cy)
		y += 72
	}
}

func drawSectionTitle(dc *gg.Context, text string, y float64) {
	dc.SetColor(colAccent)
	dc.DrawRoundedRectangle(drawLeft, y+8, 8, 32, 4)
	dc.Fill()
	if err := faces.boldFace(dc, 28); err != nil {
		return
	}
	dc.SetColor(colText)
	dc.DrawString(text, drawLeft+22, y+24)
}

// niceCeil 取不小于 v 的“整洁”数值。
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Pow(10, math.Floor(math.Log10(v)))
	f := v / exp
	var n float64
	switch {
	case f <= 1:
		n = 1
	case f <= 2:
		n = 2
	case f <= 5:
		n = 5
	default:
		n = 10
	}
	return n * exp
}

// compactNumber 千分位压缩：1250 -> 1,250；4200 -> 4.2k。
func compactNumber(v float64) string {
	if v >= 10000 {
		return fmt.Sprintf("%.1fw", v/10000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.1fk", v/1000)
	}
	return fmt.Sprintf("%.0f", v)
}
