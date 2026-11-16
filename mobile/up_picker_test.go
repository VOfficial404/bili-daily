package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMobileParseUP(t *testing.T) {
	for _, text := range []string{"123", " 00123 ", "https://space.bilibili.com/123?spm_id_from=test", "分享 UP 主 space.bilibili.com/123 的空间", "UID:123", "uid 123", "UiD: 123"} {
		id, err := mobileParseUP(text)
		if err != nil || id != 123 {
			t.Errorf("解析 %q：%d %v", text, id, err)
		}
	}
	for _, text := range []string{"", "0", "-1", "+123", "123,456", "123abc", "https://bilibili.com/video/123", "9223372036854775808"} {
		if _, err := mobileParseUP(text); err == nil || err.Error() != mobileUPInputHint {
			t.Errorf("无效输入 %q：%v", text, err)
		}
	}
}

func TestMobileUPDraftAddRemoveOrder(t *testing.T) {
	f := mobileTestEngine(t, false)
	app, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node { return a.taskOptionsPage(cx, "daily") })
	for _, text := range []string{"UID:11", "https://space.bilibili.com/22", "11"} {
		app.Type("label:粘贴 UID 或空间链接", text)
		app.Press("role:button 添加")
	}
	if app.Exists("label:优先 UP 主 UID") {
		t.Fatal("仍显示旧 UID 输入框")
	}
	app.Press("id:up-up:22")
	if app.Enabled("id:up-up:22") || !app.Enabled("id:up-down:22") {
		t.Fatal("上移后优先顺序未更新")
	}
	app.Press("id:up-remove:11")
	if app.Exists("id:up-remove:11") {
		t.Fatal("重复添加了同一个 UID")
	}
	app.Type("label:粘贴 UID 或空间链接", "无效链接")
	app.Press("role:button 添加")
	app.WaitFor("text:"+mobileUPInputHint, 5*time.Second)
	state, err := f.client.State(context.Background())
	if err != nil || len(state.Settings.Tasks.Daily.UpIDs) != 0 {
		t.Fatalf("未保存时写入服务端：%v %v", state.Settings.Tasks.Daily.UpIDs, err)
	}
	app.Press("role:button 保存任务选项")
	mobileWaitSaved(t, app, f, "保存任务选项", func(s api.Settings) bool { return slices.Equal(s.Tasks.Daily.UpIDs, []int64{22}) })
}

func TestMobileUPPickerSelectionAndPaging(t *testing.T) {
	f := mobileTestEngine(t, true)
	const route = "api.bilibili.com/x/relation/followings"
	f.fake.Handle(route, func(call bilitest.Call) bilitest.Reply {
		id := int64(501)
		if call.Query.Get("pn") == "2" {
			id = 502
		}
		return bilitest.OK(map[string]any{"list": []map[string]any{{"mid": id, "uname": fmt.Sprintf("关注 UP %d", id)}}, "total": 21})
	})
	f.fake.JSON(route+"/search", map[string]any{"list": []map[string]any{{"mid": 503, "uname": "昵称搜索结果"}}, "total": 1})
	app, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node { return a.taskOptionsPage(cx, "daily") })
	app.Press("role:button 从关注里选")
	app.WaitFor("id:following:501", 15*time.Second)
	app.Press("id:following:501")
	app.WaitFor("text:已选择", 5*time.Second)
	if got := f.fake.Count(route); got != 1 {
		t.Fatalf("未点加载更多就请求了 %d 页", got)
	}
	app.Press("role:button 加载更多")
	app.WaitFor("id:following:502", 15*time.Second)
	app.Press("id:following:502")
	app.Press("id:following:501")
	app.Type("label:搜索昵称", "昵称")
	if got := f.fake.Count(route + "/search"); got != 0 {
		t.Fatalf("输入时立即发起了 %d 次搜索", got)
	}
	app.Press("role:button 搜索")
	app.WaitFor("id:following:503", 15*time.Second)
	app.Press("id:following:503")
	app.Press("role:button 完成")
	app.WaitFor("id:up-remove:502", 5*time.Second)
	if !app.Exists("text:关注 UP 502") || !app.Exists("text:昵称搜索结果") || app.Exists("id:up-remove:501") {
		t.Fatalf("选择未立即反映到草稿卡片：%v", app.Texts())
	}
	state, err := f.client.State(context.Background())
	if err != nil || len(state.Settings.Tasks.Daily.UpIDs) != 0 {
		t.Fatalf("选择直接保存了设置：%v %v", state.Settings.Tasks.Daily.UpIDs, err)
	}
	app.Press("role:button 保存任务选项")
	mobileWaitSaved(t, app, f, "保存任务选项", func(s api.Settings) bool { return slices.Equal(s.Tasks.Daily.UpIDs, []int64{502, 503}) })
}

func TestMobileChargePickerReplacesAndConfirmsName(t *testing.T) {
	f := mobileTestEngine(t, true)
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state.Settings.Tasks.Charge.Enabled = false
	state.Settings.Tasks.Charge.UpID = 0
	if _, err := f.client.SaveSettings(context.Background(), state.Settings); err != nil {
		t.Fatal(err)
	}
	f.fake.JSON("api.bilibili.com/x/relation/followings", map[string]any{"list": []map[string]any{{"mid": 601, "uname": "充电对象昵称"}}, "total": 1})
	app, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node { return a.taskOptionsPage(cx, "charge") }, nsgotest.PendingAlerts())
	app.WaitFor("text:还没有选充电对象", 5*time.Second)
	app.Type("label:粘贴 UID 或空间链接", "600")
	app.Press("role:button 添加")
	app.Press("role:button 从关注里选")
	app.WaitFor("id:following:601", 15*time.Second)
	app.Press("id:following:601")
	app.WaitFor("id:up-remove:601", 5*time.Second)
	if app.Exists("id:up-remove:600") || app.Exists("label:充电 UP 主 UID") {
		t.Fatal("单选未替换或保留了旧输入框")
	}
	app.Press("label:启用任务")
	app.Press("role:button 保存任务选项")
	alerts := app.Alerts()
	if len(alerts) != 1 || !strings.Contains(alerts[0].Message, "充电对象昵称（UID 601）") {
		t.Fatalf("充电确认未显示昵称：%+v", alerts)
	}
	app.Answer(0)
	state, err = f.client.State(context.Background())
	if err != nil || state.Settings.Tasks.Charge.UpID != 0 || state.Settings.Tasks.Charge.Enabled {
		t.Fatalf("取消确认后保存了草稿：%+v %v", state.Settings.Tasks.Charge, err)
	}
	app.Press("role:button 保存任务选项")
	app.Answer(1)
	mobileWaitSaved(t, app, f, "保存任务选项", func(s api.Settings) bool { return s.Tasks.Charge.Enabled && s.Tasks.Charge.UpID == 601 })
	app.Press("id:up-remove:601")
	app.WaitFor("text:还没有选充电对象", 5*time.Second)
}

func TestMobileUPPickerNoAccountAndDiscard(t *testing.T) {
	f := mobileTestEngine(t, false)
	app, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node {
		show := core.NewSignal(cx, true)
		return core.View{Children: []core.Node{
			core.Show{When: show.Get, Then: func(cx *core.Scope) core.Node { return a.taskOptionsPage(cx, "daily") }},
			a.button("关闭草稿", func() { show.Set(false) }), a.button("重新打开草稿", func() { show.Set(true) }),
		}}
	})
	app.Press("role:button 从关注里选")
	app.WaitFor("text:先添加并登录一个账号才能从关注里选，也可以直接粘贴 UID", 5*time.Second)
	app.Press("role:button 完成")
	app.Type("label:粘贴 UID 或空间链接", "123")
	app.Press("role:button 添加")
	app.Press("role:button 关闭草稿")
	app.Press("role:button 重新打开草稿")
	app.WaitFor("text:尚未设置优先 UP 主", 5*time.Second)
	if app.Exists("id:up-remove:123") {
		t.Fatal("关闭页面未放弃未保存的选择")
	}
}

func TestMobileUPPickerRetry(t *testing.T) {
	f := mobileTestEngine(t, true)
	const route = "api.bilibili.com/x/relation/followings"
	f.fake.Fail(route, -400, "关注读取失败")
	app, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node { return a.taskOptionsPage(cx, "daily") })
	app.Press("role:button 从关注里选")
	app.WaitFor("role:button 重试", 5*time.Second)
	if !strings.Contains(strings.Join(app.Texts(), "\n"), "关注读取失败") {
		t.Fatalf("未显示 API 错误：%v", app.Texts())
	}
	f.fake.JSON(route, map[string]any{"list": []map[string]any{{"mid": 701, "uname": "重试成功"}}, "total": 1})
	app.Press("role:button 重试")
	app.WaitFor("id:following:701", 15*time.Second)
	if got := f.fake.Count(route); got != 2 {
		t.Fatalf("请求次数=%d，期望失败一次和手动重试一次", got)
	}
}
