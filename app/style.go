package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/egoist/mygo/ui"
)

// B 站粉只用在主要操作、选中项和今日环上；其余颜色跟随系统的深浅色主题。
var (
	brandPink     = ui.Hex("#fb7299")
	brandPinkDeep = ui.Hex("#d63e72") // 浅色主题里白字放在它上面才够清楚
	brandBlue     = ui.Hex("#00aeec")
	brandBlueDeep = ui.Hex("#0079a8") // 浅色主题里当文字用
)

// applyTheme 每帧在系统主题上换上本应用的颜色。MyGo 每帧从系统主题开始，所以这里不会叠加。
func applyTheme(c *ui.Context) {
	t := *c.Theme()
	if t.Dark {
		t = *t.WithAccent(brandPink)
		t.AccentText = ui.Hex("#2a0b16")
		t.Background = ui.Hex("#131316")
		t.Surface = ui.Hex("#1c1c21")
		t.SurfaceHover = ui.Hex("#25252c")
		t.SurfacePressed = ui.Hex("#2d2d35")
		t.Border = ui.Hex("#2c2c34")
		t.TextMuted = ui.Hex("#9a9aa8")
	} else {
		t = *t.WithAccent(brandPinkDeep)
		t.Background = ui.Hex("#f5f5f7")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#f0f0f3")
		t.SurfacePressed = ui.Hex("#e6e6eb")
		t.Border = ui.Hex("#e3e3e8")
		t.TextMuted = ui.Hex("#6b6b78")
	}
	t.Radius = 8
	c.SetTheme(&t)
}

// infoColor 是「进行中」这类提示的蓝色，浅色主题里用深一档的，放在白底上才看得清
func infoColor(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return brandBlue
	}
	return brandBlueDeep
}

// panel 是内容卡片：比窗口背景亮一级，细边框
func panel(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(20).Gap(14).Radius(14).Background(t.Surface).Border(1, t.Border)
}

func muted(c *ui.Context, s string) ui.Element { return ui.Text(c, s).TextColor(c.Theme().TextMuted) }

func errorText(c *ui.Context, s string) {
	if s != "" {
		ui.Text(c, s).TextColor(c.Theme().Danger).Selectable()
	}
}

// sectionTitle 是卡片里一组内容的小标题
func sectionTitle(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(12).FontWeight(600).TextColor(c.Theme().TextMuted)
}

/* 状态 */

func statusColor(c *ui.Context, s api.Status) ui.Color {
	t := c.Theme()
	switch s {
	case api.StatusOK:
		return t.Success
	case api.StatusPartial:
		return t.Warning
	case api.StatusFailed:
		return t.Danger
	case api.StatusRunning:
		return infoColor(c)
	}
	return t.TextMuted
}

// statusGlyph 是步骤前的符号，颜色之外再用形状区分状态
func statusGlyph(s api.Status) string {
	switch s {
	case api.StatusOK:
		return "✓"
	case api.StatusPartial:
		return "◐"
	case api.StatusFailed:
		return "✕"
	case api.StatusRunning:
		return "●"
	}
	return "–"
}

// pill 是带底色的小标签
func pill(c *ui.Context, text string, col ui.Color) ui.Element {
	return ui.Box(c).Padding(2, 8).Radius(999).Background(col.Alpha(0.14)).Children(func() {
		ui.Text(c, text).FontSize(11).FontWeight(600).TextColor(col).SingleLine()
	})
}

/* B 站等级：沿用 B 站自己的等级配色 */

var levelColors = [...]ui.Color{
	ui.Hex("#bfbfbf"), ui.Hex("#bfbfbf"), ui.Hex("#8bd29b"), ui.Hex("#7bcdef"),
	ui.Hex("#feb68b"), ui.Hex("#ee672a"), ui.Hex("#f04c49"),
}

func levelColor(l int) ui.Color { return levelColors[max(0, min(l, 6))] }

// levelBadge 在深色主题里是描边加同色字；等级色大多很浅，浅色主题里改成浅底加正文色的字
func levelBadge(c *ui.Context, l int) ui.Element {
	t, col := c.Theme(), levelColor(l)
	box, text := ui.Box(c).Padding(1, 6).Radius(4).Border(1, col), col
	if !t.Dark {
		box.Background(col.Alpha(0.28))
		text = t.Text
	}
	return box.Children(func() {
		ui.Text(c, "LV"+strconv.Itoa(l)).FontSize(11).Bold().TextColor(text).SingleLine()
	})
}

// dailyExpCap 是每天能拿到的经验上限：登录、观看、分享各 5，投币 50
const dailyExpCap = 65

// levelForecast 说明离下一级还差多少，按每天经验上限要多久
func levelForecast(acc api.Account) string {
	if acc.NextExp <= 0 || acc.Level >= 6 {
		return "已满级"
	}
	need := acc.NextExp - acc.Exp
	days := int(math.Ceil(float64(need) / dailyExpCap))
	return fmt.Sprintf("距 LV%d 还差 %s 经验 · 每天拿满约 %d 天", acc.Level+1, thousands(need), days)
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	neg := n < 0
	if neg {
		s = s[1:]
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

/* 今日环：头像外的一圈，今天计划里的每个任务一段 */

type ringSeg struct{ color ui.Color }

// arc 用折线画一段圆弧，角度从正上方顺时针，单位是弧度
func arc(cx, cy, r, from, to float32) *ui.Path {
	p := &ui.Path{}
	steps := max(6, int((to-from)*24))
	for i := range steps + 1 {
		a := float64(from + (to-from)*float32(i)/float32(steps))
		x := cx + r*float32(math.Sin(a))
		y := cy - r*float32(math.Cos(a))
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p
}

// todayRing 画一圈分段，中间放头像
func todayRing(c *ui.Context, size float32, segs []ringSeg, track ui.Color, center func()) ui.Element {
	const stroke = 3.5
	box := ui.Box(c).Size(size, size).Center()
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		rad := r.W/2 - stroke/2
		if len(segs) == 0 {
			p.StrokePath(arc(cx, cy, rad, 0, 2*math.Pi), stroke, track)
			return
		}
		full := float32(2 * math.Pi)
		gap := float32(0.16)
		if len(segs) == 1 {
			gap = 0
		}
		each := full / float32(len(segs))
		for i, s := range segs {
			from := each*float32(i) + gap/2
			p.StrokePath(arc(cx, cy, rad, from, from+each-gap).Cap(ui.ButtCap), stroke, s.color)
		}
	})
	box.Children(center)
	return box
}

/* 图标：线条风格，24×24 */

func icon(d string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + d + `</svg>`))
}

var (
	iconOverview = icon(`<circle cx="12" cy="12" r="8.5"/><path d="M12 7v5l3.5 2"/>`)
	iconAccounts = icon(`<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20c.8-3.6 3.4-5.5 6.5-5.5s5.7 1.9 6.5 5.5"/><path d="M16 4.5a3.5 3.5 0 0 1 0 7M18.5 14.8c1.6.8 2.6 2.6 3 5.2"/>`)
	iconTasks    = icon(`<path d="M10 6h10M10 12h10M10 18h10"/><path d="m3.5 6 1.5 1.5L7.5 5M3.5 12l1.5 1.5 2.5-2.5M3.5 18l1.5 1.5 2.5-2.5"/>`)
	iconReports  = icon(`<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h4"/>`)
	iconSettings = icon(`<path d="M4 7h10M18 7h2M4 17h4M12 17h8"/><circle cx="16" cy="7" r="2"/><circle cx="10" cy="17" r="2"/>`)
	iconLogs     = icon(`<rect x="3" y="4" width="18" height="16" rx="2.5"/><path d="m7 9 3 3-3 3M13 15h4"/>`)
	iconPlay     = icon(`<path d="M7 5v14l11-7z"/>`)
	iconPlus     = icon(`<path d="M12 5v14M5 12h14"/>`)
	iconChevron  = icon(`<path d="m9 6 6 6-6 6"/>`)
	iconBack     = icon(`<path d="m15 6-6 6 6 6"/>`)
	iconRefresh  = icon(`<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>`)
)

var navPages = []struct {
	id   string
	icon *ui.SVG
}{
	{"今天", iconOverview}, {"账号", iconAccounts}, {"任务", iconTasks},
	{"报告", iconReports}, {"设置", iconSettings}, {"日志", iconLogs},
}

// brandMark 是界面里的小标记：一台小电视，屏幕里一枚对勾。和应用图标是同一个图案
func brandMark(c *ui.Context, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		s := r.W / 24
		line := func(pts ...float32) *ui.Path {
			path := &ui.Path{}
			path.MoveTo(r.X+pts[0]*s, r.Y+pts[1]*s)
			for i := 2; i < len(pts); i += 2 {
				path.LineTo(r.X+pts[i]*s, r.Y+pts[i+1]*s)
			}
			return path
		}
		p.StrokePath(line(7.5, 3.5, 10, 6.8), 2*s, brandPink)
		p.StrokePath(line(16.5, 3.5, 14, 6.8), 2*s, brandPink)
		p.Fill(ui.Rect{X: r.X + 2.5*s, Y: r.Y + 6.5*s, W: 19 * s, H: 14 * s}, brandPink, 4*s)
		p.StrokePath(line(8, 13.6, 10.9, 16.3, 16.2, 10.9), 2.2*s, ui.Hex("#ffffff"))
	})
}
