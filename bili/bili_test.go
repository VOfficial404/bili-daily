package bili

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

// bilibili-API-collect 的 docs/misc/sign/wbi.md 里给的例子
func TestWBISignMatchesPublishedExample(t *testing.T) {
	mixin := mixinKey("7cd084941338484aae1ad9425b84077c" + "4932caff0ff746eab6f01bf08b70ac45")
	if mixin != "ea1db124af3c7062474693fa704f4ff8" {
		t.Fatalf("mixin key = %s", mixin)
	}
	got := wbiSign(url.Values{"foo": {"114"}, "bar": {"514"}, "zab": {"1919810"}}, mixin, time.Unix(1702204169, 0))
	want := "bar=514&foo=114&wts=1702204169&zab=1919810&w_rid=8f6f2b5b3d485fe1886cec6a0be8c5d4"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestWBISignStripsUnsafeCharsAndEncodesSpaces(t *testing.T) {
	got := wbiSign(url.Values{"keyword": {"a b!'()*c"}}, strings.Repeat("0", 32), time.Unix(1, 0))
	if !strings.HasPrefix(got, "keyword=a%20bc&wts=1&w_rid=") {
		t.Fatalf("got %s", got)
	}
}

func TestErrorsAreClassified(t *testing.T) {
	srv := bilitest.New()
	c := New(srv, map[string]string{"bili_jct": "csrf"})
	ctx := context.Background()

	srv.Fail("api.bilibili.com/a", -352, "风控校验失败")
	if r := Classify(c.Get(ctx, "https://api.bilibili.com/a", nil, nil)); r != ReasonRisk {
		t.Errorf("-352 → %q", r)
	}
	srv.Handle("api.bilibili.com/b", func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Status: http.StatusPreconditionFailed, Body: "<html>"}
	})
	if r := Classify(c.Get(ctx, "https://api.bilibili.com/b", nil, nil)); r != ReasonRisk {
		t.Errorf("HTTP 412 → %q", r)
	}
	srv.Handle("api.bilibili.com/c", func(bilitest.Call) bilitest.Reply { return bilitest.Reply{Body: "<html>"} })
	if r := Classify(c.Get(ctx, "https://api.bilibili.com/c", nil, nil)); r != ReasonChanged {
		t.Errorf("not JSON → %q", r)
	}
	srv.Fail("api.bilibili.com/d", -101, "账号未登录")
	if err := c.Get(ctx, "https://api.bilibili.com/d", nil, nil); !IsNotLogin(err) {
		t.Errorf("-101 → %v", err)
	}
}

// 漫画接口出错时返回 HTTP 400，code 是字符串
func TestTwirpErrorKeepsStringCode(t *testing.T) {
	srv := bilitest.New()
	srv.Handle("manga.bilibili.com/twirp/x", func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Status: 400, Body: `{"code":"invalid_argument","msg":"clockin clockin is duplicate","meta":{"argument":"clockin"}}`}
	})
	err := New(srv, nil).PostForm(context.Background(), "https://manga.bilibili.com/twirp/x", nil, nil)
	var e *Error
	if !asError(err, &e) || e.Key != "invalid_argument" || e.Message != "clockin clockin is duplicate" {
		t.Fatalf("got %#v", err)
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	*target = e
	return ok
}

func TestPostFormAddsCSRFAndDecodesData(t *testing.T) {
	srv := bilitest.New()
	srv.JSON("api.bilibili.com/x/do", map[string]any{"n": 7})
	c := New(srv, map[string]string{"SESSDATA": "s", "bili_jct": "csrf"})
	var out struct{ N int }
	if err := c.PostForm(context.Background(), "https://api.bilibili.com/x/do", url.Values{"aid": {"1"}}, &out); err != nil {
		t.Fatal(err)
	}
	call := srv.Calls("api.bilibili.com/x/do")[0]
	if out.N != 7 || call.Form.Get("csrf") != "csrf" || call.Form.Get("aid") != "1" || call.Cookie["SESSDATA"] != "s" {
		t.Fatalf("out=%+v form=%v cookie=%v", out, call.Form, call.Cookie)
	}
}

func TestRefreshCookieSwapsSessionAndConfirmsWithOldToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	saved := refreshPublicKey
	refreshPublicKey = &key.PublicKey
	defer func() { refreshPublicKey = saved }()

	srv := bilitest.New()
	var plain string
	// correspond 的路径是密文，每次不同：登记不了固定路径，用一个包装的 RoundTripper 接住
	rt := roundTripper(func(req *http.Request) (*http.Response, error) {
		if p, ok := strings.CutPrefix(req.URL.Path, "/correspond/1/"); ok && req.URL.Host == "www.bilibili.com" {
			raw, _ := hex.DecodeString(p)
			b, err := rsa.DecryptOAEP(sha256.New(), nil, key, raw, nil)
			if err != nil {
				t.Errorf("correspondPath 解不开：%v", err)
			}
			plain = string(b)
			req.URL.Path = "/correspond/1/page"
		}
		return srv.RoundTrip(req)
	})
	srv.Handle("www.bilibili.com/correspond/1/page", func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Body: `<html><div id="1-name">refresh-csrf-value</div></html>`}
	})
	srv.Handle("passport.bilibili.com/x/passport-login/web/cookie/refresh", func(bilitest.Call) bilitest.Reply {
		h := http.Header{}
		h.Add("Set-Cookie", "SESSDATA=new-sess; Max-Age=15552000; Path=/")
		h.Add("Set-Cookie", "bili_jct=new-csrf; Path=/")
		return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"status":0,"refresh_token":"new-token"}}`}
	})
	srv.JSON("passport.bilibili.com/x/passport-login/web/confirm/refresh", nil)

	c := New(rt, map[string]string{"SESSDATA": "old-sess", "bili_jct": "old-csrf", "DedeUserID": "42", "buvid3": "dev"})
	s, err := c.RefreshCookie(context.Background(), "old-token", 1700000000123)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "refresh_1700000000123" {
		t.Errorf("加密的内容是 %q", plain)
	}
	if s.RefreshToken != "new-token" || s.Cookies["SESSDATA"] != "new-sess" || s.Cookies["buvid3"] != "dev" || s.Cookies["DedeUserID"] != "42" {
		t.Errorf("session = %+v", s)
	}
	if time.Until(s.Expires) < 179*24*time.Hour {
		t.Errorf("expires = %v", s.Expires)
	}
	refresh := srv.Calls("passport.bilibili.com/x/passport-login/web/cookie/refresh")[0]
	if refresh.Form.Get("refresh_csrf") != "refresh-csrf-value" || refresh.Form.Get("refresh_token") != "old-token" || refresh.Form.Get("csrf") != "old-csrf" {
		t.Errorf("refresh form = %v", refresh.Form)
	}
	confirm := srv.Calls("passport.bilibili.com/x/passport-login/web/confirm/refresh")[0]
	if confirm.Form.Get("refresh_token") != "old-token" || confirm.Form.Get("csrf") != "new-csrf" || confirm.Cookie["SESSDATA"] != "new-sess" {
		t.Errorf("confirm form = %v cookie = %v", confirm.Form, confirm.Cookie)
	}
	if c.CSRF() != "new-csrf" {
		t.Errorf("客户端还在用旧 Cookie")
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWebQRLoginCollectsCookies(t *testing.T) {
	srv := bilitest.New()
	srv.JSON("passport.bilibili.com/x/passport-login/web/qrcode/generate", map[string]any{"url": "https://account.bilibili.com/h5/account-h5/auth/scan-web?qrcode_key=k", "qrcode_key": "k"})
	state := 86101
	srv.Handle("passport.bilibili.com/x/passport-login/web/qrcode/poll", func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("qrcode_key") != "k" {
			t.Errorf("qrcode_key = %q", c.Query.Get("qrcode_key"))
		}
		if state != 0 {
			return bilitest.OK(map[string]any{"code": state})
		}
		h := http.Header{}
		for _, ck := range []string{"SESSDATA=s; Path=/", "bili_jct=j; Path=/", "DedeUserID=42; Path=/"} {
			h.Add("Set-Cookie", ck)
		}
		return bilitest.Reply{Header: h, Body: `{"code":0,"data":{"code":0,"url":"https://passport.biligame.com/x/passport-login/web/crossDomain?Expires=15552000&gourl=x","refresh_token":"rt"}}`}
	})
	c := New(srv, nil)
	ctx := context.Background()
	q, err := c.NewWebQR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st, _, err := c.PollWebQR(ctx, q); err != nil || st != QRWaiting {
		t.Fatalf("waiting: %v %v", st, err)
	}
	state = 86090
	if st, _, _ := c.PollWebQR(ctx, q); st != QRScanned {
		t.Fatalf("scanned: %v", st)
	}
	state = 86038
	if st, _, _ := c.PollWebQR(ctx, q); st != QRExpired {
		t.Fatalf("expired: %v", st)
	}
	state = 0
	st, s, err := c.PollWebQR(ctx, q)
	if err != nil || st != QRSuccess {
		t.Fatalf("success: %v %v", st, err)
	}
	if s.RefreshToken != "rt" || s.Cookies["SESSDATA"] != "s" || s.Cookies["bili_jct"] != "j" || s.Cookies["DedeUserID"] != "42" {
		t.Fatalf("session = %+v", s)
	}
}
