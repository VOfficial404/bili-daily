package main

import (
	"fmt"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

func (a *app) reportsView(c *ui.Context) {
	t := c.Theme()
	if a.report != nil {
		a.reportDetail(c, a.report)
		return
	}
	a.pageHeader(c, "报告", "每一轮运行都会留下一份：每个任务做了哪几步，失败的写着原因和怎么办。", func() {
		ui.Button(c.Key("reload-reports"), "").Label("刷新报告").Disabled(a.reportsBusy).OnClick(a.loadReports).Children(func() {
			ui.Icon(c, iconRefresh)
			ui.Text(c, "刷新").SingleLine()
		})
	})
	if len(a.st.Accounts) > 1 && a.accountFilter(c, "reports-filter", &a.reportMid) {
		a.loadReports()
	}
	errorText(c, a.reportErr)
	if a.reportsBusy && len(a.runs) == 0 {
		ui.Spinner(c)
	}
	if len(a.runs) == 0 && !a.reportsBusy {
		panel(c).Padding(32).AlignItems(ui.Center).Children(func() {
			muted(c, "还没有运行报告。任务跑过一轮后，这里会逐项说明结果。")
		})
		return
	}
	panel(c).Padding(6).Gap(0).Children(func() {
		lastDay := ""
		for _, r := range a.runs {
			day := r.Started.Local().Format("1月2日")
			if day != lastDay {
				lastDay = day
				sectionTitle(c, day).Padding(12, 14, 4)
			}
			row := ui.ButtonBase(c.Key("run-"+r.ID)).Row().Gap(14).Padding(10, 14).Radius(8).Label("查看报告 " + r.ID).
				Disabled(a.reportsBusy).OnClick(func() { a.openReport(r.ID) })
			if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				col := statusColor(c, r.Status)
				ui.Box(c).Size(10, 10).Radius(5).Background(col)
				ui.Text(c, r.Started.Local().Format("15:04")).FontSize(13).Width(42).TextColor(t.TextMuted)
				ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, r.Uname).FontSize(14).FontWeight(600).SingleLine()
						pill(c, client.TriggerText(r.Trigger), t.TextMuted)
					})
					muted(c, r.Summary).FontSize(13).SingleLine().Ellipsis("…")
				})
				ui.Text(c, client.StatusText(r.Status)).FontSize(12).TextColor(col)
				ui.Icon(c, iconChevron).FontSize(14).TextColor(t.TextMuted)
			})
		}
	})
}

func (a *app) reportDetail(c *ui.Context, r *api.Run) {
	t := c.Theme()
	ui.ButtonBase(c.Key("back-reports")).Row().Gap(4).Padding(4, 6).Radius(6).AlignSelf(ui.Start).Label("返回报告列表").
		OnClick(func() { a.report = nil; a.loadReports() }).Children(func() {
		ui.Icon(c, iconBack).TextColor(t.TextMuted)
		muted(c, "全部报告").FontSize(13)
	})
	col := statusColor(c, r.Status)
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, r.Uname).FontSize(26).Bold()
			pill(c, client.StatusText(r.Status), col)
		})
		muted(c, fmt.Sprintf("%s · %s · 用时 %s", r.Started.Local().Format("2006-01-02 15:04"), client.TriggerText(r.Trigger), client.Duration(r.Ended.Sub(r.Started)))).FontSize(13)
		ui.Text(c, r.Summary).FontSize(15)
	})
	for _, task := range r.Tasks {
		tc := statusColor(c, task.Status)
		panel(c.Key("task-" + task.ID)).Padding(0).Gap(0).Children(func() {
			ui.Row(c).Gap(12).Padding(16, 20).Children(func() {
				ui.Box(c).Size(26, 26).Radius(13).Background(tc.Alpha(0.15)).Center().Children(func() {
					ui.Text(c, statusGlyph(task.Status)).FontSize(13).Bold().TextColor(tc)
				})
				ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, task.Title).FontSize(16).Bold()
					muted(c, task.Note).FontSize(13)
				})
				muted(c, fmt.Sprintf("%.1f 秒", float64(task.Millis)/1000)).FontSize(12)
			})
			ui.Column(c).Gap(0).Padding(4, 20, 14, 58).Children(func() {
				for i, s := range task.Steps {
					sc := statusColor(c, s.Status)
					ui.Row(c).Gap(12).AlignItems(ui.Start).Padding(8, 0).Children(func() {
						// 一条竖线把步骤串起来
						ui.Column(c).Width(12).AlignItems(ui.Center).AlignSelf(ui.Stretch).Children(func() {
							ui.Box(c).Size(10, 10).Radius(5).Margin(5, 0, 0).Background(sc)
							if i < len(task.Steps)-1 {
								ui.Box(c).Width(2).Grow(1).Margin(4, 0, -12).Background(t.Border)
							}
						})
						ui.Column(c).Gap(5).Grow(1).MinWidth(0).Children(func() {
							ui.Row(c).Gap(8).Children(func() {
								ui.Text(c, s.Name).FontSize(14).FontWeight(600)
								ui.Text(c, client.StatusText(s.Status)).FontSize(12).TextColor(sc)
							})
							if s.Detail != "" {
								muted(c, s.Detail).FontSize(13).Selectable()
							}
							if s.Hint != "" {
								ui.Column(c).Gap(4).Padding(10, 12).Radius(8).Background(sc.Alpha(0.1)).BorderWidth(0, 0, 0, 3).BorderColor(sc).Children(func() {
									ui.Text(c, "建议："+s.Hint).FontSize(13).Selectable()
									if s.Code != 0 {
										muted(c, fmt.Sprintf("返回码 %d", s.Code)).FontSize(11).Selectable()
									}
								})
							} else if s.Code != 0 {
								muted(c, fmt.Sprintf("返回码 %d", s.Code)).FontSize(11)
							}
						})
					})
				}
			})
		})
	}
}
