//go:build !js

package main

import (
	"slices"
	"testing"

	"github.com/VOfficial404/bili-daily/api"
)

func releaseVersions(rs []api.Release) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.Version)
	}
	return out
}

func TestCheckUpdates(t *testing.T) {
	// 服务端是 v0.2.0：Newer 相对服务端
	releases := []api.Release{
		{Version: "v0.3.0", Newer: true},
		{Version: "v0.2.0"},
		{Version: "v0.1.2"},
	}
	// 本机运行时服务端就是客户端自己
	local := []api.Release{{Version: "v0.3.0", Newer: true}, {Version: "v0.2.0", Newer: true}, {Version: "v0.1.2"}}
	for _, tc := range []struct {
		name             string
		st               *api.State
		appVersion       string
		app, server, all []string
		status           []string
	}{
		{name: "没有连接", st: nil, appVersion: "0.1.2"},
		{name: "没查到版本", st: &api.State{Version: "v0.2.0"}, appVersion: "0.1.2"},
		{
			name: "客户端和服务端都落后", st: &api.State{Version: "v0.2.0", Releases: releases}, appVersion: "0.1.2",
			app: []string{"v0.3.0", "v0.2.0"}, server: []string{"v0.3.0"}, all: []string{"v0.3.0", "v0.2.0"},
			status: []string{"客户端有新版本 v0.3.0", "服务端有新版本 v0.3.0"},
		},
		{
			name: "只有服务端落后", st: &api.State{Version: "v0.2.0", Releases: releases}, appVersion: "0.3.0",
			server: []string{"v0.3.0"}, all: []string{"v0.3.0"},
			status: []string{"客户端已是最新版本", "服务端有新版本 v0.3.0"},
		},
		{
			name: "只有客户端落后", st: &api.State{Version: "v0.3.0", Releases: []api.Release{{Version: "v0.3.0"}, {Version: "v0.2.0"}}}, appVersion: "0.2.0",
			app: []string{"v0.3.0"}, all: []string{"v0.3.0"},
			status: []string{"客户端有新版本 v0.3.0", "服务端已是最新版本"},
		},
		{
			name: "都是最新", st: &api.State{Version: "v0.3.0", Releases: []api.Release{{Version: "v0.3.0"}}}, appVersion: "0.3.0",
			status: []string{"客户端已是最新版本", "服务端已是最新版本"},
		},
		{
			name: "本机运行不提示服务端", st: &api.State{Local: true, Version: "0.1.2", Releases: local}, appVersion: "0.1.2",
			app: []string{"v0.3.0", "v0.2.0"}, all: []string{"v0.3.0", "v0.2.0"},
			status: []string{"客户端有新版本 v0.3.0"},
		},
		{
			name: "认不出客户端版本", st: &api.State{Version: "v0.2.0", Releases: releases}, appVersion: "dev",
			server: []string{"v0.3.0"}, all: []string{"v0.3.0"},
			status: []string{"服务端有新版本 v0.3.0"},
		},
		{
			name: "认不出服务端版本", st: &api.State{Version: "latest", Releases: []api.Release{{Version: "v0.3.0"}}}, appVersion: "0.3.0",
			status: []string{"客户端已是最新版本"},
		},
		{name: "都认不出", st: &api.State{Local: true, Version: "dev", Releases: []api.Release{{Version: "v0.3.0"}}}, appVersion: "dev"},
	} {
		u := checkUpdates(tc.st, tc.appVersion)
		for _, got := range []struct {
			what      string
			got, want []string
		}{
			{"客户端", releaseVersions(u.app), tc.app},
			{"服务端", releaseVersions(u.server), tc.server},
			{"合并", releaseVersions(u.all), tc.all},
			{"状态", u.status(), tc.status},
		} {
			if !slices.Equal(got.got, got.want) {
				t.Errorf("%s：%s = %q，应为 %q", tc.name, got.what, got.got, got.want)
			}
		}
	}
}

func TestReleaseNotes(t *testing.T) {
	got := releaseNotes(api.Release{Notes: "- 修了登录\r\n\n* 加了提示\n其他"})
	if want := []string{"· 修了登录", "· 加了提示", "其他"}; !slices.Equal(got, want) {
		t.Fatalf("更新说明 = %q，应为 %q", got, want)
	}
	if got := releaseNotes(api.Release{Notes: " \n"}); !slices.Equal(got, []string{"这个版本没有写更新说明，详见发布页"}) {
		t.Fatalf("没写说明时 = %q", got)
	}
}

func TestUpdateNotice(t *testing.T) {
	e, _, base := newTestEngine(t)
	h := newHarness(t, &memStore{dir: t.TempDir()})
	var opened []string
	h.a.openURL = func(url string) error { opened = append(opened, url); return nil }
	h.connect(base, e.Token())
	if h.tt.HasText("有新版本") {
		t.Fatal("没查到版本时出现了更新提示")
	}

	h.a.st.Version = "v9.0.0"
	h.a.st.UpdateHint = "docker compose pull && docker compose up -d"
	h.a.st.Releases = []api.Release{
		{Version: "v9.1.0", URL: "https://github.com/VOfficial404/bili-daily/releases/tag/v9.1.0", Notes: "- 新的任务", Newer: true},
		{Version: "v9.0.0", URL: "https://github.com/VOfficial404/bili-daily/releases/tag/v9.0.0"},
	}
	h.tt.Frame()
	h.must("有新版本 v9.1.0")
	h.must("服务端有新版本 v9.1.0")
	h.must("docker compose pull && docker compose up -d")
	h.must("客户端有新版本 v9.1.0")
	h.click("更新了什么")
	h.must("· 新的任务")
	h.must("这个版本没有写更新说明，详见发布页")
	h.click("去下载")
	if !slices.Equal(opened, []string{"https://github.com/VOfficial404/bili-daily/releases/tag/v9.1.0"}) {
		t.Fatalf("打开的网址 = %q", opened)
	}

	// 不是 GitHub 的地址不打开
	h.a.openRelease("https://example.com/releases")
	h.a.openRelease("http://github.com/VOfficial404/bili-daily")
	h.settle()
	if len(opened) != 1 {
		t.Fatalf("打开了不该打开的网址：%q", opened)
	}
}
