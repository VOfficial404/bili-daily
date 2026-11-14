package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) logsView(c *ui.Context) {
	t := c.Theme()
	reset := false
	ui.Row(c).Gap(12).Wrap().Children(func() {
		levels := []string{"", "info", "warn", "error", "debug"}
		labels := []string{"全部", "信息", "警告", "错误", "调试"}
		idx := max(0, slices.Index(levels, a.logLevel))
		if ui.Segmented(c.Key("log-level"), &idx, labels...).Changed() {
			a.logLevel = levels[idx]
			reset = true
		}
		ui.Spacer(c)
		ui.Checkbox(c, &a.autoScroll, "自动滚动")
	})
	if len(a.st.Accounts) > 1 && a.accountFilter(c, "logs-filter", &a.logMid) {
		reset = true
	}
	if reset {
		a.logs = nil
		a.logSeq = 0
		a.loadLogs()
	}
	consoleBg := ui.Hex("#0e0e11")
	if !t.Dark {
		consoleBg = ui.Hex("#fbfbfc")
	}
	ui.Scroll(c.Key("log-lines")).Height(max(320, a.winHeight()-220)).FillWidth().TrackScroll(&a.logScroll).
		Background(consoleBg).Radius(12).Border(1, t.Border).Padding(14, 16).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			if len(a.logs) == 0 {
				muted(c, "还没有符合筛选条件的日志").FontSize(12)
			}
			for _, l := range a.logs {
				lc := t.TextMuted
				switch l.Level {
				case "warn":
					lc = t.Warning
				case "error":
					lc = t.Danger
				case "info":
					lc = t.Success
				}
				ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
					ui.Text(c, l.Time.Local().Format("15:04:05")).Font("monospace").FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, strings.ToUpper(l.Level)).Font("monospace").FontSize(11).Bold().TextColor(lc).Width(42)
					who := ""
					if l.Mid != "" {
						who = a.name(l.Mid) + "  "
					}
					ui.Text(c, who+l.Text).Font("monospace").FontSize(12).Grow(1).MinWidth(0).Selectable()
				})
			}
		})
	})
}

func (a *app) winHeight() float32 { return a.height }
