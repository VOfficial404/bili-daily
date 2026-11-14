package main

import (
	"fmt"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (a *mobileApp) qrPage(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	nav := a.navOf(cx)
	qr := core.NewSignalFunc(cx, api.QR{}, nil)
	loading := core.NewSignal(cx, false)
	errText := core.NewSignal(cx, "")
	cl := a.session.Client()
	ctx := cx.Context()
	generation := 0
	var request func()
	var poll func(string, int)
	showError := func(err error) { errText.Set(err.Error()); loading.Set(false) }
	poll = func(id string, g int) {
		cx.AfterFunc(2*time.Second, func() {
			if g != generation || ctx.Err() != nil {
				return
			}
			if !a.active {
				poll(id, g)
				return
			}
			go func() {
				v, err := cl.PollQR(ctx, id)
				cx.Post(func() {
					if g != generation {
						return
					}
					if err != nil {
						showError(err)
						return
					}
					old := qr.GetUntracked()
					v.PNG = old.PNG
					v.URL = old.URL
					qr.Set(v)
					switch v.State {
					case "success":
						a.message("账号已添加")
						nav.Pop(1)
						a.refresh()
					case "expired":
						errText.Set("二维码已过期，请重新获取")
					default:
						poll(id, g)
					}
				})
			}()
		})
	}
	request = func() {
		if loading.GetUntracked() {
			return
		}
		generation++
		g := generation
		loading.Set(true)
		errText.Set("")
		qr.Set(api.QR{})
		go func() {
			v, err := cl.StartQR(ctx)
			cx.Post(func() {
				if g != generation {
					return
				}
				if err != nil {
					showError(err)
					return
				}
				loading.Set(false)
				qr.Set(v)
				poll(v.ID, g)
			})
		}()
	}
	cx.Post(request)
	errBox := core.Show{When: func() bool { return errText.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		t := ui.Textf(errText.Get, 14, ui.Medium, colors.Danger)
		t.Style.LineHeight, t.Style.TextAlign = 20, core.TextAlignCenter
		return t
	}}
	return mobilePage(cx, nil, nil,
		core.Dynamic(func(cx *core.Scope) core.Node {
			q := qr.Get()
			var code core.Node
			status, statusColor := "等待扫码", colors.Muted
			switch {
			case loading.Get():
				code, status = mobileLabel(cx, "正在获取二维码…", 15, ui.Regular, ui.Fixed(core.RGB(0x6b6b74)), 1), "请稍候"
			case len(q.PNG) == 0:
				code, status = mobileLabel(cx, "二维码暂不可用", 15, ui.Regular, ui.Fixed(core.RGB(0x6b6b74)), 1), "请重新获取"
			default:
				// 二维码始终放在白底上，深色模式下也能被扫到。
				code = core.Image{Source: core.ImageSource{Data: q.PNG, Size: core.Size{Width: 240, Height: 240}}, ResizeMode: core.Contain, Style: core.Style{Width: core.Pt(220), Height: core.Pt(220)}}
			}
			switch q.State {
			case "scanned":
				status, statusColor = "已扫码，请在 B 站确认", colors.Accent
			case "expired":
				status, statusColor = "二维码已过期", colors.Danger
			case "success":
				status, statusColor = "登录成功", colors.Up
			}
			return mobileCard(cx, core.Style{Padding: core.All(20), Gap: 14, AlignItems: core.AlignCenter},
				core.View{Style: core.Style{Width: core.Pt(244), Height: core.Pt(244), CornerRadius: 16, Background: core.RGB(0xffffff), BorderWidth: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					StyleFunc: func() core.Style { return core.Style{BorderColor: colors.Border()} },
					Children:  []core.Node{code}},
				mobileLabel(cx, status, 16, ui.Semibold, statusColor, 1),
				mobileLabel(cx, "用 B 站 App 扫码并确认登录。无法打开链接时，截图后在 B 站「扫一扫」里从相册选择。", 14, ui.Regular, colors.Muted, 0),
				errBox,
				core.View{Style: core.Style{AlignSelf: core.AlignStretch, Gap: 10}, Children: []core.Node{
					mobileButton{Text: "在 B 站 App 里打开", Icon: lucide.ExternalLink, Disabled: func() bool { return q.State == "expired" || q.URL == "" }, OnPress: func() {
						cx.OpenURL(q.URL, func(opened bool) {
							if !opened {
								errText.Set("无法打开链接。请截图，在 B 站扫一扫中从相册选择二维码。")
							}
						})
					}},
					core.View{Style: core.Style{Direction: core.Row, Gap: 10}, Children: []core.Node{
						mobileButton{Text: "复制链接", Icon: lucide.Copy, Kind: buttonSecondary, Flex: 1, Disabled: func() bool { return q.URL == "" }, OnPress: func() { cx.SetClipboard(q.URL); a.message("链接已复制") }},
						mobileButton{Text: "重新获取", Icon: lucide.RefreshCw, Kind: buttonSecondary, Flex: 1, OnPress: request, Disabled: loading.Get},
					}},
				}},
			)
		}),
	)
}

func (a *mobileApp) reportsPage(cx *core.Scope) core.Node {
	nav := a.navOf(cx)
	filter := core.NewSignal(cx, "")
	runs := core.NewSignalFunc(cx, []api.RunSummary{}, nil)
	loading := core.NewSignal(cx, false)
	errText := core.NewSignal(cx, "")
	cl := a.session.Client()
	generation := 0
	load := func(mid string) {
		generation++
		g := generation
		loading.Set(true)
		errText.Set("")
		ctx := cx.Context()
		go func() {
			v, err := cl.Runs(ctx, mid, 60)
			cx.Post(func() {
				if g != generation {
					return
				}
				loading.Set(false)
				if err != nil {
					errText.Set(err.Error())
					return
				}
				runs.Set(v)
			})
		}()
	}
	cx.Post(func() { load("") })
	colors := ui.ColorsOf(cx)
	accounts := core.Dynamic(func(cx *core.Scope) core.Node {
		opts := []ui.Option[string]{{Value: "", Label: "全部账号"}}
		for _, acc := range a.snapshot().Accounts {
			opts = append(opts, ui.Option[string]{Value: acc.Mid, Label: acc.Uname})
		}
		return mobileSection(cx, "", "下拉刷新报告。时间为北京时间。",
			mobileRow{Icon: lucide.Users, Tint: colors.Accent, Title: "账号", Control: ui.Select[string]{Label: "筛选账号", Options: opts, Value: filter.Get,
				OnChange: func(mid string) { filter.Set(mid); load(mid) }, Presentation: base.PresentSheet}})
	})
	return mobilePage(cx, func() { load(filter.GetUntracked()) }, loading.Get,
		accounts,
		core.Show{When: func() bool { return errText.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return ui.Textf(errText.Get, 14, ui.Medium, colors.Danger)
		}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			values := runs.Get()
			if loading.Get() && len(values) == 0 {
				return mobileText(cx, "正在读取报告…")
			}
			if len(values) == 0 {
				return mobileCard(cx, core.Style{Padding: core.All(24), Gap: 10, AlignItems: core.AlignCenter},
					ui.Icon(lucide.FileText, 32, colors.Faint),
					mobileLabel(cx, "还没有报告。运行任务后可在这里查看每一步的结果。", 14, ui.Regular, colors.Muted, 0))
			}
			var sections []core.Node
			var day, key string
			var rows []core.Node
			flush := func() {
				if len(rows) > 0 {
					sections = append(sections, mobileSection(cx, day, "", rows...))
				}
				rows = nil
			}
			today := time.Now().In(engine.Beijing).Format("2006-01-02")
			for _, run := range values {
				started := run.Started.In(engine.Beijing)
				d := started.Format("2006-01-02")
				if d != key {
					flush()
					key, day = d, mobileWeekday(started.Format("1月2日 Mon"))
					if d == today {
						day = "今天 · " + day
					}
				}
				_, color := mobileStatusLook(cx, run.Status)
				rows = append(rows, mobileRow{Leading: mobileStatusIcon(cx, run.Status, 22), Title: run.Uname, Detail: run.Summary, Lines: 2, TestID: "run:" + run.ID,
					Label: run.Uname + " · " + client.StatusShort(run.Status),
					Control: core.View{Style: core.Style{AlignItems: core.AlignEnd, Gap: 2}, Children: []core.Node{
						ui.Text(started.Format("15:04"), 14, ui.Medium, colors.Text),
						ui.Text(client.TriggerText(run.Trigger)+" · "+client.StatusShort(run.Status), 12, ui.Regular, color),
					}},
					OnPress: func() { nav.Push("report:" + run.ID) }})
			}
			flush()
			return core.View{Style: core.Style{Gap: 20}, Children: sections}
		}),
	)
}

// mobileWeekday 把英文星期换成中文。
func mobileWeekday(label string) string {
	for en, zh := range map[string]string{"Mon": "周一", "Tue": "周二", "Wed": "周三", "Thu": "周四", "Fri": "周五", "Sat": "周六", "Sun": "周日"} {
		if n := len(label) - len(en); n >= 0 && label[n:] == en {
			return label[:n] + zh
		}
	}
	return label
}

func (a *mobileApp) reportPage(cx *core.Scope, id string) core.Node {
	colors := ui.ColorsOf(cx)
	report := core.NewSignalFunc(cx, api.Run{}, nil)
	loading := core.NewSignal(cx, true)
	errText := core.NewSignal(cx, "")
	cl := a.session.Client()
	load := func() {
		loading.Set(true)
		errText.Set("")
		ctx := cx.Context()
		go func() {
			v, err := cl.RunDetail(ctx, id)
			cx.Post(func() {
				loading.Set(false)
				if err != nil {
					errText.Set(err.Error())
					return
				}
				report.Set(v)
			})
		}()
	}
	cx.Post(load)
	return mobilePage(cx, load, loading.Get,
		core.Show{When: func() bool { return errText.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return ui.Textf(errText.Get, 14, ui.Medium, colors.Danger)
		}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			run := report.Get()
			if run.ID == "" {
				if loading.Get() {
					return mobileText(cx, "正在读取报告…")
				}
				return mobileText(cx, "报告暂不可用，下拉重试")
			}
			meta := run.Started.In(engine.Beijing).Format("01-02 15:04") + " · " + client.TriggerText(run.Trigger)
			if !run.Ended.IsZero() {
				meta += " · 用时 " + client.Duration(run.Ended.Sub(run.Started))
			}
			nodes := []core.Node{mobileCard(cx, core.Style{Padding: core.All(16), Gap: 8},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					mobileLabel(cx, run.Uname, 20, ui.Bold, colors.Text, 1), core.View{Style: core.Style{FlexGrow: 1}}, mobileStatusChip(cx, run.Status)}},
				mobileLabel(cx, meta, 13, ui.Regular, colors.Muted, 1),
				mobileLabel(cx, run.Summary, 15, ui.Regular, colors.Text, 0),
			)}
			for _, task := range run.Tasks {
				rows := []core.Node{mobileRow{Leading: mobileStatusIcon(cx, task.Status, 22), Title: client.StatusShort(task.Status), Detail: task.Note}}
				for _, step := range task.Steps {
					rows = append(rows, mobileStep(cx, step))
				}
				nodes = append(nodes, mobileSection(cx, task.Title, "", rows...))
			}
			return core.View{Style: core.Style{Gap: 20}, Children: nodes}
		}),
	)
}

// mobileStep 是报告里的一步；失败时显示原因和建议。
func mobileStep(cx *core.Scope, step api.Step) core.Node {
	colors := ui.ColorsOf(cx)
	texts := []core.Node{mobileLabel(cx, step.Name, 15, ui.Regular, colors.Text, 0)}
	if step.Detail != "" {
		texts = append(texts, mobileLabel(cx, step.Detail, 13, ui.Regular, colors.Muted, 0))
	}
	if step.Status == api.StatusFailed {
		reason := client.ReasonText(step.Reason)
		if step.Code != 0 {
			reason += fmt.Sprintf("（代码 %d）", step.Code)
		}
		box := []core.Node{mobileLabel(cx, "原因："+reason, 13, ui.Semibold, colors.Danger, 0)}
		if step.Hint != "" {
			box = append(box, mobileLabel(cx, "建议："+step.Hint, 14, ui.Regular, colors.Text, 0))
		}
		texts = append(texts, core.View{Style: core.Style{Gap: 4, Padding: core.Symmetric(8, 10), CornerRadius: 8, Margin: core.Edges{Top: 4}},
			StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), colors.Danger(), 0.1)} }, Children: box})
	}
	return core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignStart, Padding: core.Symmetric(10, mobileRowPad)}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 2, Left: 3}}, Children: []core.Node{mobileStatusIcon(cx, step.Status, 16)}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts},
	}}
}
