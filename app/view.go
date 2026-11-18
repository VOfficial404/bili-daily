package main

import (
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

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

func (a *app) ringSegs(c *ui.Context, lines []client.TodayLine) []ringSeg {
	segs := make([]ringSeg, len(lines))
	for i, l := range lines {
		col := statusColor(c, l.Status)
		switch l.Status {
		case "":
			col = c.Theme().Border
		case api.StatusSkipped:
			col = c.Theme().TextMuted.Alpha(0.55)
		}
		segs[i] = ringSeg{color: col}
	}
	return segs
}

// avatar 画头像；没有头像时用名字的第一个字
func (a *app) avatar(c *ui.Context, acc api.Account, size float32) {
	t := c.Theme()
	if bm := a.faces[acc.Face]; bm != nil {
		ui.Image(c, bm).Size(size, size).Radius(size / 2).Fit(ui.Cover)
		return
	}
	first := []rune(accountName(acc))[0:1]
	ui.Box(c).Size(size, size).Radius(size / 2).Background(t.Accent.Alpha(0.16)).Center().Children(func() {
		ui.Text(c, string(first)).Bold().FontSize(size * 0.4).TextColor(t.Accent)
	})
}

func (a *app) ringAvatar(c *ui.Context, acc api.Account, size float32) {
	lines := client.TodayPlan(*a.st, acc)
	todayRing(c, size, a.ringSegs(c, lines), c.Theme().Border, func() { a.avatar(c, acc, size-14) })
}

/* 外框 */

func (a *app) view(c *ui.Context) {
	applyTheme(c)
	for _, s := range a.toasts {
		c.Toast(s)
	}
	a.toasts = nil
	a.width, a.height = c.Size()
	a.narrow = a.width < 760
	if a.session.Client() == nil || a.st == nil {
		a.connectView(c)
		return
	}
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		if !a.narrow {
			a.sidebar(c)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if a.loadErr != "" {
				ui.Row(c).Gap(10).Padding(10, 24).Background(t.Danger.Alpha(0.12)).Children(func() {
					ui.Text(c, a.loadErr).TextColor(t.Danger).FontSize(13).Grow(1).Selectable()
					muted(c, "正在重试，下面是上次的数据").FontSize(12)
				})
			}
			ui.Scroll(c.Key("page-" + a.page)).Grow(1).MinHeight(0).Children(func() {
				pad := [3]float32{28, 32, 32}
				if a.narrow {
					pad = [3]float32{16, 16, 24}
				}
				ui.Column(c).Padding(pad[0], pad[1], pad[2]).Gap(20).MaxWidth(1040).Children(func() {
					switch a.page {
					case "今天":
						a.overviewView(c)
					case "账号":
						a.pageHeader(c, "账号", "每个账号的登录状态、等级和它要做的任务。", func() {
							ui.Button(c.Key("add-account"), "").Label("添加账号").Disabled(a.qrBusy).OnClick(a.startQR).Children(func() {
								ui.Icon(c, iconPlus)
								ui.Text(c, "添加账号").SingleLine()
							})
						})
						a.accountsView(c)
					case "任务":
						a.pageHeader(c, "任务", "所有账号共用这一份设置。改完记得保存。", nil)
						a.tasksView(c)
					case "报告":
						a.reportsView(c)
					case "设置":
						a.pageHeader(c, "设置", "", nil)
						a.settingsView(c)
					case "日志":
						a.pageHeader(c, "日志", "最近的运行日志，打开这一页时自动拉取新的。", nil)
						a.logsView(c)
					}
				})
			})
			if a.narrow {
				a.tabBar(c)
			}
		})
	})
	a.dialogs(c)
}

// pageHeader 是页面顶部：标题、一句说明，右边是这一页的操作
func (a *app) pageHeader(c *ui.Context, title, desc string, actions func()) {
	ui.Row(c).Gap(12).AlignItems(ui.End).Children(func() {
		ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(26).Bold()
			if desc != "" {
				muted(c, desc).FontSize(13)
			}
		})
		if actions != nil {
			ui.Row(c).Gap(8).Children(actions)
		}
	})
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(216).Padding(20, 14, 16).Gap(20).Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Gap(10).Padding(0, 6).Children(func() {
			brandMark(c, 26)
			ui.Text(c, "Bili Daily").FontSize(17).Bold()
		})
		ui.Sidebar(c, &a.page, func() {
			for _, p := range navPages {
				item := ui.SidebarItem(c, p.id, p.icon, p.id)
				if p.id == "账号" {
					if n := a.loggedOut(); n > 0 {
						item.Children(func() { pill(c, strconv.Itoa(n)+" 个失效", t.Danger) })
					}
				}
				if item.Clicked() {
					a.navigate(p.id)
				}
			}
		}).Grow(1).Background(t.Surface)
		a.connectionInfo(c)
	})
}

func (a *app) loggedOut() int {
	n := 0
	for _, acc := range a.st.Accounts {
		if !acc.LoggedIn {
			n++
		}
	}
	return n
}

// tabBar 是窄窗口和手机底部的标签栏：图标在上，文字在下。
// 手机上铺到屏幕底边（系统的手势条下面也是同一个颜色），键盘弹出时让出位置。
func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	_, _, bottom, _ := c.SafeArea()
	bar := ui.Row(c).Padding(4, 6, 6+bottom, 6).Margin(0, 0, -bottom, 0).Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
	if bottom > 120 {
		bar.Hidden(true)
		return
	}
	bar.Children(func() {
		for _, p := range navPages {
			col := t.TextMuted
			if a.page == p.id {
				col = t.Accent
			}
			ui.ButtonBase(c.Key("tab-"+p.id)).Grow(1).Basis(0).Gap(2).Padding(6, 0).Center().Label(p.id).
				OnClick(func() { a.navigate(p.id) }).Children(func() {
				ui.Icon(c, p.icon).FontSize(20).TextColor(col)
				ui.Text(c, p.id).FontSize(11).TextColor(col).SingleLine()
			})
		}
	})
}

// connectionInfo 是侧边栏底部：连的是哪里、是否正常、B 站看到的出口 IP
func (a *app) connectionInfo(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(6).Padding(12).Radius(10).Background(t.Background).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			dot := t.Success
			label := "已连接服务器"
			if a.session.LocalEngine() != nil {
				label = "本机运行中"
			}
			if a.loadErr != "" {
				dot, label = t.Danger, "连接中断"
			}
			ui.Box(c).Size(8, 8).Radius(4).Background(dot)
			ui.Text(c, label).FontSize(12).FontWeight(600).SingleLine()
		})
		where := a.session.Client().Base()
		if a.session.LocalEngine() != nil {
			where = "这台电脑（本机运行）"
		}
		muted(c, where).FontSize(11).Selectable().SingleLine().Ellipsis("…").Tooltip(where)
		if e := a.st.Egress; e != nil {
			if e.Error != "" {
				ui.Text(c, "出口 IP 查询失败").FontSize(11).TextColor(t.Danger).Tooltip(e.Error)
			} else {
				muted(c, "出口 "+e.Addr+" · "+e.Place).FontSize(11).Selectable().SingleLine().Ellipsis("…").Tooltip("B 站看到的出口 IP")
			}
		}
		muted(c, "服务端 "+a.st.Version+" · 客户端 "+version).FontSize(11)
		for _, s := range checkUpdates(a.st, version).status() {
			muted(c, s).FontSize(11)
		}
	})
}

/* 连接页 */

func (a *app) connectView(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Background(t.Background).Children(func() {
		ui.Column(c).FillWidth().MinHeight(560).Padding(48, 24).Center().Children(func() {
			ui.Column(c).Width(min(440, a.width-48)).Gap(22).Children(func() {
				ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
					brandMark(c, 52)
					ui.Text(c, "Bili Daily").FontSize(28).Bold()
					muted(c, "每天替你做完 B 站的日常任务，做没做成、为什么，都写得明白。").TextAlign(ui.Center)
				})
				panel(c).Padding(24).Gap(18).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18)).Children(func() {
					ui.Segmented(c.Key("mode"), &a.mode, "连接服务器", "在本机运行").Disabled(a.connecting)
					if a.mode == 0 {
						ui.Field(c, "服务器地址", func() {
							ui.TextInput(c.Key("server"), &a.server).Placeholder("http://127.0.0.1:8686").Disabled(a.connecting).
								Keyboard(ui.KeyboardURL).Capitalize(ui.CapitalizeNone).Autocorrect(false)
						})
						ui.Field(c, "令牌", func() { ui.TextInput(c.Key("token"), &a.token).Password().Label("令牌").Disabled(a.connecting) })
						if ui.Checkbox(c, &a.remember, "记住令牌，下次打开自动连接").Disabled(a.connecting).Changed() && !a.remember {
							// 取消勾选时马上删掉记住的令牌，不等下次连接成功
							if err := a.savePrefs(); err != nil {
								a.connectErr = "删除记住的令牌失败：" + err.Error()
							}
						}
						muted(c, "令牌在服务器上用 bili-daily token 查看，记住后保存在系统的凭据存储里。").FontSize(12)
					} else {
						local := "没有服务器也能用：任务由这台电脑执行，只在程序开着时运行。关掉窗口后留在托盘里继续跑，错过的那一轮下次打开时补上。"
						if a.mobile {
							local = "没有服务器也能用：任务由这台手机执行。系统会暂停后台的 App，到点的那一轮在系统给出时间时或下次打开 App 时补上；要准时运行，请连接服务器。"
						}
						muted(c, local).FontSize(13)
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
						ui.Row(c).Gap(8).Center().Children(func() {
							ui.Spinner(c)
							muted(c, "正在连接…").FontSize(13)
						})
					}
				})
			})
		})
	})
}
