//go:build !ios && !android

package main

import (
	_ "embed"

	"github.com/egoist/mygo"
)

// 桌面入口：本机运行时留在托盘里，关闭窗口只是隐藏。手机入口见 platform_mobile.go。

//go:embed resources/tray.png
var trayIcon []byte

// beforeRun 在 App.Run 之前登记的东西；桌面没有
func beforeRun(*app) {}

func configurePlatform(a *app, win *mygo.Window) {
	var tray *mygo.Tray
	quitting := false
	a.openURL = mygo.Shell.OpenExternal
	a.onLocal = func(running bool) {
		if running && tray == nil {
			t, err := mygo.NewTray(mygo.TrayOptions{ID: "main", Icon: trayIcon, IconIsTemplate: true, ToolTip: "Bili Daily：本机运行中", Menu: mygo.NewMenu([]*mygo.MenuItem{
				{Label: "打开", Click: func(*mygo.MenuItem, *mygo.Window) { win.Show(); win.Focus() }},
				{Label: "全部立即运行", Click: func(*mygo.MenuItem, *mygo.Window) { win.Update(a.runAll) }},
				mygo.Separator(), {Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
			})})
			if err != nil {
				a.toast("托盘不可用，关闭窗口将退出应用：" + err.Error())
				return
			}
			tray = t
		} else if !running && tray != nil {
			tray.Destroy()
			tray = nil
		}
	}
	win.OnClose(func(e *mygo.CloseEvent) {
		if !quitting && a.session.LocalEngine() != nil && tray != nil {
			e.PreventDefault()
			win.Hide()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
}
