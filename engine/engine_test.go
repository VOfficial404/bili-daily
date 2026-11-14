package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

func toggle(id, title string, run func(x *Ctx)) Task {
	return Task{
		Info:    api.TaskInfo{ID: id, Title: title, Group: "测试"},
		Enabled: func(*api.Settings) bool { return true },
		Run:     run,
	}
}

func TestRunReportExplainsEachFailure(t *testing.T) {
	useTasks(t,
		toggle(TaskDaily, "每日任务", func(x *Ctx) {
			x.OK("观看视频", "BV1xx")
			x.Note("经验 +5")
		}),
		toggle(TaskManga, "漫画", func(x *Ctx) {
			x.OK("签到", "成功")
			x.Fail("阅读", &bili.Error{Code: -352, Message: "风控校验失败", Path: "manga.bilibili.com/x"})
		}),
		toggle(TaskSilver2Coin, "银瓜子换硬币", func(x *Ctx) { x.Skip("兑换", "今天已经换过") }),
		toggle(TaskVipPoint, "大积分", func(x *Ctx) { panic("boom") }),
	)
	v := newEnv(t)
	v.login(42)
	run := v.run(42)

	if run.Status != api.StatusPartial {
		t.Errorf("status = %s", run.Status)
	}
	if run.Summary != "1 个任务完成，1 个部分完成，1 个失败，1 个不需要做" {
		t.Errorf("summary = %q", run.Summary)
	}
	byID := map[string]api.TaskResult{}
	for _, r := range run.Tasks {
		byID[r.ID] = r
	}
	if r := byID[TaskDaily]; r.Status != api.StatusOK || r.Note != "经验 +5" {
		t.Errorf("daily = %+v", r)
	}
	manga := byID[TaskManga]
	read := step(t, manga, "阅读")
	if manga.Status != api.StatusPartial || read.Reason != "risk" || read.Code != -352 || !strings.Contains(read.Hint, "风控") {
		t.Errorf("manga = %+v", manga)
	}
	if !strings.HasPrefix(manga.Note, "阅读失败：被 B 站风控拦截") {
		t.Errorf("manga note = %q", manga.Note)
	}
	if r := byID[TaskSilver2Coin]; r.Status != api.StatusSkipped || r.Note != "今天已经换过" {
		t.Errorf("silver = %+v", r)
	}
	if r := byID[TaskVipPoint]; r.Status != api.StatusFailed || !strings.Contains(r.Note, "boom") {
		t.Errorf("panicking task = %+v", r)
	}

	// 报告保存了下来，账号上记着今天各任务的结果
	saved, err := v.e.loadRun(run.ID)
	if err != nil || len(saved.Tasks) != 5 {
		t.Fatalf("saved run: %v %+v", err, saved)
	}
	acc := v.e.State().Accounts[0]
	if len(acc.Today) != 4 || acc.Today[0].ID != TaskDaily || acc.LastRun == nil || acc.LastRun.ID != run.ID {
		t.Errorf("account = %+v", acc)
	}
}

func TestLoggedOutAccountStopsAndIsNotScheduled(t *testing.T) {
	ran := false
	useTasks(t, toggle(TaskDaily, "每日任务", func(x *Ctx) { ran = true }))
	v := newEnv(t)
	a := v.login(42)
	v.b.Fail(navPath, -101, "账号未登录")

	run := v.run(42)
	if ran {
		t.Error("登录失效后任务还是跑了")
	}
	if run.Status != api.StatusFailed || !strings.Contains(run.Summary, "重新扫码") {
		t.Errorf("run = %s %q", run.Status, run.Summary)
	}
	if acc := v.e.State().Accounts[0]; acc.LoggedIn || acc.NextRun != nil {
		t.Errorf("account = %+v", acc)
	}
	if _, _, _, ok := a.nextDue(v.now); ok {
		t.Error("登录失效的账号还在排程")
	}
}

func TestTaskHittingNotLoginStopsTheRest(t *testing.T) {
	second := false
	useTasks(t,
		toggle(TaskDaily, "每日任务", func(x *Ctx) {
			x.Fail("分享", &bili.Error{Code: -101, Message: "账号未登录"})
			if !x.Aborted() {
				t.Error("Aborted 应该为 true")
			}
		}),
		toggle(TaskManga, "漫画", func(x *Ctx) { second = true }),
	)
	v := newEnv(t)
	v.login(42)
	v.run(42)
	if second {
		t.Error("登录失效后后面的任务还是跑了")
	}
	if v.e.State().Accounts[0].LoggedIn {
		t.Error("账号应该标成登录失效")
	}
}

func TestPlanRespectsSettingsAndPerAccountSwitches(t *testing.T) {
	var ran []string
	mk := func(id string, on bool, manual bool) Task {
		return Task{
			Info:    api.TaskInfo{ID: id, Title: id, Manual: manual},
			Enabled: func(*api.Settings) bool { return on },
			Run:     func(x *Ctx) { ran = append(ran, id) },
		}
	}
	useTasks(t, mk(TaskDaily, true, false), mk(TaskManga, false, false), mk(TaskSilver2Coin, true, false), mk(TaskUnfollow, true, true))
	v := newEnv(t)
	a := v.login(42)
	a.update(func(st *accountState) { st.DisabledTasks = []string{TaskSilver2Coin} })

	v.run(42)
	if strings.Join(ran, ",") != TaskDaily {
		t.Errorf("每日计划跑了 %v", ran)
	}
	// 点名要跑的任务不看开关
	ran = nil
	v.run(42, TaskManga, TaskUnfollow)
	if strings.Join(ran, ",") != TaskManga+","+TaskUnfollow {
		t.Errorf("点名运行跑了 %v", ran)
	}
}

func TestNextDue(t *testing.T) {
	useTasks(t, toggle(TaskLiveLottery, "天选时刻", func(x *Ctx) {}))
	v := newEnv(t)
	a := v.login(42)
	v.set(func(s *api.Settings) { s.RunAt, s.Jitter = "09:30", 0 })
	at := func(h, m int) time.Time { return time.Date(2026, 11, 13, h, m, 0, 0, Beijing) }

	when, _, trigger, ok := a.nextDue(at(8, 0))
	if !ok || !when.Equal(at(9, 30)) || trigger != "schedule" {
		t.Errorf("到点之前：%v %s %v", when, trigger, ok)
	}
	when, _, trigger, _ = a.nextDue(at(12, 0))
	if !when.Equal(at(9, 30)) || trigger != "catchup" {
		t.Errorf("错过之后：%v %s", when, trigger)
	}
	a.update(func(st *accountState) { st.DailyDate = "2026-11-13" })
	when, ids, _, _ := a.nextDue(at(12, 0))
	if !when.Equal(at(9, 30).AddDate(0, 0, 1)) || ids != nil {
		t.Errorf("今天跑过之后：%v %v", when, ids)
	}

	// 天选时刻按自己的间隔单独跑
	v.set(func(s *api.Settings) { s.Tasks.LiveLottery.Enabled, s.Tasks.LiveLottery.EveryHours = true, 6 })
	a.update(func(st *accountState) { st.LotteryAt = at(10, 0) })
	when, ids, _, _ = a.nextDue(at(12, 0))
	if !when.Equal(at(16, 0)) || len(ids) != 1 || ids[0] != TaskLiveLottery {
		t.Errorf("天选时刻：%v %v", when, ids)
	}

	a.update(func(st *accountState) { st.Enabled = false })
	if _, _, _, ok := a.nextDue(at(12, 0)); ok {
		t.Error("停用的账号还在排程")
	}
}

func TestJitterIsStablePerAccountAndDayAndWithinRange(t *testing.T) {
	set := DefaultSettings()
	set.RunAt, set.Jitter = "09:30", 60
	base := time.Date(2026, 11, 13, 9, 30, 0, 0, Beijing)
	seen := map[time.Duration]bool{}
	for d := range 30 {
		date := base.AddDate(0, 0, d)
		got := dailyTime(&set, "42", date)
		if !got.Equal(dailyTime(&set, "42", date.Add(5*time.Hour))) {
			t.Fatalf("同一天算出了不同的时间")
		}
		off := got.Sub(date)
		if off < 0 || off > time.Hour {
			t.Fatalf("推迟了 %v", off)
		}
		seen[off] = true
	}
	if len(seen) < 20 {
		t.Errorf("30 天里只有 %d 种推迟，不够随机", len(seen))
	}
}

func TestNetworkFailureSchedulesRetry(t *testing.T) {
	useTasks(t, toggle(TaskDaily, "每日任务", func(x *Ctx) {}))
	v := newEnv(t)
	a := v.login(42)
	down := errors.New("dial tcp: connection refused")
	v.e.cfg.Transport = roundTripper(func(*http.Request) (*http.Response, error) { return nil, down })

	run := a.execute(context.Background(), "schedule", nil)
	if run.Status != api.StatusFailed || run.Tasks[0].Steps[0].Reason != "network" {
		t.Fatalf("run = %+v", run)
	}
	when, _, trigger, ok := a.nextDue(v.now)
	if !ok || trigger != "retry" || !when.Equal(v.now.Add(retryAfter)) {
		t.Errorf("重试：%v %s %v", when, trigger, ok)
	}
	if !v.e.State().Accounts[0].LoggedIn {
		t.Error("网络不通不该当成登录失效")
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCookieIsRefreshedWhenBilibiliAsks(t *testing.T) {
	useTasks(t)
	v := newEnv(t)
	v.login(42)
	v.b.JSON("passport.bilibili.com/x/passport-login/web/cookie/info", map[string]any{"refresh": true, "timestamp": 1700000000000})
	// correspond 的路径每次不同，换成固定的
	inner := v.e.cfg.Transport
	v.e.cfg.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
		if strings.HasPrefix(req.URL.Path, "/correspond/1/") {
			req.URL.Path = "/correspond/1/page"
		}
		return inner.RoundTrip(req)
	})
	v.b.Handle("www.bilibili.com/correspond/1/page", func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Body: `<div id="1-name">rc</div>`}
	})
	v.b.Handle("passport.bilibili.com/x/passport-login/web/cookie/refresh", func(bilitest.Call) bilitest.Reply {
		h := http.Header{}
		h.Add("Set-Cookie", "SESSDATA=fresh; Max-Age=15552000; Path=/")
		h.Add("Set-Cookie", "bili_jct=fresh-csrf; Path=/")
		return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"refresh_token":"fresh-token"}}`}
	})
	v.b.JSON("passport.bilibili.com/x/passport-login/web/confirm/refresh", nil)

	run := v.run(42)
	s := step(t, run.Tasks[0], "Cookie 刷新")
	if s.Status != api.StatusOK {
		t.Fatalf("step = %+v", s)
	}
	sess, err := v.e.loadSession("42")
	if err != nil || sess.Cookies["SESSDATA"] != "fresh" || sess.RefreshToken != "fresh-token" || sess.Cookies["buvid3"] != "buvid3-42" {
		t.Fatalf("session = %+v %v", sess, err)
	}
	if v.e.State().Accounts[0].CookieExpires == nil {
		t.Error("没有记下新的有效期")
	}
	// 同一天不再问第二次
	n := v.b.Count("passport.bilibili.com/x/passport-login/web/cookie/info")
	v.run(42)
	if v.b.Count("passport.bilibili.com/x/passport-login/web/cookie/info") != n {
		t.Error("同一天又问了一次 Cookie 是否要刷新")
	}
}

func TestAPIOverHTTP(t *testing.T) {
	useTasks(t, toggle(TaskDaily, "每日任务", func(x *Ctx) { x.OK("观看视频", "ok") }))
	v := newEnv(t)
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	ctx := context.Background()

	if _, err := api.NewClient(srv.URL, "wrong").State(ctx); !api.IsUnauthorized(err) {
		t.Fatalf("错误的令牌：%v", err)
	}
	c := api.NewClient(srv.URL, "test-token")
	if p, err := c.Ping(ctx); err != nil || p.Version != "test" {
		t.Fatalf("ping: %+v %v", p, err)
	}

	// 扫码登录
	v.b.JSON("passport.bilibili.com/x/passport-login/web/qrcode/generate", map[string]any{"url": "https://example.com/scan?k=1", "qrcode_key": "k"})
	scanned := false
	v.b.Handle("passport.bilibili.com/x/passport-login/web/qrcode/poll", func(bilitest.Call) bilitest.Reply {
		if !scanned {
			return bilitest.OK(map[string]any{"code": 86101})
		}
		h := http.Header{}
		for _, ck := range []string{"SESSDATA=s; Max-Age=15552000; Path=/", "bili_jct=j; Path=/", "DedeUserID=42; Path=/"} {
			h.Add("Set-Cookie", ck)
		}
		return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"code":0,"refresh_token":"rt"}}`}
	})
	v.b.JSON("api.bilibili.com/x/frontend/finger/spi", map[string]any{"b_3": "B3", "b_4": "B4"})
	v.b.JSON(navPath, navData(42))
	v.b.JSON("passport.bilibili.com/x/passport-login/web/cookie/info", map[string]any{"refresh": false})

	q, err := c.StartQR(ctx)
	if err != nil || len(q.PNG) == 0 || q.URL == "" {
		t.Fatalf("start qr: %+v %v", q, err)
	}
	if st, err := c.PollQR(ctx, q.ID); err != nil || st.State != "waiting" {
		t.Fatalf("poll: %+v %v", st, err)
	}
	scanned = true
	st, err := c.PollQR(ctx, q.ID)
	if err != nil || st.State != "success" || st.Mid != "42" {
		t.Fatalf("poll: %+v %v", st, err)
	}
	// 再问一次还是成功，不会再去 B 站轮询
	if st, err := c.PollQR(ctx, q.ID); err != nil || st.State != "success" {
		t.Fatalf("poll again: %+v %v", st, err)
	}
	sess, _ := v.e.loadSession("42")
	if sess.RefreshToken != "rt" || sess.Cookies["buvid3"] != "B3" || sess.Cookies["buvid4"] != "B4" {
		t.Errorf("session = %+v", sess)
	}
	state, err := c.State(ctx)
	if err != nil || len(state.Accounts) != 1 || state.Accounts[0].Uname != "测试用户" || state.Accounts[0].CookieExpires == nil || len(state.Tasks) != 1 {
		t.Fatalf("state: %+v %v", state, err)
	}

	// 设置：不对的值被拒绝并说明原因；只传一部分时其他项不变
	bad := state.Settings
	bad.Tasks.Daily.Coins = 9
	if _, err := c.SaveSettings(ctx, bad); err == nil || !strings.Contains(err.Error(), "0 到 5") {
		t.Errorf("save bad settings: %v", err)
	}
	good := state.Settings
	good.RunAt = "07:05"
	if got, err := c.SaveSettings(ctx, good); err != nil || got.RunAt != "07:05" || !got.Tasks.Daily.Enabled {
		t.Errorf("save settings: %+v %v", got, err)
	}

	// 运行并取报告
	run := v.run(42)
	runs, err := c.Runs(ctx, "", 10)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	detail, err := c.RunDetail(ctx, run.ID)
	if err != nil || len(detail.Tasks) != 2 || detail.Tasks[1].Steps[0].Name != "观看视频" {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	if _, err := c.RunDetail(ctx, "../../token"); err == nil {
		t.Error("不合法的报告 ID 没有被拒绝")
	}
	logs, err := c.Logs(ctx, 0, "info", "42")
	if err != nil || len(logs) == 0 {
		t.Fatalf("logs: %v %v", logs, err)
	}

	// 开关和退出
	off := false
	if err := c.Patch(ctx, "42", api.AccountPatch{Enabled: &off, DisabledTasks: &[]string{TaskDaily}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Patch(ctx, "42", api.AccountPatch{DisabledTasks: &[]string{"nope"}}); err == nil {
		t.Error("不存在的任务没有被拒绝")
	}
	state, _ = c.State(ctx)
	if a := state.Accounts[0]; a.Enabled || len(a.DisabledTasks) != 1 || a.NextRun != nil {
		t.Errorf("after patch: %+v", a)
	}
	v.b.JSON("passport.bilibili.com/login/exit/v2", nil)
	if err := c.Remove(ctx, "42"); err != nil {
		t.Fatal(err)
	}
	if v.b.Calls("passport.bilibili.com/login/exit/v2")[0].Form.Get("biliCSRF") != "j" {
		t.Error("退出时没有带 biliCSRF")
	}
	if state, _ = c.State(ctx); len(state.Accounts) != 0 || len(v.e.listAccounts()) != 0 {
		t.Errorf("账号没有删掉：%+v", state.Accounts)
	}
}

func TestNotifyOnlyWhenConfigured(t *testing.T) {
	useTasks(t, toggle(TaskDaily, "每日任务", func(x *Ctx) {
		x.Fail("投币", &bili.Error{Code: -412, Message: "请求被拦截"})
	}))
	v := newEnv(t)
	a := v.login(42)
	sent := make(chan bilitest.Call, 4)
	v.b.Handle("api.day.app/KEY", func(c bilitest.Call) bilitest.Reply {
		sent <- c
		return bilitest.Reply{Body: `{"code":200,"message":"success"}`}
	})

	// 没选渠道：不推送
	a.execute(context.Background(), "schedule", nil)
	v.set(func(s *api.Settings) { s.Notify = api.Notify{When: "failed", Channel: "bark", Token: "KEY"} })
	// 手动运行不推送
	v.run(42)
	a.execute(context.Background(), "schedule", nil)
	select {
	case c := <-sent:
		body := string(c.Body)
		if !strings.Contains(body, "有任务没完成") && !strings.Contains(body, "任务失败") {
			t.Errorf("标题不对：%s", body)
		}
		if !strings.Contains(body, "风控") {
			t.Errorf("正文里没有失败原因：%s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到推送")
	}
	select {
	case c := <-sent:
		t.Errorf("多推送了一次：%s", c.Body)
	case <-time.After(100 * time.Millisecond):
	}
}
