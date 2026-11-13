package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

const refreshPath = "passport.bilibili.com/x/passport-login/web/cookie/refresh"
const confirmPath = "passport.bilibili.com/x/passport-login/web/confirm/refresh"

func prepareRefresh(v *testEnv) {
	v.b.JSON("passport.bilibili.com/x/passport-login/web/cookie/info", map[string]any{"refresh": true, "timestamp": 1700000000000})
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
}

func refreshedSessionReply() bilitest.Reply {
	h := http.Header{}
	h.Add("Set-Cookie", "SESSDATA=fresh; Path=/")
	h.Add("Set-Cookie", "bili_jct=fresh-csrf; Path=/")
	return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"refresh_token":"fresh-token"}}`}
}

func TestRefreshPersistsBeforeConfirmAndKeepsNewSessionOnConfirmFailure(t *testing.T) {
	useTasks(t)
	v := newEnv(t)
	v.login(42)
	prepareRefresh(v)
	v.b.Handle(refreshPath, func(bilitest.Call) bilitest.Reply { return refreshedSessionReply() })
	v.b.Handle(confirmPath, func(c bilitest.Call) bilitest.Reply {
		s, err := v.e.loadSession("42")
		if err != nil || s.RefreshToken != "fresh-token" || s.Cookies["SESSDATA"] != "fresh" {
			t.Errorf("confirmation preceded persistence: %+v %v", s, err)
		}
		if c.Form.Get("refresh_token") != "refresh-42" || c.Cookie["SESSDATA"] != "fresh" {
			t.Errorf("confirmation credentials: %+v", c)
		}
		return bilitest.Code(-400, "confirmation failed")
	})
	run := v.run(42)
	if s := step(t, run.Tasks[0], "Cookie 刷新"); s.Status != api.StatusOK {
		t.Fatalf("confirmation failure discarded successful refresh: %+v", s)
	}
	s, err := v.e.loadSession("42")
	if err != nil || s.RefreshToken != "fresh-token" || v.b.Count(confirmPath) != 1 {
		t.Fatalf("session=%+v error=%v confirmations=%d", s, err, v.b.Count(confirmPath))
	}
}

func TestRefreshSaveFailureNeverConfirms(t *testing.T) {
	useTasks(t)
	v := newEnv(t)
	v.login(42)
	prepareRefresh(v)
	v.b.Handle(refreshPath, func(bilitest.Call) bilitest.Reply {
		if err := os.Remove(v.e.sessionFile("42")); err != nil {
			t.Error(err)
		}
		if err := os.Mkdir(v.e.sessionFile("42"), 0o700); err != nil {
			t.Error(err)
		}
		return refreshedSessionReply()
	})
	v.b.JSON(confirmPath, nil)
	run := v.run(42)
	if s := step(t, run.Tasks[0], "Cookie 刷新"); s.Status != api.StatusFailed || !strings.Contains(s.Detail, "保存失败") {
		t.Fatalf("save failure not reported: %+v", s)
	}
	if v.b.Count(confirmPath) != 0 {
		t.Fatal("old credentials invalidated despite save failure")
	}
}

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for operation")
	}
}

func TestRemoveClosesAdmissionAndWaitsForRunBeforeLogout(t *testing.T) {
	entered, canceled, releaseRun := make(chan struct{}), make(chan struct{}), make(chan struct{})
	useTasks(t, toggle(TaskDaily, "blocked", func(x *Ctx) {
		close(entered)
		<-x.Done()
		close(canceled)
		<-releaseRun // simulate a task finishing its own work after cancellation
	}))
	v := newEnv(t)
	a := v.login(42)
	finished := make(chan struct{})
	go func() { defer close(finished); v.e.RunNow(context.Background(), "42", nil) }()
	awaitSignal(t, entered)
	logout, releaseLogout := make(chan struct{}), make(chan struct{})
	v.b.Handle("passport.bilibili.com/login/exit/v2", func(bilitest.Call) bilitest.Reply {
		close(logout)
		<-releaseLogout
		return bilitest.OK(nil)
	})
	removed := make(chan error, 1)
	go func() { removed <- v.e.removeAccount(context.Background(), "42") }()
	awaitSignal(t, canceled)
	if a.runNow(nil) || a.execute(context.Background(), "manual", nil) != nil {
		t.Error("closing account admitted a new run")
	}
	if _, _, _, ok := a.nextDue(v.now); ok {
		t.Error("closing account remained schedulable")
	}
	select {
	case <-logout:
		t.Error("logout started before active run completed")
	default:
	}
	close(releaseRun)
	awaitSignal(t, finished)
	awaitSignal(t, logout)
	if _, err := v.e.RunNow(context.Background(), "42", nil); !errors.Is(err, errBusy) {
		t.Errorf("run during blocked logout: %v", err)
	}
	close(releaseLogout)
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
	if err := a.update(func(st *accountState) { st.Uname = "recreated" }); !errors.Is(err, errNoAccount) {
		t.Errorf("post-removal update: %v", err)
	}
	if _, err := os.Stat(v.e.accountDir("42")); !errors.Is(err, os.ErrNotExist) || v.e.account("42") != nil {
		t.Fatalf("removed account recreated: %v", err)
	}
}

func TestCanceledHTTPRemovalStillFinishesCleanup(t *testing.T) {
	v := newEnv(t)
	a := v.login(42)
	entered, release := make(chan struct{}), make(chan struct{})
	v.b.Handle("passport.bilibili.com/login/exit/v2", func(bilitest.Call) bilitest.Reply {
		close(entered)
		<-release
		return bilitest.OK(nil)
	})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodDelete, "/api/accounts/42", nil).WithContext(ctx)
	r.SetPathValue("mid", "42")
	finished := make(chan struct{})
	go func() { defer close(finished); v.e.handleRemove(httptest.NewRecorder(), r) }()
	awaitSignal(t, entered)
	cancel()
	awaitSignal(t, finished)
	if a.runNow(nil) {
		t.Error("canceled removal reopened scheduling")
	}
	close(release)
	waitFor(t, "canceled request cleanup", func() bool { return v.e.account("42") == nil })
	if _, err := os.Stat(v.e.accountDir("42")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("account files remain: %v", err)
	}
}

func TestStopCancelsDirectRunWithIndependentContext(t *testing.T) {
	entered := make(chan struct{})
	useTasks(t, toggle(TaskDaily, "blocked", func(x *Ctx) { close(entered); <-x.Done() }))
	v := newEnv(t)
	v.login(42)
	finished := make(chan struct{})
	go func() { defer close(finished); v.e.RunNow(context.Background(), "42", nil) }()
	awaitSignal(t, entered)
	v.e.Stop()
	awaitSignal(t, finished)
	if _, err := v.e.RunNow(context.Background(), "42", nil); !errors.Is(err, errBusy) {
		t.Fatalf("stopped engine admitted run: %v", err)
	}
}

func TestConcurrentJSONWritesUseIndependentTemporaryFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	start := make(chan struct{})
	errs := make(chan error, 32)
	for i := range 32 {
		go func() {
			<-start
			errs <- writeJSON(path, map[string]any{"id": i, "payload": strings.Repeat(fmt.Sprint(i), 4096)})
		}()
	}
	close(start)
	for range 32 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var got struct {
		ID      int    `json:"id"`
		Payload string `json:"payload"`
	}
	if err := readJSON(path, &got); err != nil || got.Payload != strings.Repeat(fmt.Sprint(got.ID), 4096) {
		t.Fatalf("corrupt final document: %+v %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func TestRejectedSettingsCannotMutateActiveSlices(t *testing.T) {
	v := newEnv(t)
	v.set(func(s *api.Settings) {
		s.Tasks.Daily.UpIDs = []int64{42, 43}
		s.Tasks.LiveLottery.Include = []string{"gift"}
		s.Tasks.LiveLottery.Exclude = []string{"coupon"}
		s.Tasks.Unfollow.Keep = []int64{44}
	})
	before := v.e.Settings()
	w := httptest.NewRecorder()
	v.e.handleSettings(w, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"tasks":{"daily":{"coins":6,"up_ids":[99]}}}`)))
	if w.Code != http.StatusBadRequest || !reflect.DeepEqual(before, v.e.Settings()) {
		t.Fatalf("rejected PUT mutated active settings: status=%d settings=%+v", w.Code, v.e.Settings())
	}
	copy := v.e.Settings()
	copy.Tasks.Daily.UpIDs[0] = 99
	copy.Tasks.LiveLottery.Include[0] = "changed"
	copy.Tasks.LiveLottery.Exclude[0] = "changed"
	copy.Tasks.Unfollow.Keep[0] = 99
	if !reflect.DeepEqual(before, v.e.Settings()) {
		t.Fatal("Settings returned shared slices")
	}
	accepted, err := v.e.UpdateSettings(before)
	if err != nil {
		t.Fatal(err)
	}
	accepted.Tasks.Daily.UpIDs[0] = 100
	before.Tasks.LiveLottery.Include[0] = "input mutated"
	if v.e.Settings().Tasks.Daily.UpIDs[0] != 42 || v.e.Settings().Tasks.LiveLottery.Include[0] != "gift" {
		t.Fatal("accepted settings share caller-owned slices")
	}
}

type gatedSettingsReader struct {
	io.Reader
	once    sync.Once
	entered chan struct{}
	release <-chan struct{}
}

func (r *gatedSettingsReader) Read(b []byte) (int, error) {
	r.once.Do(func() {
		close(r.entered)
		if r.release != nil {
			<-r.release
		}
	})
	return r.Reader.Read(b)
}

func TestConcurrentPartialSettingsPUTsPreserveBothChanges(t *testing.T) {
	v := newEnv(t)
	firstRead, secondRead, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	first := &gatedSettingsReader{Reader: strings.NewReader(`{"run_at":"07:15"}`), entered: firstRead, release: release}
	second := &gatedSettingsReader{Reader: strings.NewReader(`{"jitter":17}`), entered: secondRead}
	one, two := httptest.NewRecorder(), httptest.NewRecorder()
	doneOne, doneTwo := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(doneOne)
		v.e.handleSettings(one, httptest.NewRequest(http.MethodPut, "/api/settings", first))
	}()
	awaitSignal(t, firstRead)
	go func() {
		defer close(doneTwo)
		v.e.handleSettings(two, httptest.NewRequest(http.MethodPut, "/api/settings", second))
	}()
	select {
	case <-secondRead:
		t.Error("second PUT read an uncommitted settings snapshot")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	awaitSignal(t, doneOne)
	awaitSignal(t, doneTwo)
	got := v.e.Settings()
	var disk api.Settings
	if err := readJSON(v.e.settingsFile(), &disk); err != nil {
		t.Fatal(err)
	}
	if one.Code != http.StatusOK || two.Code != http.StatusOK || got.RunAt != "07:15" || got.Jitter != 17 || !reflect.DeepEqual(got, disk) {
		t.Fatalf("lost partial update: status=%d/%d memory=%+v disk=%+v", one.Code, two.Code, got, disk)
	}
}

func TestConcurrentSettingsSavesKeepMemoryAndDiskConsistent(t *testing.T) {
	v := newEnv(t)
	start, errs := make(chan struct{}), make(chan error, 24)
	for i := range 24 {
		go func() {
			<-start
			s := v.e.Settings()
			s.Jitter = i
			_, err := v.e.UpdateSettings(s)
			errs <- err
		}()
	}
	close(start)
	for range 24 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var disk api.Settings
	if err := readJSON(v.e.settingsFile(), &disk); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.e.Settings(), disk) {
		t.Fatal("memory differs from final saved settings")
	}
}

func TestLateDailyJitterStaysInBeijingDayAndSchedulesToday(t *testing.T) {
	v := newEnv(t)
	a := v.login(42)
	v.set(func(s *api.Settings) { s.RunAt, s.Jitter = "23:59", 720 })
	s := v.e.Settings()
	base := time.Date(2026, 11, 13, 23, 59, 0, 0, Beijing)
	for i := range 100 {
		got := dailyTime(&s, fmt.Sprint(i), base)
		if got.Before(base) || got.After(base.Add(59*time.Second)) || day(got) != day(base) {
			t.Fatalf("jitter crossed midnight: %v", got)
		}
	}
	when, _, _, ok := a.nextDue(base.Add(59 * time.Second))
	if !ok || day(when) != day(base) || when.After(base.Add(59*time.Second)) {
		t.Fatalf("today's occurrence was skipped: %v %v", when, ok)
	}
}

func TestAccountUpdateAndPATCHAreTransactionalOnFilesystemFailure(t *testing.T) {
	v := newEnv(t)
	a := v.login(42)
	if err := a.update(func(st *accountState) {
		st.DisabledTasks = []string{TaskDaily}
		st.Today = map[string]api.TaskStatus{"old": {ID: "old"}}
		st.Marks = map[string]string{"old": "value"}
	}); err != nil {
		t.Fatal(err)
	}
	before := a.snapshot()
	if err := os.Remove(v.e.stateFile("42")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(v.e.stateFile("42"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := a.update(func(st *accountState) {
		st.Enabled = false
		st.DisabledTasks[0] = "mutated"
		st.Today["old"] = api.TaskStatus{ID: "mutated"}
		st.Marks["old"] = "mutated"
	})
	if err == nil || !reflect.DeepEqual(before, a.snapshot()) {
		t.Fatalf("failed update committed: %v %+v", err, a.snapshot())
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/accounts/42", strings.NewReader(`{"enabled":false}`))
	r.SetPathValue("mid", "42")
	v.e.handlePatch(w, r)
	if w.Code < 400 || !reflect.DeepEqual(before, a.snapshot()) {
		t.Fatalf("PATCH hid disk failure: status=%d state=%+v", w.Code, a.snapshot())
	}
}

func TestPushRequiresValidProviderAcknowledgement(t *testing.T) {
	for _, channel := range []string{"telegram", "wecom", "dingtalk", "pushplus", "serverchan", "bark", "webhook"} {
		for _, body := range []string{"not JSON", `{}`, `{"ok":false}`, `{"ok":true}`, `{"errcode":0}`, `{"code":0}`, `{"code":200}`} {
			t.Run(channel+"/"+body, func(t *testing.T) {
				v := newEnv(t)
				v.e.cfg.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: req}, nil
				})
				n := api.Notify{Channel: channel, Token: "token", Target: "target"}
				if channel == "webhook" {
					n.Token = "https://example.com/push"
				}
				wantOK := channel == "webhook" || channel == "telegram" && body == `{"ok":true}` || (channel == "wecom" || channel == "dingtalk") && body == `{"errcode":0}` || channel == "serverchan" && body == `{"code":0}` || (channel == "pushplus" || channel == "bark") && body == `{"code":200}`
				err := v.e.push(context.Background(), n, "title", "body")
				if (err == nil) != wantOK {
					t.Fatalf("success=%v want=%v error=%v", err == nil, wantOK, err)
				}
			})
		}
	}
}

type failedPushBody struct{}

func (failedPushBody) Read([]byte) (int, error) { return 0, errors.New("response stream failed") }
func (failedPushBody) Close() error             { return nil }

func TestPushPropagatesResponseReadErrors(t *testing.T) {
	hc := &http.Client{Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: failedPushBody{}, Header: http.Header{}, Request: req}, nil
	})}
	req, err := http.NewRequest(http.MethodPost, "https://example.com/push", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := doPush(hc, req, nil); err == nil || !strings.Contains(err.Error(), "response stream failed") {
		t.Fatalf("read error ignored: %v", err)
	}
}

func TestOverlappingQRPollsPersistSuccessOnlyOnce(t *testing.T) {
	v := newEnv(t)
	v.b.JSON("passport.bilibili.com/x/passport-login/web/qrcode/generate", map[string]any{"url": "https://example.com/scan", "qrcode_key": "key"})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	v.b.Handle("passport.bilibili.com/x/passport-login/web/qrcode/poll", func(bilitest.Call) bilitest.Reply {
		once.Do(func() { close(entered) })
		<-release
		h := http.Header{}
		for _, c := range []string{"SESSDATA=qr-session; Path=/", "bili_jct=qr-csrf; Path=/", "DedeUserID=42; Path=/"} {
			h.Add("Set-Cookie", c)
		}
		return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"code":0,"refresh_token":"qr-token"}}`}
	})
	v.b.JSON("api.bilibili.com/x/frontend/finger/spi", nil)
	v.b.JSON(navPath, navData(42))
	q, err := v.e.StartQR(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	poll := func() {
		out, err := v.e.PollQR(context.Background(), q.ID)
		if err == nil && (out.State != string(bili.QRSuccess) || out.Mid != "42") {
			err = fmt.Errorf("unexpected QR result: %+v", out)
		}
		results <- err
	}
	go poll()
	awaitSignal(t, entered)
	go poll()
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if v.b.Count("passport.bilibili.com/x/passport-login/web/qrcode/poll") != 1 || v.b.Count(navPath) != 1 || len(v.e.Accounts()) != 1 {
		t.Fatal("QR success was committed more than once")
	}
}

func TestReloginWaitsForActiveRefreshBeforeReplacingCredentials(t *testing.T) {
	useTasks(t)
	v := newEnv(t)
	a := v.login(42)
	prepareRefresh(v)
	entered, release := make(chan struct{}), make(chan struct{})
	v.b.Handle(refreshPath, func(bilitest.Call) bilitest.Reply { close(entered); <-release; return refreshedSessionReply() })
	v.b.JSON(confirmPath, nil)
	runDone := make(chan struct{})
	go func() { defer close(runDone); v.e.RunNow(context.Background(), "42", nil) }()
	awaitSignal(t, entered)
	loginDone := make(chan error, 1)
	go func() {
		_, err := v.e.saveLoginSession(context.Background(), session{Cookies: map[string]string{"SESSDATA": "replacement", "bili_jct": "replacement-csrf", "DedeUserID": "42"}, RefreshToken: "replacement-token"})
		loginDone <- err
	}()
	waitFor(t, "relogin paused admission", func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.paused })
	if a.runNow(nil) {
		t.Error("relogin admitted a new run")
	}
	select {
	case err := <-loginDone:
		t.Fatalf("relogin completed before refresh: %v", err)
	default:
	}
	close(release)
	awaitSignal(t, runDone)
	if err := <-loginDone; err != nil {
		t.Fatal(err)
	}
	s, err := v.e.loadSession("42")
	if err != nil || s.Cookies["SESSDATA"] != "replacement" || s.RefreshToken != "replacement-token" {
		b, _ := json.Marshal(s)
		t.Fatalf("refresh overwrote replacement: %s %v", b, err)
	}
}
