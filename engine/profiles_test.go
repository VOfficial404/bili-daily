package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const upProfileTestPath = "api.bilibili.com/x/space/wbi/acc/info"

func TestUPProfileAPIUsesStoredAccountAndOnlyReturnsPublicFields(t *testing.T) {
	v := newEnv(t)
	v.login(41).update(func(st *accountState) { st.LoggedIn = false })
	v.login(42)
	const mid = "9007199254740993"
	v.b.JSON(upProfileTestPath, map[string]any{
		"mid": int64(9007199254740993), "name": "公开 UP", "face": "http://i0.hdslb.com/bfs/face/up.jpg",
		"SESSDATA": "must-not-leak", "coins": 50,
	})
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	c := api.NewClient(srv.URL, v.e.Token())
	p, err := c.UPProfile(context.Background(), mid)
	if err != nil || p.Mid != mid || p.Uname != "公开 UP" || p.Face != "https://i0.hdslb.com/bfs/face/up.jpg" {
		t.Fatalf("profile=%+v error=%v", p, err)
	}
	calls := v.b.Calls(upProfileTestPath)
	if len(calls) != 1 || calls[0].Cookie["DedeUserID"] != "42" || calls[0].Cookie["SESSDATA"] != "sess-42" {
		t.Fatal("查询没有采用有效登录账号")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/ups/"+mid, nil)
	req.Header.Set("Authorization", "Bearer "+v.e.Token())
	rr := httptest.NewRecorder()
	v.e.Handler().ServeHTTP(rr, req)
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || len(body) != 3 || body["mid"] != mid || rr.Header().Get("Cache-Control") != "no-store" || len(rr.Result().Cookies()) != 0 {
		t.Fatalf("response status=%d body=%v headers=%v", rr.Code, body, rr.Header())
	}
}

func TestUPProfileAPIRequiresAuthorizationAndCanonicalUID(t *testing.T) {
	v := newEnv(t)
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	if _, err := api.NewClient(srv.URL, "wrong").UPProfile(context.Background(), "42"); !api.IsUnauthorized(err) {
		t.Fatalf("unauthorized error = %v", err)
	}
	c := api.NewClient(srv.URL, v.e.Token())
	for _, mid := range []string{"0", "-42", "+42", "042", "42.0", "1e3", " 42", "42 ", "9223372036854775808", "42/other"} {
		t.Run(mid, func(t *testing.T) {
			_, err := c.UPProfile(context.Background(), mid)
			var ae *api.Error
			if !errors.As(err, &ae) || ae.Status != http.StatusBadRequest || !strings.Contains(ae.Message, "正整数") {
				t.Fatalf("UID %q error=%v", mid, err)
			}
		})
	}
	if len(v.b.Calls("")) != 0 {
		t.Fatal("无效 UID 或未授权请求不应访问 B 站")
	}
}

func TestUPProfileAPIAnonymousLookupAndInformativeErrors(t *testing.T) {
	v := newEnv(t)
	v.b.JSON(navPath, navData(42))
	v.b.JSON(upProfileTestPath, map[string]any{"mid": 42, "name": "UP", "face": "https://i0.hdslb.com/up.jpg"})
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	c := api.NewClient(srv.URL, v.e.Token())
	if _, err := c.UPProfile(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	if len(v.b.Calls(upProfileTestPath)[0].Cookie) != 0 {
		t.Fatal("匿名查询意外带有账号 Cookie")
	}
	for _, tc := range []struct {
		name    string
		code    int
		status  int
		message string
	}{
		{"missing", -404, http.StatusNotFound, "不存在或已注销"},
		{"not logged in", -101, http.StatusBadGateway, "扫码登录"},
		{"forbidden", -403, http.StatusBadGateway, "有效的登录账号"},
		{"risk control", -412, http.StatusBadGateway, "查询 UP 主资料失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v.b.Fail(upProfileTestPath, tc.code, "上游查询失败")
			_, err := c.UPProfile(context.Background(), "43")
			var ae *api.Error
			if !errors.As(err, &ae) || ae.Status != tc.status || !strings.Contains(ae.Message, tc.message) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	v.b.JSON(upProfileTestPath, map[string]any{"mid": 43, "name": "错误 UID", "face": "https://i0.hdslb.com/up.jpg"})
	if _, err := c.UPProfile(context.Background(), "44"); err == nil || !strings.Contains(err.Error(), "接口可能已变更") {
		t.Fatalf("changed response error=%v", err)
	}
}

func TestUPProfileLookupsShareAccountPacingAndCache(t *testing.T) {
	v := newEnv(t)
	var mu sync.Mutex
	var waits []time.Duration
	v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		return ctx.Err()
	}
	v.login(42)
	v.b.Handle(upProfileTestPath, func(c bilitest.Call) bilitest.Reply {
		mid, _ := strconv.ParseInt(c.Query.Get("mid"), 10, 64)
		return bilitest.OK(map[string]any{"mid": mid, "name": "UP", "face": "https://i0.hdslb.com/up.jpg"})
	})
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	c := api.NewClient(srv.URL, v.e.Token())
	for _, mid := range []string{"100", "200", "100"} {
		if p, err := c.UPProfile(context.Background(), mid); err != nil || p.Mid != mid {
			t.Fatalf("UID %s profile=%+v error=%v", mid, p, err)
		}
	}
	if n := v.b.Count(upProfileTestPath); n != 2 {
		t.Fatalf("查到过的 UID 不应再问 B 站：%d 次", n)
	}
	// 测试里时间不走：除了第一个请求，后面每个都要排在上一个的间隔之后
	mu.Lock()
	defer mu.Unlock()
	if len(waits) != 4 || waits[0] != 0 {
		t.Fatalf("等待 = %v", waits)
	}
	for _, d := range waits[1:] {
		if d <= 0 {
			t.Fatalf("资料查询没有按账号的节奏排队：%v", waits)
		}
	}
}

func TestFollowingsAPIListsSearchesAndFillsProfileCache(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	const listPath, searchPath = "api.bilibili.com/x/relation/followings", "api.bilibili.com/x/relation/followings/search"
	v.b.JSON(listPath, map[string]any{"total": 21, "list": []any{
		map[string]any{"mid": 100, "uname": " 甲 ", "face": "//i0.hdslb.com/a.jpg"},
		map[string]any{"mid": 0, "uname": "无效"},
	}})
	v.b.JSON(searchPath, map[string]any{"total": 1, "list": []any{map[string]any{"mid": 200, "uname": "乙", "face": "https://i0.hdslb.com/b.jpg"}}})
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	c := api.NewClient(srv.URL, v.e.Token())

	f, err := c.Followings(context.Background(), "42", 1, "")
	if err != nil || f.Total != 21 || !f.More || len(f.List) != 1 || f.List[0] != (api.UPProfile{Mid: "100", Uname: "甲", Face: "https://i0.hdslb.com/a.jpg"}) {
		t.Fatalf("followings=%+v error=%v", f, err)
	}
	call := v.b.Calls(listPath)[0]
	if call.Query.Get("vmid") != "42" || call.Query.Get("order_type") != "attention" || call.Cookie["SESSDATA"] != "sess-42" {
		t.Fatalf("关注请求 = %+v", call)
	}
	f, err = c.Followings(context.Background(), "42", 2, "乙")
	if err != nil || f.More || len(f.List) != 1 || f.List[0].Mid != "200" {
		t.Fatalf("search=%+v error=%v", f, err)
	}
	if q := v.b.Calls(searchPath)[0].Query; q.Get("name") != "乙" || q.Get("pn") != "2" || q.Get("vmid") != "42" {
		t.Fatalf("搜索请求 = %v", q)
	}
	// 列出来的 UP 主选上后，预览直接用缓存，不再问 B 站
	if p, err := c.UPProfile(context.Background(), "100"); err != nil || p.Uname != "甲" || v.b.Count(upProfileTestPath) != 0 {
		t.Fatalf("profile=%+v error=%v calls=%d", p, err, v.b.Count(upProfileTestPath))
	}
	for _, bad := range []string{"0", "251", "x"} {
		req := httptest.NewRequest(http.MethodGet, "/api/accounts/42/followings?pn="+bad, nil)
		req.Header.Set("Authorization", "Bearer "+v.e.Token())
		rr := httptest.NewRecorder()
		v.e.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("pn=%s status=%d", bad, rr.Code)
		}
	}
	if _, err := c.Followings(context.Background(), "99", 1, ""); err == nil {
		t.Fatal("不存在的账号应报错")
	}
}

func TestUPProfileAPIHonorsRequestDeadlineAndCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			v := newEnv(t)
			var ctx context.Context
			var cancel context.CancelFunc
			if deadline {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()
			v.e.cfg.Transport = roundTripper(func(r *http.Request) (*http.Response, error) {
				if r.Context().Err() == nil {
					t.Error("B 站请求没有继承调用方的取消状态")
				}
				return nil, r.Context().Err()
			})
			req := httptest.NewRequest(http.MethodGet, "/api/ups/42", nil).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+v.e.Token())
			rr := httptest.NewRecorder()
			v.e.Handler().ServeHTTP(rr, req)
			want := http.StatusBadGateway
			if deadline {
				want = http.StatusGatewayTimeout
			}
			if rr.Code != want {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestUPProfileAPIUsesConfiguredProxy(t *testing.T) {
	requests := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.Host
		http.Error(w, "test proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()
	v := newEnv(t)
	v.e.cfg.Transport = nil
	v.set(func(s *api.Settings) { s.Proxy = proxy.URL })
	req := httptest.NewRequest(http.MethodGet, "/api/ups/42", nil)
	req.Header.Set("Authorization", "Bearer "+v.e.Token())
	rr := httptest.NewRecorder()
	v.e.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-requests:
		if got != "CONNECT api.bilibili.com:443" {
			t.Fatalf("proxy request = %q", got)
		}
	default:
		t.Fatal("UP 主资料查询没有经过设置的代理")
	}
}
