//go:build !js

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

func profileHarness(t *testing.T, handler http.HandlerFunc) *harness {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	h := newHarness(t, &memStore{})
	h.a.st = &api.State{}
	h.a.session = client.NewSession(api.NewClient(srv.URL, "test"), *h.a.st)
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Gap(12).Children(func() {
			ui.Form(c.Key("daily"), func() { h.a.settingsTaskOptions(c, "daily") })
			ui.Form(c.Key("charge"), func() { h.a.settingsTaskOptions(c, "charge") })
		})
	}, 1120, 1800)
	return h
}

func TestUPPreviewsExplicitLookupCacheAndUnavailable(t *testing.T) {
	var requests atomic.Int32
	h := profileHarness(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/api/ups/202" {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(api.Error{Message: "资料查询失败"})
			return
		}
		if r.URL.Path != "/api/ups/101" {
			t.Errorf("unexpected lookup: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(api.UPProfile{Mid: "101", Uname: "测试 UP"})
	})
	h.a.form.Tasks.Daily.UpIDs = []int64{101, 202}
	h.a.form.Tasks.Charge.UpID = 303
	h.a.inputs = map[string]string{}
	h.tt.Frame()
	h.must("UID 101")
	h.must("UID 202")
	h.must("UID 303")
	if requests.Load() != 0 {
		t.Fatal("展示已保存的 UID 不应自动发起请求")
	}
	h.a.loadUPProfile("101")
	h.a.loadUPProfile("202")
	h.settle()
	h.must("测试 UP")
	h.must("资料暂不可用：资料查询失败 可直接保存 UID。")
	if h.a.settingsInputError() != "" || len(h.a.form.Tasks.Daily.UpIDs) != 2 {
		t.Fatal("查询失败不应阻止保存有效 UID")
	}
	// 模拟保存和再次展开：原始输入重建，资料仍按 UID 缓存。
	h.a.inputs = map[string]string{}
	h.tt.Frame()
	h.tt.Frame()
	h.must("测试 UP")
	h.must("UID 202")
	h.must("UID 303")
	if requests.Load() != 2 {
		t.Fatal("重新绘制或重建输入不应重复查询资料")
	}
	h.a.form.Tasks.Charge.UpID = 101
	h.a.form.Tasks.Charge.Enabled = true
	if !h.a.needsChargeConfirm() || !strings.Contains(h.a.guardText, "测试 UP（UID 101）") || !strings.Contains(h.a.guardText, "拿不回来") {
		t.Fatal("充电确认应同时显示昵称、UID 和花费警告")
	}
}

func TestUPPreviewsIgnoreOldInputAndDisconnectedResults(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := profileHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ups/101" {
			close(started)
			<-release
		}
		mid := strings.TrimPrefix(r.URL.Path, "/api/ups/")
		_ = json.NewEncoder(w).Encode(api.UPProfile{Mid: mid, Uname: "UP " + mid})
	})
	h.a.form.Tasks.Daily.UpIDs = []int64{101}
	h.a.inputs = map[string]string{}
	h.tt.Frame()
	h.a.loadUPProfile("101")
	<-started
	h.a.form.Tasks.Daily.UpIDs = []int64{202}
	h.a.inputs = map[string]string{}
	h.a.loadUPProfile("202")
	close(release)
	h.settle()
	h.must("UP 202")
	if h.tt.HasText("UP 101") || h.tt.HasText("UID 101") {
		t.Fatal("旧 UID 的响应不能显示在已修改的输入下")
	}
	h.a.loadUPProfile("303")
	h.a.wg.Wait() // 响应已排队但还没有应用到界面。
	h.a.disconnect()
	h.settle()
	if len(h.a.upProfiles) != 0 {
		t.Fatal("断开连接后不能应用旧请求的资料")
	}
}
