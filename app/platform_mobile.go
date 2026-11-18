//go:build ios || android

package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/VOfficial404/bili-daily/engine"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/background"
)

// 手机入口。系统会暂停后台的 App，本机运行时登记一个后台任务：系统给出时间时补上到点的那一轮。
// 什么时候给由系统决定（iOS 很少给），准时运行要连服务器；回到前台时也会补跑。
const backgroundTask = "daily"

// beforeRun 在 App.Run 之前登记后台任务的处理函数：系统可能只为它启动进程，不打开界面
func beforeRun(a *app) {
	background.Handle(backgroundTask, func(ctx context.Context) error { return settleLocal(ctx, a) })
}

// settleLocal 等本机引擎启动（选了「下次打开时自动开始」时 autoConnect 会启动它），再补上到点的那一轮
func settleLocal(ctx context.Context, a *app) error {
	if p := a.store.load(); p.Mode != 1 || !p.AutoLocal {
		return nil
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		var local *engine.Engine
		failed := ""
		mygo.RunOnMain(func() {
			local = a.session.LocalEngine()
			if !a.connecting {
				failed = a.connectErr
			}
		})
		if local != nil {
			return local.Settle(ctx)
		}
		if failed != "" {
			return errors.New(failed)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func configurePlatform(a *app, win *mygo.Window) {
	a.mobile = true
	migrateLegacy(a)
	a.openURL = mygo.Shell.OpenExternal
	a.copyText = mygo.Clipboard.WriteText
	a.onLocal = func(running bool) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var err error
			if running {
				err = background.Schedule(ctx, backgroundTask, background.Options{Every: time.Hour, Network: true})
			} else {
				err = background.Cancel(ctx, backgroundTask)
			}
			if err != nil {
				log.Printf("后台任务：%v", err)
			}
		}()
	}
	// 回到前台：睡眠时错过的那一轮立即补上，不等计时器
	mygo.Power.OnResume(func() { win.Update(a.resume) })
}

// nsgoID 是 NSGo 版的应用标识；iOS 上它的数据在 Application Support 下以它命名的目录里
const nsgoID = "io.github.vofficial404.bilidaily"

// migrateLegacy 在覆盖安装 NSGo 版之后的第一次启动时搬来它的数据，再重新读连接偏好。
// NSGo 的数据目录：Android 是 files/（MyGo 的数据目录是它下面的 files/Bili Daily），
// iOS 是 Application Support/<nsgoID>。
func migrateLegacy(a *app) {
	root, err := mygo.App.Path(mygo.PathAppData)
	if err != nil {
		log.Printf("搬旧版数据：%v", err)
		return
	}
	legacy := root
	if runtime.GOOS == "ios" {
		legacy = filepath.Join(root, nsgoID)
	}
	user, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		var moved bool
		if moved, err = migrateNSGo(legacy, user, a.store.setToken); moved {
			a.loadPrefs()
			a.toast("已从旧版搬来连接信息和本机数据")
		}
	}
	if err != nil {
		log.Printf("搬旧版数据：%v", err)
		a.toast("搬旧版数据失败：" + err.Error())
	}
}

func (a *app) resume() {
	if local := a.session.LocalEngine(); local != nil {
		local.Wake()
	}
	a.poll()
}
