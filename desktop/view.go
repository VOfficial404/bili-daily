package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/egoist/mygo/ui"
)

var navPages = []string{"概览", "账号", "任务", "报告", "设置", "日志"}
var navIcon = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M4 5h16v14H4zM8 9h8M8 13h5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`))

func card(c *ui.Context) ui.Element {
	return ui.Column(c).Padding(18).Gap(10).Radius(c.Theme().Radius*1.5).Background(c.Theme().Background).Border(1, c.Theme().Border)
}
func muted(c *ui.Context, s string) ui.Element { return ui.Text(c, s).TextColor(c.Theme().TextMuted) }
func errorText(c *ui.Context, s string) {
	if s != "" {
		ui.Text(c, s).TextColor(c.Theme().Danger).Selectable()
	}
}
func statusText(s api.Status) string {
	switch s {
	case api.StatusOK:
		return "已完成"
	case api.StatusPartial:
		return "部分完成"
	case api.StatusFailed:
		return "失败"
	case api.StatusSkipped:
		return "已跳过"
	case api.StatusRunning:
		return "运行中"
	}
	return "等待运行"
}
func statusColor(c *ui.Context, s api.Status) ui.Color {
	switch s {
	case api.StatusOK:
		return c.Theme().Success
	case api.StatusFailed, api.StatusPartial:
		return c.Theme().Danger
	case api.StatusRunning:
		return c.Theme().Accent
	}
	return c.Theme().TextMuted
}
func stamp(t *time.Time) string {
	if t == nil {
		return "暂无"
	}
	return t.In(time.Local).Format("01-02 15:04")
}
func joinIDs(ids []int64) string {
	ss := make([]string, len(ids))
	for i, id := range ids {
		ss[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(ss, ", ")
}
func accountName(acc api.Account) string {
	if acc.Uname != "" {
		return acc.Uname
	}
	return "UID " + acc.Mid
}
func (a *app) name(mid string) string {
	for _, acc := range a.st.Accounts {
		if acc.Mid == mid {
			return accountName(acc)
		}
	}
	return mid
}
func (a *app) title(id string) string {
	for _, t := range a.st.Tasks {
		if t.ID == id {
			return t.Title
		}
	}
	return id
}
func (a *app) navigate(page string) {
	a.page = page
	if page == "报告" {
		a.report = nil
		a.loadReports()
	}
	if page == "日志" {
		a.loadLogs()
	}
}
func (a *app) view(c *ui.Context) {
	for _, s := range a.toasts {
		c.Toast(s)
	}
	a.toasts = nil
	a.width, _ = c.Size()
	a.narrow = a.width < 760
	if a.cl == nil || a.st == nil {
		a.connectView(c)
		return
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(c.Theme().Surface).Children(func() {
		if !a.narrow {
			ui.Column(c).Width(202).Padding(16).Gap(18).Children(func() {
				ui.Text(c, "Bili Tools").FontSize(22).Bold()
				muted(c, "把每天的结果看明白").FontSize(12)
				ui.Sidebar(c, &a.page, func() {
					for _, p := range navPages {
						if ui.SidebarItem(c, p, navIcon, p).Clicked() {
							a.navigate(p)
						}
					}
				}).Grow(1)
				a.connectionInfo(c)
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if a.loadErr != "" {
				ui.Column(c).Padding(12).Background(c.Theme().Background).Children(func() { errorText(c, a.loadErr); muted(c, "正在重试，下面保留上次数据").FontSize(12) })
			}
			ui.Scroll(c.Key("page-" + a.page)).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(24).Gap(18).Children(func() {
					ui.Text(c, a.page).FontSize(25).Bold()
					switch a.page {
					case "概览":
						a.overviewView(c)
					case "账号":
						a.accountsView(c)
					case "任务":
						a.tasksView(c)
					case "报告":
						a.reportsView(c)
					case "设置":
						a.settingsView(c)
					case "日志":
						a.logsView(c)
					}
				})
			})
			if a.narrow {
				ui.Row(c).Padding(6).Gap(2).Background(c.Theme().Background).Children(func() {
					for _, p := range navPages {
						ui.Button(c.Key("tab-"+p), p).Grow(1).OnClick(func() { a.navigate(p) })
					}
				})
			}
		})
	})
	a.dialogs(c)
}
func (a *app) connectView(c *ui.Context) {
	ui.Scroll(c).Fill().Background(c.Theme().Surface).Children(func() {
		ui.Column(c).FillWidth().Padding(24).AlignItems(ui.Center).Children(func() {
			card(c).Width(min(460, a.width-48)).Gap(18).Margin(0, 0, 28).Children(func() {
				ui.Text(c, "Bili Tools").FontSize(28).Bold()
				muted(c, "每日任务，一处查看")
				ui.Segmented(c.Key("mode"), &a.mode, "连接服务器", "在本机运行").Disabled(a.connecting)
				if a.mode == 0 {
					ui.Field(c, "服务器地址", func() {
						ui.TextInput(c.Key("server"), &a.server).Placeholder("http://127.0.0.1:8686").Disabled(a.connecting)
					})
					ui.Field(c, "令牌", func() { ui.TextInput(c.Key("token"), &a.token).Password().Label("令牌").Disabled(a.connecting) })
					ui.Checkbox(c, &a.remember, "记住令牌，下次打开自动连接").Disabled(a.connecting)
					muted(c, "令牌保存在系统凭据管理器中。").FontSize(12)
				} else {
					muted(c, "任务只在应用运行时执行；关闭窗口后可在托盘中继续运行。")
					ui.Field(c, "代理（可选）", func() {
						ui.Row(c).Gap(8).Children(func() {
							ui.TextInput(c.Key("local-proxy"), &a.proxy).Grow(1).Placeholder("http://127.0.0.1:7890").Disabled(a.connecting)
							ui.Button(c, "测试").Disabled(a.connecting).OnClick(func() { a.testProxy(a.proxy) })
						})
					})
					ui.Checkbox(c, &a.autoLocal, "下次打开时自动开始").Disabled(a.connecting)
				}
				errorText(c, a.connectErr)
				label := "连接"
				if a.mode == 1 {
					label = "开始运行"
				}
				ui.PrimaryButton(c.Key("connect"), label).Disabled(a.connecting).OnClick(func() {
					if a.mode == 0 {
						a.connect()
					} else {
						a.startLocal()
					}
				})
				if a.connecting {
					ui.Spinner(c)
					muted(c, "正在连接…")
				}
			})
		})
	})
}
func (a *app) connectionInfo(c *ui.Context) {
	label := a.cl.Base()
	if a.localEngine != nil {
		label = "这台电脑（本机运行）"
	}
	muted(c, label).FontSize(11).Selectable()
	muted(c, "服务端 "+a.st.Version+" · 客户端 "+version).FontSize(11)
	if e := a.st.Egress; e != nil {
		if e.Error != "" {
			errorText(c, e.Error)
		} else {
			muted(c, e.Addr+" "+e.Place).FontSize(12).Selectable()
		}
	}
}
func (a *app) overviewView(c *ui.Context) {
	ui.Row(c).Gap(8).Wrap().Children(func() {
		ui.PrimaryButton(c, "全部立即运行").Disabled(a.busy["all"] || len(a.st.Accounts) == 0).OnClick(a.runAll)
		ui.Button(c, "添加账号").Disabled(a.qrBusy).OnClick(a.startQR)
	})
	if len(a.st.Accounts) == 0 {
		card(c).Padding(32).Children(func() {
			ui.Text(c, "还没有账号").FontSize(20).Bold()
			muted(c, "扫码添加账号后，这里会显示今天每个任务的结果。")
			ui.PrimaryButton(c, "扫码添加账号").OnClick(a.startQR)
		})
	}
	for _, acc := range a.st.Accounts {
		acc := acc
		card(c).Children(func() {
			a.accountHeader(c, acc)
			if !acc.LoggedIn {
				errorText(c, "登录已失效，请到账号页重新扫码")
			}
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, r := range acc.Today {
					ui.Badge(c, a.title(r.ID)+" · "+statusText(r.Status)).TextColor(statusColor(c, r.Status)).Tooltip(r.Note)
				}
			})
			for _, r := range acc.Today {
				if r.Note != "" {
					muted(c, a.title(r.ID)+"："+r.Note).FontSize(12)
				}
			}
			if p := acc.Running; p != nil {
				ui.Row(c).Gap(12).Children(func() {
					ui.Spinner(c)
					ui.Text(c, fmt.Sprintf("%s  %d / %d", p.Task, p.Done, p.Total))
					ui.Button(c, "中止").Disabled(a.busy[acc.Mid]).OnClick(func() {
						a.action(acc.Mid, "已请求中止", func(cl *api.Client, ctx context.Context) error { return cl.Cancel(ctx, acc.Mid) })
					})
				})
			}
			muted(c, "下次运行："+stamp(acc.NextRun))
			if r := acc.LastRun; r != nil {
				ui.Button(c, "上次："+r.Summary).Tooltip("查看完整报告").OnClick(func() { a.openReport(r.ID) })
			}
			ui.Button(c, "立即运行").Disabled(!acc.LoggedIn || acc.Running != nil || a.busy[acc.Mid]).OnClick(func() { a.run(acc.Mid, nil) })
		})
	}
	if a.narrow {
		a.connectionInfo(c)
	}
}
func (a *app) accountHeader(c *ui.Context, acc api.Account) {
	ui.Row(c).Gap(12).Children(func() {
		if bm := a.faces[acc.Face]; bm != nil {
			ui.Image(c, bm).Size(46, 46).Radius(23).Fit(ui.Cover)
		} else {
			ui.Box(c).Size(46, 46).Radius(23).Background(c.Theme().Surface).Center().Children(func() { ui.Text(c, "哔").Bold().TextColor(c.Theme().Accent) })
		}
		ui.Column(c).Gap(3).Grow(1).Children(func() {
			ui.Text(c, accountName(acc)).Bold().FontSize(18)
			muted(c, fmt.Sprintf("等级 %d · 硬币 %.1f", acc.Level, acc.Coins)).FontSize(12)
		})
	})
}
func (a *app) accountsView(c *ui.Context) {
	ui.Button(c, "添加账号").OnClick(a.startQR)
	if len(a.st.Accounts) == 0 {
		muted(c, "还没有账号，先扫码添加一个。")
		return
	}
	for _, acc := range a.st.Accounts {
		acc := acc
		card(c).Children(func() {
			a.accountHeader(c, acc)
			muted(c, "UID "+acc.Mid).FontSize(12)
			if !acc.LoggedIn {
				errorText(c, "登录已失效，任务已暂停。请重新扫码。")
				ui.PrimaryButton(c, "重新扫码").OnClick(a.startQR)
			}
			if acc.NextExp > 0 {
				ui.Text(c, fmt.Sprintf("经验 %d / %d", acc.Exp, acc.NextExp))
				ui.Box(c).Height(5).FillWidth().Radius(3).Background(c.Theme().Surface).Children(func() {
					ui.Box(c).Height(5).Width(min(float32(1), float32(acc.Exp)/float32(acc.NextExp)) * min(420, a.width-100)).Background(c.Theme().Accent)
				})
			} else {
				muted(c, "已到最高等级")
			}
			ui.Text(c, fmt.Sprintf("B 币券 %.2f", acc.BCoupon))
			vip := acc.VIP
			if vip == "" {
				vip = "普通会员"
			}
			ui.Text(c, vip+" · 到期 "+stamp(acc.VIPDue))
			muted(c, "Cookie 到期："+stamp(acc.CookieExpires))
			enabled := acc.Enabled
			if ui.Checkbox(c.Key("plan-"+acc.Mid), &enabled, "参加每日计划").Disabled(a.busy[acc.Mid]).Changed() {
				a.action(acc.Mid, "每日计划已更新", func(cl *api.Client, ctx context.Context) error {
					return cl.Patch(ctx, acc.Mid, api.AccountPatch{Enabled: &enabled})
				})
			}
			ui.Column(c).Gap(6).Children(func() {
				for _, t := range a.st.Tasks {
					if t.Manual {
						continue
					}
					on := !slices.Contains(acc.DisabledTasks, t.ID)
					if ui.Checkbox(c.Key(acc.Mid+"-"+t.ID), &on, t.Title).Disabled(a.busy[acc.Mid]).Changed() {
						disabled := slices.Clone(acc.DisabledTasks)
						if on {
							disabled = slices.DeleteFunc(disabled, func(id string) bool { return id == t.ID })
						} else {
							disabled = append(disabled, t.ID)
						}
						a.action(acc.Mid, "账号任务已更新", func(cl *api.Client, ctx context.Context) error {
							return cl.Patch(ctx, acc.Mid, api.AccountPatch{DisabledTasks: &disabled})
						})
					}
				}
			})
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Button(c, "立即运行").Disabled(!acc.LoggedIn || acc.Running != nil || a.busy[acc.Mid]).OnClick(func() { a.run(acc.Mid, nil) })
				ui.Button(c, "刷新资料").Disabled(a.busy[acc.Mid]).OnClick(func() {
					a.action(acc.Mid, "资料已刷新", func(cl *api.Client, ctx context.Context) error { return cl.Refresh(ctx, acc.Mid) })
				})
				ui.Button(c, "退出账号").Disabled(a.busy[acc.Mid]).OnClick(func() {
					a.confirmOpen = true
					a.confirmTitle = "退出「" + accountName(acc) + "」？"
					a.confirmText = "这会在 B 站注销当前会话，并删除本机或服务器上这个账号的数据。"
					a.confirmAction = func() {
						a.action(acc.Mid, "账号已退出", func(cl *api.Client, ctx context.Context) error { return cl.Remove(ctx, acc.Mid) })
					}
				})
			})
		})
	}
}
func (a *app) accountFilter(c *ui.Context, key string, mid *string) bool {
	changed := false
	ui.Row(c.Key(key)).Gap(6).Wrap().Children(func() {
		ui.Button(c, "全部账号").OnClick(func() { *mid = ""; changed = true })
		for _, acc := range a.st.Accounts {
			ui.Button(c.Key(acc.Mid), accountName(acc)).OnClick(func() { *mid = acc.Mid; changed = true })
		}
	})
	if *mid != "" {
		muted(c, "当前账号："+a.name(*mid))
	}
	return changed
}
func triggerText(s string) string {
	switch s {
	case "schedule":
		return "定时"
	case "catchup":
		return "补跑"
	case "retry":
		return "重试"
	case "manual":
		return "手动"
	}
	return s
}
func (a *app) reportsView(c *ui.Context) {
	if a.report != nil {
		r := a.report
		ui.Button(c, "返回报告列表").OnClick(func() { a.report = nil; a.loadReports() })
		ui.Text(c, r.Uname+" · "+r.Started.Local().Format("01-02 15:04")+" · "+triggerText(r.Trigger))
		ui.Text(c, r.Summary).Bold()
		for _, task := range r.Tasks {
			card(c).Children(func() {
				ui.Row(c).Gap(10).Wrap().Children(func() {
					ui.Text(c, task.Title).Bold().FontSize(18)
					ui.Badge(c, statusText(task.Status)).TextColor(statusColor(c, task.Status))
					muted(c, fmt.Sprintf("%.1f 秒", float64(task.Millis)/1000))
				})
				ui.Text(c, task.Note)
				for _, s := range task.Steps {
					ui.Column(c).Gap(5).Padding(12).BorderWidth(0, 0, 0, 3).BorderColor(statusColor(c, s.Status)).Background(c.Theme().Surface).Children(func() {
						ui.Text(c, s.Name+" · "+statusText(s.Status)).Bold().TextColor(statusColor(c, s.Status))
						if s.Detail != "" {
							ui.Text(c, s.Detail).Selectable()
						}
						if s.Hint != "" {
							ui.Text(c, "建议："+s.Hint).Bold().FontSize(15).TextColor(c.Theme().Danger).Selectable()
						}
						if s.Code != 0 {
							muted(c, fmt.Sprintf("返回码 %d", s.Code)).FontSize(11)
						}
					})
				}
			})
		}
		return
	}
	if a.accountFilter(c, "reports-filter", &a.reportMid) {
		a.loadReports()
	}
	ui.Button(c, "刷新报告").Disabled(a.reportsBusy).OnClick(a.loadReports)
	if a.reportsBusy {
		ui.Spinner(c)
	}
	errorText(c, a.reportErr)
	if len(a.runs) == 0 && !a.reportsBusy {
		muted(c, "还没有运行报告。任务运行后，会在这里逐步说明结果和失败原因。")
	}
	for _, r := range a.runs {
		r := r
		card(c).Children(func() {
			ui.Row(c).Gap(10).Wrap().Children(func() {
				ui.Text(c, r.Started.Local().Format("01-02 15:04")+" · "+r.Uname).Bold()
				ui.Badge(c, statusText(r.Status)).TextColor(statusColor(c, r.Status))
				muted(c, triggerText(r.Trigger))
			})
			ui.Text(c, r.Summary)
			ui.Button(c, "查看报告").Disabled(a.reportsBusy).OnClick(func() { a.openReport(r.ID) })
		})
	}
}
func (a *app) logsView(c *ui.Context) {
	reset := a.accountFilter(c, "logs-filter", &a.logMid)
	levels := []string{"", "debug", "info", "warn", "error"}
	labels := []string{"全部", "调试", "信息", "警告", "错误"}
	idx := slices.Index(levels, a.logLevel)
	if idx < 0 {
		idx = 0
	}
	if ui.Segmented(c, &idx, labels...).Changed() {
		a.logLevel = levels[idx]
		reset = true
	}
	ui.Checkbox(c, &a.autoScroll, "自动滚动")
	if reset {
		a.logs = nil
		a.logSeq = 0
		a.loadLogs()
	}
	if len(a.logs) == 0 {
		muted(c, "还没有符合筛选条件的日志")
	}
	ui.Scroll(c.Key("log-lines")).Height(480).FillWidth().TrackScroll(&a.logScroll).Background(c.Theme().Background).Padding(12).Children(func() {
		ui.Column(c).Gap(5).Children(func() {
			for _, l := range a.logs {
				color := c.Theme().Text
				if l.Level == "error" || l.Level == "warn" {
					color = c.Theme().Danger
				}
				ui.Text(c, l.Time.Local().Format("15:04:05")+" ["+l.Level+"] "+l.Mid+" "+l.Text).Font("monospace").FontSize(12).TextColor(color).Selectable()
			}
		})
	})
}
func (a *app) dialogs(c *ui.Context) {
	ui.Modal(c, &a.qrOpen, func() {
		ui.Column(c).Width(min(350, a.width-64)).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "添加 B 站账号").Bold().FontSize(20)
			muted(c, "用 B 站 App 扫码并确认。已有账号会更新登录。")
			if a.qrImage != nil && a.qr.State != "expired" {
				ui.Box(c).Padding(10).Background(ui.Hex("#ffffff")).Children(func() { ui.Image(c, a.qrImage).Size(220, 220) })
			} else if a.qrBusy {
				ui.Spinner(c)
			}
			s := "正在获取二维码"
			switch a.qr.State {
			case "waiting":
				s = "用 B 站 App 扫一扫"
			case "scanned":
				s = "已扫码，请在手机上确认"
			case "expired":
				s = "二维码已过期"
			}
			ui.Text(c, s)
			errorText(c, a.qrErr)
			ui.Row(c).Gap(8).Children(func() {
				if a.qr.State == "expired" || a.qrErr != "" {
					ui.Button(c, "换一张").Disabled(a.qrBusy).OnClick(a.startQR)
				}
				ui.Button(c, "关闭").OnClick(func() { a.qrOpen = false })
			})
		})
	})
	if ui.AlertDialog(c, &a.confirmOpen, a.confirmTitle, a.confirmText, "取消", "退出账号") == 1 && a.confirmAction != nil {
		a.confirmAction()
	}
	a.runDialog(c)
}
func (a *app) runDialog(c *ui.Context) {
	ui.Modal(c, &a.pickOpen, func() {
		ui.Column(c).Width(min(420, a.width-64)).Gap(12).Children(func() {
			ui.Text(c, "运行一次："+a.title(a.pickTask)).Bold().FontSize(18)
			muted(c, "选择账号")
			for _, acc := range a.st.Accounts {
				acc := acc
				ui.Button(c, accountName(acc)).Disabled(!acc.LoggedIn || a.groupsBusy).OnClick(func() { a.chooseRunAccount(acc.Mid) })
			}
			if a.pickTask == "unfollow" && a.pickMid != "" {
				if a.groupsBusy {
					ui.Spinner(c)
				} else if len(a.groups) == 0 {
					muted(c, "这个账号还没有关注分组")
				} else {
					labels := make([]string, len(a.groups))
					for i, g := range a.groups {
						labels[i] = fmt.Sprintf("%s（%d 人）", g.Name, g.Count)
					}
					ui.Field(c, "关注分组", func() {
						selected := labels[a.groupIdx]
						if ui.Select(c, &selected, labels).Changed() {
							a.groupIdx = slices.Index(labels, selected)
						}
					})
					ui.Field(c, "最多取关数量（0 表示全部）", func() { ui.TextInput(c, &a.unfollowCount) })
					ui.Field(c, "保留的 UID（逗号分隔）", func() { ui.TextInput(c, &a.unfollowKeep) })
					ui.PrimaryButton(c, "保存选项并运行").Disabled(a.busy["unfollow"]).OnClick(a.submitUnfollow)
				}
			}
			errorText(c, a.pickErr)
			ui.Button(c, "取消").OnClick(func() { a.pickOpen = false })
		})
	})
}
func (a *app) submitUnfollow() {
	count, err := strconv.Atoi(strings.TrimSpace(a.unfollowCount))
	if err != nil || count < 0 {
		a.pickErr = "取关数量要填大于或等于 0 的整数"
		return
	}
	keep := []int64{}
	for _, s := range strings.FieldsFunc(a.unfollowKeep, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\n' }) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			a.pickErr = "保留列表要填有效的 UID"
			return
		}
		keep = append(keep, id)
	}
	s := cloneSettings(a.form)
	s.Tasks.Unfollow = api.UnfollowOptions{Group: a.groups[a.groupIdx].Name, Count: count, Keep: keep}
	mid := a.pickMid
	cl, ctx, epoch := a.cl, a.ctx, a.epoch
	a.busy["unfollow"] = true
	a.pickErr = ""
	a.async(func() func() {
		saved, err := cl.SaveSettings(ctx, s)
		if err == nil {
			err = cl.Run(ctx, mid, []string{"unfollow"})
		}
		return func() {
			if epoch != a.epoch {
				return
			}
			a.busy["unfollow"] = false
			if err != nil {
				a.pickErr = err.Error()
				return
			}
			a.form = saved
			a.dirty = false
			a.inputs = map[string]string{}
			a.pickOpen = false
			a.toast("已安排批量取关")
			a.poll()
		}
	})
}
