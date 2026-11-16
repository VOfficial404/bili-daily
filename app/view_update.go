package main

import (
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/egoist/mygo/ui"
)

// updateInfo 是版本检查的结论：客户端和服务端各自落后了哪些版本
type updateInfo struct {
	app, server []api.Release // 比客户端、比服务端新的版本，从新到旧
	all         []api.Release // 上面两组合在一起，同一个版本只出现一次
	// 能不能说「已是最新」：查到了版本列表，而且自己的版本号认得出来
	appKnown, serverKnown bool
}

// versionKnown 判断版本号认不认得出来；dev、latest 这类比不了新旧
func versionKnown(v string) bool { return api.NewerVersion(v, "0.0.0-0") }

// checkUpdates 拿服务端查到的版本列表和客户端、服务端的版本比。
// 本机运行时服务端就是客户端自己，只看客户端。
func checkUpdates(st *api.State, appVersion string) updateInfo {
	var u updateInfo
	if st == nil || len(st.Releases) == 0 {
		return u
	}
	u.appKnown = versionKnown(appVersion)
	u.serverKnown = !st.Local && versionKnown(st.Version)
	for _, r := range st.Releases {
		forApp := api.NewerVersion(r.Version, appVersion)
		forServer := !st.Local && r.Newer
		if forApp {
			u.app = append(u.app, r)
		}
		if forServer {
			u.server = append(u.server, r)
		}
		if forApp || forServer {
			u.all = append(u.all, r)
		}
	}
	return u
}

// status 是设置页和侧边栏里的一两行：是不是最新版本。不知道时不写
func (u updateInfo) status() []string {
	line := func(who string, newer []api.Release) string {
		if len(newer) > 0 {
			return who + "有新版本 " + newer[0].Version
		}
		return who + "已是最新版本"
	}
	var out []string
	if u.appKnown {
		out = append(out, line("客户端", u.app))
	}
	if u.serverKnown {
		out = append(out, line("服务端", u.server))
	}
	return out
}

// releaseNotes 把更新说明拆成一行一行的纯文本
func releaseNotes(r api.Release) []string {
	var out []string
	for _, l := range strings.Split(r.Notes, "\n") {
		l = strings.TrimSpace(l)
		if s, ok := strings.CutPrefix(l, "- "); ok {
			l = "· " + s
		} else if s, ok := strings.CutPrefix(l, "* "); ok {
			l = "· " + s
		}
		if l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return []string{"这个版本没有写更新说明，详见发布页"}
	}
	return out
}

func isReleaseURL(url string) bool { return strings.HasPrefix(url, "https://github.com/") }

// openRelease 用系统浏览器打开发布页；只打开 GitHub 上的地址
func (a *app) openRelease(url string) {
	open := a.openURL
	if open == nil || !isReleaseURL(url) {
		return
	}
	// 系统打开浏览器可能要等一会儿，不放在界面线程里
	a.async(func() func() {
		if err := open(url); err != nil {
			return func() { a.toast("打不开浏览器：" + err.Error()) }
		}
		return nil
	})
}

// updateNotice 是「今天」页顶部的更新提示，没有新版本时不占位置
func (a *app) updateNotice(c *ui.Context) {
	u := checkUpdates(a.st, version)
	if len(u.all) == 0 {
		return
	}
	panel(c.Key("update")).Padding(16, 20).Gap(10).Children(func() {
		if len(u.app) > 0 {
			r := u.app[0]
			ui.Row(c).Gap(12).Children(func() {
				ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, "有新版本 "+r.Version).FontSize(15).Bold()
					muted(c, "这台电脑上是 "+version+"，更新后才有新功能和修复。").FontSize(12)
				})
				if isReleaseURL(r.URL) {
					ui.PrimaryButton(c.Key("update-download"), "去下载").OnClick(func() { a.openRelease(r.URL) })
				}
			})
		}
		if len(u.server) > 0 {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "服务端有新版本 "+u.server[0].Version).FontSize(15).Bold()
				muted(c, "服务端现在是 "+a.st.Version+"，要在服务器上更新。").FontSize(12)
				if a.st.UpdateHint != "" {
					ui.Text(c, a.st.UpdateHint).FontSize(13).Selectable()
				}
			})
		}
		ui.Collapsible(c.Key("update-notes"), "更新了什么", &a.updateOpen, func() {
			ui.Column(c).Gap(12).Padding(4, 0, 0).Children(func() {
				for _, r := range u.all {
					ui.Column(c.Key("release-" + r.Version)).Gap(4).Children(func() {
						ui.Row(c).Gap(8).Children(func() {
							ui.Text(c, r.Version).FontSize(14).Bold()
							if !r.PublishedAt.IsZero() {
								muted(c, r.PublishedAt.In(engine.Beijing).Format("2006-01-02")).FontSize(12)
							}
							ui.Spacer(c)
							if isReleaseURL(r.URL) {
								ui.Button(c.Key("release-page-"+r.Version), "发布页").OnClick(func() { a.openRelease(r.URL) })
							}
						})
						for _, l := range releaseNotes(r) {
							ui.Text(c, l).FontSize(13).Selectable()
						}
					})
				}
			})
		})
	})
}
