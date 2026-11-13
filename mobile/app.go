package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/engine"
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/ui"
)

const mobileVersion = "0.1.0"

type mobileApp struct {
	cx                          *core.Scope
	ctx                         context.Context
	client                      *api.Client
	local                       *engine.Engine
	localStopRoot               func() bool
	state                       *core.Signal[api.State]
	connected, busy, refreshing *core.Signal[bool]
	errorText                   *core.Signal[string]
	tab                         *core.Signal[int]
	nav                         *base.Nav[string]
	toaster                     *ui.Toaster
	epoch                       int
	active                      bool
	stopPoll                    func() bool
	cancel                      context.CancelFunc
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	theme := core.DefaultTheme()
	theme.Light.Accent, theme.Dark.Accent = core.RGB(0xc12c5a), core.RGB(0xbf2858)
	cx.SetTheme(theme)
	cx.SetAppearance(core.AppearanceSystem)
	a := &mobileApp{cx: cx, state: core.NewSignalFunc(cx, api.State{}, nil), connected: core.NewSignal(cx, false), busy: core.NewSignal(cx, false), refreshing: core.NewSignal(cx, false), errorText: core.NewSignal(cx, ""), tab: core.NewSignal(cx, 0), nav: base.NewNav(cx, "root"), toaster: ui.NewToaster(cx), active: true}
	a.ctx = cx.Context()
	cx.Cleanup(func() { a.stop() })
	cx.OnQuit(func(context.Context) { a.stop() })
	cx.Effect(func() {
		active := cx.Env().Lifecycle() == core.LifecycleActive
		if active == a.active {
			return
		}
		a.active = active
		if !active {
			if a.stopPoll != nil {
				a.stopPoll()
			}
		} else if a.connected.GetUntracked() {
			if a.local != nil {
				a.local.Wake()
			}
			a.refresh()
		}
	})
	return core.View{TestID: "mobile-root", Style: core.Style{FlexGrow: 1, FlexShrink: 1, PaddingEnv: core.EnvAll(core.EnvSafeArea)}, StyleFunc: func() core.Style { return core.Style{Background: cx.Palette().Background} }, Children: []core.Node{
		core.Show{When: a.connected.Get, Then: func(cx *core.Scope) core.Node { return a.shell(cx) }, Else: func(cx *core.Scope) core.Node { return a.connectPage(cx) }},
		core.Show{When: func() bool { return a.errorText.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{ui.Textf(a.errorText.Get, 13, ui.Medium, ui.ColorsOf(cx).Danger)}}
		}},
		ui.Toasts{Toaster: a.toaster, Placement: ui.ToastBottom},
	}}
}

func (a *mobileApp) snapshot() api.State { return a.state.Get() }
func (a *mobileApp) isBusy() bool        { return a.busy.Get() }
func (a *mobileApp) message(text string) {
	a.toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: text, Duration: 3 * time.Second})
}
func (a *mobileApp) fail(err error) {
	a.errorText.Set(err.Error())
	a.toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: err.Error()})
}
func (a *mobileApp) stop() {
	if a.stopPoll != nil {
		a.stopPoll()
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.local != nil {
		a.local.Stop()
		a.local = nil
		if a.localStopRoot != nil {
			a.localStopRoot()
			a.localStopRoot = nil
		}
	}
}
func (a *mobileApp) disconnect() {
	a.stop()
	a.epoch++
	a.client = nil
	a.connected.Set(false)
	a.busy.Set(false)
	a.refreshing.Set(false)
	a.errorText.Set("")
	a.nav.SetRoutes("root")
	a.cx.Storage().RemoveItem("remember")
	a.cx.Storage().RemoveItem("token")
}
func (a *mobileApp) perform(label string, work func(context.Context) error, done func()) {
	if a.busy.GetUntracked() {
		return
	}
	a.busy.Set(true)
	a.errorText.Set("")
	ctx, cancel := context.WithTimeout(a.ctx, 35*time.Second)
	a.cancel = cancel
	epoch := a.epoch
	go func() {
		err := work(ctx)
		cancel()
		a.cx.Post(func() {
			if epoch != a.epoch {
				return
			}
			a.busy.Set(false)
			if err != nil {
				a.fail(err)
				return
			}
			if done != nil {
				done()
			}
			if label != "" {
				a.message(label)
			}
		})
	}()
}
func (a *mobileApp) connect(address, token string, remember bool) {
	baseURL, err := api.NormalizeBase(address)
	if err != nil {
		a.fail(err)
		return
	}
	client := api.NewClient(baseURL, token)
	var state api.State
	a.perform("已连接", func(ctx context.Context) error {
		if _, err := client.Ping(ctx); err != nil {
			return err
		}
		var err error
		state, err = client.State(ctx)
		if api.IsUnauthorized(err) {
			return errors.New("令牌不对，请检查服务器令牌")
		}
		return err
	}, func() {
		store := a.cx.Storage()
		store.RemoveItem("token")
		store.RemoveItem("remember")
		if remember {
			if err := store.SetItem("address", baseURL); err != nil {
				a.fail(err)
			}
			if err := store.SetItem("token", token); err != nil {
				a.fail(err)
			}
			if err := store.SetItem("remember", "true"); err != nil {
				a.fail(err)
			}
		}
		a.client = client
		a.state.Set(state)
		a.connected.Set(true)
		a.schedulePoll()
	})
}
func (a *mobileApp) startLocal(dir, proxy string) {
	var e *engine.Engine
	var client *api.Client
	var state api.State
	var stopRoot func() bool
	a.perform("本机运行已启动", func(ctx context.Context) error {
		var err error
		e, err = engine.New(engine.Config{DataDir: dir, Version: mobileVersion, Local: true})
		if err != nil {
			return err
		}
		stopRoot = context.AfterFunc(a.ctx, e.Stop)
		ok := false
		defer func() {
			if !ok {
				stopRoot()
				e.Stop()
			}
		}()
		addr, err := e.ServeLocal()
		if err != nil {
			return err
		}
		client = api.NewClient(addr, e.Token())
		s := e.Settings()
		s.Proxy = proxy
		if _, err = client.SaveSettings(ctx, s); err != nil {
			return err
		}
		state, err = client.State(ctx)
		ok = err == nil
		return err
	}, func() {
		a.local = e
		a.localStopRoot = stopRoot
		e.Start()
		a.client = client
		a.state.Set(state)
		a.connected.Set(true)
		a.schedulePoll()
	})
}
func (a *mobileApp) schedulePoll() {
	if a.stopPoll != nil {
		a.stopPoll()
	}
	if !a.active || !a.connected.GetUntracked() {
		return
	}
	delay := 3 * time.Second
	for _, v := range a.state.GetUntracked().Accounts {
		if v.Running != nil {
			delay = time.Second
			break
		}
	}
	owner := a.cx.Child()
	owner.AfterFunc(delay, a.refresh)
	a.stopPoll = func() bool { owner.Dispose(); return true }
}
func (a *mobileApp) refresh() {
	if !a.active || !a.connected.GetUntracked() || a.refreshing.GetUntracked() {
		return
	}
	a.refreshing.Set(true)
	client, epoch := a.client, a.epoch
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	go func() {
		state, err := client.State(ctx)
		cancel()
		a.cx.Post(func() {
			if epoch != a.epoch {
				return
			}
			a.refreshing.Set(false)
			if err != nil {
				a.errorText.Set(err.Error())
			} else {
				a.state.Set(state)
				a.errorText.Set("")
			}
			a.schedulePoll()
		})
	}()
}
func (a *mobileApp) saveSettings(s api.Settings) {
	var saved api.Settings
	a.perform("设置已保存", func(ctx context.Context) error { var err error; saved, err = a.client.SaveSettings(ctx, s); return err }, func() { state := a.state.GetUntracked(); state.Settings = saved; a.state.Set(state); a.refresh() })
}
func mobileText(cx *core.Scope, text string) core.Node {
	return ui.Text(text, 14, ui.Regular, ui.ColorsOf(cx).Text)
}
func mobileColumn(children ...core.Node) core.Node {
	return core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{core.View{Style: core.Style{Padding: core.All(16), Gap: 14, PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}}, Children: children}}}
}
func (a *mobileApp) button(text string, fn func()) core.Node {
	return ui.Button{Text: text, OnPress: fn, Disabled: a.isBusy}
}

func (a *mobileApp) connectPage(cx *core.Scope) core.Node {
	mode := core.NewSignal(cx, 0)
	address := core.NewSignal(cx, "")
	token := core.NewSignal(cx, "")
	remember := core.NewSignal(cx, false)
	proxy := core.NewSignal(cx, "")
	store := cx.Storage()
	if v, ok := store.GetItem("address"); ok {
		address.Set(v)
	}
	if v, ok := store.GetItem("remember"); ok && v == "true" {
		remember.Set(true)
		if v, ok := store.GetItem("token"); ok {
			token.Set(v)
		}
		cx.Post(func() { a.connect(address.GetUntracked(), token.GetUntracked(), true) })
	}
	return mobileColumn(ui.Text("Bili Tools", 28, ui.Medium, ui.ColorsOf(cx).Text), mobileText(cx, "手机推荐连接服务器，关掉 App 后服务器仍会按计划运行。"),
		ui.Tabs[int]{Options: []ui.Option[int]{{Value: 0, Label: "连接服务器"}, {Value: 1, Label: "在本机运行"}}, Value: mode.Get, OnChange: mode.Set, Label: "运行方式"},
		core.Show{When: func() bool { return mode.Get() == 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 14}, Children: []core.Node{
				mobileText(cx, "服务器地址"),
				ui.Input{Label: "服务器地址", TestID: "server-address", Value: address.Get, OnChange: address.Set, Placeholder: "https://bili.example.com", KeyboardType: core.KeyboardURL, Disabled: a.isBusy},
				mobileText(cx, "令牌"),
				ui.Input{Label: "令牌", TestID: "server-token", Value: token.Get, OnChange: token.Set, Masked: true, Disabled: a.isBusy},
				mobileText(cx, "记住并在下次自动连接"),
				ui.Switch{Label: "记住并在下次自动连接", Checked: remember.Get, OnChange: func(v bool) {
					remember.Set(v)
					if !v {
						store.RemoveItem("token")
						store.RemoveItem("remember")
					}
				}}, mobileText(cx, "仅勾选记住时保存令牌；令牌以明文保存在 App 私有目录。"), a.button("连接", func() { a.connect(address.Get(), token.Get(), remember.Get()) }),
			}}
		}, Else: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 14}, Children: []core.Node{
				mobileText(cx, "本机任务只能在 App 打开时可靠运行。手机系统可能暂停后台运行；稳定定时请连接服务器，回到 App 时补跑。"),
				mobileText(cx, "代理（可不填）"),
				ui.Input{Label: "代理（可不填）", Value: proxy.Get, OnChange: proxy.Set, Placeholder: "http://127.0.0.1:7890", Disabled: a.isBusy},
				a.button("启动本机运行", func() {
					dir := cx.DataDir()
					if dir == "" {
						a.fail(errors.New("网页没有本机数据目录，请连接服务器"))
						return
					}
					a.startLocal(filepath.Join(dir, "data"), proxy.Get())
				}),
			}}
		}},
	)
}
func (a *mobileApp) shell(cx *core.Scope) core.Node {
	titles := []string{"今天", "账号", "报告", "设置"}
	selectTab := func(i int) { a.tab.Set(i); a.nav.SetRoutes("root") }
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		a.nav.Stack(core.Style{FlexGrow: 1, FlexShrink: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
			title := ""
			var content core.Node
			switch {
			case route == "root":
				return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return titles[a.tab.Get()] }, Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
					switch a.tab.Get() {
					case 0:
						return a.todayPage(cx)
					case 1:
						return a.accountsPage(cx)
					case 2:
						return a.reportsPage(cx)
					default:
						return a.settingsPage(cx)
					}
				})}}, ui.NavHeader{})
			case route == "qr":
				title = "添加账号"
				content = a.qrPage(cx)
			case strings.HasPrefix(route, "account:"):
				title = "账号详情"
				content = a.accountPage(cx, strings.TrimPrefix(route, "account:"))
			case strings.HasPrefix(route, "report:"):
				title = "运行报告"
				content = a.reportPage(cx, strings.TrimPrefix(route, "report:"))
			case strings.HasPrefix(route, "task:"):
				id := strings.TrimPrefix(route, "task:")
				title = a.taskTitle(id)
				content = a.taskOptionsPage(cx, id)
			case route == "unfollow":
				title = "批量取关"
				content = a.unfollowPage(cx)
			}
			return ui.NavPage(cx, base.NavPage{Title: title, Children: []core.Node{content}}, ui.NavHeader{BackLabel: "返回", OnBack: func() { a.nav.Pop(1) }})
		}),
		mobile.TabBar{Items: []mobile.TabBarItem{{Title: "今天", Icon: mobileTabIcon(cx, "house", "android:ic_menu_today")}, {Title: "账号", Icon: mobileTabIcon(cx, "person.crop.circle", "android:ic_menu_myplaces")}, {Title: "报告", Icon: mobileTabIcon(cx, "doc.text", "android:ic_menu_agenda")}, {Title: "设置", Icon: mobileTabIcon(cx, "gearshape", "android:ic_menu_preferences")}}, Selected: a.tab.Get, OnChange: selectTab},
		core.Show{When: func() bool { return !mobile.IsAvailable(cx) }, Then: func(cx *core.Scope) core.Node {
			return ui.Tabs[int]{Options: []ui.Option[int]{{Value: 0, Label: "今天"}, {Value: 1, Label: "账号"}, {Value: 2, Label: "报告"}, {Value: 3, Label: "设置"}}, Value: a.tab.Get, OnChange: selectTab, Label: "页面"}
		}},
	}}
}
func mobileTabIcon(cx *core.Scope, apple, android string) string {
	if cx.Env().OS() == core.OSAndroid {
		return android
	}
	return apple
}
func (a *mobileApp) openTask(id string) { a.nav.Push("task:" + id) }
func (a *mobileApp) openUnfollow()      { a.nav.Push("unfollow") }
func (a *mobileApp) taskTitle(id string) string {
	for _, task := range a.state.GetUntracked().Tasks {
		if task.ID == id {
			return task.Title
		}
	}
	if id == "login" {
		return "登录检查"
	}
	return id
}
func mobileStatus(status api.Status) string {
	switch status {
	case api.StatusOK:
		return "✓ 完成"
	case api.StatusPartial:
		return "! 部分完成"
	case api.StatusFailed:
		return "× 失败"
	case api.StatusSkipped:
		return "− 跳过"
	case api.StatusRunning:
		return "正在运行"
	}
	return "等待运行"
}
func mobileTime(t *time.Time) string {
	if t == nil {
		return "未安排"
	}
	return t.In(engine.Beijing).Format("01-02 15:04")
}
func (a *mobileApp) run(mid string) {
	a.perform("已开始运行", func(ctx context.Context) error { return a.client.Run(ctx, mid, nil) }, a.refresh)
}
func (a *mobileApp) todayPage(cx *core.Scope) core.Node {
	return core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, OnRefresh: a.refresh, Refreshing: a.refreshing.Get, Children: []core.Node{core.View{Style: core.Style{Padding: core.All(16), Gap: 16}, Children: []core.Node{
		mobileText(cx, "今天的结果按北京时间统计"), a.button("全部立即运行", func() { a.perform("已开始全部运行", a.client.RunAll, a.refresh) }), a.button("添加账号", func() { a.nav.Push("qr") }),
		core.Dynamic(func(cx *core.Scope) core.Node {
			state := a.snapshot()
			var rows []core.Node
			if state.Local {
				rows = append(rows, mobileText(cx, "本机运行：手机系统可能暂停后台运行；稳定定时请连接服务器，回到 App 时补跑。"))
			}
			if len(state.Accounts) == 0 {
				rows = append(rows, mobileText(cx, "还没有账号，点击添加账号扫码登录。"))
			}
			for _, acc := range state.Accounts {
				children := []core.Node{}
				if acc.Face != "" {
					children = append(children, core.Image{Source: core.Src(acc.Face), ResizeMode: core.Cover, Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 24}})
				}
				children = append(children, ui.FormRow{Title: acc.Uname, Detail: fmt.Sprintf("Lv%d · 硬币 %.1f", acc.Level, acc.Coins), OnPress: func() { a.nav.Push("account:" + acc.Mid) }})
				for _, task := range acc.Today {
					children = append(children, mobileText(cx, fmt.Sprintf("%s  %s\n%s", mobileStatus(task.Status), a.taskTitle(task.ID), task.Note)))
				}
				if len(acc.Today) == 0 {
					children = append(children, mobileText(cx, "今天还没有运行结果"))
				}
				if acc.Running != nil {
					children = append(children, mobileText(cx, fmt.Sprintf("正在运行 %s（%d/%d）", acc.Running.Task, acc.Running.Done, acc.Running.Total)), a.button("中止", func() {
						a.perform("已请求中止", func(ctx context.Context) error { return a.client.Cancel(ctx, acc.Mid) }, a.refresh)
					}))
				} else {
					children = append(children, a.button("立即运行", func() { a.run(acc.Mid) }))
				}
				children = append(children, mobileText(cx, "下次运行（北京时间）："+mobileTime(acc.NextRun)))
				if acc.LastRun != nil {
					run := *acc.LastRun
					children = append(children, ui.FormRow{Title: "上次：" + mobileStatus(run.Status), Detail: run.Summary, OnPress: func() { a.nav.Push("report:" + run.ID) }})
				}
				rows = append(rows, ui.FormSection{Title: acc.Uname, Children: children})
			}
			return core.View{Style: core.Style{Gap: 16}, Children: rows}
		}),
	}}}}
}
func (a *mobileApp) accountsPage(cx *core.Scope) core.Node {
	return mobileColumn(a.button("添加账号", func() { a.nav.Push("qr") }), core.Dynamic(func(cx *core.Scope) core.Node {
		state := a.snapshot()
		var rows []core.Node
		for _, acc := range state.Accounts {
			detail := "已登录"
			if !acc.LoggedIn {
				detail = "登录失效，请重新扫码"
			}
			rows = append(rows, ui.FormRow{Title: acc.Uname, Detail: detail, OnPress: func() { a.nav.Push("account:" + acc.Mid) }})
		}
		if len(rows) == 0 {
			rows = append(rows, mobileText(cx, "还没有账号，添加后参加每日计划。"))
		}
		return ui.FormSection{Title: "账号", Children: rows}
	}))
}
func (a *mobileApp) accountPage(cx *core.Scope, mid string) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		state := a.snapshot()
		var acc api.Account
		found := false
		for _, v := range state.Accounts {
			if v.Mid == mid {
				acc = v
				found = true
				break
			}
		}
		if !found {
			return mobileColumn(mobileText(cx, "账号已退出"))
		}
		rows := []core.Node{mobileText(cx, fmt.Sprintf("%s · UID %s\n等级 %d · 经验 %d / %d\n硬币 %.1f · B 币券 %.1f", acc.Uname, acc.Mid, acc.Level, acc.Exp, acc.NextExp, acc.Coins, acc.BCoupon))}
		vip := acc.VIP
		if vip == "" {
			vip = "普通会员"
		}
		rows = append(rows, mobileText(cx, vip+"\n大会员到期："+mobileTime(acc.VIPDue)+"\nCookie 到期："+mobileTime(acc.CookieExpires)))
		rows = append(rows, mobileText(cx, "参加每日计划"))
		rows = append(rows, core.View{Disabled: a.isBusy, Children: []core.Node{ui.Switch{Label: "参加每日计划", Checked: func() bool { return acc.Enabled }, OnChange: func(v bool) {
			a.perform("每日计划已更新", func(ctx context.Context) error { return a.client.Patch(ctx, mid, api.AccountPatch{Enabled: &v}) }, a.refresh)
		}}}})
		for _, task := range state.Tasks {
			if task.Manual {
				continue
			}
			id := task.ID
			rows = append(rows, mobileText(cx, task.Title))
			rows = append(rows, core.View{Disabled: a.isBusy, Children: []core.Node{ui.Switch{Label: task.Title, Checked: func() bool { return !slices.Contains(acc.DisabledTasks, id) }, OnChange: func(v bool) {
				disabled := slices.Clone(acc.DisabledTasks)
				if v {
					disabled = slices.DeleteFunc(disabled, func(s string) bool { return s == id })
				} else {
					disabled = append(disabled, id)
				}
				a.perform("任务开关已更新", func(ctx context.Context) error {
					return a.client.Patch(ctx, mid, api.AccountPatch{DisabledTasks: &disabled})
				}, a.refresh)
			}}}})
		}
		rows = append(rows, a.button("立即运行", func() { a.run(mid) }), a.button("刷新资料", func() {
			a.perform("资料已刷新", func(ctx context.Context) error { return a.client.Refresh(ctx, mid) }, a.refresh)
		}))
		if !acc.LoggedIn {
			rows = append(rows, a.button("重新扫码", func() { a.nav.Push("qr") }))
		}
		rows = append(rows, a.button("退出账号", func() {
			cx.ShowAlert(core.AlertConfig{Title: "退出账号？", Message: "将注销 B 站会话并删除此账号在服务端保存的数据。", Actions: []core.AlertAction{{Label: "取消", Role: core.AlertCancel}, {Label: "退出账号", Role: core.AlertDestructive}}}, func(i int) {
				if i == 1 {
					a.perform("已退出账号", func(ctx context.Context) error { return a.client.Remove(ctx, mid) }, func() { a.nav.Pop(1); a.refresh() })
				}
			})
		}))
		return mobileColumn(rows...)
	})
}
