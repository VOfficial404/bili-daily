package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/VOfficial404/bili-daily/api"
)

func TestMobileUpdates(t *testing.T) {
	releases := []api.Release{
		{Version: "v0.4.0", Newer: true},
		{Version: "v0.3.0", Newer: false},
		{Version: "v0.2.0", Newer: false},
	}
	tests := []struct {
		name       string
		state      api.State
		appVersion string
		want       mobileUpdateInfo
	}{
		{
			name:  "app older than server",
			state: api.State{Version: "0.3.0", Releases: releases}, appVersion: "0.2.0",
			want: mobileUpdateInfo{AppVersion: "v0.4.0", ServerVersion: "v0.4.0", Releases: []mobileReleaseUpdate{
				{Release: releases[0], App: true, Server: true},
				{Release: releases[1], App: true},
			}},
		},
		{
			name:  "only server outdated",
			state: api.State{Version: "0.3.0", Releases: releases}, appVersion: "0.4.0",
			want: mobileUpdateInfo{ServerVersion: "v0.4.0", Releases: []mobileReleaseUpdate{
				{Release: releases[0], Server: true},
			}},
		},
		{
			name:  "only app outdated",
			state: api.State{Version: "0.4.0", Releases: []api.Release{{Version: "v0.4.0"}}}, appVersion: "0.3.0",
			want: mobileUpdateInfo{AppVersion: "v0.4.0", Releases: []mobileReleaseUpdate{
				{Release: api.Release{Version: "v0.4.0"}, App: true},
			}},
		},
		{
			name:  "local suppresses server notice",
			state: api.State{Local: true, Version: "0.3.0", Releases: releases}, appVersion: "0.3.0",
			want: mobileUpdateInfo{AppVersion: "v0.4.0", Releases: []mobileReleaseUpdate{
				{Release: releases[0], App: true},
			}},
		},
		{
			name:  "local current ignores server flag",
			state: api.State{Local: true, Version: "0.4.0", Releases: releases}, appVersion: "0.4.0",
		},
		{
			name:  "empty releases are unknown",
			state: api.State{Version: "0.1.0"}, appVersion: "0.1.0",
		},
		{
			name:  "unknown app and server versions",
			state: api.State{Version: "dev", Releases: []api.Release{{Version: "v0.4.0"}}}, appVersion: "latest",
		},
		{
			name:  "unknown release version",
			state: api.State{Version: "0.1.0", Releases: []api.Release{{Version: "dev"}}}, appVersion: "0.1.0",
		},
		{
			name:  "unknown app still allows known server notice",
			state: api.State{Version: "0.3.0", Releases: releases}, appVersion: "dev",
			want: mobileUpdateInfo{ServerVersion: "v0.4.0", Releases: []mobileReleaseUpdate{
				{Release: releases[0], Server: true},
			}},
		},
		{
			name:  "git describe app uses shared comparison",
			state: api.State{Local: true, Releases: releases}, appVersion: "v0.3.0-2-gabcdef",
			want: mobileUpdateInfo{AppVersion: "v0.4.0", Releases: []mobileReleaseUpdate{
				{Release: releases[0], App: true},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mobileUpdates(tt.state, tt.appVersion); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("mobileUpdates() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMobileVersionSummary(t *testing.T) {
	state := api.State{Version: "0.1.0"}
	base := "App " + mobileVersion + " · 服务端 0.1.0"
	if got := mobileVersionSummary(state); got != base {
		t.Fatalf("未知版本状态不应增加说明：%q", got)
	}
	state.Releases = []api.Release{{Version: "v99.0.0", Newer: true}}
	if got := mobileVersionSummary(state); got != base+"\nApp：有新版本 v99.0.0\n服务端：有新版本 v99.0.0" {
		t.Fatalf("未分别显示 App 和服务端更新：%q", got)
	}
	state.Releases = []api.Release{{Version: "v" + mobileVersion, Newer: true}}
	if got := mobileVersionSummary(state); got != base+"\nApp：已是最新版本\n服务端：有新版本 v"+mobileVersion {
		t.Fatalf("App 应独立于服务端判断版本：%q", got)
	}
	state.Local = true
	if got := mobileVersionSummary(state); got != base+"\nApp：已是最新版本" {
		t.Fatalf("本机模式不应增加服务端更新状态：%q", got)
	}
	state.Local = false
	state.Releases[0].Newer = false
	if got := mobileVersionSummary(state); strings.Count(got, "已是最新版本") != 2 {
		t.Fatalf("未显示两个最新状态：%q", got)
	}
}

func TestMobileReleaseURL(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/VOfficial404/bili-daily/releases/tag/v0.2.0",
		"https://github.com/VOfficial404/bili-daily/releases/tag/v0.2.0#assets",
	} {
		if got := mobileReleaseURL(raw); got != raw {
			t.Errorf("拒绝有效发布页 %q：%q", raw, got)
		}
	}
	for _, raw := range []string{
		"", "http://github.com/a/b", "https://example.com/a", "javascript:alert(1)",
		"https://github.com.evil.test/a", "https://github.com@evil.test/a",
		"https://user@github.com/a", "https://github.com:443/a", "https://github.com",
		"//github.com/a", "https://github.com\\@evil.test/a", "https://github.com/a\nb",
	} {
		if got := mobileReleaseURL(raw); got != "" {
			t.Errorf("未阻止非 GitHub HTTPS 发布页 %q：%q", raw, got)
		}
	}
}

func TestMobileReleaseNotes(t *testing.T) {
	for _, notes := range []string{"", " \n\t"} {
		if got := mobileReleaseNotes(notes); got != "这个版本没有写更新说明，详见发布页" {
			t.Fatalf("缺少空更新说明的提示：%q", got)
		}
	}
	notes := "- 修复登录\n- 新增 **功能**\n<script>plain text</script>"
	if got := mobileReleaseNotes(notes); got != notes {
		t.Fatalf("更新说明应保持纯文本：%q", got)
	}
}
