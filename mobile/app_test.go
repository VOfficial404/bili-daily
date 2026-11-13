package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
	"github.com/VOfficial404/bili-tools/engine"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

const mobileTestToken = "mobile-test-token"

type mobileFixture struct {
	client *api.Client
	server *httptest.Server
	run    *api.Run
	fake   *bilitest.Server
}

func mobileTestEngine(t *testing.T, account bool) *mobileFixture {
	t.Helper()
	fake := bilitest.New()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, engine.Beijing)
	e, err := engine.New(engine.Config{DataDir: t.TempDir(), Token: mobileTestToken, Version: "test", Transport: fake, Now: func() time.Time { return now }, Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Stop)
	server := httptest.NewServer(e.Handler())
	t.Cleanup(server.Close)
	f := &mobileFixture{client: api.NewClient(server.URL, "mobile-test-token"), server: server, fake: fake}
	if !account {
		return f
	}
	fake.JSON("passport.bilibili.com/x/passport-login/web/qrcode/generate", map[string]any{"url": "https://example.invalid/scan?k=mobile", "qrcode_key": "mobile"})
	fake.Handle("passport.bilibili.com/x/passport-login/web/qrcode/poll", func(bilitest.Call) bilitest.Reply {
		h := http.Header{}
		for _, cookie := range []string{"SESSDATA=test-session; Max-Age=15552000; Path=/", "bili_jct=test-csrf; Path=/", "DedeUserID=42; Path=/"} {
			h.Add("Set-Cookie", cookie)
		}
		return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"code":0,"refresh_token":"test-refresh"}}`}
	})
	fake.JSON("api.bilibili.com/x/frontend/finger/spi", map[string]any{"b_3": "B3", "b_4": "B4"})
	fake.JSON("api.bilibili.com/x/web-interface/nav", map[string]any{"isLogin": true, "mid": 42, "uname": "手机测试账号", "face": "", "money": 100.0, "vipStatus": 0, "vipType": 0, "level_info": map[string]any{"current_level": 4, "current_exp": 5000, "next_exp": 10800}, "wallet": map[string]any{"bcoin_balance": 0, "coupon_balance": 0}})
	fake.JSON("passport.bilibili.com/x/passport-login/web/cookie/info", map[string]any{"refresh": false, "timestamp": now.UnixMilli()})
	ctx := context.Background()
	qr, err := f.client.StartQR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if qr, err = f.client.PollQR(ctx, qr.ID); err != nil || qr.State != "success" {
		t.Fatalf("扫码：%+v %v", qr, err)
	}
	settings := e.Settings()
	settings.Tasks.Daily.Watch = false
	settings.Tasks.Daily.Share = false
	settings.Tasks.Daily.Coins = 0
	if _, err = f.client.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	fake.Fail("api.bilibili.com/x/member/web/exp/reward", -352, "请求被拦截")
	fake.JSON("api.bilibili.com/x/web-interface/coin/today/exp", 50)
	f.run, err = e.RunNow(ctx, "42", []string{"daily"})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func mobileConnectTest(t *testing.T, f *mobileFixture, token string) *nsgotest.App {
	t.Helper()
	app := nsgotest.Start(t, Root, nsgotest.Offscreen())
	app.Type("id:server-address", f.server.URL)
	app.Type("id:server-token", token)
	app.Press("role:button 连接")
	return app
}
func TestMobileWrongToken(t *testing.T) {
	f := mobileTestEngine(t, false)
	app := mobileConnectTest(t, f, "wrong-token")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:令牌不对，请检查服务器令牌") })
	if app.Exists("text:今天的结果按北京时间统计") {
		t.Fatal("令牌不对时进入了主界面")
	}
}
func TestMobileConnectShowsToday(t *testing.T) {
	f := mobileTestEngine(t, true)
	app := mobileConnectTest(t, f, "mobile-test-token")
	app.WaitFor("text:今天的结果按北京时间统计", 5*time.Second)
	if !app.Exists("text:手机测试账号") || !app.Exists("text:硬币 100.0") || !app.Exists("text:只统计已知奖励") {
		t.Fatalf("未显示账号或今日结果：%v", app.Texts())
	}
}
func TestMobileReportFailureHint(t *testing.T) {
	f := mobileTestEngine(t, true)
	app := mobileConnectTest(t, f, "mobile-test-token")
	app.WaitFor("text:今天的结果按北京时间统计", 5*time.Second)
	app.Press("text:报告")
	app.WaitFor("text:"+f.run.Uname+" · "+mobileStatus(f.run.Status), 5*time.Second)
	app.Press("text:" + f.run.Uname + " · " + mobileStatus(f.run.Status))
	var hint string
	for _, task := range f.run.Tasks {
		for _, step := range task.Steps {
			if step.Status == api.StatusFailed && step.Hint != "" {
				hint = step.Hint
				break
			}
		}
	}
	if hint == "" {
		t.Fatal("真实引擎没有生成失败建议")
	}
	app.WaitFor("text:建议："+hint, 5*time.Second)
}
func TestMobileInvalidSettingsShowsServerMessage(t *testing.T) {
	f := mobileTestEngine(t, false)
	app := mobileConnectTest(t, f, "mobile-test-token")
	app.WaitFor("text:今天的结果按北京时间统计", 5*time.Second)
	app.Press("text:设置")
	app.Clear("label:每日开始时间（HH:MM）")
	app.Type("label:每日开始时间（HH:MM）", "25:00")
	app.Press("role:button 保存设置")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:开始时间要写成 HH:MM，例如 09:30") })
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Settings.RunAt == "25:00" {
		t.Fatal("无效设置被保存")
	}
}
func TestMobileRejectsOtherServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"name":"other"}`) }))
	defer server.Close()
	f := &mobileFixture{server: server}
	app := mobileConnectTest(t, f, "unused")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:这个地址不是 bili-tools 的服务端") })
}
func TestMobileUnreachableServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	app := mobileConnectTest(t, &mobileFixture{server: server}, "unused")
	app.WaitUntil(5*time.Second, func() bool { return strings.Contains(strings.Join(app.Texts(), "\n"), "连不上服务器") })
}

func TestMobileTokenRememberOptIn(t *testing.T) {
	for _, remember := range []bool{false, true} {
		t.Run(fmt.Sprint(remember), func(t *testing.T) {
			f := mobileTestEngine(t, false)
			app := nsgotest.Start(t, Root, nsgotest.Offscreen())
			app.Type("id:server-address", f.server.URL)
			app.Type("id:server-token", "mobile-test-token")
			if remember {
				app.Press("label:记住并在下次自动连接")
			}
			app.Press("role:button 连接")
			app.WaitFor("text:今天的结果按北京时间统计", 5*time.Second)
			token, exists := app.Storage().GetItem("token")
			if exists != remember || remember && token != "mobile-test-token" {
				t.Fatalf("记住=%v，令牌保存=%v", remember, exists)
			}
			app.Press("text:设置")
			app.Press("role:button 断开连接")
			if _, exists = app.Storage().GetItem("token"); exists {
				t.Fatal("断开连接后仍保存令牌")
			}
		})
	}
}

func TestMobileRememberedServerAutoConnects(t *testing.T) {
	f := mobileTestEngine(t, true)
	app := nsgotest.Start(t, Root, nsgotest.Offscreen(), nsgotest.Storage(map[string]string{"remember": "true", "address": f.server.URL, "token": mobileTestToken}))
	app.WaitFor("text:今天的结果按北京时间统计", 5*time.Second)
	if !app.Exists("text:手机测试账号") {
		t.Fatal("自动连接后未显示账号")
	}
}

func TestMobileRootFollowsSystemBackground(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Offscreen(), nsgotest.Environment(core.Environment{Dark: false}))
	light := app.Find("id:mobile-root").Style.Background
	app.SetEnvironment(core.Environment{Dark: true})
	dark := app.Find("id:mobile-root").Style.Background
	if light == 0 || dark == 0 || light == dark {
		t.Fatalf("背景未跟随系统主题：浅色=%v，深色=%v", light, dark)
	}
	app.SetEnvironment(core.Environment{Dark: false})
	if got := app.Find("id:mobile-root").Style.Background; got != light {
		t.Fatalf("切回浅色后背景=%v，原背景=%v", got, light)
	}
}

func TestMobileUnfollowRequiresConfirmation(t *testing.T) {
	f := mobileTestEngine(t, true)
	f.fake.JSON("api.bilibili.com/x/relation/tags", []map[string]any{{"tagid": 7, "name": "天选时刻", "count": 2}})
	app := nsgotest.Start(t, Root, nsgotest.Offscreen(), nsgotest.PendingAlerts())
	app.Type("id:server-address", f.server.URL)
	app.Type("id:server-token", "mobile-test-token")
	app.Press("role:button 连接")
	app.WaitFor("text:今天的结果按北京时间统计", 5*time.Second)
	app.Press("text:设置")
	app.Press("role:button 批量取关选项")
	app.Press("role:button 加载关注分组")
	app.WaitFor("text:天选时刻（2 人）", 5*time.Second)
	app.WaitUntil(5*time.Second, func() bool { return app.Enabled("role:button 保存并运行取关") })
	app.Clear("label:最多取关人数（0 为全部）")
	app.Type("label:最多取关人数（0 为全部）", "0")
	app.Press("role:button 保存并运行取关")
	alerts := app.Alerts()
	if len(alerts) != 1 || !strings.Contains(alerts[0].Message, "手机测试账号（42）") || !strings.Contains(alerts[0].Message, "天选时刻") || !strings.Contains(alerts[0].Message, "全部") {
		t.Fatalf("确认信息不完整：%+v", alerts)
	}
	app.Answer(0)
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Settings.Tasks.Unfollow.Count == 0 {
		t.Fatal("取消确认后仍保存取关设置")
	}
	runs, err := f.client.Runs(context.Background(), "42", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("取消确认后启动取关：%d 份报告", len(runs))
	}
}
