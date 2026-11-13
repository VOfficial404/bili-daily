package main

import (
	_ "embed"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/tray.png
var trayIcon []byte

func main() {
	var win *mygo.Window
	var tray *mygo.Tray
	quitting := false
	a := newApp(systemStore{}, func(fn func()) { win.Update(fn) })
	a.onLocal = func(running bool) {
		if running && tray == nil {
			t, err := mygo.NewTray(mygo.TrayOptions{ID: "main", Icon: trayIcon, IconIsTemplate: true, ToolTip: "Bili Tools：本机运行中", Menu: mygo.NewMenu([]*mygo.MenuItem{
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
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{Title: "Bili Tools", Width: 1120, Height: 800, MinWidth: 420, MinHeight: 560, StateKey: "main", Content: ui.View(a.view)})
		win.OnClose(func(e *mygo.CloseEvent) {
			if !quitting && a.localEngine != nil && tray != nil {
				e.PreventDefault()
				win.Hide()
			}
		})
		a.autoConnect()
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for range ticker.C {
				win.Update(a.tick)
			}
		}()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	mygo.App.OnQuit(func() {
		a.cancel()
		if a.localEngine != nil {
			a.localEngine.Stop()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
