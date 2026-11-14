package main

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

func (a *app) settingsText(c *ui.Context, key, label string, value *string, hint string) {
	ui.Field(c, label, func() {
		if ui.TextInput(c.Key("settings-"+key), value).Grow(1).Changed() {
			a.dirty = true
		}
	}).Description(hint)
}

func (a *app) settingsSwitch(c *ui.Context, key, label string, value *bool) {
	ui.Field(c, label, func() {
		if ui.Switch(c.Key("settings-"+key), value).Changed() {
			a.dirty = true
		}
	})
}

func (a *app) settingsChoice(c *ui.Context, key, label string, value *string, values, names []string, hint string) {
	f := ui.Field(c, label, func() {
		i := 0
		for n, v := range values {
			if v == *value {
				i = n
				break
			}
		}
		if len(names) > 4 {
			selected := names[i]
			if ui.Select(c.Key("settings-"+key), &selected, names).Label(label).Changed() {
				for n, name := range names {
					if name == selected {
						*value = values[n]
						a.dirty = true
						break
					}
				}
			}
		} else if ui.Segmented(c.Key("settings-"+key), &i, names...).Label(label).Changed() {
			*value = values[i]
			a.dirty = true
		}
	})
	if hint != "" {
		f.Description(hint)
	}
}

func (a *app) settingsRaw(key, initial string) string {
	if a.inputs == nil {
		a.inputs = make(map[string]string)
	}
	if s, ok := a.inputs[key]; ok {
		return s
	}
	a.inputs[key] = initial
	return initial
}

func settingsIntegerError(s string) string {
	if _, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err != nil {
		return "请输入有效的整数"
	}
	return ""
}

func (a *app) settingsNumber64(c *ui.Context, label string, value *int64, hint string) {
	key := "整数:" + label
	s := a.settingsRaw(key, strconv.FormatInt(*value, 10))
	ui.Field(c, label, func() {
		if ui.TextInput(c.Key("settings-"+key), &s).Width(240).Changed() {
			a.inputs[key] = s
			a.dirty = true
			if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				*value = n
			}
		}
	}).Description(hint).Error(settingsIntegerError(s))
}

func (a *app) settingsNumber(c *ui.Context, label string, value *int, hint string) {
	n := int64(*value)
	a.settingsNumber64(c, label, &n, hint)
	*value = int(n)
}

func settingsParts(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == '，' || r == '、' || r == ';' || r == '；'
	})
}

func settingsIDs(s string) ([]int64, string) {
	ids := make([]int64, 0)
	for _, p := range settingsParts(s) {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 {
			return nil, "UID 必须是正整数，用空格或逗号分隔"
		}
		ids = append(ids, n)
	}
	return ids, ""
}

func (a *app) settingsUIDs(c *ui.Context, label string, value *[]int64) {
	parts := make([]string, 0, len(*value))
	for _, n := range *value {
		parts = append(parts, strconv.FormatInt(n, 10))
	}
	key := "UID:" + label
	s := a.settingsRaw(key, strings.Join(parts, ", "))
	ui.Field(c, label, func() {
		if ui.TextInput(c.Key("settings-"+key), &s).Grow(1).Changed() {
			a.inputs[key] = s
			a.dirty = true
			if ids, err := settingsIDs(s); err == "" {
				*value = ids
			}
		}
	}).Description("用空格或逗号分隔，留空自动选择。").Error(func() string { _, err := settingsIDs(s); return err }())
	a.settingsUPPreviews(c, "daily", s)
}

func (a *app) settingsWords(c *ui.Context, label string, value *[]string, hint string) {
	key := "关键词:" + label
	s := a.settingsRaw(key, strings.Join(*value, "，"))
	ui.Field(c, label, func() {
		if ui.TextInput(c.Key("settings-"+key), &s).Grow(1).Changed() {
			a.inputs[key] = s
			*value = settingsParts(s)
			a.dirty = true
		}
	}).Description(hint)
}

func (a *app) settingsInputError() string {
	for key, s := range a.inputs {
		if strings.HasPrefix(key, "整数:") && settingsIntegerError(s) != "" {
			return strings.TrimPrefix(key, "整数:") + "：请输入有效的整数"
		}
		if strings.HasPrefix(key, "UID:") {
			if _, err := settingsIDs(s); err != "" {
				return strings.TrimPrefix(key, "UID:") + "：" + err
			}
		}
	}
	return ""
}

// settingsSave 只在有修改、正在保存或出错时出现，平时不占位置
func (a *app) settingsSave(c *ui.Context, key string) {
	err := a.settingsInputError()
	if !a.dirty && !a.saving && a.formErr == "" && err == "" {
		return
	}
	if a.formErr != "" {
		ui.Text(c, a.formErr).TextColor(c.Theme().Danger).Selectable()
	}
	if err != "" {
		ui.Text(c, err).TextColor(c.Theme().Danger)
	}
	ui.Row(c).Gap(12).Children(func() {
		label := "保存"
		if a.saving {
			label = "正在保存"
		}
		ui.PrimaryButton(c.Key(key), label).Disabled(!a.dirty || a.saving || err != "").OnClick(a.saveSettings)
		muted(c, "有未保存的修改，所有账号共用")
	})
}

func (a *app) tasksView(c *ui.Context) {
	if a.st == nil {
		muted(c, "连接后可查看任务设置。")
		return
	}
	if a.taskOptions == nil {
		a.taskOptions = make(map[string]bool)
	}
	groups := make([]string, 0)
	for _, task := range a.st.Tasks {
		found := false
		for _, group := range groups {
			if group == task.Group {
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, task.Group)
		}
	}
	ui.Column(c).Gap(20).MaxWidth(840).Disabled(a.saving).Children(func() {
		a.settingsSave(c, "tasks-save-top")
		for _, group := range groups {
			ui.Text(c, group).FontSize(19).Bold()
			for _, task := range a.st.Tasks {
				if task.Group != group {
					continue
				}
				task := task
				panel(c.Key("task-" + task.ID)).Padding(16).Gap(8).Children(func() {
					ui.Row(c).Gap(12).Children(func() {
						ui.Text(c, task.Title).FontSize(16).Bold().Grow(1)
						if enabled := client.TaskEnabled(&a.form.Tasks, task.ID); enabled != nil {
							if ui.Switch(c.Key("task-enabled-"+task.ID), enabled).Label("启用" + task.Title).Changed() {
								a.dirty = true
							}
						}
						ui.Button(c.Key("task-run-"+task.ID), "运行一次").Disabled(a.saving || a.dirty || a.busy["run"]).OnClick(func() { a.runTask(task.ID) })
					})
					if task.Desc != "" {
						muted(c, task.Desc).FontSize(13)
					}
					if task.Needs != "" {
						muted(c, "账号要求："+task.Needs).FontSize(12)
					}
					if task.Manual {
						muted(c, "仅手动运行，不参加每日计划。").FontSize(12)
						return
					}
					switch task.ID {
					case "daily", "charge", "manga", "fans_medal", "live_lottery":
						open := a.taskOptions[task.ID]
						if ui.Collapsible(c.Key("task-options-"+task.ID), "选项", &open, func() {
							ui.Form(c, func() { a.settingsTaskOptions(c, task.ID) })
						}).Changed() {
							a.taskOptions[task.ID] = open
						}
					}
				})
			}
		}
		a.settingsSave(c, "tasks-save-bottom")
	})
}

func (a *app) settingsTaskOptions(c *ui.Context, id string) {
	t := &a.form.Tasks
	switch id {
	case "daily":
		d := &t.Daily
		a.settingsSwitch(c, "daily-watch", "观看视频", &d.Watch)
		a.settingsSwitch(c, "daily-share", "分享视频", &d.Share)
		a.settingsNumber(c, "每日投币数", &d.Coins, "0～5 个，0 表示不投币。")
		a.settingsNumber(c, "保留硬币数", &d.KeepCoins, "余额至少保留这么多硬币。")
		a.settingsNumber(c, "停止投币等级", &d.StopAtLevel, "0～6 级，0 表示一直投币。")
		a.settingsSwitch(c, "daily-like", "投币时点赞", &d.Like)
		a.settingsUIDs(c, "优先观看的 UP 主 UID", &d.UpIDs)
	case "charge":
		d := &t.Charge
		a.settingsNumber64(c, "充电对象 UID", &d.UpID, "会消耗 B 币券，启用前请确认对象。")
		if mid, err := strconv.ParseInt(strings.TrimSpace(a.settingsRaw("整数:充电对象 UID", strconv.FormatInt(d.UpID, 10))), 10, 64); err == nil && mid > 0 {
			a.settingsUPPreviews(c, "charge", strconv.FormatInt(mid, 10))
		}
		a.settingsNumber(c, "每月充电日期", &d.Day, "1～28 号，从这一天开始尝试充电。")
		a.settingsText(c, "charge-comment", "充电留言", &d.Comment, "留空不留言。")
	case "manga":
		d := &t.Manga
		a.settingsSwitch(c, "manga-read", "签到后阅读", &d.Read)
		a.settingsNumber64(c, "漫画 ID", &d.ComicID, "开启阅读时必须填写有效漫画 ID。")
		a.settingsNumber64(c, "章节 ID", &d.EpID, "填写该漫画对应章节的 ID。")
	case "fans_medal":
		d := &t.FansMedal
		a.settingsSwitch(c, "medal-like", "直播间点赞", &d.Like)
		a.settingsText(c, "medal-danmaku", "直播间弹幕", &d.Danmaku, "最多 30 个字，留空不发送。")
		a.settingsSwitch(c, "medal-watch", "累计观看时长", &d.Watch)
		a.settingsNumber(c, "每个粉丝牌观看分钟数", &d.Minutes, "1～120 分钟。")
		a.settingsNumber(c, "每轮粉丝牌数量", &d.Max, "0～200，0 表示全部。")
	case "live_lottery":
		d := &t.LiveLottery
		a.settingsNumber(c, "天选时刻间隔小时数", &d.EveryHours, "0～24 小时，0 表示仅随每日计划运行。")
		a.settingsWords(c, "奖品包含关键词", &d.Include, "用空格或逗号分隔，留空不限制。")
		a.settingsWords(c, "奖品排除关键词", &d.Exclude, "包含任一关键词的奖品不参加。")
		a.settingsSwitch(c, "lottery-follow", "允许关注主播", &d.Follow)
		a.settingsSwitch(c, "lottery-group", "关注后加入「天选时刻」分组", &d.Group)
	}
}

func (a *app) settingsView(c *ui.Context) {
	if a.st == nil {
		muted(c, "连接后可修改设置。")
		return
	}
	ui.Column(c).Gap(20).MaxWidth(840).Disabled(a.saving).Children(func() {
		panel(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, "运行计划").FontSize(17).Bold()
			ui.Form(c, func() {
				a.settingsText(c, "run-at", "每天开始时间（北京时间）", &a.form.RunAt, "使用 24 小时制，例如 09:30。")
				a.settingsNumber(c, "随机推迟分钟数", &a.form.Jitter, "0～720 分钟，每个账号每天独立随机。")
				a.settingsChoice(c, "pace", "请求节奏", &a.form.Pace, []string{"slow", "normal", "fast"}, []string{"慢速", "正常", "快速"}, "遇到风控或请求过于频繁时，改用慢速并稍后再试。")
				a.settingsText(c, "proxy", "代理地址", &a.form.Proxy, "留空直连，支持 HTTP、HTTPS、SOCKS5 代理。")
			})
			ui.Row(c).Children(func() {
				ui.Button(c.Key("settings-proxy-test"), "测试代理").Disabled(a.busy["proxy"]).OnClick(func() { a.testProxy(a.form.Proxy) })
			})
		})
		panel(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, "结果推送").FontSize(17).Bold()
			ui.Form(c, func() {
				n := &a.form.Notify
				a.settingsChoice(c, "notify-when", "推送时机", &n.When, []string{"never", "failed", "always"}, []string{"不推送", "有失败时", "每次运行后"}, "")
				a.settingsChoice(c, "notify-channel", "推送渠道", &n.Channel, []string{"", "telegram", "pushplus", "wecom", "dingtalk", "serverchan", "bark", "webhook"}, []string{"未选择", "Telegram", "PushPlus", "企业微信", "钉钉", "Server酱", "Bark", "自定义"}, "")
				switch n.Channel {
				case "telegram":
					a.settingsText(c, "notify-token", "机器人 Token", &n.Token, "从 Telegram 的 BotFather 获取。")
					a.settingsText(c, "notify-target", "聊天 ID", &n.Target, "填写接收消息的 chat_id。")
					a.settingsText(c, "notify-endpoint", "Telegram 反代地址", &n.Endpoint, "留空使用官方服务。")
				case "pushplus":
					a.settingsText(c, "notify-token", "PushPlus Token", &n.Token, "填写 PushPlus 的推送令牌。")
				case "wecom":
					a.settingsText(c, "notify-token", "企业微信机器人地址", &n.Token, "填写群机器人的完整 Webhook 地址。")
				case "dingtalk":
					a.settingsText(c, "notify-token", "钉钉机器人地址", &n.Token, "填写机器人的完整 Webhook 地址。")
					a.settingsText(c, "notify-target", "钉钉加签密钥", &n.Target, "未开启加签时留空。")
				case "serverchan":
					a.settingsText(c, "notify-token", "Server酱 SendKey", &n.Token, "填写 Server酱的发送密钥。")
				case "bark":
					a.settingsText(c, "notify-token", "Bark 设备密钥", &n.Token, "填写设备的推送密钥。")
					a.settingsText(c, "notify-endpoint", "Bark 服务器地址", &n.Endpoint, "留空使用官方服务。")
				case "webhook":
					a.settingsText(c, "notify-token", "推送 Webhook 地址", &n.Token, "填写接收运行报告的完整地址。")
				}
			})
			ui.Row(c).Children(func() {
				ui.Button(c.Key("settings-notify-test"), "发送测试").Disabled(a.busy["notify"] || a.form.Notify.Channel == "").OnClick(a.testNotify)
			})
		})
		panel(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, "报告与日志").FontSize(17).Bold()
			ui.Form(c, func() {
				a.settingsNumber(c, "报告保留天数", &a.form.ReportRetentionDays, "保留最近 "+strconv.Itoa(a.form.ReportRetentionDays)+" 天的报告（1～365 天），每个账号最多保留 60 份。")
				a.settingsChoice(c, "log-level", "日志级别", &a.form.LogLevel, []string{"debug", "info", "warn", "error"}, []string{"调试", "信息", "警告", "错误"}, "排查问题时改成「调试」。")
			})
		})
		a.settingsSave(c, "settings-save")
		panel(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, "连接与服务").FontSize(17).Bold()
			label := "断开连接"
			if a.session.LocalEngine() != nil {
				label = "停止本机运行"
				muted(c, "停止后，本机的运行任务和每日计划也会停止。")
			} else {
				muted(c, "断开后，远程服务会继续运行。")
			}
			ui.Button(c.Key("settings-disconnect"), label).OnClick(a.disconnect)
		})
	})
}
