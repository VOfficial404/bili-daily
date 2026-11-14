package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

const mobileVersion = "0.1.1"

type mobileApp struct {
	cx                          *core.Scope
	ctx                         context.Context
	session                     *client.Session
	stopSessionRoot             func() bool
	state                       *core.Signal[api.State]
	connected, busy, refreshing *core.Signal[bool]
	errorText                   *core.Signal[string]
	tab                         *core.Signal[int]
	navs                        []*base.Nav[string] // 每个标签页一个导航栈
	toaster                     *ui.Toaster
	epoch                       int
	active                      bool
	stopPoll                    func() bool
	cancel                      context.CancelFunc
	refreshPending              bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	cx.SetTheme(mobileTheme())
	cx.SetAppearance(core.AppearanceSystem)
	a := &mobileApp{cx: cx, session: &client.Session{}, state: core.NewSignalFunc(cx, api.State{}, nil), connected: core.NewSignal(cx, false), busy: core.NewSignal(cx, false), refreshing: core.NewSignal(cx, false), errorText: core.NewSignal(cx, ""), tab: core.NewSignal(cx, 0), toaster: ui.NewToaster(cx), active: true}
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
			if local := a.session.LocalEngine(); local != nil {
				local.Wake()
			}
			a.refresh()
		}
	})
	return core.View{TestID: "mobile-root", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, StyleFunc: func() core.Style { return core.Style{Background: cx.Palette().Background} }, Children: []core.Node{
		core.Show{When: a.connected.Get, Then: func(cx *core.Scope) core.Node { return a.shell(cx) }, Else: func(cx *core.Scope) core.Node { return a.connectPage(cx) }},
		ui.Toasts{Toaster: a.toaster, Placement: ui.ToastTop},
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
	a.epoch++
	if a.stopPoll != nil {
		a.stopPoll()
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.stopSessionRoot != nil {
		a.stopSessionRoot()
		a.stopSessionRoot = nil
	}
	if a.session != nil {
		a.session.Stop()
	}
}
func (a *mobileApp) disconnect() {
	a.stop()
	a.connected.Set(false)
	a.busy.Set(false)
	a.refreshing.Set(false)
	a.refreshPending = false
	a.errorText.Set("")
	a.navs = nil
	a.tab.Set(0)
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
	session, epoch := a.session, a.epoch
	go func() {
		err := work(ctx)
		cancel()
		a.cx.Post(func() {
			if epoch != a.epoch || session != a.session {
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
	if a.isBusy() {
		return
	}
	a.stop()
	session := &client.Session{}
	a.session = session
	a.stopSessionRoot = context.AfterFunc(a.ctx, session.Stop)
	var state api.State
	a.perform("已连接", func(ctx context.Context) error {
		var err error
		state, err = session.Connect(ctx, baseURL, token)
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
		a.state.Set(state)
		a.connected.Set(true)
		a.schedulePoll()
	})
}
func (a *mobileApp) startLocal(dir string, proxy *string) {
	if a.isBusy() {
		return
	}
	a.stop()
	session := &client.Session{}
	a.session = session
	a.stopSessionRoot = context.AfterFunc(a.ctx, session.Stop)
	var state api.State
	a.perform("本机运行已启动", func(ctx context.Context) error {
		var err error
		state, err = session.StartLocal(ctx, client.LocalOptions{Config: engine.Config{DataDir: dir, Version: mobileVersion}, Proxy: proxy})
		return err
	}, func() {
		a.state.Set(state)
		if err := a.cx.Storage().SetItem("local-proxy", state.Settings.Proxy); err != nil {
			a.fail(err)
		}
		a.connected.Set(true)
		a.schedulePoll()
	})
}
func (a *mobileApp) schedulePoll() {
	// Scope.Child and AfterFunc use the current owner, so enter the root first,
	// then the timer's child. Disposing that child releases its timer cleanup.
	a.cx.Post(func() {
		if a.stopPoll != nil {
			a.stopPoll()
			a.stopPoll = nil
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
		a.stopPoll = func() bool { owner.Dispose(); return true }
		owner.Post(func() { owner.AfterFunc(delay, a.refresh) })
	})
}
func (a *mobileApp) refresh() {
	if !a.active || !a.connected.GetUntracked() {
		return
	}
	if a.refreshing.GetUntracked() {
		a.refreshPending = true
		return
	}
	a.refreshing.Set(true)
	session, epoch := a.session, a.epoch
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	go func() {
		_, err := session.Refresh(ctx)
		cancel()
		a.cx.Post(func() {
			if epoch != a.epoch || session != a.session {
				return
			}
			a.refreshing.Set(false)
			if err != nil {
				a.errorText.Set(err.Error())
			} else {
				a.state.Set(session.Snapshot())
				a.errorText.Set("")
			}
			if a.refreshPending {
				a.refreshPending = false
				a.refresh()
			} else {
				a.schedulePoll()
			}
		})
	}()
}

// saveSettings 让各设置表单共享保存流程，保存并运行也走这里。
func (a *mobileApp) saveSettings(cx *core.Scope, base, edited api.Settings, done func(api.Settings)) {
	if a.isBusy() {
		return
	}
	session, epoch := a.session, a.epoch
	base, edited = client.CloneSettings(base), client.CloneSettings(edited)
	save := func() {
		if session != a.session || epoch != a.epoch {
			return
		}
		var saved api.Settings
		a.perform("设置已保存", func(ctx context.Context) error {
			var err error
			saved, err = session.SaveSettings(ctx, base, edited)
			return err
		}, func() {
			a.state.Set(session.Snapshot())
			if session.LocalEngine() != nil {
				if err := a.cx.Storage().SetItem("local-proxy", saved.Proxy); err != nil {
					a.fail(err)
				}
			}
			if done != nil {
				done(saved)
			}
			a.refresh()
		})
	}
	if client.NeedsChargeConfirm(base, edited) {
		cx.ShowAlert(core.AlertConfig{Title: "确认自动充电？", Message: fmt.Sprintf("充电 UP 主 UID：%d\n启用后，每月会将全部 B 币券余额充电给该 UP 主，请确认 UID 正确。", edited.Tasks.Charge.UpID), Actions: []core.AlertAction{{Label: "取消", Role: core.AlertCancel}, {Label: "确认并保存", Role: core.AlertDestructive}}}, func(index int) {
			if index == 1 {
				save()
			}
		})
		return
	}
	save()
}
