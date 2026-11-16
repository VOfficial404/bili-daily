//go:build !js

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	h.a.width, h.a.height = 1120, 1800
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Gap(12).Children(func() {
			ui.Form(c.Key("daily"), func() { h.a.settingsTaskOptions(c, "daily") })
			ui.Form(c.Key("charge"), func() { h.a.settingsTaskOptions(c, "charge") })
		})
		h.a.upPickerDialog(c)
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
	// 模拟保存和再次展开：资料仍按 UID 缓存。
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

func TestParseUPInput(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int64
	}{
		{"123", 123}, {" 00123 ", 123},
		{"https://space.bilibili.com/123?spm_id_from=333", 123},
		{"看看这个 UP：space.bilibili.com/456 的空间", 456},
		{"UID:123", 123}, {"uid 456", 456}, {"uId : 789", 789},
		{"", 0}, {"0", 0}, {"-12", 0}, {"1,2", 0}, {"UID:abc", 0},
		{"https://bilibili.com/video/123", 0}, {"9223372036854775808", 0},
	} {
		got, ok := parseUPInput(tc.input)
		if got != tc.want && ok || ok != (tc.want > 0) {
			t.Errorf("parseUPInput(%q) = %d, %v; want %d", tc.input, got, ok, tc.want)
		}
	}
}

func TestUPSelectionDraftOrderAndDiscard(t *testing.T) {
	h := newHarness(t, &memStore{})
	a := h.a
	a.formBase.Tasks.Daily.UpIDs = []int64{101}
	a.formBase.Tasks.Charge.UpID = 303
	a.form = client.CloneSettings(a.formBase)
	a.addUP(false, 202)
	a.addUP(false, 202)
	a.moveUP(202, -1)
	a.moveUP(202, -1)
	if !slices.Equal(a.form.Tasks.Daily.UpIDs, []int64{202, 101}) || !a.dirty {
		t.Fatal("添加应去重，上移不能越界，并标记草稿已修改")
	}
	a.removeUP(false, 101)
	a.addUP(true, 404)
	if !slices.Equal(a.form.Tasks.Daily.UpIDs, []int64{202}) || a.form.Tasks.Charge.UpID != 404 {
		t.Fatal("移除应修改列表，单选应替换充电对象")
	}
	if !slices.Equal(a.formBase.Tasks.Daily.UpIDs, []int64{101}) || a.formBase.Tasks.Charge.UpID != 303 {
		t.Fatal("编辑不能更改已保存设置")
	}
	a.removeUP(true, 404)
	if a.form.Tasks.Charge.UpID != 0 {
		t.Fatal("移除充电对象应清空单选")
	}
	a.discardSettings()
	if a.dirty || !slices.Equal(a.form.Tasks.Daily.UpIDs, []int64{101}) || a.form.Tasks.Charge.UpID != 303 {
		t.Fatal("放弃修改应恢复已保存设置")
	}
	input := upPasteInput{text: "无效链接"}
	a.pasteUP(false, &input)
	if input.err != upPasteHint || a.dirty {
		t.Fatal("无效粘贴应提示格式且不能修改草稿")
	}
	input.text = "space.bilibili.com/505"
	a.pasteUP(true, &input)
	if a.form.Tasks.Charge.UpID != 505 || input.err != "" || input.text != "" || !a.dirty {
		t.Fatal("有效粘贴应替换单选、清空输入并标记草稿")
	}
}

func TestUPPickerSelectionPaginationSearchAndRetry(t *testing.T) {
	var requests atomic.Int32
	var fail atomic.Bool
	h := profileHarness(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/accounts/7/followings" {
			t.Errorf("不应额外查询所选 UP 的资料：%s", r.URL.Path)
		}
		if fail.Swap(false) {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(api.Error{Message: "关注查询失败"})
			return
		}
		if r.URL.Query().Get("q") == "测试" {
			_ = json.NewEncoder(w).Encode(api.Followings{List: []api.UPProfile{{Mid: "303", Uname: "测试结果"}}, Total: 1})
		} else if r.URL.Query().Get("pn") == "2" {
			_ = json.NewEncoder(w).Encode(api.Followings{List: []api.UPProfile{{Mid: "202", Uname: "第二位 UP"}}, Total: 2})
		} else {
			_ = json.NewEncoder(w).Encode(api.Followings{List: []api.UPProfile{{Mid: "101", Uname: "第一位 UP"}}, Total: 2, More: true})
		}
	})
	h.a.st.Accounts = []api.Account{{Mid: "6", Uname: "未登录"}, {Mid: "7", Uname: "已登录", LoggedIn: true}}
	h.a.openUPPicker(false)
	if !h.a.upPicker.loading || h.a.upPicker.mid != "7" {
		t.Fatal("应默认第一个已登录账号并显示加载状态")
	}
	h.a.loadFollowings(false)
	h.settle()
	h.must("第一位 UP")
	h.must("加载更多")
	if requests.Load() != 1 {
		t.Fatal("不能重复请求正在加载的列表或自动翻页")
	}
	h.click("第一位 UP UID 101")
	h.must("已选")
	if !slices.Equal(h.a.form.Tasks.Daily.UpIDs, []int64{101}) || !h.a.dirty || len(h.a.formBase.Tasks.Daily.UpIDs) != 0 {
		t.Fatal("选择应仅写入表单草稿")
	}
	if h.a.upProfiles["101"].profile.Uname != "第一位 UP" || requests.Load() != 1 {
		t.Fatal("选择应直接缓存资料，不能再发查询请求")
	}
	h.click("第一位 UP UID 101")
	if len(h.a.form.Tasks.Daily.UpIDs) != 0 {
		t.Fatal("多选再次点击应取消")
	}
	fail.Store(true)
	h.click("加载更多")
	h.must("关注查询失败")
	h.click("重试")
	h.must("第二位 UP")
	if h.a.upPicker.page != 2 || len(h.a.upPicker.list) != 2 {
		t.Fatal("翻页重试应保留前页并重试失败页")
	}
	if err := h.tt.Click("搜索关注的昵称"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("测试")
	if requests.Load() != 3 {
		t.Fatal("输入昵称不能逐字请求")
	}
	h.tt.Key(0, ui.KeyEnter)
	h.settle()
	h.must("测试结果")
	if h.a.upPicker.page != 1 || len(h.a.upPicker.list) != 1 {
		t.Fatal("提交搜索应重置分页结果")
	}
	h.click("完成")
	h.a.openUPPicker(true)
	h.settle()
	h.click("第一位 UP UID 101")
	if h.a.form.Tasks.Charge.UpID != 101 || h.a.upPicker.open {
		t.Fatal("单选应替换充电对象并关闭选择器")
	}
	h.a.form.Tasks.Charge.Enabled = true
	if !h.a.needsChargeConfirm() || !strings.Contains(h.a.guardText, "第一位 UP（UID 101）") {
		t.Fatal("保存充电设置仍应确认，并使用选择器缓存的昵称")
	}
}

func TestUPPickerWithoutLoggedInAccount(t *testing.T) {
	h := profileHarness(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("没有已登录账号不应请求：%s", r.URL.Path)
	})
	h.a.openUPPicker(true)
	h.tt.Frame()
	h.must("先添加并登录一个账号才能从关注里选，也可以直接粘贴 UID")
	h.click("完成")
	h.must("还没有选充电对象")
}
