package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/egoist/mygo/ui"
)

var weekdays = [...]string{"日", "一", "二", "三", "四", "五", "六"}

func (a *app) overviewView(c *ui.Context) {
	t := c.Theme()
	now := time.Now().In(engine.Beijing)
	total, done, problems := 0, 0, 0
	for _, acc := range a.st.Accounts {
		n, d, p := client.Tally(client.TodayPlan(*a.st, acc))
		total += n
		done += d
		problems += p
		if !acc.LoggedIn {
			problems++
		}
	}
	desc := fmt.Sprintf("%d月%d日 星期%s · 北京时间", now.Month(), now.Day(), weekdays[now.Weekday()])
	a.pageHeader(c, "今天", desc, func() {
		ui.Button(c.Key("add"), "").Label("添加账号").Disabled(a.qrBusy).OnClick(a.startQR).Children(func() {
			ui.Icon(c, iconPlus)
			ui.Text(c, "添加账号").SingleLine()
		})
		if len(a.st.Accounts) > 0 {
			ui.PrimaryButton(c.Key("run-all"), "").Label("全部立即运行").Disabled(a.busy["all"]).OnClick(a.runAll).Children(func() {
				ui.Icon(c, iconPlay).TextColor(t.AccentText)
				ui.Text(c, "全部立即运行").SingleLine().TextColor(t.AccentText)
			})
		}
	})

	if len(a.st.Accounts) == 0 {
		panel(c).Padding(40).Gap(14).AlignItems(ui.Center).Children(func() {
			brandMark(c, 44)
			ui.Text(c, "还没有账号").FontSize(20).Bold()
			muted(c, "用 B 站 App 扫一次码，之后每天会自动做完任务，这里显示每一项的结果。").TextAlign(ui.Center)
			ui.PrimaryButton(c.Key("first-qr"), "扫码添加账号").OnClick(a.startQR)
		})
		return
	}

	// 一行总数：只有在有意义时才显示需要处理的数量
	ui.Row(c).Gap(10).Wrap().Children(func() {
		stat := func(value, label string, col ui.Color) {
			ui.Row(c).Gap(6).AlignItems(ui.Baseline).Padding(10, 14).Radius(10).Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Text(c, value).FontSize(20).Bold().TextColor(col)
				muted(c, label).FontSize(12)
			})
		}
		stat(strconv.Itoa(len(a.st.Accounts)), "个账号", t.Text)
		stat(fmt.Sprintf("%d/%d", done, total), "项今天已完成", t.Success)
		if problems > 0 {
			stat(strconv.Itoa(problems), "处需要看一下", t.Danger)
		}
	})

	cols := 1
	if a.width >= 1180 && len(a.st.Accounts) > 1 {
		cols = 2
	}
	ui.Grid(c).Columns(cols).Gap(16).Children(func() {
		for _, acc := range a.st.Accounts {
			a.accountToday(c, acc)
		}
	})
}

// accountToday 是「今天」页里一个账号的卡片
func (a *app) accountToday(c *ui.Context, acc api.Account) {
	t := c.Theme()
	lines := client.TodayPlan(*a.st, acc)
	_, n, _ := client.Tally(lines)
	panel(c.Key("today-" + acc.Mid)).Padding(0).Gap(0).Children(func() {
		// 头部：今日环、名字、等级、今日完成数和运行按钮
		ui.Row(c).Gap(16).Padding(18, 20).Children(func() {
			a.ringAvatar(c, acc, 64)
			ui.Column(c).Gap(5).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, accountName(acc)).FontSize(18).Bold().SingleLine().Ellipsis("…")
					levelBadge(c, acc.Level)
					if acc.VIP != "" {
						pill(c, acc.VIP, t.Accent)
					}
				})
				muted(c, fmt.Sprintf("硬币 %s · B 币券 %s", trimNum(acc.Coins), trimNum(acc.BCoupon))).FontSize(12)
			})
			ui.Column(c).Gap(2).AlignItems(ui.End).Children(func() {
				ui.Row(c).AlignItems(ui.Baseline).Gap(2).Children(func() {
					ui.Text(c, strconv.Itoa(n)).FontSize(26).Bold()
					muted(c, "/"+strconv.Itoa(len(lines))).FontSize(15)
				})
				muted(c, "今天完成").FontSize(11)
			})
		})

		if !acc.LoggedIn {
			ui.Row(c).Gap(10).Padding(10, 20).Background(t.Danger.Alpha(0.1)).Children(func() {
				ui.Text(c, "登录已失效，任务暂停了。重新扫码就能恢复。").TextColor(t.Danger).FontSize(13).Grow(1)
				ui.Button(c.Key("relogin-"+acc.Mid), "重新扫码").OnClick(a.startQR)
			})
		}

		// 经验
		ui.Column(c).Gap(6).Padding(0, 20, 14).Children(func() {
			if acc.NextExp > 0 && acc.Level < 6 {
				frac := min(1, max(0, float32(acc.Exp)/float32(acc.NextExp)))
				ui.Box(c).Height(6).FillWidth().Radius(3).Background(t.Border).Draw(func(p *ui.Painter, r ui.Rect) {
					p.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W * frac, H: r.H}, ui.LinearGradient{From: brandPink.Alpha(0.5), To: brandPink, Angle: 90, Oklab: true}, 3)
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				muted(c, fmt.Sprintf("经验 %s / %s", thousands(acc.Exp), thousands(acc.NextExp))).FontSize(12).Hidden(acc.NextExp <= 0)
				ui.Spacer(c)
				muted(c, levelForecast(acc)).FontSize(12).SingleLine()
			})
		})

		// 正在运行
		if p := acc.Running; p != nil {
			ui.Row(c).Gap(12).Padding(10, 20).Background(brandBlue.Alpha(0.1)).Children(func() {
				ui.Spinner(c).TextColor(brandBlue)
				label := p.Task
				if p.Total > 0 {
					label = fmt.Sprintf("正在做：%s（%d/%d）", p.Task, p.Done, p.Total)
				}
				ui.Text(c, label).FontSize(13).Grow(1)
				ui.Button(c.Key("cancel-"+acc.Mid), "中止").Disabled(a.busy[acc.Mid]).OnClick(func() {
					a.action(acc.Mid, "已请求中止", func(cl *api.Client, ctx context.Context) error { return cl.Cancel(ctx, acc.Mid) })
				})
			})
		}

		// 今天的每一项
		ui.Column(c).Padding(4, 12).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			for _, l := range lines {
				ui.Row(c.Key(acc.Mid+"/"+l.ID)).Gap(12).Padding(9, 8).Children(func() {
					col := statusColor(c, l.Status)
					ui.Box(c).Size(20, 20).Radius(10).Background(col.Alpha(0.14)).Center().Children(func() {
						ui.Text(c, statusGlyph(l.Status)).FontSize(11).Bold().TextColor(col)
					})
					ui.Text(c, l.Title).FontSize(14).Width(108).SingleLine()
					note := l.Detail
					if note == "" && l.Status == "" {
						note = "等今天这一轮"
					}
					muted(c, note).FontSize(13).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(l.Detail)
					ui.Text(c, client.StatusText(l.Status)).FontSize(12).TextColor(col).SingleLine()
				})
			}
			if len(lines) == 0 {
				muted(c, "这个账号没有开着的任务。").FontSize(13).Padding(10, 8)
			}
		})

		// 底部：下次运行、上次报告、立即运行
		ui.Row(c).Gap(10).Padding(12, 20).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			muted(c, "下次 "+client.HumanTime(acc.NextRun, "暂无")).FontSize(12).SingleLine()
			if r := acc.LastRun; r != nil {
				ui.ButtonBase(c.Key("last-"+acc.Mid)).Row().Gap(4).Padding(4, 8).Radius(6).Label("查看上次报告").
					OnClick(func() { a.openReport(r.ID) }).Children(func() {
					ui.Text(c, "上次："+r.Summary).FontSize(12).TextColor(statusColor(c, r.Status)).SingleLine().Ellipsis("…").MinWidth(0)
					ui.Icon(c, iconChevron).FontSize(12).TextColor(t.TextMuted)
				}).Shrink(1).MinWidth(0)
			}
			ui.Spacer(c)
			ui.Button(c.Key("run-"+acc.Mid), "").Label("立即运行").
				Disabled(!acc.LoggedIn || acc.Running != nil || a.busy[acc.Mid]).
				OnClick(func() { a.run(acc.Mid, nil) }).Children(func() {
				ui.Icon(c, iconPlay)
				ui.Text(c, "立即运行").SingleLine()
			})
		})
	})
}

func trimNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
