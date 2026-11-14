package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
	"github.com/VOfficial404/bili-daily/engine"
)

const zonePath = "api.bilibili.com/x/web-interface/zone"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func localConfig(t *testing.T) (engine.Config, *bilitest.Server) {
	t.Helper()
	b := bilitest.New()
	b.JSON(zonePath, map[string]any{"addr": "203.0.113.7", "country": "中国", "province": "广东", "city": "深圳"})
	return engine.Config{
		DataDir: t.TempDir(), Version: "test", Token: "session-test-token", Transport: b,
		Now:   func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, engine.Beijing) },
		Sleep: func(ctx context.Context, d time.Duration) error { return ctx.Err() },
	}, b
}

func remoteEngine(t *testing.T, wrap func(http.Handler) http.Handler) (*engine.Engine, *httptest.Server) {
	t.Helper()
	cfg, _ := localConfig(t)
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := e.Handler()
	if wrap != nil {
		h = wrap(h)
	}
	server := httptest.NewServer(h)
	t.Cleanup(func() { server.Close(); e.Stop() })
	return e, server
}

func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("等待后台操作超时")
	}
}

func assertRunning(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("操作在本机清理完成前已经返回")
	default:
	}
}

func TestSessionConnect(t *testing.T) {
	e, server := remoteEngine(t, nil)
	var s Session
	t.Cleanup(s.Stop)
	if s.Client() != nil || s.LocalEngine() != nil || !reflect.DeepEqual(s.Snapshot(), api.State{}) {
		t.Fatal("零值会话不为空")
	}
	s.Stop()
	if _, err := s.Refresh(testContext(t)); err == nil {
		t.Fatal("未连接时刷新应该失败")
	}
	if _, err := s.SaveSettings(testContext(t), api.Settings{}, api.Settings{}); err == nil {
		t.Fatal("未连接时保存应该失败")
	}
	if _, err := s.Connect(testContext(t), server.URL, "wrong-token"); !api.IsUnauthorized(err) {
		t.Fatalf("错误令牌：%v", err)
	}
	if s.Client() != nil {
		t.Fatal("认证失败后不应发布客户端")
	}
	st, err := s.Connect(testContext(t), server.URL+"/", e.Token())
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != "test" || s.Client().Base() != server.URL || s.LocalEngine() != nil {
		t.Fatalf("连接状态不对：%+v", st)
	}
	s.Stop()
	if s.Client() != nil || !reflect.DeepEqual(s.Snapshot(), api.State{}) {
		t.Fatal("停止后仍有旧连接")
	}
}

func TestSessionLocalLifecycleAndProxy(t *testing.T) {
	for _, name := range []string{"保留", "直连", "替换"} {
		t.Run(name, func(t *testing.T) {
			cfg, _ := localConfig(t)
			seed, err := engine.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			settings := seed.Settings()
			settings.Proxy = "http://127.0.0.1:7890"
			if _, err := seed.UpdateSettings(settings); err != nil {
				seed.Stop()
				t.Fatal(err)
			}
			seed.Stop()
			want := settings.Proxy
			var proxy *string
			if name == "直连" {
				want = ""
				proxy = &want
			} else if name == "替换" {
				want = "http://127.0.0.1:7891"
				proxy = &want
			}
			var s Session
			t.Cleanup(s.Stop)
			st, err := s.StartLocal(testContext(t), LocalOptions{Config: cfg, Proxy: proxy})
			if err != nil {
				t.Fatal(err)
			}
			if !st.Local || st.Settings.Proxy != want || s.LocalEngine() == nil {
				t.Fatalf("本机状态不对：%+v", st)
			}
			cl := s.Client()
			if _, err := cl.State(testContext(t)); err != nil {
				t.Fatal(err)
			}
			s.Stop()
			if _, err := cl.Ping(testContext(t)); err == nil {
				t.Fatal("停止后本机 HTTP 接口仍然可用")
			}
			if s.Client() != nil || s.LocalEngine() != nil {
				t.Fatal("停止后仍然发布本机连接")
			}
		})
	}
}

func TestSessionStopWaitsForStartup(t *testing.T) {
	cfg, _ := localConfig(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var s Session
	t.Cleanup(s.Stop)
	t.Cleanup(unblock)
	started := make(chan error, 1)
	go func() {
		_, err := s.StartLocal(context.Background(), LocalOptions{Config: cfg, NewEngine: func(cfg engine.Config) (*engine.Engine, error) {
			close(entered)
			<-release
			return engine.New(cfg)
		}})
		started <- err
	}()
	await(t, entered)
	s.mu.Lock()
	invalidated := s.current.ctx.Done()
	s.mu.Unlock()
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	await(t, invalidated)
	assertRunning(t, stopped)
	unblock()
	await(t, stopped)
	if err := <-started; !errors.Is(err, context.Canceled) {
		t.Fatalf("被停止的启动返回 %v", err)
	}
	if s.Client() != nil || s.LocalEngine() != nil {
		t.Fatal("旧启动在 Stop 之后发布了连接")
	}
}

func TestSessionConcurrentStopWaitsForShutdown(t *testing.T) {
	cfg, b := localConfig(t)
	entered, release, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var s Session
	t.Cleanup(s.Stop)
	t.Cleanup(unblock)
	cfg.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
		<-release
		return b.RoundTrip(r)
	})
	if _, err := s.StartLocal(testContext(t), LocalOptions{Config: cfg}); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	first, second := make(chan struct{}), make(chan struct{})
	go func() { s.Stop(); close(first) }()
	await(t, canceled)
	go func() { s.Stop(); close(second) }()
	assertRunning(t, first)
	assertRunning(t, second)
	unblock()
	await(t, first)
	await(t, second)
}

func TestSessionLocalFailureCleansEngine(t *testing.T) {
	cfg, b := localConfig(t)
	entered, canceled := make(chan struct{}), make(chan struct{})
	cfg.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
		return b.RoundTrip(r)
	})
	failure := errors.New("创建引擎失败")
	var s Session
	t.Cleanup(s.Stop)
	_, err := s.StartLocal(testContext(t), LocalOptions{Config: cfg, NewEngine: func(cfg engine.Config) (*engine.Engine, error) {
		e, err := engine.New(cfg)
		if err != nil {
			return nil, err
		}
		e.Start()
		await(t, entered)
		return e, failure
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("启动错误：%v", err)
	}
	await(t, canceled)
	if s.Client() != nil || s.LocalEngine() != nil {
		t.Fatal("失败后仍有本机连接")
	}
}

// 暂扣一次已生成的真实引擎响应，保存请求仍可通过同一个接口执行。
func TestSessionStaleRefreshPreservesSavedSettings(t *testing.T) {
	var hold atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	e, server := remoteEngine(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/state" && hold.CompareAndSwap(true, false) {
				recorded := httptest.NewRecorder()
				next.ServeHTTP(recorded, r)
				close(entered)
				<-release
				for key, values := range recorded.Header() {
					w.Header()[key] = values
				}
				w.WriteHeader(recorded.Code)
				_, _ = w.Write(recorded.Body.Bytes())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	t.Cleanup(unblock)
	var s Session
	t.Cleanup(s.Stop)
	initial, err := s.Connect(testContext(t), server.URL, e.Token())
	if err != nil {
		t.Fatal(err)
	}
	hold.Store(true)
	refreshed := make(chan error, 1)
	go func() { _, err := s.Refresh(testContext(t)); refreshed <- err }()
	await(t, entered)
	edited := CloneSettings(initial.Settings)
	edited.RunAt = "10:45"
	saved, err := s.SaveSettings(testContext(t), initial.Settings, edited)
	if err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Settings; !reflect.DeepEqual(got, saved) {
		t.Fatalf("旧刷新覆盖了设置：%+v，期望 %+v", got, saved)
	}
}

func TestSessionSaveMergesLatestServerEdits(t *testing.T) {
	e, server := remoteEngine(t, nil)
	var s Session
	t.Cleanup(s.Stop)
	initial, err := s.Connect(testContext(t), server.URL, e.Token())
	if err != nil {
		t.Fatal(err)
	}
	other := api.NewClient(server.URL, e.Token())
	latest := CloneSettings(initial.Settings)
	latest.Tasks.Daily.UpIDs = []int64{11, 22}
	latest.Tasks.LiveLottery.Exclude = []string{"其他客户端"}
	latest.Notify.When = "always"
	if _, err := other.SaveSettings(testContext(t), latest); err != nil {
		t.Fatal(err)
	}
	edited := CloneSettings(initial.Settings)
	edited.RunAt = "11:20"
	edited.Tasks.Daily.Coins = 2
	saved, err := s.SaveSettings(testContext(t), initial.Settings, edited)
	if err != nil {
		t.Fatal(err)
	}
	want := CloneSettings(latest)
	want.RunAt, want.Tasks.Daily.Coins = "11:20", 2
	if !reflect.DeepEqual(saved, want) {
		t.Fatalf("合并设置：%+v，期望 %+v", saved, want)
	}
	serverState, err := other.State(testContext(t))
	if err != nil || !reflect.DeepEqual(serverState.Settings, want) {
		t.Fatalf("服务端设置：%+v，错误 %v", serverState.Settings, err)
	}
	// 返回的设置也不能和会话共享数组。
	saved.Tasks.Daily.UpIDs[0] = 99
	if got := s.Snapshot().Settings.Tasks.Daily.UpIDs[0]; got != 11 {
		t.Fatalf("返回值改到了会话：%d", got)
	}
}

func TestSessionSnapshotCopiesNestedState(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, engine.Beijing)
	original := api.State{
		Settings: engine.DefaultSettings(),
		Tasks:    []api.TaskInfo{{ID: "daily"}},
		Egress:   &api.Egress{Addr: "203.0.113.7"},
		Accounts: []api.Account{{
			Mid: "123", DisabledTasks: []string{"daily"}, Today: []api.TaskStatus{{ID: "daily"}},
			VIPDue: &now, CookieExpires: &now, UpdatedAt: &now, NextRun: &now,
			Running: &api.Progress{Task: "每日任务"}, LastRun: &api.RunSummary{ID: "old"},
		}},
	}
	original.Settings.Tasks.Daily.UpIDs = []int64{1}
	original.Settings.Tasks.LiveLottery.Include = []string{"包含"}
	original.Settings.Tasks.Unfollow.Keep = []int64{2}
	want := cloneState(original)
	s := NewSession(nil, original)
	t.Cleanup(s.Stop)
	mutate := func(st *api.State) {
		st.Settings.Tasks.Daily.UpIDs[0] = 9
		st.Settings.Tasks.LiveLottery.Include[0] = "改动"
		st.Settings.Tasks.LiveLottery.Exclude[0] = "改动"
		st.Settings.Tasks.Unfollow.Keep[0] = 9
		st.Tasks[0].ID = "changed"
		st.Egress.Addr = "changed"
		a := &st.Accounts[0]
		a.Mid, a.DisabledTasks[0], a.Today[0].ID = "changed", "changed", "changed"
		*a.VIPDue = time.Time{}
		*a.CookieExpires = time.Time{}
		*a.UpdatedAt = time.Time{}
		*a.NextRun = time.Time{}
		a.Running.Task, a.LastRun.ID = "changed", "changed"
	}
	mutate(&original)
	if got := s.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatal("NewSession 保留了传入状态的引用")
	}
	copied := s.Snapshot()
	mutate(&copied)
	if got := s.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatal("Snapshot 返回了会话内部的引用")
	}
}

func TestNeedsChargeConfirm(t *testing.T) {
	cases := []struct {
		name               string
		baseOn, editedOn   bool
		baseUID, editedUID int64
		want               bool
	}{
		{"保持关闭", false, false, 0, 123, false},
		{"新启用", false, true, 123, 123, true},
		{"启用时未填 UID", false, true, 0, 0, true},
		{"保持对象", true, true, 123, 123, false},
		{"更换对象", true, true, 123, 456, true},
		{"关闭并更换", true, false, 123, 456, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, edited := engine.DefaultSettings(), engine.DefaultSettings()
			base.Tasks.Charge = api.ChargeOptions{Enabled: tc.baseOn, UpID: tc.baseUID}
			edited.Tasks.Charge = api.ChargeOptions{Enabled: tc.editedOn, UpID: tc.editedUID, Day: 1, Comment: "留言"}
			if got := NeedsChargeConfirm(base, edited); got != tc.want {
				t.Fatalf("需要确认 = %v，期望 %v", got, tc.want)
			}
		})
	}
}

func TestSessionSaveWaitIsCancelable(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var saves atomic.Int32
	e, server := remoteEngine(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && r.URL.Path == "/api/settings" && saves.Add(1) == 1 {
				close(entered)
				<-release
			}
			next.ServeHTTP(w, r)
		})
	})
	t.Cleanup(unblock)
	var s Session
	t.Cleanup(s.Stop)
	initial, err := s.Connect(testContext(t), server.URL, e.Token())
	if err != nil {
		t.Fatal(err)
	}
	first := CloneSettings(initial.Settings)
	first.RunAt = "10:15"
	done := make(chan error, 1)
	go func() { _, err := s.SaveSettings(testContext(t), initial.Settings, first); done <- err }()
	await(t, entered)
	second := CloneSettings(initial.Settings)
	second.Jitter = 12
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.SaveSettings(ctx, initial.Settings, second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等待保存时取消：%v", err)
	}
	if got := saves.Load(); got != 1 {
		t.Fatalf("前一个保存完成前又发出了请求：%d", got)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveSettings(testContext(t), initial.Settings, second)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RunAt != first.RunAt || saved.Jitter != second.Jitter {
		t.Fatalf("连续表单保存丢失改动：%+v", saved)
	}
}

func TestSessionCanceledStartupContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var s Session
	t.Cleanup(s.Stop)
	cfg, _ := localConfig(t)
	called := false
	if _, err := s.StartLocal(ctx, LocalOptions{Config: cfg, NewEngine: func(cfg engine.Config) (*engine.Engine, error) {
		called = true
		return engine.New(cfg)
	}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消的本机启动：%v", err)
	}
	if called {
		t.Fatal("取消后仍创建了本机引擎")
	}
	if _, err := s.Connect(ctx, "http://127.0.0.1:1", "token"); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消的连接：%v", err)
	}
}

func TestSessionStopInvalidatesRefresh(t *testing.T) {
	var hold atomic.Bool
	entered := make(chan struct{})
	e, server := remoteEngine(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/state" && hold.Load() {
				close(entered)
				<-r.Context().Done()
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	var s Session
	t.Cleanup(s.Stop)
	if _, err := s.Connect(testContext(t), server.URL, e.Token()); err != nil {
		t.Fatal(err)
	}
	hold.Store(true)
	done := make(chan error, 1)
	go func() { _, err := s.Refresh(testContext(t)); done <- err }()
	await(t, entered)
	s.Stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("旧刷新没有返回取消原因：%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("停止后旧刷新仍未退出")
	}
	if !reflect.DeepEqual(s.Snapshot(), api.State{}) {
		t.Fatal("旧刷新恢复了停止前的状态")
	}
}
