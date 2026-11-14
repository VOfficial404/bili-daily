package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/VOfficial404/bili-daily/engine"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (a *mobileApp) connectPage(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	mode := core.NewSignal(cx, 0)
	address := core.NewSignal(cx, "")
	token := core.NewSignal(cx, "")
	remember := core.NewSignal(cx, false)
	proxy := core.NewSignal(cx, "")
	proxySpecified := false
	store := cx.Storage()
	if v, ok := store.GetItem("address"); ok {
		address.Set(v)
	}
	if v, ok := store.GetItem("local-proxy"); ok {
		proxy.Set(v)
		proxySpecified = true
	}
	if v, ok := store.GetItem("remember"); ok && v == "true" {
		remember.Set(true)
		if v, ok := store.GetItem("token"); ok {
			token.Set(v)
		}
		cx.Post(func() { a.connect(address.GetUntracked(), token.GetUntracked(), true) })
	}
	hero := core.View{Style: core.Style{Gap: 10, Padding: core.Edges{Top: 28, Bottom: 4}}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(60), Height: core.Pt(60), CornerRadius: 18, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			StyleFunc: ui.Filled(colors.Accent), Children: []core.Node{ui.Icon(lucide.CalendarCheck, 32, colors.OnAccent)}},
		mobileLabel(cx, "Bili Daily", 30, ui.Bold, colors.Text, 1),
		mobileLabel(cx, "B 站每日任务。手机推荐连接服务器，关掉 App 后服务器仍会按计划运行。", 15, ui.Regular, colors.Muted, 0),
	}}
	server := func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 20}, Children: []core.Node{
			mobileSection(cx, "服务器", "仅打开「记住」时保存令牌；令牌以明文保存在 App 私有目录，断开连接会清除。",
				mobileInput{Label: "服务器地址", TestID: "server-address", Value: address.Get, OnChange: address.Set, Placeholder: "https://bili.example.com", Keyboard: core.KeyboardURL, Disabled: a.isBusy},
				mobileInput{Label: "令牌", TestID: "server-token", Value: token.Get, OnChange: token.Set, Placeholder: "服务端配置的访问令牌", Masked: true, Disabled: a.isBusy},
				mobileRow{Title: "记住并在下次自动连接", Control: ui.Switch{Label: "记住并在下次自动连接", Checked: remember.Get, OnChange: func(v bool) {
					remember.Set(v)
					if !v {
						store.RemoveItem("token")
						store.RemoveItem("remember")
					}
				}}},
			),
			mobileButton{Text: "连接", Icon: lucide.Server, Disabled: a.isBusy, OnPress: func() { a.connect(address.Get(), token.Get(), remember.Get()) }},
		}}
	}
	local := func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 20}, Children: []core.Node{
			mobileSection(cx, "本机运行", "本机任务只能在 App 打开时可靠运行。手机系统可能暂停后台运行；稳定定时请连接服务器，回到 App 时补跑。",
				mobileInput{Label: "代理（可不填）", Value: proxy.Get, OnChange: func(v string) { proxy.Set(v); proxySpecified = true }, Placeholder: "http://127.0.0.1:7890", Keyboard: core.KeyboardURL, Disabled: a.isBusy},
			),
			mobileButton{Text: "启动本机运行", Icon: lucide.Smartphone, Disabled: a.isBusy, OnPress: func() {
				dir := cx.DataDir()
				if dir == "" {
					a.fail(errors.New("网页没有本机数据目录，请连接服务器"))
					return
				}
				var override *string
				if proxySpecified {
					value := proxy.Get()
					override = &value
				}
				a.startLocal(filepath.Join(dir, "data"), override)
			}},
		}}
	}
	content := core.View{Style: core.Style{Padding: core.Edges{Top: 16, Right: 20, Bottom: 32, Left: 20}, Gap: 20, MaxWidth: core.Pt(520), Width: core.Pct(100), AlignSelf: core.AlignCenter,
		PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Bottom: core.EnvKeyboard, Left: core.EnvSafeArea}}, Children: []core.Node{
		hero,
		a.banner(cx),
		ui.Tabs[int]{Options: []ui.Option[int]{{Value: 0, Label: "连接服务器"}, {Value: 1, Label: "在本机运行"}}, Value: mode.Get, OnChange: mode.Set, Label: "运行方式", Equal: true, Size: ui.SizeLarge},
		core.Show{When: func() bool { return mode.Get() == 0 }, Then: server, Else: local},
		mobileLabel(cx, "App "+mobileVersion, 12, ui.Regular, colors.Faint, 1),
	}}
	return core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}},
		Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissOnDrag, KeyboardShouldPersistTaps: core.KeyboardTapsHandled},
		Children: []core.Node{content}}
}

// mobileTabs 是底部的四个标签页。iOS 用系统标签栏和 SF Symbols；Android 系统自带的
// android:ic_menu_* 图标过时，所以 Android 和网页一样画 Lucide 图标的标签栏。
var mobileTabs = []struct {
	title, symbol string
	icon          func() *core.SVG
}{
	{"今天", "sun.max", lucide.House},
	{"账号", "person.crop.circle", lucide.CircleUser},
	{"报告", "doc.text", lucide.FileText},
	{"设置", "gearshape", lucide.Settings},
}

func (a *mobileApp) shell(cx *core.Scope) core.Node {
	a.navs = make([]*base.Nav[string], len(mobileTabs))
	tabs := make([]ui.ShellTab, len(mobileTabs))
	for i, t := range mobileTabs {
		a.navs[i] = base.NewNav(cx, "root")
		symbol := ""
		if cx.Env().OS() == core.OSIOS {
			symbol = t.symbol
		}
		tabs[i] = ui.ShellTab{Text: t.title, Icon: t.icon, SystemIcon: symbol, TestID: "tab-section-" + t.title, Content: func(cx *core.Scope) core.Node { return a.stack(cx, i) }}
	}
	return ui.TabShell{Tabs: tabs, Selected: a.tab.Get, Label: "页面", OnChange: func(i int) {
		if i == a.tab.GetUntracked() {
			a.navs[i].PopToRoot() // 再点当前标签回到首页
			return
		}
		a.tab.Set(i)
	}}
}

// nav 是当前标签页的导航栈。
func (a *mobileApp) nav() *base.Nav[string] {
	if i := a.tab.GetUntracked(); i < len(a.navs) {
		return a.navs[i]
	}
	return a.navs[0]
}

// mobileNavKey 把页面所在标签页的导航栈交给页面：定时刷新时，其他标签页里重建的页面也推入自己的栈。
var mobileNavKey = core.NewKey[*base.Nav[string]]("mobileNav", nil)

func (a *mobileApp) navOf(cx *core.Scope) *base.Nav[string] {
	if nav := mobileNavKey.Use(cx); nav != nil {
		return nav
	}
	return a.nav()
}

func (a *mobileApp) stack(cx *core.Scope, tab int) core.Node {
	nav := a.navs[tab]
	mobileNavKey.Provide(cx, nav)
	return nav.Stack(core.Style{FlexGrow: 1, FlexShrink: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
		if route == "root" {
			var trailing []core.Node
			var content core.Node
			switch tab {
			case 0:
				content = a.todayPage(cx)
				trailing = []core.Node{a.addAccountButton(cx)}
			case 1:
				content = a.accountsPage(cx)
				trailing = []core.Node{a.addAccountButton(cx)}
			case 2:
				content = a.reportsPage(cx)
			default:
				content = a.settingsPage(cx)
			}
			title := mobileTabs[tab].title
			return ui.NavPage(cx, base.NavPage{Title: title, Header: mobileHeader{Title: title, Trailing: trailing}, Children: []core.Node{content}}, ui.NavHeader{Trailing: trailing})
		}
		title := ""
		var content core.Node
		switch {
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
		back := func() { nav.Pop(1) }
		return ui.NavPage(cx, base.NavPage{Title: title, Header: mobileHeader{Title: title, OnBack: back}, Children: []core.Node{content}}, ui.NavHeader{BackLabel: "返回", OnBack: back})
	})
}

// mobileHeader 是 Android 和网页上的标题栏：首页大标题，详情页带返回。iOS 用系统导航栏。
type mobileHeader struct {
	Title    string
	OnBack   func()
	Trailing []core.Node
}

func (h mobileHeader) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	size, pad := core.Points(26), core.Points(20)
	var children []core.Node
	if h.OnBack != nil {
		size, pad = 18, 4
		children = append(children, ui.IconButton{Icon: lucide.ChevronLeft, Label: "返回", Size: 44, IconSize: 26, Tint: colors.Accent, OnPress: h.OnBack})
	}
	title := mobileLabel(cx, h.Title, size, ui.Bold, colors.Text, 1)
	title.Role = core.RoleHeader
	title.Style.FlexGrow = 1
	children = append(children, title)
	children = append(children, h.Trailing...)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, MinHeight: core.Pt(56), Padding: core.Edges{Left: pad, Right: 8}, Borders: core.Edges{Bottom: 0.5}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), BorderColors: core.EdgeColors{Bottom: colors.Line()}}
		}, Children: children}
}

func (a *mobileApp) addAccountButton(cx *core.Scope) core.Node {
	nav := a.navOf(cx)
	return ui.IconButton{Icon: lucide.Plus, Label: "添加账号", Size: 44, IconSize: 24, Tint: ui.ColorsOf(cx).Accent, Disabled: a.isBusy, OnPress: func() { nav.Push("qr") }}
}

func (a *mobileApp) openTask(id string) { a.nav().Push("task:" + id) }
func (a *mobileApp) openUnfollow()      { a.nav().Push("unfollow") }
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
func mobileDate(t *time.Time) string {
	if t == nil {
		return "未知"
	}
	return t.In(engine.Beijing).Format("2006-01-02")
}
func (a *mobileApp) run(mid string) {
	cl := a.session.Client()
	a.perform("已开始运行", func(ctx context.Context) error { return cl.Run(ctx, mid, nil) }, a.refresh)
}
