package main

import (
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
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
