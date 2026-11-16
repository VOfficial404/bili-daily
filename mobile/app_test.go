package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
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
	app.Press("label:报告")
	app.WaitFor("label:"+f.run.Uname+" · "+client.StatusShort(f.run.Status), 5*time.Second)
	app.Press("label:" + f.run.Uname + " · " + client.StatusShort(f.run.Status))
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
	app.Press("label:设置")
	app.Clear("label:每日开始时间")
	app.Type("label:每日开始时间", "25:00")
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
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:这个地址不是 bili-daily 的服务端") })
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
			app.Press("label:设置")
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
	app.Press("label:设置")
	app.Press("role:button 批量取关选项")
	app.Press("role:button 加载关注分组")
	app.WaitFor("text:天选时刻（2 人）", 5*time.Second)
	app.WaitUntil(5*time.Second, func() bool { return app.Enabled("role:button 保存并运行取关") })
	app.Clear("label:最多取关人数")
	app.Type("label:最多取关人数", "0")
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

func mobileFormTest(t *testing.T, f *mobileFixture, build func(*mobileApp, *core.Scope) core.Node, options ...nsgotest.Option) (*nsgotest.App, *mobileApp) {
	t.Helper()
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var a *mobileApp
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a = &mobileApp{cx: cx, ctx: cx.Context(), session: client.NewSession(f.client, state), state: core.NewSignalFunc(cx, state, nil), connected: core.NewSignal(cx, true), busy: core.NewSignal(cx, false), refreshing: core.NewSignal(cx, false), errorText: core.NewSignal(cx, ""), toaster: ui.NewToaster(cx), tab: core.NewSignal(cx, 0), navs: []*base.Nav[string]{base.NewNav(cx, "root")}}
		cx.Cleanup(a.stop)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{build(a, cx), ui.Textf(func() string {
			s := a.state.Get().Settings
			return fmt.Sprintf("状态：%s/%s", s.RunAt, s.Pace)
		}, 14, ui.Regular, ui.ColorsOf(cx).Text)}}
	}, append([]nsgotest.Option{nsgotest.Offscreen()}, options...)...)
	return app, a
}

func mobileWaitSaved(t *testing.T, app *nsgotest.App, f *mobileFixture, button string, check func(api.Settings) bool) {
	t.Helper()
	app.WaitUntil(5*time.Second, func() bool {
		state, err := f.client.State(context.Background())
		return err == nil && check(state.Settings) && app.Enabled("role:button "+button)
	})
}

func TestMobileSettingsMergeAndRepeatedSaves(t *testing.T) {
	f := mobileTestEngine(t, false)
	app, a := mobileFormTest(t, f, (*mobileApp).settingsPage)
	initial := a.state.GetUntracked().Settings.Tasks.Daily.Enabled
	app.Press("label:每日任务")
	app.Press("role:button 保存设置")
	mobileWaitSaved(t, app, f, "保存设置", func(s api.Settings) bool { return s.Tasks.Daily.Enabled != initial })
	// 其他客户端更新服务端设置，表单仍然保持打开。
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state.Settings.RunAt = "14:15"
	state.Settings.Tasks.Daily.Coins = 2
	if _, err := f.client.SaveSettings(context.Background(), state.Settings); err != nil {
		t.Fatal(err)
	}
	a.cx.Post(func() { a.state.Set(state) })
	app.WaitFor("text:状态：14:15/"+a.state.GetUntracked().Settings.Pace, 5*time.Second)
	app.Press("label:每日任务")
	app.Press("role:button 保存设置")
	mobileWaitSaved(t, app, f, "保存设置", func(s api.Settings) bool {
		return s.Tasks.Daily.Enabled == initial && s.RunAt == "14:15" && s.Tasks.Daily.Coins == 2
	})
}

func TestMobileTaskOptionsMergeAndRepeatedSaves(t *testing.T) {
	f := mobileTestEngine(t, false)
	app, a := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node { return a.taskOptionsPage(cx, "daily") })
	initial := a.state.GetUntracked().Settings.Tasks.Daily.Enabled
	app.Press("label:启用任务")
	app.Press("role:button 保存任务选项")
	mobileWaitSaved(t, app, f, "保存任务选项", func(s api.Settings) bool { return s.Tasks.Daily.Enabled != initial })
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state.Settings.RunAt = "15:16"
	state.Settings.Tasks.Daily.KeepCoins = 17
	if _, err := f.client.SaveSettings(context.Background(), state.Settings); err != nil {
		t.Fatal(err)
	}
	a.cx.Post(func() { a.state.Set(state) })
	app.WaitFor("text:状态：15:16/"+a.state.GetUntracked().Settings.Pace, 5*time.Second)
	app.Press("label:启用任务")
	app.Press("role:button 保存任务选项")
	mobileWaitSaved(t, app, f, "保存任务选项", func(s api.Settings) bool {
		return s.Tasks.Daily.Enabled == initial && s.RunAt == "15:16" && s.Tasks.Daily.KeepCoins == 17
	})
}

func TestMobileChargeSavesRequireConfirmation(t *testing.T) {
	for _, general := range []bool{false, true} {
		t.Run(fmt.Sprintf("general=%t", general), func(t *testing.T) {
			f := mobileTestEngine(t, false)
			state, err := f.client.State(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			state.Settings.Tasks.Charge.Enabled = false
			state.Settings.Tasks.Charge.UpID = 12345
			if _, err := f.client.SaveSettings(context.Background(), state.Settings); err != nil {
				t.Fatal(err)
			}
			app, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node {
				if general {
					return a.settingsPage(cx)
				}
				return a.taskOptionsPage(cx, "charge")
			}, nsgotest.PendingAlerts())
			toggle, save := "label:启用任务", "保存任务选项"
			if general {
				toggle, save = "label:B 币券充电", "保存设置"
			}
			app.Press(toggle)
			app.Press("role:button " + save)
			alerts := app.Alerts()
			if len(alerts) != 1 || !strings.Contains(alerts[0].Message, "12345") || !strings.Contains(alerts[0].Message, "每月") || !strings.Contains(alerts[0].Message, "全部 B 币券余额") {
				t.Fatalf("充电确认信息不完整：%+v", alerts)
			}
			app.Answer(0)
			state, err = f.client.State(context.Background())
			if err != nil || state.Settings.Tasks.Charge.Enabled {
				t.Fatalf("取消确认后仍开启充电：%+v %v", state.Settings.Tasks.Charge, err)
			}
			app.Press("role:button " + save)
			app.Answer(1)
			mobileWaitSaved(t, app, f, save, func(s api.Settings) bool { return s.Tasks.Charge.Enabled })
			if !general {
				app.Type("label:粘贴 UID 或空间链接", "67890")
				app.Press("role:button 添加")
				app.Press("role:button " + save)
				alerts = app.Alerts()
				if !strings.Contains(alerts[len(alerts)-1].Message, "67890") {
					t.Fatalf("更换 UID 未重新确认：%+v", alerts)
				}
				app.Answer(1)
				mobileWaitSaved(t, app, f, save, func(s api.Settings) bool { return s.Tasks.Charge.UpID == 67890 })
			}
		})
	}
}

func TestMobileLocalProxyPrefillAndSave(t *testing.T) {
	const proxy = "http://127.0.0.1:7890"
	app := nsgotest.Start(t, Root, nsgotest.Offscreen(), nsgotest.Storage(map[string]string{"local-proxy": proxy}))
	app.Press("text:在本机运行")
	if !app.Exists("text:" + proxy) {
		t.Fatal("本机代理未从存储回填")
	}
	f := mobileTestEngine(t, false)
	session := &client.Session{}
	if _, err := session.StartLocal(context.Background(), client.LocalOptions{Config: engine.Config{DataDir: t.TempDir(), Transport: f.fake}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Stop)
	f.client = session.Client()
	form, _ := mobileFormTest(t, f, func(a *mobileApp, cx *core.Scope) core.Node {
		a.session = session
		return a.settingsPage(cx)
	})
	form.Clear("label:代理")
	form.Type("label:代理", proxy)
	form.Press("role:button 保存设置")
	mobileWaitSaved(t, form, f, "保存设置", func(s api.Settings) bool { return s.Proxy == proxy })
	if got, ok := form.Storage().GetItem("local-proxy"); !ok || got != proxy {
		t.Fatalf("本机保存未更新代理存储：%q, %v", got, ok)
	}
}

func TestMobileRefreshPreservesNewSaveAndQueuesRefresh(t *testing.T) {
	f := mobileTestEngine(t, false)
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	initialPace := state.Settings.Pace
	oldEntered, oldRelease := make(chan struct{}), make(chan struct{})
	newEntered, newRelease := make(chan struct{}), make(chan struct{})
	var oldOnce, newOnce sync.Once
	releaseOld := func() { oldOnce.Do(func() { close(oldRelease) }) }
	releaseNew := func() { newOnce.Do(func() { close(newRelease) }) }
	defer releaseOld()
	defer releaseNew()
	var mu sync.Mutex
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			var saved api.Settings
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			state.Settings = saved
			mu.Unlock()
			json.NewEncoder(w).Encode(saved)
			return
		}
		read := reads.Add(1)
		mu.Lock()
		response := state
		mu.Unlock()
		switch read {
		case 2:
			close(oldEntered)
			<-oldRelease
		case 4:
			close(newEntered)
			<-newRelease
		}
		json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	g := &mobileFixture{client: api.NewClient(server.URL, mobileTestToken), server: server}
	app, _ := mobileFormTest(t, g, func(a *mobileApp, cx *core.Scope) core.Node {
		a.active = true
		return mobileColumn(a.button("刷新状态", a.refresh), a.button("保存新时间", func() {
			base := a.state.GetUntracked().Settings
			s := client.CloneSettings(base)
			s.RunAt = "16:17"
			a.saveSettings(cx, base, s, nil)
		}))
	})
	app.Press("role:button 刷新状态")
	app.WaitUntil(5*time.Second, func() bool {
		select {
		case <-oldEntered:
			return true
		default:
			return false
		}
	})
	app.Press("role:button 保存新时间")
	app.WaitFor("text:状态：16:17/"+initialPace, 5*time.Second)
	mu.Lock()
	state.Settings.Pace = "fast"
	mu.Unlock()
	releaseOld()
	app.WaitUntil(5*time.Second, func() bool {
		select {
		case <-newEntered:
			return true
		default:
			return false
		}
	})
	if !app.Exists("text:状态：16:17/" + initialPace) {
		t.Fatal("旧刷新覆盖了刚保存的时间")
	}
	releaseNew()
	app.WaitFor("text:状态：16:17/fast", 5*time.Second)
	if reads.Load() != 4 {
		t.Fatalf("保存后读取次数=%d，期望初始、旧请求、保存前读取和排队请求共 4 次", reads.Load())
	}
}

func TestMobilePollDisposalCancelsTimers(t *testing.T) {
	f := mobileTestEngine(t, false)
	state, err := f.client.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}))
	t.Cleanup(server.Close)
	app, _ := mobileFormTest(t, &mobileFixture{client: api.NewClient(server.URL, mobileTestToken)}, func(a *mobileApp, cx *core.Scope) core.Node {
		a.active = true
		return mobileColumn(a.button("重新排程", a.schedulePoll), a.button("暂停轮询", func() {
			a.active = false
			a.stopPoll()
		}), a.button("恢复轮询", func() { a.active = true; a.schedulePoll() }),
			ui.Textf(func() string { return fmt.Sprintf("刷新中：%t", a.refreshing.Get()) }, 14, ui.Regular, ui.ColorsOf(cx).Text))
	})
	for range 20 {
		app.Press("role:button 重新排程")
	}
	app.Press("role:button 暂停轮询")
	app.Advance(4 * time.Second)
	if reads.Load() != 1 {
		t.Fatalf("暂停后旧计时器仍发出请求：%d", reads.Load())
	}
	app.Press("role:button 恢复轮询")
	app.Advance(3 * time.Second)
	app.WaitUntil(5*time.Second, func() bool { return reads.Load() == 2 && app.Exists("text:刷新中：false") })
	app.Advance(3 * time.Second)
	app.WaitUntil(5*time.Second, func() bool { return reads.Load() == 3 && app.Exists("text:刷新中：false") })
}
