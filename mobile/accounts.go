package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (a *mobileApp) accountsPage(cx *core.Scope) core.Node {
	nav := a.navOf(cx)
	return mobilePage(cx, a.refresh, a.refreshing.Get, a.banner(cx), core.Dynamic(func(cx *core.Scope) core.Node {
		state := a.snapshot()
		colors := ui.ColorsOf(cx)
		var rows []core.Node
		for _, acc := range state.Accounts {
			detail := []string{fmt.Sprintf("Lv%d", acc.Level), fmt.Sprintf("硬币 %.1f", acc.Coins)}
			if acc.VIP != "" {
				detail = append(detail, acc.VIP)
			}
			status, color := "已登录", colors.Up
			switch {
			case !acc.LoggedIn:
				status, color = "登录失效", colors.Danger
			case !acc.Enabled:
				status, color = "未参加计划", colors.Muted
			}
			rows = append(rows, mobileRow{Leading: mobileAvatar(cx, acc.Uname, acc.Face, 40), Title: acc.Uname, Detail: strings.Join(detail, " · "), Lines: 1,
				Control: mobileBadge(cx, status, color), OnPress: func() { nav.Push("account:" + acc.Mid) }})
		}
		rows = append(rows, mobileRow{Icon: lucide.Plus, Tint: colors.Accent, Title: "扫码添加账号", Accent: true, Disabled: a.isBusy, OnPress: func() { nav.Push("qr") }})
		footer := "添加的账号默认参加每日计划，可在账号详情里单独关闭某些任务。"
		if len(state.Accounts) == 0 {
			footer = "还没有账号。用 B 站 App 扫码登录后，账号会参加每日计划。"
		}
		return mobileSection(cx, "", footer, rows...)
	}))
}

func (a *mobileApp) accountPage(cx *core.Scope, mid string) core.Node {
	cl := a.session.Client()
	nav := a.navOf(cx)
	return mobilePage(cx, a.refresh, a.refreshing.Get, core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
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
			return mobileText(cx, "账号已退出")
		}
		head := mobileCard(cx, core.Style{Padding: core.All(16), Direction: core.Row, Gap: 14, AlignItems: core.AlignCenter},
			mobileAvatar(cx, acc.Uname, acc.Face, 60),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 6}, Children: []core.Node{
				mobileLabel(cx, acc.Uname, 20, ui.Bold, colors.Text, 1),
				mobileLabel(cx, "UID "+acc.Mid, 13, ui.Regular, colors.Muted, 1),
				core.View{Style: core.Style{Direction: core.Row, Gap: 6, Wrap: core.Wrap}, Children: mobileAccountBadges(cx, acc)},
			}})
		level := fmt.Sprintf("经验 %d / %d", acc.Exp, acc.NextExp)
		ratio := float32(1)
		if acc.NextExp > 0 {
			ratio = float32(acc.Exp) / float32(acc.NextExp)
		} else {
			level = fmt.Sprintf("经验 %d · 已满级", acc.Exp)
		}
		levelRow := core.View{Style: core.Style{Gap: 8, Padding: core.Symmetric(12, mobileRowPad)}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter}, Children: []core.Node{
				mobileLabel(cx, fmt.Sprintf("等级 Lv%d", acc.Level), 16, ui.Regular, colors.Text, 1),
				core.View{Style: core.Style{FlexGrow: 1}},
				mobileLabel(cx, level, 14, ui.Regular, colors.Muted, 1),
			}},
			core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{mobileBar(cx, ratio, colors.Accent)}},
		}}
		vip, vipDue := acc.VIP, ""
		if vip == "" {
			vip = "普通会员"
		} else {
			vipDue = mobileDate(acc.VIPDue) + " 到期"
		}
		cookie := "未知"
		if acc.CookieExpires != nil {
			cookie = mobileDate(acc.CookieExpires) + " 到期"
		}
		facts := mobileSection(cx, "资料", "资料更新于 "+client.HumanTime(acc.UpdatedAt, "未安排")+"（北京时间）",
			levelRow,
			mobileRow{Icon: lucide.Coins, Tint: colors.Warn, Title: "硬币", Value: fmt.Sprintf("%.1f", acc.Coins)},
			mobileRow{Icon: lucide.Ticket, Tint: colors.Accent, Title: "B 币券", Value: fmt.Sprintf("%.1f", acc.BCoupon)},
			mobileRow{Icon: lucide.Crown, Tint: colors.Accent, Title: "大会员", Detail: vip, Value: vipDue},
			mobileRow{Icon: lucide.Cookie, Tint: colors.Muted, Title: "登录有效期", Value: cookie},
		)
		plan := mobileSection(cx, "每日计划", "下次运行（北京时间）："+client.HumanTime(acc.NextRun, "未安排"),
			mobileRow{Title: "参加每日计划", Control: core.View{Disabled: a.isBusy, Children: []core.Node{ui.Switch{Label: "参加每日计划", Checked: func() bool { return acc.Enabled }, OnChange: func(v bool) {
				a.perform("每日计划已更新", func(ctx context.Context) error { return cl.Patch(ctx, mid, api.AccountPatch{Enabled: &v}) }, a.refresh)
			}}}}},
		)
		var tasks []core.Node
		for _, task := range state.Tasks {
			if task.Manual {
				continue
			}
			id := task.ID
			global := client.TaskOn(&state.Settings, id)
			detail := task.Needs
			if detail != "" {
				detail = "需要" + detail
			}
			if !global {
				// 全局关闭的任务对所有账号都不运行，这里显示为关闭且不可改。
				tasks = append(tasks, mobileRow{Title: task.Title, Detail: "已在设置中关闭", Control: core.View{Disabled: func() bool { return true }, Style: core.Style{Opacity: core.Alpha(0.4)},
					Children: []core.Node{ui.Switch{Label: task.Title, Checked: func() bool { return false }, OnChange: func(bool) {}}}}})
				continue
			}
			tasks = append(tasks, mobileRow{Title: task.Title, Detail: detail, Control: core.View{Disabled: a.isBusy, Children: []core.Node{ui.Switch{Label: task.Title, Checked: func() bool { return !slices.Contains(acc.DisabledTasks, id) }, OnChange: func(v bool) {
				disabled := slices.Clone(acc.DisabledTasks)
				if v {
					disabled = slices.DeleteFunc(disabled, func(s string) bool { return s == id })
				} else {
					disabled = append(disabled, id)
				}
				a.perform("任务开关已更新", func(ctx context.Context) error {
					return cl.Patch(ctx, mid, api.AccountPatch{DisabledTasks: &disabled})
				}, a.refresh)
			}}}}})
		}
		taskSection := mobileSection(cx, "这个账号的任务", "关闭后这个账号不做该任务；全局开关在「设置」里。", tasks...)
		actions := []core.Node{
			mobileRow{Icon: lucide.Play, Tint: colors.Accent, Title: "立即运行", Accent: true, Disabled: func() bool { return a.isBusy() || acc.Running != nil }, OnPress: func() { a.run(mid) }},
			mobileRow{Icon: lucide.RefreshCw, Tint: colors.Accent, Title: "刷新资料", Accent: true, Disabled: a.isBusy, OnPress: func() {
				a.perform("资料已刷新", func(ctx context.Context) error { return cl.Refresh(ctx, mid) }, a.refresh)
			}},
		}
		if !acc.LoggedIn {
			actions = append(actions, mobileRow{Icon: lucide.ScanQrCode, Tint: colors.Accent, Title: "重新扫码", Accent: true, Disabled: a.isBusy, OnPress: func() { nav.Push("qr") }})
		}
		if acc.LastRun != nil {
			id := acc.LastRun.ID
			actions = append(actions, mobileRow{Icon: lucide.FileText, Tint: colors.Accent, Title: "上次运行报告", Detail: acc.LastRun.Summary, Lines: 1, OnPress: func() { nav.Push("report:" + id) }})
		}
		logout := mobileSection(cx, "", "将注销 B 站会话并删除此账号在服务端保存的数据。",
			mobileRow{Icon: lucide.LogOut, Tint: colors.Danger, Title: "退出账号", Danger: true, Disabled: a.isBusy, OnPress: func() {
				cx.ShowAlert(core.AlertConfig{Title: "退出账号？", Message: "将注销 B 站会话并删除此账号在服务端保存的数据。", Actions: []core.AlertAction{{Label: "取消", Role: core.AlertCancel}, {Label: "退出账号", Role: core.AlertDestructive}}}, func(i int) {
					if i == 1 {
						a.perform("已退出账号", func(ctx context.Context) error { return cl.Remove(ctx, mid) }, func() { nav.Pop(1); a.refresh() })
					}
				})
			}})
		return core.View{Style: core.Style{Gap: 20}, Children: []core.Node{head, mobileSection(cx, "", "", actions...), facts, plan, taskSection, logout}}
	}))
}
