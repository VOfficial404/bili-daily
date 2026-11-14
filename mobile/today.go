package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (a *mobileApp) todayPage(cx *core.Scope) core.Node {
	return mobilePage(cx, a.refresh, a.refreshing.Get, a.banner(cx), core.Dynamic(func(cx *core.Scope) core.Node {
		state := a.snapshot()
		var nodes []core.Node
		if state.Local {
			nodes = append(nodes, a.notice(cx, lucide.Smartphone, "本机运行：手机系统可能暂停后台运行；稳定定时请连接服务器，回到 App 时补跑。"))
		}
		nodes = append(nodes, a.summaryCard(cx, state))
		for _, acc := range state.Accounts {
			nodes = append(nodes, a.accountCard(cx, acc))
		}
		return core.View{Style: core.Style{Gap: 16}, Children: nodes}
	}))
}

func (a *mobileApp) notice(cx *core.Scope, icon func() *core.SVG, text string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, Gap: 10, Padding: core.Symmetric(12, 14), CornerRadius: 12, AlignItems: core.AlignStart},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), colors.Warn(), 0.12)} },
		Children:  []core.Node{ui.Icon(icon, 18, colors.Warn), mobileLabel(cx, text, 14, ui.Regular, colors.Text, 0)}}
}

func (a *mobileApp) summaryCard(cx *core.Scope, state api.State) core.Node {
	cl := a.session.Client()
	colors := ui.ColorsOf(cx)
	if len(state.Accounts) == 0 {
		return mobileCard(cx, core.Style{Padding: core.All(20), Gap: 14, AlignItems: core.AlignCenter},
			mobileAvatarIcon(cx, lucide.ScanQrCode, 56),
			mobileLabel(cx, "还没有账号", 18, ui.Semibold, colors.Text, 1),
			mobileLabel(cx, "用 B 站 App 扫码登录后，账号会按每日计划自动完成任务。今天的结果按北京时间统计。", 14, ui.Regular, colors.Muted, 0),
			core.View{Style: core.Style{AlignSelf: core.AlignStretch}, Children: []core.Node{mobileButton{Text: "扫码添加账号", Icon: lucide.Plus, Disabled: a.isBusy, OnPress: func() { a.navOf(cx).Push("qr") }}}},
		)
	}
	var done, total, problems int
	running := false
	for _, acc := range state.Accounts {
		d, t, p := client.AccountTally(acc)
		done, total, problems = done+d, total+t, problems+p
		running = running || acc.Running != nil
	}
	problemColor := colors.Up
	if problems > 0 {
		problemColor = colors.Danger
	}
	stat := func(value, label string, color func() core.Color) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 2}, Children: []core.Node{
			mobileLabel(cx, value, 26, ui.Bold, color, 1),
			mobileLabel(cx, label, 13, ui.Regular, colors.Muted, 1),
		}}
	}
	ratio := float32(0)
	if total > 0 {
		ratio = float32(done) / float32(total)
	}
	return mobileCard(cx, core.Style{Padding: core.All(16), Gap: 14},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			stat(strconv.Itoa(len(state.Accounts)), "账号", colors.Text),
			stat(fmt.Sprintf("%d/%d", done, total), "今日完成", colors.Text),
			stat(strconv.Itoa(problems), "需要处理", problemColor),
		}},
		mobileBar(cx, ratio, colors.Accent),
		mobileLabel(cx, "今天的结果按北京时间统计，下拉刷新。", 13, ui.Regular, colors.Muted, 0),
		mobileButton{Text: "全部立即运行", Icon: lucide.Play, Disabled: func() bool { return a.isBusy() || running }, OnPress: func() { a.perform("已开始全部运行", cl.RunAll, a.refresh) }},
	)
}

func mobileAvatarIcon(cx *core.Scope, icon func() *core.SVG, size core.Points) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), colors.Accent(), 0.16)} },
		Children:  []core.Node{ui.Icon(icon, size*0.5, colors.Accent)}}
}

// mobileAccountBadges 是等级、大会员、登录和计划状态。
func mobileAccountBadges(cx *core.Scope, acc api.Account) []core.Node {
	colors := ui.ColorsOf(cx)
	badges := []core.Node{mobileBadge(cx, fmt.Sprintf("Lv%d", acc.Level), colors.Accent)}
	if acc.VIP != "" {
		badges = append(badges, mobileBadge(cx, acc.VIP, colors.Accent))
	}
	if !acc.LoggedIn {
		badges = append(badges, mobileBadge(cx, "登录失效", colors.Danger))
	} else if !acc.Enabled {
		badges = append(badges, mobileBadge(cx, "未参加计划", colors.Muted))
	}
	return badges
}

func (a *mobileApp) accountCard(cx *core.Scope, acc api.Account) core.Node {
	cl := a.session.Client()
	colors := ui.ColorsOf(cx)
	nav := a.navOf(cx)
	done, total, _ := client.AccountTally(acc)
	head := base.Button{Label: acc.Uname, OnPress: func(core.PressEvent) { nav.Push("account:" + acc.Mid) },
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 14, Right: 12, Bottom: 10, Left: 16}},
		StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} },
		Children: []core.Node{
			mobileAvatar(cx, acc.Uname, acc.Face, 44),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 4}, Children: []core.Node{
				mobileLabel(cx, acc.Uname, 17, ui.Semibold, colors.Text, 1),
				core.View{Style: core.Style{Direction: core.Row, Gap: 6, Wrap: core.Wrap, AlignItems: core.AlignCenter}, Children: append(mobileAccountBadges(cx, acc),
					mobileLabel(cx, fmt.Sprintf("硬币 %.1f", acc.Coins), 13, ui.Regular, colors.Muted, 1))},
			}},
			ui.Icon(lucide.ChevronRight, 18, colors.Faint),
		}}
	nodes := []core.Node{head}
	if total > 0 {
		nodes = append(nodes, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Symmetric(0, 16)}, Children: []core.Node{
			mobileBar(cx, float32(done)/float32(total), colors.Accent),
			mobileLabel(cx, fmt.Sprintf("今日 %d/%d", done, total), 12, ui.Medium, colors.Muted, 1),
		}})
	}
	var tasks []core.Node
	for _, task := range acc.Today {
		tasks = append(tasks, a.taskLine(cx, acc, task))
	}
	if len(tasks) == 0 {
		tasks = append(tasks, core.View{Style: core.Style{Padding: core.Symmetric(10, 16)}, Children: []core.Node{mobileLabel(cx, "今天还没有运行结果", 14, ui.Regular, colors.Muted, 0)}})
	}
	nodes = append(nodes, core.View{Style: core.Style{Padding: core.Edges{Top: 6, Bottom: 6}}, Children: tasks})
	// 底部：下次运行时间和这个账号的操作。
	var info core.Node
	var action core.Node
	if acc.Running != nil {
		info = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			ui.Icon(lucide.LoaderCircle, 15, colors.Accent),
			mobileLabel(cx, fmt.Sprintf("正在运行 %s（%d/%d）", acc.Running.Task, acc.Running.Done, acc.Running.Total), 13, ui.Medium, colors.Accent, 1),
		}}
		action = mobileButton{Text: "中止", Icon: lucide.Square, Kind: buttonDanger, Small: true, Disabled: a.isBusy, OnPress: func() {
			a.perform("已请求中止", func(ctx context.Context) error { return cl.Cancel(ctx, acc.Mid) }, a.refresh)
		}}
	} else {
		info = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			ui.Icon(lucide.Clock, 15, colors.Muted),
			mobileLabel(cx, "下次 "+client.HumanTime(acc.NextRun, "未安排"), 13, ui.Regular, colors.Muted, 1),
		}}
		action = mobileButton{Text: "立即运行", Icon: lucide.Play, Kind: buttonTonal, Small: true, Disabled: a.isBusy, OnPress: func() { a.run(acc.Mid) }}
	}
	nodes = append(nodes, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 10, Right: 12, Bottom: 12, Left: 16}, Borders: core.Edges{Top: 0.5}},
		StyleFunc: func() core.Style { return core.Style{BorderColors: core.EdgeColors{Top: colors.Line()}} },
		Children:  []core.Node{info, action}})
	return mobileCard(cx, core.Style{}, nodes...)
}

// taskLine 是今天一个任务的结果：状态图标、任务名和一行说明；点开查看上次的运行报告。
func (a *mobileApp) taskLine(cx *core.Scope, acc api.Account, task api.TaskStatus) core.Node {
	colors := ui.ColorsOf(cx)
	_, color := mobileStatusLook(cx, task.Status)
	texts := []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		mobileLabel(cx, a.taskTitle(task.ID), 15, ui.Medium, colors.Text, 1),
		mobileLabel(cx, client.StatusShort(task.Status), 12, ui.Medium, color, 1),
	}}}
	if task.Note != "" {
		texts = append(texts, mobileLabel(cx, task.Note, 13, ui.Regular, colors.Muted, 1))
	}
	children := []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 1}}, Children: []core.Node{mobileStatusIcon(cx, task.Status, 18)}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: texts},
	}
	style := core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 10, Padding: core.Symmetric(7, 16)}
	if acc.LastRun == nil {
		return core.View{Style: style, Children: children}
	}
	nav, id := a.navOf(cx), acc.LastRun.ID
	return base.Button{Label: a.taskTitle(task.ID) + " " + client.StatusShort(task.Status), OnPress: func(core.PressEvent) { nav.Push("report:" + id) }, Style: style,
		StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} }, Children: children}
}
