package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/egoist/mygo/ui"
)

// 资料按 UID 缓存；输入、折叠选项和状态轮询都不会触发网络请求。
type upProfilePreview struct {
	profile api.UPProfile
	loading bool
	err     string
}

func (a *app) loadUPProfile(mid string) {
	n, err := strconv.ParseInt(mid, 10, 64)
	if err != nil || n <= 0 || a.st == nil || a.session.Client() == nil {
		return
	}
	mid = strconv.FormatInt(n, 10)
	if a.upProfiles == nil {
		a.upProfiles = make(map[string]*upProfilePreview)
	}
	if current := a.upProfiles[mid]; current != nil && current.loading {
		return
	}
	preview := &upProfilePreview{loading: true}
	a.upProfiles[mid] = preview
	session, ctx, epoch := a.session, a.ctx, a.epoch
	cl := session.Client()
	a.async(func() func() {
		profile, err := cl.UPProfile(ctx, mid)
		return func() {
			if epoch != a.epoch || session != a.session || a.upProfiles[mid] != preview {
				return
			}
			preview.loading = false
			if err != nil {
				preview.err = "资料暂不可用：" + err.Error()
				return
			}
			if profile.Mid != mid || strings.TrimSpace(profile.Uname) == "" {
				preview.err = "资料暂不可用，请稍后重试。"
				return
			}
			preview.profile = profile
			a.loadFace(profile.Face)
		}
	})
}

// preloadSavedUPProfiles 在连接和保存成功后查一次已保存的 UID；已有结果或失败记录的不再自动查。
func (a *app) preloadSavedUPProfiles() {
	ids := append([]int64{a.formBase.Tasks.Charge.UpID}, a.formBase.Tasks.Daily.UpIDs...)
	for _, id := range ids {
		if mid := strconv.FormatInt(id, 10); id > 0 && a.upProfiles[mid] == nil {
			a.loadUPProfile(mid)
		}
	}
}

func (a *app) settingsUPPreviews(c *ui.Context, key, raw string) {
	ids, err := settingsIDs(raw)
	if err != "" || len(ids) == 0 {
		return
	}
	ui.Field(c, "UP 主资料", func() {
		ui.Column(c).Gap(8).Children(func() {
			for i, id := range ids {
				mid := strconv.FormatInt(id, 10)
				preview := a.upProfiles[mid]
				profile := api.UPProfile{Mid: mid}
				status, button := "点击获取头像和昵称，不影响保存 UID。", "获取资料"
				loading := false
				if preview != nil {
					loading = preview.loading
					switch {
					case loading:
						status, button = "正在获取资料…", "正在获取"
					case preview.err != "":
						status, button = preview.err+" 可直接保存 UID。", "重试"
					default:
						profile = preview.profile
						status, button = "", "刷新资料"
					}
				}
				ui.Row(c.Key(fmt.Sprintf("up-preview-%s-%d-%s", key, i, mid))).Gap(10).Children(func() {
					a.avatar(c, api.Account{Mid: mid, Uname: profile.Uname, Face: profile.Face}, 36)
					ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
						if profile.Uname != "" {
							ui.Text(c, profile.Uname).Bold()
						}
						muted(c, "UID "+mid).FontSize(12).Selectable()
						if status != "" {
							muted(c, status).FontSize(12)
						}
					})
					ui.Button(c.Key("refresh"), button).Disabled(loading).OnClick(func() { a.loadUPProfile(mid) })
				})
			}
		})
	})
}
