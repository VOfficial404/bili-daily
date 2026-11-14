package bili

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const userProfilePath = "api.bilibili.com/x/space/wbi/acc/info"

func userProfileServer() *bilitest.Server {
	s := bilitest.New()
	// nav 未登录时仍带 WBI 密钥，不能用要求登录成功的 Nav 方法取密钥。
	s.Handle("api.bilibili.com/x/web-interface/nav", func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Body: map[string]any{
			"code": -101,
			"data": map[string]any{"wbi_img": map[string]any{
				"img_url": "https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png",
				"sub_url": "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png",
			}},
		}}
	})
	return s
}

func TestUPProfileUsesSignedSpaceRequest(t *testing.T) {
	s := userProfileServer()
	const mid int64 = 9007199254740993
	s.JSON(userProfilePath, map[string]any{"mid": mid, "name": " 测试 UP ", "face": "//i0.hdslb.com/bfs/face/test.jpg"})
	c := New(s, map[string]string{"SESSDATA": "private-session", "bili_jct": "private-csrf", "DedeUserID": "42"})
	p, err := c.UPProfile(context.Background(), mid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mid != mid || p.Uname != "测试 UP" || p.Face != "https://i0.hdslb.com/bfs/face/test.jpg" {
		t.Fatalf("profile = %+v", p)
	}
	calls := s.Calls(userProfilePath)
	if len(calls) != 1 {
		t.Fatalf("profile calls = %d", len(calls))
	}
	call := calls[0]
	if call.Method != http.MethodGet || call.Query.Get("mid") != strconv.FormatInt(mid, 10) || call.Header.Get("Origin") != "https://space.bilibili.com" {
		t.Fatalf("request method=%s query=%v origin=%s", call.Method, call.Query, call.Header.Get("Origin"))
	}
	if call.Cookie["SESSDATA"] != "private-session" || len(call.Cookie) != 3 {
		t.Error("空间资料查询没有沿用账号 Cookie")
	}
	wts, err := strconv.ParseInt(call.Query.Get("wts"), 10, 64)
	if err != nil || wts <= 0 {
		t.Fatalf("wts = %q", call.Query.Get("wts"))
	}
	want := wbiSign(url.Values{"mid": {strconv.FormatInt(mid, 10)}}, c.wbi.mixin, time.Unix(wts, 0))
	if call.Query.Encode() != mustProfileQuery(t, want).Encode() {
		t.Fatalf("signature query = %v", call.Query)
	}
	if _, err := c.UPProfile(context.Background(), mid); err != nil {
		t.Fatal(err)
	}
	if s.Count("api.bilibili.com/x/web-interface/nav") != 1 {
		t.Error("同一客户端没有复用 WBI 密钥")
	}
}

func mustProfileQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestUPProfileRejectsIncompleteOrUnsafeResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		data any
	}{
		{"null", nil},
		{"missing", map[string]any{}},
		{"wrong UID", map[string]any{"mid": 43, "name": "UP", "face": "https://i0.hdslb.com/a.jpg"}},
		{"empty name", map[string]any{"mid": 42, "name": "  ", "face": "https://i0.hdslb.com/a.jpg"}},
		{"missing avatar", map[string]any{"mid": 42, "name": "UP"}},
		{"script avatar", map[string]any{"mid": 42, "name": "UP", "face": "javascript:alert(1)"}},
		{"relative avatar", map[string]any{"mid": 42, "name": "UP", "face": "/a.jpg"}},
		{"credential avatar", map[string]any{"mid": 42, "name": "UP", "face": "https://secret@i0.hdslb.com/a.jpg"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := userProfileServer()
			s.JSON(userProfilePath, tc.data)
			p, err := New(s, nil).UPProfile(context.Background(), 42)
			if p != nil || !IsCode(err, CodeBadResponse) || !strings.Contains(err.Error(), "接口可能已变更") {
				t.Fatalf("profile=%+v error=%v", p, err)
			}
		})
	}
}

func TestUPProfilePropagatesMissingAndCancellation(t *testing.T) {
	s := userProfileServer()
	s.Fail(userProfilePath, -404, "用户不存在")
	c := New(s, nil)
	if p, err := c.UPProfile(context.Background(), 42); p != nil || !IsCode(err, -404) {
		t.Fatalf("missing profile=%+v error=%v", p, err)
	}
	before := len(s.Calls(""))
	if _, err := c.UPProfile(context.Background(), 0); !IsCode(err, -400) || len(s.Calls("")) != before {
		t.Fatalf("invalid UID error=%v calls=%d", err, len(s.Calls("")))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelClient := New(roundTripper(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	}), nil)
	if _, err := cancelClient.UPProfile(ctx, 42); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}
