package main

import (
	"net/url"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type mobileReleaseUpdate struct {
	Release     api.Release
	App, Server bool
}

type mobileUpdateInfo struct {
	AppVersion, ServerVersion string
	Releases                  []mobileReleaseUpdate
}

func mobileUpdates(state api.State, appVersion string) mobileUpdateInfo {
	var info mobileUpdateInfo
	for _, release := range state.Releases {
		app := api.NewerVersion(release.Version, appVersion)
		server := !state.Local && release.Newer
		if !app && !server {
			continue
		}
		if app && info.AppVersion == "" {
			info.AppVersion = release.Version
		}
		if server && info.ServerVersion == "" {
			info.ServerVersion = release.Version
		}
		info.Releases = append(info.Releases, mobileReleaseUpdate{Release: release, App: app, Server: server})
	}
	return info
}

func mobileVersionSummary(state api.State) string {
	text := "App " + mobileVersion + " · 服务端 " + state.Version
	if len(state.Releases) == 0 {
		return text
	}
	info := mobileUpdates(state, mobileVersion)
	status := func(version string) string {
		if version != "" {
			return "有新版本 " + version
		}
		return "已是最新版本"
	}
	text += "\nApp：" + status(info.AppVersion)
	if !state.Local {
		text += "\n服务端：" + status(info.ServerVersion)
	}
	return text
}

func mobileReleaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || !strings.HasPrefix(u.Path, "/") {
		return ""
	}
	return raw
}

func mobileReleaseNotes(notes string) string {
	if strings.TrimSpace(notes) == "" {
		return "这个版本没有写更新说明，详见发布页"
	}
	return notes
}

func (a *mobileApp) updateNotice(cx *core.Scope, state api.State, open *core.Signal[bool]) core.Node {
	info := mobileUpdates(state, mobileVersion)
	if len(info.Releases) == 0 {
		return nil
	}
	colors := ui.ColorsOf(cx)
	var titles []string
	var heading []core.Node
	if info.AppVersion != "" {
		titles = append(titles, "有新版本 "+info.AppVersion)
	}
	if info.ServerVersion != "" {
		titles = append(titles, "服务端有新版本 "+info.ServerVersion)
	}
	for _, title := range titles {
		heading = append(heading, mobileLabel(cx, title, 16, ui.Semibold, colors.Text, 0))
	}
	heading = append(heading, mobileLabel(cx, "点开查看更新说明。", 13, ui.Regular, colors.Muted, 0))
	var details []core.Node
	var downloadURL string
	for _, update := range info.Releases {
		target := "App、服务端"
		if !update.Server {
			target = "App"
		} else if !update.App {
			target = "服务端"
		}
		details = append(details, core.View{Style: core.Style{Gap: 6}, Selectable: true, Children: []core.Node{
			mobileLabel(cx, update.Release.Version+" · "+target, 14, ui.Semibold, colors.Text, 0),
			mobileLabel(cx, mobileReleaseNotes(update.Release.Notes), 14, ui.Regular, colors.Text, 0),
		}})
		if update.App && update.Release.Version == info.AppVersion {
			downloadURL = mobileReleaseURL(update.Release.URL)
		}
	}
	if info.ServerVersion != "" && state.UpdateHint != "" {
		hint := mobileLabel(cx, state.UpdateHint, 14, ui.Regular, colors.Text, 0)
		hint.Selectable = core.SelectableOn
		details = append(details,
			mobileLabel(cx, "更新服务端", 14, ui.Semibold, colors.Text, 0), hint,
			mobileButton{Text: "复制更新方法", Icon: lucide.Copy, Kind: buttonSecondary, OnPress: func() {
				cx.SetClipboard(state.UpdateHint)
				a.message("更新方法已复制")
			}},
		)
	}
	if info.AppVersion != "" {
		details = append(details, mobileButton{Text: "去下载", Icon: lucide.ExternalLink, Kind: buttonTonal,
			Disabled: func() bool { return downloadURL == "" }, OnPress: func() {
				if downloadURL == "" {
					return
				}
				cx.OpenURL(downloadURL, func(opened bool) {
					if !opened {
						a.message("无法打开发布页，请稍后重试")
					}
				})
			}})
	}
	return mobileCard(cx, core.Style{}, base.Collapsible{Open: open.Get,
		Children: []core.Node{base.Button{Label: strings.Join(titles, "，") + "，查看更新说明", Expanded: open.Get,
			OnPress:   func(core.PressEvent) { open.Set(!open.GetUntracked()) },
			Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.All(16)},
			StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} },
			Children: []core.Node{
				ui.Icon(lucide.Download, 20, colors.Accent),
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 4}, Children: heading},
				core.Dynamic(func(cx *core.Scope) core.Node {
					icon := lucide.ChevronDown
					if open.Get() {
						icon = lucide.ChevronUp
					}
					return ui.Icon(icon, 18, colors.Faint)
				}),
			}}},
		Content: core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Left: 16, Right: 16, Bottom: 16}}, Children: details},
	})
}
