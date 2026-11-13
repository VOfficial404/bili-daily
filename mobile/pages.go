package main

import (
	"fmt"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func (a *mobileApp) qrPage(cx *core.Scope) core.Node {
	qr := core.NewSignalFunc(cx, api.QR{}, nil)
	loading := core.NewSignal(cx, false)
	errText := core.NewSignal(cx, "")
	client := a.client
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
				v, err := client.PollQR(ctx, id)
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
						a.nav.Pop(1)
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
			v, err := client.StartQR(ctx)
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
	return mobileColumn(
		mobileText(cx, "用 B 站 App 扫码并确认登录。也可以打开链接；无法打开时，截图后在 B 站扫一扫中从相册选择。"),
		ui.Textf(errText.Get, 14, ui.Medium, ui.ColorsOf(cx).Danger),
		core.Dynamic(func(cx *core.Scope) core.Node {
			q := qr.Get()
			if loading.Get() {
				return mobileText(cx, "正在获取二维码…")
			}
			if len(q.PNG) == 0 {
				return mobileText(cx, "二维码暂不可用")
			}
			status := "等待扫码"
			switch q.State {
			case "scanned":
				status = "已扫码，请在 B 站确认"
			case "expired":
				status = "二维码已过期"
			case "success":
				status = "登录成功"
			}
			return core.View{Style: core.Style{Gap: 14}, Children: []core.Node{
				core.Image{Source: core.ImageSource{Data: q.PNG, Size: core.Size{Width: 240, Height: 240}}, ResizeMode: core.Contain, Style: core.Style{Width: core.Pt(240), Height: core.Pt(240)}},
				mobileText(cx, status),
				ui.Button{Text: "在 B 站 App 里打开", Disabled: func() bool { return q.State == "expired" || q.URL == "" }, OnPress: func() {
					cx.OpenURL(q.URL, func(opened bool) {
						if !opened {
							errText.Set("无法打开链接。请截图，在 B 站扫一扫中从相册选择二维码。")
						}
					})
				}},
				ui.Button{Text: "复制链接", Disabled: func() bool { return q.URL == "" }, OnPress: func() { cx.SetClipboard(q.URL); a.message("链接已复制") }},
			}}
		}),
		ui.Button{Text: "重新获取二维码", OnPress: request, Disabled: loading.Get},
	)
}

func (a *mobileApp) reportsPage(cx *core.Scope) core.Node {
	filter := core.NewSignal(cx, "")
	runs := core.NewSignalFunc(cx, []api.RunSummary{}, nil)
	loading := core.NewSignal(cx, false)
	errText := core.NewSignal(cx, "")
	client := a.client
	generation := 0
	load := func(mid string) {
		generation++
		g := generation
		loading.Set(true)
		errText.Set("")
		ctx := cx.Context()
		go func() {
			v, err := client.Runs(ctx, mid, 60)
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
	return mobileColumn(
		ui.Select[string]{Label: "筛选账号", Options: func() []ui.Option[string] {
			opts := []ui.Option[string]{{Value: "", Label: "全部账号"}}
			for _, acc := range a.state.GetUntracked().Accounts {
				opts = append(opts, ui.Option[string]{Value: acc.Mid, Label: acc.Uname})
			}
			return opts
		}(), Value: filter.Get, OnChange: func(mid string) { filter.Set(mid); load(mid) }, Presentation: base.PresentSheet},
		ui.Button{Text: "刷新报告", Disabled: loading.Get, OnPress: func() { load(filter.Get()) }},
		ui.Textf(errText.Get, 14, ui.Medium, ui.ColorsOf(cx).Danger),
		core.Dynamic(func(cx *core.Scope) core.Node {
			values := runs.Get()
			if loading.Get() && len(values) == 0 {
				return mobileText(cx, "正在读取报告…")
			}
			if len(values) == 0 {
				return mobileText(cx, "还没有报告。运行任务后可在这里查看每一步的结果。")
			}
			rows := make([]core.Node, 0, len(values))
			for _, run := range values {
				rows = append(rows, ui.FormRow{Title: fmt.Sprintf("%s · %s", run.Uname, mobileStatus(run.Status)), Detail: run.Started.Format("01-02 15:04") + "\n" + run.Summary, OnPress: func() { a.nav.Push("report:" + run.ID) }})
			}
			return ui.FormSection{Title: "最近运行", Children: rows}
		}),
	)
}
func (a *mobileApp) reportPage(cx *core.Scope, id string) core.Node {
	report := core.NewSignalFunc(cx, api.Run{}, nil)
	loading := core.NewSignal(cx, true)
	errText := core.NewSignal(cx, "")
	client := a.client
	load := func() {
		loading.Set(true)
		errText.Set("")
		ctx := cx.Context()
		go func() {
			v, err := client.RunDetail(ctx, id)
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
	return mobileColumn(ui.Textf(errText.Get, 14, ui.Medium, ui.ColorsOf(cx).Danger), ui.Button{Text: "刷新报告", OnPress: load, Disabled: loading.Get}, core.Dynamic(func(cx *core.Scope) core.Node {
		if loading.Get() {
			return mobileText(cx, "正在读取报告…")
		}
		run := report.Get()
		if run.ID == "" {
			return mobileText(cx, "报告暂不可用，请重试")
		}
		rows := []core.Node{mobileText(cx, run.Uname+"\n"+mobileStatus(run.Status)+"\n"+run.Summary)}
		for _, task := range run.Tasks {
			children := []core.Node{mobileText(cx, mobileStatus(task.Status)+"\n"+task.Note)}
			for _, step := range task.Steps {
				children = append(children, mobileText(cx, mobileStatus(step.Status)+"  "+step.Name))
				if step.Detail != "" {
					children = append(children, mobileText(cx, step.Detail))
				}
				if step.Status == api.StatusFailed && step.Hint != "" {
					children = append(children, ui.Text("建议："+step.Hint, 15, ui.Medium, ui.ColorsOf(cx).Danger))
				}
			}
			rows = append(rows, ui.FormSection{Title: task.Title, Children: children})
		}
		return core.View{Style: core.Style{Gap: 16}, Children: rows}
	}))
}
