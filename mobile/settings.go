package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// mobileSettingsForm 保存页面里的编辑值，定时刷新不会覆盖尚未保存的修改。
type mobileSettingsForm struct {
	cx      *core.Scope
	nodes   []core.Node
	readers []func() error
}

func (f *mobileSettingsForm) text(label, initial string, put func(string)) {
	value := core.NewSignal(f.cx, initial)
	f.nodes = append(f.nodes, mobileText(f.cx, label), ui.Input{Label: label, Value: value.Get, OnChange: value.Set, Placeholder: label})
	f.readers = append(f.readers, func() error { put(value.Get()); return nil })
}

func (f *mobileSettingsForm) number(label string, initial int64, put func(int64)) {
	value := core.NewSignal(f.cx, strconv.FormatInt(initial, 10))
	f.nodes = append(f.nodes, mobileText(f.cx, label), ui.Input{Label: label, Value: value.Get, OnChange: value.Set, Placeholder: label, KeyboardType: core.KeyboardNumber})
	f.readers = append(f.readers, func() error {
		n, err := strconv.ParseInt(strings.TrimSpace(value.Get()), 10, 64)
		if err != nil {
			return fmt.Errorf("%s：请输入整数", label)
		}
		put(n)
		return nil
	})
}

func (f *mobileSettingsForm) toggle(label string, target *bool) {
	value := core.NewSignal(f.cx, *target)
	f.nodes = append(f.nodes, mobileText(f.cx, label), ui.Switch{Label: label, Checked: value.Get, OnChange: value.Set})
	f.readers = append(f.readers, func() error { *target = value.Get(); return nil })
}

func (f *mobileSettingsForm) selectValue(label string, target *string, options []ui.Option[string]) {
	value := core.NewSignal(f.cx, *target)
	f.nodes = append(f.nodes, mobileText(f.cx, label), ui.Select[string]{Label: label, Options: options, Value: value.Get, OnChange: value.Set})
	f.readers = append(f.readers, func() error { *target = value.Get(); return nil })
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

func (f *mobileSettingsForm) ids(label string, initial []int64, put func([]int64)) {
	parts := make([]string, len(initial))
	for i, id := range initial {
		parts[i] = strconv.FormatInt(id, 10)
	}
	value := core.NewSignal(f.cx, strings.Join(parts, ","))
	f.nodes = append(f.nodes, mobileText(f.cx, label+"（逗号分隔）"), ui.Input{Label: label, Value: value.Get, OnChange: value.Set})
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
}

func (a *mobileApp) settingsPage(cx *core.Scope) core.Node {
	state := a.state.GetUntracked()
	s := state.Settings
	f := &mobileSettingsForm{cx: cx}
	f.nodes = append(f.nodes, mobileText(cx, "每日计划"), mobileText(cx, "时间按北京时间运行，每个账号分别随机延后。"))
	f.text("每日开始时间（HH:MM）", s.RunAt, func(v string) { s.RunAt = v })
	f.number("随机延后上限（分钟）", int64(s.Jitter), func(v int64) { s.Jitter = int(v) })
	f.selectValue("请求节奏", &s.Pace, mobileStringOptions("slow", "慢速", "normal", "正常", "fast", "快速"))
	f.text("代理地址（留空直连）", s.Proxy, func(v string) { s.Proxy = v })
	f.nodes = append(f.nodes, ui.Button{Text: "测试代理出口", Variant: ui.ButtonSecondary, Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		proxy := s.Proxy
		var egress api.Egress
		a.perform("测试代理", func(ctx context.Context) error {
			var err error
			egress, err = a.client.TestProxy(ctx, proxy)
			return err
		}, func() {
			if egress.Error != "" {
				a.message(egress.Error)
				return
			}
			a.message(strings.TrimSpace(egress.Addr + " " + egress.Place + " " + egress.ISP))
		})
	}})
	f.nodes = append(f.nodes, mobileText(cx, "运行通知"))
	f.selectValue("何时通知", &s.Notify.When, mobileStringOptions("never", "不通知", "failed", "有失败时", "always", "每次运行后"))
	f.selectValue("通知渠道", &s.Notify.Channel, mobileStringOptions("telegram", "Telegram", "pushplus", "PushPlus", "wecom", "企业微信", "dingtalk", "钉钉", "serverchan", "Server 酱", "bark", "Bark", "webhook", "Webhook"))
	f.text("密钥、机器人令牌或 Webhook 地址", s.Notify.Token, func(v string) { s.Notify.Token = v })
	f.text("Telegram chat_id / 钉钉加签密钥", s.Notify.Target, func(v string) { s.Notify.Target = v })
	f.text("自建通知服务地址（留空使用官方）", s.Notify.Endpoint, func(v string) { s.Notify.Endpoint = v })
	f.nodes = append(f.nodes, ui.Button{Text: "发送测试通知", Variant: ui.ButtonSecondary, Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		n := s.Notify
		a.perform("发送测试通知", func(ctx context.Context) error { return a.client.TestNotify(ctx, n) }, func() { a.message("测试通知已发送") })
	}})
	f.selectValue("日志级别", &s.LogLevel, mobileStringOptions("debug", "调试", "info", "信息", "warn", "警告", "error", "错误"))
	f.nodes = append(f.nodes, mobileText(cx, "任务开关与选项"))
	for _, task := range state.Tasks {
		id := task.ID
		if enabled := mobileTaskEnabled(&s.Tasks, id); enabled != nil {
			f.toggle(task.Title, enabled)
		}
		f.nodes = append(f.nodes, ui.Button{Text: task.Title + "选项", Variant: ui.ButtonSecondary, Disabled: a.isBusy, OnPress: func() {
			if id == "unfollow" {
				a.openUnfollow()
			} else {
				a.openTask(id)
			}
		}})
	}
	f.nodes = append(f.nodes, ui.Button{Text: "保存设置", Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		latest := a.state.GetUntracked().Settings
		latest.RunAt, latest.Jitter, latest.Pace, latest.Proxy, latest.LogLevel, latest.Notify = s.RunAt, s.Jitter, s.Pace, s.Proxy, s.LogLevel, s.Notify
		for _, task := range state.Tasks {
			initial, edited, current := mobileTaskEnabled(&state.Settings.Tasks, task.ID), mobileTaskEnabled(&s.Tasks, task.ID), mobileTaskEnabled(&latest.Tasks, task.ID)
			if initial != nil && edited != nil && current != nil && *initial != *edited {
				*current = *edited
			}
		}
		a.saveSettings(latest)
	}})
	mode, action := "远程服务器", "断开连接"
	if state.Local {
		mode, action = "本机运行", "停止本机运行"
	}
	f.nodes = append(f.nodes, mobileText(cx, "连接与版本"), mobileText(cx, mode+"\n"+a.client.Base()), mobileText(cx, "App "+mobileVersion+" / 服务端 "+state.Version), ui.Button{Text: action, Variant: ui.ButtonSecondary, Disabled: a.isBusy, OnPress: a.disconnect})
	return mobileColumn(f.nodes...)
}

func mobileTaskEnabled(tasks *api.TaskSettings, id string) *bool {
	switch id {
	case "daily":
		return &tasks.Daily.Enabled
	case "vip_point":
		return &tasks.VipPoint.Enabled
	case "vip_privilege":
		return &tasks.VipPrivilege.Enabled
	case "charge":
		return &tasks.Charge.Enabled
	case "manga":
		return &tasks.Manga.Enabled
	case "manga_privilege":
		return &tasks.MangaPrivilege.Enabled
	case "silver2coin":
		return &tasks.Silver2Coin.Enabled
	case "fans_medal":
		return &tasks.FansMedal.Enabled
	case "live_lottery":
		return &tasks.LiveLottery.Enabled
	}
	return nil
}

func (a *mobileApp) taskOptionsPage(cx *core.Scope, id string) core.Node {
	state := a.state.GetUntracked()
	s := state.Settings
	f := &mobileSettingsForm{cx: cx}
	for _, task := range state.Tasks {
		if task.ID == id {
			f.nodes = append(f.nodes, mobileText(cx, task.Title), mobileText(cx, task.Desc))
		}
	}
	if enabled := mobileTaskEnabled(&s.Tasks, id); enabled != nil {
		f.toggle("启用任务", enabled)
	}
	switch id {
	case "daily":
		o := &s.Tasks.Daily
		f.toggle("观看视频", &o.Watch)
		f.toggle("分享视频", &o.Share)
		f.number("每天投币数（0–5）", int64(o.Coins), func(v int64) { o.Coins = int(v) })
		f.number("至少保留硬币", int64(o.KeepCoins), func(v int64) { o.KeepCoins = int(v) })
		f.number("停止投币等级（0 为不限）", int64(o.StopAtLevel), func(v int64) { o.StopAtLevel = int(v) })
		f.toggle("投币时点赞", &o.Like)
		f.ids("优先 UP 主 UID", o.UpIDs, func(v []int64) { o.UpIDs = v })
	case "charge":
		o := &s.Tasks.Charge
		f.number("充电 UP 主 UID（0 为不充电）", o.UpID, func(v int64) { o.UpID = v })
		f.number("每月几号之后充电（1–28）", int64(o.Day), func(v int64) { o.Day = int(v) })
		f.text("充电留言", o.Comment, func(v string) { o.Comment = v })
	case "manga":
		o := &s.Tasks.Manga
		f.toggle("签到后记录阅读", &o.Read)
		f.number("漫画 ID", o.ComicID, func(v int64) { o.ComicID = v })
		f.number("章节 ID", o.EpID, func(v int64) { o.EpID = v })
	case "fans_medal":
		o := &s.Tasks.FansMedal
		f.toggle("给直播间点赞", &o.Like)
		f.text("弹幕内容（留空不发送）", o.Danmaku, func(v string) { o.Danmaku = v })
		f.toggle("积累观看时长", &o.Watch)
		f.number("每个粉丝牌最多观看分钟", int64(o.Minutes), func(v int64) { o.Minutes = int(v) })
		f.number("每轮粉丝牌数量（0 为全部）", int64(o.Max), func(v int64) { o.Max = int(v) })
	case "live_lottery":
		o := &s.Tasks.LiveLottery
		f.number("查找间隔（小时，0 为仅每日运行）", int64(o.EveryHours), func(v int64) { o.EveryHours = int(v) })
		f.text("奖品包含关键词（逗号分隔）", strings.Join(o.Include, ","), func(v string) { o.Include = mobileSplitValues(v) })
		f.text("排除关键词（逗号分隔）", strings.Join(o.Exclude, ","), func(v string) { o.Exclude = mobileSplitValues(v) })
		f.toggle("参加需要关注的抽奖", &o.Follow)
		f.toggle("将抽奖关注放进天选时刻分组", &o.Group)
	case "unfollow":
		return a.unfollowPage(cx)
	case "vip_point", "vip_privilege", "manga_privilege", "silver2coin":
		f.nodes = append(f.nodes, mobileText(cx, "此任务只需设置是否启用。"))
	default:
		return mobileColumn(mobileText(cx, "未找到任务："+id))
	}
	f.nodes = append(f.nodes, ui.Button{Text: "保存任务选项", Disabled: a.isBusy, OnPress: func() {
		if err := f.read(); err != nil {
			a.message(err.Error())
			return
		}
		latest := a.state.GetUntracked().Settings
		mobileCopyTaskOptions(&latest.Tasks, s.Tasks, id)
		a.saveSettings(latest)
	}})
	return mobileColumn(f.nodes...)
}

func (a *mobileApp) unfollowPage(cx *core.Scope) core.Node {
	state := a.state.GetUntracked()
	s := state.Settings
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
		a.perform("加载关注分组", func(ctx context.Context) error { var err error; result, err = a.client.Groups(ctx, mid); return err }, func() {
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
	f.nodes = append(f.nodes, mobileText(cx, "批量取关"), mobileText(cx, "仅手动运行。请核对账号和分组，运行后会取消关注其中的用户。"), mobileText(cx, "账号"), ui.Select[string]{Label: "取关账号", Options: options, Value: account.Get, Disabled: a.isBusy, Placeholder: "请选择账号", OnChange: func(v string) { account.Set(v); groups.Set(nil); loadedFor.Set(""); group.Set(""); loadGroups() }}, ui.Button{Text: "加载关注分组", Variant: ui.ButtonSecondary, Disabled: a.isBusy, OnPress: loadGroups}, core.Dynamic(func(cx *core.Scope) core.Node {
		items := groups.Get()
		choices := make([]ui.Option[string], 0, len(items))
		for _, item := range items {
			choices = append(choices, ui.Option[string]{Value: item.Name, Label: fmt.Sprintf("%s（%d 人）", item.Name, item.Count)})
		}
		return ui.Select[string]{Label: "取关分组", Placeholder: "加载后选择关注分组", Options: choices, Value: group.Get, OnChange: group.Set, Disabled: a.isBusy}
	}))
	f.number("最多取关人数（0 为全部）", int64(s.Tasks.Unfollow.Count), func(v int64) { s.Tasks.Unfollow.Count = int(v) })
	f.ids("保留关注的 UID", s.Tasks.Unfollow.Keep, func(v []int64) { s.Tasks.Unfollow.Keep = v })
	f.nodes = append(f.nodes, ui.Button{Text: "保存并运行取关", Disabled: a.isBusy, OnPress: func() {
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
		settings := a.state.GetUntracked().Settings
		settings.Tasks.Unfollow = s.Tasks.Unfollow
		name := mid
		for _, user := range state.Accounts {
			if user.Mid == mid {
				name = user.Uname + "（" + mid + "）"
				break
			}
		}
		count := strconv.Itoa(settings.Tasks.Unfollow.Count) + " 人"
		if settings.Tasks.Unfollow.Count == 0 {
			count = "全部"
		}
		cx.ShowAlert(core.AlertConfig{Title: "确认批量取关？", Message: fmt.Sprintf("账号：%s\n分组：%s\n最多取关：%s\n保留名单中的用户不会被取关。", name, settings.Tasks.Unfollow.Group, count), Actions: []core.AlertAction{{Label: "取消", Role: core.AlertCancel}, {Label: "确认取关", Role: core.AlertDestructive}}}, func(index int) {
			if index != 1 {
				return
			}
			a.perform("批量取关", func(ctx context.Context) error {
				if _, err := a.client.SaveSettings(ctx, settings); err != nil {
					return err
				}
				return a.client.Run(ctx, mid, []string{"unfollow"})
			}, func() { a.refresh(); a.message("取关任务已开始，可在报告中查看进度") })
		})
	}})
	return mobileColumn(f.nodes...)
}

// mobileCopyTaskOptions 只保存当前任务，保留其他页面刚保存的设置。
func mobileCopyTaskOptions(dst *api.TaskSettings, src api.TaskSettings, id string) {
	switch id {
	case "daily":
		dst.Daily = src.Daily
	case "vip_point":
		dst.VipPoint = src.VipPoint
	case "vip_privilege":
		dst.VipPrivilege = src.VipPrivilege
	case "charge":
		dst.Charge = src.Charge
	case "manga":
		dst.Manga = src.Manga
	case "manga_privilege":
		dst.MangaPrivilege = src.MangaPrivilege
	case "silver2coin":
		dst.Silver2Coin = src.Silver2Coin
	case "fans_medal":
		dst.FansMedal = src.FansMedal
	case "live_lottery":
		dst.LiveLottery = src.LiveLottery
	case "unfollow":
		dst.Unfollow = src.Unfollow
	}
}
