//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
	"github.com/VOfficial404/bili-tools/engine"
	"github.com/egoist/mygo/ui"
)

type memStore struct {
	p      prefs
	secret string
	dir    string
}

func (s *memStore) load() prefs              { return s.p }
func (s *memStore) save(p prefs) error       { s.p = p; return nil }
func (s *memStore) token() (string, error)   { return s.secret, nil }
func (s *memStore) setToken(v string) error  { s.secret = v; return nil }
func (s *memStore) dataDir() (string, error) { return s.dir, nil }

// 后台请求只排队；等待结束后在测试线程应用，代替窗口的 Update。
type harness struct {
	a     *app
	tt    *ui.Tester
	t     *testing.T
	mu    sync.Mutex
	queue []func()
}

func newHarness(t *testing.T, s store) *harness {
	t.Helper()
	h := &harness{t: t}
	h.a = newApp(s, func(fn func()) {
		h.mu.Lock()
		h.queue = append(h.queue, fn)
		h.mu.Unlock()
	})
	h.a.fetchImage = func(string) ([]byte, error) { return nil, errors.New("测试不下载头像") }
	h.tt = ui.NewTester(h.a.view, 1120, 2600)
	t.Cleanup(func() {
		h.a.disconnect()
		h.settle()
		h.a.cancel()
	})
	return h
}

func (h *harness) settle() {
	h.t.Helper()
	for range 30 {
		h.a.wg.Wait()
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		if len(q) == 0 {
			h.tt.Frame()
			return
		}
		for _, fn := range q {
			fn()
		}
	}
	h.t.Fatal("后台请求未能结束")
}

func (h *harness) click(label string) {
	h.t.Helper()
	if err := h.tt.Click(label); err != nil {
		h.t.Fatalf("点击 %q：%v；界面文本：%q", label, err, h.tt.Texts())
	}
	h.settle()
}

func (h *harness) must(text string) {
	h.t.Helper()
	if !h.tt.HasText(text) {
		h.t.Fatalf("界面缺少 %q；界面文本：%q", text, h.tt.Texts())
	}
}

func noSleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

func newTestEngine(t *testing.T) (*engine.Engine, *bilitest.Server, string) {
	t.Helper()
	b := bilitest.New()
	dir := t.TempDir()
	e, err := engine.New(engine.Config{DataDir: dir, Version: "测试服务端", Token: "desktop-test-token", Transport: b, Sleep: noSleep})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Stop)
	srv := httptest.NewServer(e.Handler())
	t.Cleanup(srv.Close)
	return e, b, srv.URL
}

func (h *harness) connect(base, token string) {
	h.t.Helper()
	h.a.server, h.a.token = base, token
	h.tt.Frame()
	h.click("连接")
	if h.a.cl == nil {
		h.t.Fatalf("连接失败：%s", h.a.connectErr)
	}
}

func prepareQR(b *bilitest.Server) {
	b.JSON("passport.bilibili.com/x/passport-login/web/qrcode/generate", map[string]any{"url": "https://example.com/scan?k=desktop", "qrcode_key": "desktop"})
	b.Handle("passport.bilibili.com/x/passport-login/web/qrcode/poll", func(bilitest.Call) bilitest.Reply {
		headers := http.Header{}
		for _, cookie := range []string{"SESSDATA=s; Max-Age=15552000; Path=/", "bili_jct=j; Path=/", "DedeUserID=42; Path=/"} {
			headers.Add("Set-Cookie", cookie)
		}
		return bilitest.Reply{Header: headers, Body: `{"code":0,"data":{"code":0,"refresh_token":"rt"}}`}
	})
	b.JSON("api.bilibili.com/x/frontend/finger/spi", map[string]any{"b_3": "B3", "b_4": "B4"})
	b.JSON("api.bilibili.com/x/web-interface/nav", map[string]any{
		"isLogin": true, "mid": 42, "uname": "测试用户", "face": "https://i0.hdslb.com/bfs/face/x.jpg", "money": 100.0,
		"vipStatus": 0, "vipType": 0, "vipDueDate": 0,
		"level_info": map[string]any{"current_level": 4, "current_exp": 5000, "next_exp": 10800},
		"wallet":     map[string]any{"bcoin_balance": 0, "coupon_balance": 0},
		"wbi_img":    map[string]any{"img_url": "https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png", "sub_url": "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"},
	})
	b.JSON("passport.bilibili.com/x/passport-login/web/cookie/info", map[string]any{"refresh": false})
}

func (h *harness) login(b *bilitest.Server) {
	h.t.Helper()
	prepareQR(b)
	h.click("扫码添加账号")
	if h.a.qr.ID == "" || h.a.qrImage == nil {
		h.t.Fatalf("二维码未生成：%s", h.a.qrErr)
	}
	h.must("用 B 站 App 扫一扫")
	h.a.pollQR()
	h.settle()
	h.must("测试用户")
	h.must("UID 42")
	if h.a.qrOpen || len(h.a.st.Accounts) != 1 || !h.a.st.Accounts[0].LoggedIn {
		h.t.Fatal("扫码成功后账号状态不正确")
	}
}

func TestWrongTokenText(t *testing.T) {
	_, _, base := newTestEngine(t)
	h := newHarness(t, &memStore{dir: t.TempDir()})
	h.a.server, h.a.token = base, "错误令牌"
	h.tt.Frame()
	h.click("连接")
	h.must("令牌不对，请检查服务器令牌")
	if h.a.cl != nil {
		t.Fatal("错误令牌建立了连接")
	}
}

func TestConnectAndQRLogin(t *testing.T) {
	e, b, base := newTestEngine(t)
	s := &memStore{dir: t.TempDir()}
	h := newHarness(t, s)
	h.a.remember = true
	h.connect(base, e.Token())
	h.must("还没有账号")
	if s.secret != e.Token() || s.p.Server != base {
		t.Fatal("连接信息没有保存")
	}
	h.login(b)
}

func TestSettingsValidationAndPersistence(t *testing.T) {
	e, _, base := newTestEngine(t)
	h := newHarness(t, &memStore{dir: t.TempDir()})
	h.connect(base, e.Token())
	h.click("设置")
	original := e.Settings().RunAt
	h.a.form.RunAt, h.a.dirty = "25:99", true
	h.tt.Frame()
	h.click("保存")
	h.must("开始时间要写成 HH:MM，例如 09:30")
	if e.Settings().RunAt != original || !h.a.dirty {
		t.Fatal("无效设置改变了已保存配置")
	}
	h.a.form.RunAt = "07:05"
	h.tt.Frame()
	h.click("保存")
	h.must("设置已保存")
	if h.a.dirty || h.a.form.RunAt != "07:05" || e.Settings().RunAt != "07:05" {
		t.Fatal("有效设置没有通过接口保存")
	}
	reopened, err := engine.New(engine.Config{DataDir: e.DataDir(), Transport: bilitest.New(), Sleep: noSleep})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Stop()
	if reopened.Settings().RunAt != "07:05" {
		t.Fatal("设置没有写入数据目录")
	}
}

func TestFailedReportHint(t *testing.T) {
	e, b, base := newTestEngine(t)
	h := newHarness(t, &memStore{dir: t.TempDir()})
	h.connect(base, e.Token())
	h.login(b)
	b.Fail("api.bilibili.com/x/web-interface/nav", -352, "风控校验失败")
	run, err := e.RunNow(context.Background(), "42", nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != api.StatusFailed || len(run.Tasks) == 0 || len(run.Tasks[0].Steps) == 0 {
		t.Fatalf("没有失败报告：%+v", run)
	}
	step := run.Tasks[0].Steps[0]
	if step.Hint == "" || step.Code != -352 {
		t.Fatalf("失败报告缺少建议或返回码：%+v", step)
	}
	h.click("报告")
	if len(h.a.runs) != 1 || h.a.runs[0].ID != run.ID {
		t.Fatal("未通过接口取得实际报告列表")
	}
	h.a.openReport(run.ID)
	h.settle()
	if h.a.report == nil || h.a.report.ID != run.ID {
		t.Fatal("未通过接口取得实际报告详情")
	}
	h.must("建议：" + step.Hint)
	h.must("返回码 -352")
}

func TestLocalModeStopsHTTP(t *testing.T) {
	h := newHarness(t, &memStore{p: prefs{Mode: 1}, dir: t.TempDir()})
	b := bilitest.New()
	created := false
	h.a.newEngine = func(cfg engine.Config) (*engine.Engine, error) {
		created = true
		cfg.Transport, cfg.Sleep, cfg.LogOutput = b, noSleep, nil
		return engine.New(cfg)
	}
	h.click("开始运行")
	if !created || h.a.localEngine == nil || h.a.cl == nil {
		t.Fatalf("本机服务未启动：%s", h.a.connectErr)
	}
	h.must("这台电脑（本机运行）")
	old := h.a.cl
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := old.State(ctx); err != nil {
		t.Fatalf("本机接口不可用：%v", err)
	}
	h.click("设置")
	h.click("停止本机运行")
	if h.a.localEngine != nil || h.a.cl != nil || h.a.connecting {
		t.Fatal("本机服务停止后界面状态未清理")
	}
	h.must("开始运行")
	if _, err := old.State(ctx); err == nil {
		t.Fatal("停止本机服务后旧 HTTP 接口仍可访问")
	}
}

func TestVersionMatchesManifest(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version == "" || manifest.Version != version {
		t.Fatalf("客户端版本 %q 与 mygo.json 版本 %q 不一致", version, manifest.Version)
	}
}
