package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	var win *mygo.Window
	a := newApp(systemStore{}, func(fn func()) { win.Update(fn) })
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{Title: "Bili Daily", Width: 1120, Height: 800, MinWidth: 420, MinHeight: 560, StateKey: "main", Content: ui.View(a.view)})
		configurePlatform(a, win)
		a.autoConnect()
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for range ticker.C {
				win.Update(a.tick)
			}
		}()
	})
	mygo.App.OnQuit(func() {
		a.cancel()
		a.session.Stop()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
