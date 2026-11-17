package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// mobileSettingsForm 保存页面里的编辑值，定时刷新不会覆盖尚未保存的修改。
// 每个字段返回一行，由页面放进分组。
type mobileSettingsForm struct {
	cx      *core.Scope
	readers []func() error
}

// value 是一个文本字段的编辑值，保存时交给 put。
func (f *mobileSettingsForm) value(initial string, put func(string)) *core.Signal[string] {
	value := core.NewSignal(f.cx, initial)
	f.readers = append(f.readers, func() error { put(value.Get()); return nil })
	return value
}

func (f *mobileSettingsForm) text(label, placeholder, initial string, keyboard core.KeyboardType, put func(string)) core.Node {
	value := f.value(initial, put)
	return mobileInput{Label: label, Placeholder: placeholder, Value: value.Get, OnChange: value.Set, Keyboard: keyboard}
}

func (f *mobileSettingsForm) number(label, detail string, initial int64, put func(int64)) core.Node {
	row, _ := f.numberSignal(label, detail, initial, put)
	return row
}

func (f *mobileSettingsForm) numberSignal(label, detail string, initial int64, put func(int64)) (core.Node, *core.Signal[string]) {
	value := core.NewSignal(f.cx, strconv.FormatInt(initial, 10))
	f.readers = append(f.readers, func() error {
		n, err := strconv.ParseInt(strings.TrimSpace(value.Get()), 10, 64)
		if err != nil {
			return fmt.Errorf("%s：请输入整数", label)
		}
		put(n)
		return nil
	})
	return mobileInput{Label: label, Detail: detail, Value: value.Get, OnChange: value.Set, Keyboard: core.KeyboardNumber, Inline: true}, value
}

func (f *mobileSettingsForm) toggle(label, detail string, target *bool) core.Node {
	value := core.NewSignal(f.cx, *target)
	f.readers = append(f.readers, func() error { *target = value.Get(); return nil })
	return mobileRow{Title: label, Detail: detail, Control: ui.Switch{Label: label, Checked: value.Get, OnChange: value.Set}}
}

func (f *mobileSettingsForm) selectValue(label string, target *string, options []ui.Option[string]) (core.Node, *core.Signal[string]) {
	value := core.NewSignal(f.cx, *target)
	f.readers = append(f.readers, func() error { *target = value.Get(); return nil })
	return mobileRow{Title: label, Control: ui.Select[string]{Label: label, Options: options, Value: value.Get, OnChange: value.Set, Placeholder: "请选择", Presentation: base.PresentSheet}}, value
}

func mobileStringOptions(values ...string) []ui.Option[string] {
	out := make([]ui.Option[string], 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		out = append(out, ui.Option[string]{Value: values[i], Label: values[i+1]})
	}
	return out
}

func (f *mobileSettingsForm) read() error {
	for _, read := range f.readers {
		if err := read(); err != nil {
			return err
		}
	}
	return nil
}

func mobileSplitValues(text string) []string {
	var values []string
	for _, value := range strings.Split(strings.ReplaceAll(text, "，", ","), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (f *mobileSettingsForm) ids(label string, initial []int64, put func([]int64)) core.Node {
	row, _ := f.idsSignal(label, initial, put)
	return row
}

func (f *mobileSettingsForm) idsSignal(label string, initial []int64, put func([]int64)) (core.Node, *core.Signal[string]) {
	parts := make([]string, len(initial))
	for i, id := range initial {
		parts[i] = strconv.FormatInt(id, 10)
	}
	value := core.NewSignal(f.cx, strings.Join(parts, ","))
	f.readers = append(f.readers, func() error {
		parts := mobileSplitValues(value.Get())
		ids := make([]int64, 0, len(parts))
		for _, part := range parts {
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return fmt.Errorf("%s：%s 不是有效的用户 ID", label, part)
			}
			ids = append(ids, id)
		}
		put(ids)
		return nil
	})
	return mobileInput{Label: label, Placeholder: "多个 UID 用逗号分隔", Value: value.Get, OnChange: value.Set}, value
}

// mobileNotifyFields 是每个推送渠道要填的字段：令牌、目标和自建服务地址的名称，空为不需要。
func mobileNotifyFields(channel string) (token, target, endpoint, endpointHint string) {
	switch channel {
	case "telegram":
		return "机器人令牌（bot token）", "chat_id", "反代地址（可不填）", "https://api.telegram.org"
	case "pushplus":
		return "PushPlus 令牌", "", "", ""
	case "wecom":
		return "机器人 Webhook 地址或 key", "", "", ""
	case "dingtalk":
		return "机器人 Webhook 地址或 access_token", "加签密钥（未开加签可不填）", "", ""
	case "serverchan":
		return "SendKey", "", "", ""
	case "bark":
		return "设备 Key", "", "服务器地址（可不填）", "https://api.day.app"
	case "webhook":
		return "Webhook 地址", "", "", ""
	}
	return "", "", "", ""
}

func (a *mobileApp) settingsPage(cx *core.Scope) core.Node {
	cl := a.session.Client()
	colors := ui.ColorsOf(cx)
	state := a.state.GetUntracked()
	s := client.CloneSettings(state.Settings)
	snapshotAtLastSave := client.CloneSettings(s)
	f := &mobileSettingsForm{cx: cx}

	runAtValue := f.value(s.RunAt, func(v string) { s.RunAt = v })
	runAt := mobileInput{Label: "每日开始时间", Detail: "HH:MM", Placeholder: "09:30", Value: runAtValue.Get, OnChange: runAtValue.Set, Inline: true}
	pace, _ := f.selectValue("请求节奏", &s.Pace, mobileStringOptions("slow", "慢速", "normal", "正常", "fast", "快速"))
	schedule := mobileSection(cx, "运行计划", "时间按北京时间。每个账号每天在开始时间后随机延后，避免同时请求。",
		runAt,
		f.number("随机延后上限", "分钟", int64(s.Jitter), func(v int64) { s.Jitter = int(v) }),
		pace,
	)

	when, whenValue := f.selectValue("何时推送", &s.Notify.When, mobileStringOptions("never", "不推送", "failed", "有失败时", "always", "每次运行后"))
	channel, channelValue := f.selectValue("推送渠道", &s.Notify.Channel, mobileStringOptions("telegram", "Telegram", "pushplus", "PushPlus", "wecom", "企业微信", "dingtalk", "钉钉", "serverchan", "Server 酱", "bark", "Bark", "webhook", "Webhook"))
	token := f.value(s.Notify.Token, func(v string) { s.Notify.Token = v })
	target := f.value(s.Notify.Target, func(v string) { s.Notify.Target = v })
	endpoint := f.value(s.Notify.Endpoint, func(v string) { s.Notify.Endpoint = v })
	notify := core.Dynamic(func(cx *core.Scope) core.Node {
		if whenValue.Get() == "never" {
			return mobileSection(cx, "结果推送", "运行结束后把结果推送到你常用的 App。", when)
		}
		rows := []core.Node{when, channel}
		tokenLabel, targetLabel, endpointLabel, endpointHint := mobileNotifyFields(channelValue.Get())
		keyboard := core.KeyboardDefault
		if channelValue.Get() == "webhook" {
			keyboard = core.KeyboardURL
		}
		if tokenLabel != "" {
			rows = append(rows, mobileInput{Label: tokenLabel, Value: token.Get, OnChange: token.Set, Keyboard: keyboard, Masked: channelValue.Get() != "webhook"})
		}
		if targetLabel != "" {
			rows = append(rows, mobileInput{Label: targetLabel, Value: target.Get, OnChange: target.Set})
		}
		if endpointLabel != "" {
			rows = append(rows, mobileInput{Label: endpointLabel, Placeholder: endpointHint, Value: endpoint.Get, OnChange: endpoint.Set, Keyboard: core.KeyboardURL})
		}
		if channelValue.Get() != "" {
			rows = append(rows, mobileRow{Icon: lucide.Bell, Tint: colors.Accent, Title: "发送测试推送", Accent: true, Disabled: a.isBusy, OnPress: func() {
				if err := f.read(); err != nil {
					a.message(err.Error())
					return
				}
				n := s.Notify
				a.perform("发送测试推送", func(ctx context.Context) error { return cl.TestNotify(ctx, n) }, func() { a.message("测试推送已发送") })
			}})
		}
		return mobileSection(cx, "结果推送", "测试会使用当前填写的内容，不需要先保存。", rows...)
	})

	logLevel, _ := f.selectValue("日志级别", &s.LogLevel, mobileStringOptions("debug", "调试", "info", "信息", "warn", "警告", "error", "错误"))
	logs := mobileSection(cx, "日志", "排查问题时可临时调到「调试」。", logLevel)
	retention := core.NewSignal(cx, strconv.Itoa(s.ReportRetentionDays))
	f.readers = append(f.readers, func() error {
		days, err := strconv.Atoi(strings.TrimSpace(retention.Get()))
		if err != nil || days < 1 || days > 365 {
			return fmt.Errorf("报告保留天数：请输入 1–365 之间的整数")
		}
		s.ReportRetentionDays = days
		return nil
	})
	reports := mobileSection(cx, "报告", "保留 1–365 天；每个账号最多保留 60 份报告。",
		mobileInput{Label: "报告保留天数", Detail: "天", Value: retention.Get, OnChange: retention.Set, Keyboard: core.KeyboardNumber, Inline: true})

	var taskRows []core.Node
	for _, task := range state.Tasks {
		taskRows = append(taskRows, a.settingsTaskRow(cx, f, task, client.TaskEnabled(&s.Tasks, task.ID)))
	}
	tasks := mobileSection(cx, "任务", "开关对所有账号生效；点任务名设置选项。", taskRows...)

	proxy := f.text("代理", "留空直连，例如 http://127.0.0.1:7890", s.Proxy, core.KeyboardURL, func(v string) { s.Proxy = v })
	mode := "远程服务器"
	if state.Local {
		mode = "本机运行"
	}
	connection := mobileSection(cx, "连接", "代理用于所有 B 站请求，推送不经过代理。",
		mobileRow{Icon: lucide.Server, Tint: colors.Accent, Title: mode, Detail: cl.Base(), Lines: 1},
		proxy,
		mobileRow{Icon: lucide.Globe, Tint: colors.Accent, Title: "测试代理出口", Accent: true, Disabled: a.isBusy, OnPress: func() {
			if err := f.read(); err != nil {
				a.message(err.Error())
				return
			}
			proxy := s.Proxy
			var egress api.Egress
			a.perform("测试代理", func(ctx context.Context) error {
				var err error
				egress, err = cl.TestProxy(ctx, proxy)
				return err
			}, func() {
				if egress.Error != "" {
					a.message(egress.Error)
					return
				}
				a.message(strings.TrimSpace(egress.Addr + " " + egress.Place + " " + egress.ISP))
			})
		}},
	)
	save := mobileButton{Text: "保存设置", Icon: lucide.Check, Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		edited := client.CloneSettings(s)
		a.saveSettings(cx, snapshotAtLastSave, edited, func(api.Settings) { snapshotAtLastSave = client.CloneSettings(edited) })
	}}
	action := "断开连接"
	if state.Local {
		action = "停止本机运行"
	}
	leave := core.Dynamic(func(cx *core.Scope) core.Node {
		return mobileSection(cx, "", mobileVersionSummary(a.snapshot()),
			mobileRow{Icon: lucide.Unplug, Tint: colors.Danger, Title: action, Danger: true, Disabled: a.isBusy, OnPress: a.disconnect})
	})
	return mobilePage(cx, nil, nil, a.banner(cx), schedule, notify, tasks, reports, logs, connection, save, leave)
}

// settingsTaskRow 是设置里的一个任务：点任务名打开选项，右侧是全局开关。
func (a *mobileApp) settingsTaskRow(cx *core.Scope, f *mobileSettingsForm, task api.TaskInfo, enabled *bool) core.Node {
	colors := ui.ColorsOf(cx)
	id := task.ID
	hasOptions := false
	switch id {
	case "daily", "charge", "manga", "fans_medal", "live_lottery", "unfollow":
		hasOptions = true
	}
	open := func() {
		if id == "unfollow" {
			a.openUnfollow()
		} else {
			a.openTask(id)
		}
	}
	detail := task.Desc
	if task.Manual {
		detail = "仅手动运行 · " + detail
	}
	texts := []core.Node{mobileLabel(cx, task.Title, 16, ui.Regular, colors.Text, 1)}
	if detail != "" {
		texts = append(texts, mobileLabel(cx, detail, 13, ui.Regular, colors.Muted, 2))
	}
	textStyle := core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2, Padding: core.Edges{Top: 10, Bottom: 10, Left: mobileRowPad}}
	var label core.Node = core.View{Style: textStyle, Children: texts}
	if hasOptions {
		textStyle.Direction, textStyle.AlignItems, textStyle.Gap = core.Row, core.AlignCenter, 6
		label = base.Button{Label: task.Title + "选项", Disabled: a.isBusy, OnPress: func(core.PressEvent) { open() }, Style: textStyle,
			StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Opacity: core.Alpha(0.6)}} },
			Children:  []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts}, ui.Icon(lucide.ChevronRight, 18, colors.Faint)}}
	}
	children := []core.Node{label}
	if enabled != nil {
		children = append(children, f.toggle(task.Title, "", enabled).(mobileRow).Control)
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(mobileRowHeight), Padding: core.Edges{Right: mobileRowPad}}, Children: children}
}

func (a *mobileApp) taskOptionsPage(cx *core.Scope, id string) core.Node {
	state := a.state.GetUntracked()
	s := client.CloneSettings(state.Settings)
	snapshotAtLastSave := client.CloneSettings(s)
	f := &mobileSettingsForm{cx: cx}
	desc := ""
	for _, task := range state.Tasks {
		if task.ID == id {
			desc = task.Desc
		}
	}
	var head []core.Node
	if enabled := client.TaskEnabled(&s.Tasks, id); enabled != nil {
		head = append(head, f.toggle("启用任务", "关闭后所有账号都不做这个任务", enabled))
	}
	var sections []core.Node
	add := func(title, footer string, rows ...core.Node) {
		sections = append(sections, mobileSection(cx, title, footer, rows...))
	}
	switch id {
	case "daily":
		o := &s.Tasks.Daily
		add("观看与分享", "", f.toggle("观看视频", "", &o.Watch), f.toggle("分享视频", "", &o.Share))
		add("投币", "到达停止等级后不再投币。",
			f.number("每天投币数", "0–5", int64(o.Coins), func(v int64) { o.Coins = int(v) }),
			f.number("至少保留硬币", "", int64(o.KeepCoins), func(v int64) { o.KeepCoins = int(v) }),
			f.number("停止投币等级", "0 为不限", int64(o.StopAtLevel), func(v int64) { o.StopAtLevel = int(v) }),
			f.number("投币前最短等待（秒）", "0–600", int64(o.CoinDelayMin), func(v int64) { o.CoinDelayMin = int(v) }),
			f.number("投币前最长等待（秒）", "0–600", int64(o.CoinDelayMax), func(v int64) { o.CoinDelayMax = int(v) }),
			f.toggle("投币时点赞", "", &o.Like))
		add("优先 UP 主", "按从上到下的顺序优先观看和投币；留空时从关注和排行榜里选视频。", a.upPicker(cx, f, o.UpIDs, false, func(ids []int64) { o.UpIDs = ids }))
	case "charge":
		o := &s.Tasks.Charge
		add("充电", "启用后每月会把全部 B 币券余额充给这位 UP 主，保存前会再次确认。",
			a.upPicker(cx, f, []int64{o.UpID}, true, func(ids []int64) {
				o.UpID = 0
				if len(ids) > 0 {
					o.UpID = ids[0]
				}
			}),
			f.number("每月几号之后充电", "1–28", int64(o.Day), func(v int64) { o.Day = int(v) }),
			f.text("充电留言", "可不填", o.Comment, core.KeyboardDefault, func(v string) { o.Comment = v }))
	case "manga":
		o := &s.Tasks.Manga
		add("阅读", "", f.toggle("签到后记录阅读", "", &o.Read),
			f.number("漫画 ID", "", o.ComicID, func(v int64) { o.ComicID = v }),
			f.number("章节 ID", "", o.EpID, func(v int64) { o.EpID = v }))
	case "fans_medal":
		o := &s.Tasks.FansMedal
		add("直播间", "", f.toggle("给直播间点赞", "", &o.Like),
			f.text("弹幕内容", "留空不发送", o.Danmaku, core.KeyboardDefault, func(v string) { o.Danmaku = v }))
		add("观看时长", "", f.toggle("积累观看时长", "", &o.Watch),
			f.number("每个粉丝牌最多观看", "分钟", int64(o.Minutes), func(v int64) { o.Minutes = int(v) }),
			f.number("每轮粉丝牌数量", "0 为全部", int64(o.Max), func(v int64) { o.Max = int(v) }))
	case "live_lottery":
		o := &s.Tasks.LiveLottery
		add("查找", "", f.number("查找间隔", "小时，0 为仅每日运行", int64(o.EveryHours), func(v int64) { o.EveryHours = int(v) }))
		add("奖品筛选", "多个关键词用逗号分隔。",
			f.text("奖品包含关键词", "留空表示都参加", strings.Join(o.Include, ","), core.KeyboardDefault, func(v string) { o.Include = mobileSplitValues(v) }),
			f.text("排除关键词", "可不填", strings.Join(o.Exclude, ","), core.KeyboardDefault, func(v string) { o.Exclude = mobileSplitValues(v) }))
		add("关注", "", f.toggle("参加需要关注的抽奖", "", &o.Follow), f.toggle("把抽奖关注放进天选时刻分组", "方便之后批量取关", &o.Group))
	case "unfollow":
		return a.unfollowPage(cx)
	case "vip_point", "vip_privilege", "manga_privilege", "silver2coin":
	default:
		return mobileColumn(mobileText(cx, "未找到任务："+id))
	}
	footer := desc
	if len(sections) == 0 {
		footer = strings.TrimSpace(desc + " 此任务只需设置是否启用。")
	}
	nodes := []core.Node{mobileSection(cx, "", footer, head...)}
	nodes = append(nodes, sections...)
	nodes = append(nodes, mobileButton{Text: "保存任务选项", Icon: lucide.Check, Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		edited := client.CloneSettings(s)
		a.saveSettings(cx, snapshotAtLastSave, edited, func(api.Settings) { snapshotAtLastSave = client.CloneSettings(edited) })
	}})
	return mobilePage(cx, nil, nil, nodes...)
}

func (a *mobileApp) unfollowPage(cx *core.Scope) core.Node {
	cl := a.session.Client()
	colors := ui.ColorsOf(cx)
	state := a.state.GetUntracked()
	s := client.CloneSettings(state.Settings)
	snapshotAtLastSave := client.CloneSettings(s)
	f := &mobileSettingsForm{cx: cx}
	account := core.NewSignal(cx, "")
	group := core.NewSignal(cx, s.Tasks.Unfollow.Group)
	groups := core.NewSignalFunc(cx, []api.Group(nil), nil)
	loadedFor := core.NewSignal(cx, "")
	options := make([]ui.Option[string], 0, len(state.Accounts))
	for _, user := range state.Accounts {
		options = append(options, ui.Option[string]{Value: user.Mid, Label: user.Uname + "（" + user.Mid + "）"})
	}
	if len(options) > 0 {
		account.Set(options[0].Value)
	}
	loadGroups := func() {
		mid := account.Get()
		if mid == "" {
			a.message("请先添加账号")
			return
		}
		var result []api.Group
		a.perform("加载关注分组", func(ctx context.Context) error { var err error; result, err = cl.Groups(ctx, mid); return err }, func() {
			if account.Get() != mid {
				return
			}
			groups.Set(result)
			loadedFor.Set(mid)
			found := false
			for _, item := range result {
				if item.Name == group.Get() {
					found = true
					break
				}
			}
			if !found {
				group.Set("")
			}
			if len(result) == 0 {
				a.message("这个账号没有可用的关注分组")
			}
		})
	}
	target := mobileSection(cx, "取关对象", "仅手动运行。请核对账号和分组，运行后会取消关注分组里的用户。",
		mobileRow{Title: "账号", Control: ui.Select[string]{Label: "取关账号", Options: options, Value: account.Get, Disabled: a.isBusy, Placeholder: "请选择账号", Presentation: base.PresentSheet,
			OnChange: func(v string) { account.Set(v); groups.Set(nil); loadedFor.Set(""); group.Set(""); loadGroups() }}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			items := groups.Get()
			choices := make([]ui.Option[string], 0, len(items))
			for _, item := range items {
				choices = append(choices, ui.Option[string]{Value: item.Name, Label: fmt.Sprintf("%s（%d 人）", item.Name, item.Count)})
			}
			return mobileRow{Title: "分组", Control: ui.Select[string]{Label: "取关分组", Placeholder: "先加载分组", Options: choices, Value: group.Get, OnChange: group.Set, Disabled: a.isBusy, Presentation: base.PresentSheet}}
		}),
		mobileRow{Icon: lucide.RefreshCw, Tint: colors.Accent, Title: "加载关注分组", Accent: true, Disabled: a.isBusy, OnPress: loadGroups},
	)
	limits := mobileSection(cx, "范围", "保留名单中的用户不会被取关。",
		f.number("最多取关人数", "0 为全部", int64(s.Tasks.Unfollow.Count), func(v int64) { s.Tasks.Unfollow.Count = int(v) }),
		f.ids("保留关注的 UID", s.Tasks.Unfollow.Keep, func(v []int64) { s.Tasks.Unfollow.Keep = v }),
	)
	run := mobileButton{Text: "保存并运行取关", Icon: lucide.Play, Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		mid := account.Get()
		if mid == "" {
			a.message("请先选择账号")
			return
		}
		if loadedFor.Get() != mid || group.Get() == "" {
			a.message("请加载并选择要取关的关注分组")
			return
		}
		s.Tasks.Unfollow.Group = group.Get()
		edited := client.CloneSettings(s)
		name := mid
		for _, user := range state.Accounts {
			if user.Mid == mid {
				name = user.Uname + "（" + mid + "）"
				break
			}
		}
		count := strconv.Itoa(edited.Tasks.Unfollow.Count) + " 人"
		if edited.Tasks.Unfollow.Count == 0 {
			count = "全部"
		}
		cx.ShowAlert(core.AlertConfig{Title: "确认批量取关？", Message: fmt.Sprintf("账号：%s\n分组：%s\n最多取关：%s\n保留名单中的用户不会被取关。", name, edited.Tasks.Unfollow.Group, count), Actions: []core.AlertAction{{Label: "取消", Role: core.AlertCancel}, {Label: "确认取关", Role: core.AlertDestructive}}}, func(index int) {
			if index != 1 {
				return
			}
			a.saveSettings(cx, snapshotAtLastSave, edited, func(api.Settings) {
				snapshotAtLastSave = client.CloneSettings(edited)
				a.perform("批量取关", func(ctx context.Context) error {
					return cl.Run(ctx, mid, []string{"unfollow"})
				}, func() { a.refresh(); a.message("取关任务已开始，可在报告中查看进度") })
			})
		})
	}}
	return mobilePage(cx, nil, nil, target, limits, run)
}
