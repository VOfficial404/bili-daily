package bili

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// QRState 是扫码登录的进度
type QRState string

const (
	QRWaiting QRState = "waiting" // 还没扫
	QRScanned QRState = "scanned" // 扫了，等手机上确认
	QRExpired QRState = "expired"
	QRSuccess QRState = "success"
)

/* 网页扫码登录：得到 Cookie 和 refresh_token */

// WebQR 是一次网页扫码登录
type WebQR struct {
	URL string // 二维码内容
	key string
}

// Session 是扫码登录或刷新 Cookie 得到的会话
type Session struct {
	Cookies      map[string]string
	RefreshToken string
	Expires      time.Time // Cookie 过期时间，不知道时为零值
}

// NewWebQR 申请一张登录二维码
func (c *Client) NewWebQR(ctx context.Context) (*WebQR, error) {
	var d struct {
		URL string `json:"url"`
		Key string `json:"qrcode_key"`
	}
	if err := c.Get(ctx, "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil, &d); err != nil {
		return nil, err
	}
	if d.Key == "" || d.URL == "" {
		return nil, &Error{Code: CodeBadResponse, Message: "没有拿到二维码", Path: "passport.bilibili.com/x/passport-login/web/qrcode/generate"}
	}
	return &WebQR{URL: d.URL, key: d.Key}, nil
}

// PollWebQR 查询一次扫码状态；成功时返回会话
func (c *Client) PollWebQR(ctx context.Context, q *WebQR) (QRState, *Session, error) {
	var d struct {
		Code         int    `json:"code"`
		URL          string `json:"url"`
		RefreshToken string `json:"refresh_token"`
	}
	resp, err := c.doResp(ctx, Request{
		URL:   "https://passport.bilibili.com/x/passport-login/web/qrcode/poll",
		Query: url.Values{"qrcode_key": {q.key}},
	}, &d)
	if err != nil {
		return QRWaiting, nil, err
	}
	switch d.Code {
	case 0:
	case 86090:
		return QRScanned, nil, nil
	case 86038:
		return QRExpired, nil, nil
	default:
		return QRWaiting, nil, nil
	}
	s := &Session{Cookies: map[string]string{}, RefreshToken: d.RefreshToken, Expires: cookieExpiry(resp.Cookies())}
	for _, ck := range resp.Cookies() {
		if ck.Value != "" {
			s.Cookies[ck.Name] = ck.Value
		}
	}
	if s.Expires.IsZero() {
		s.Expires = expiryFromURL(d.URL)
	}
	if s.Cookies["SESSDATA"] == "" || s.Cookies["bili_jct"] == "" || s.Cookies["DedeUserID"] == "" {
		return QRSuccess, nil, &Error{Code: CodeBadResponse, Message: "登录成功但响应里没有会话", Path: "passport.bilibili.com/x/passport-login/web/qrcode/poll"}
	}
	return QRSuccess, s, nil
}

// doResp 和 Do 一样，另外返回响应，用来取 Set-Cookie
func (c *Client) doResp(ctx context.Context, r Request, out any) (*http.Response, error) {
	resp, body, err := c.send(ctx, r)
	if err != nil {
		return nil, err
	}
	return resp, decodeEnvelope(body, shortURL(r.URL), out)
}

// cookieExpiry 取 SESSDATA 的过期时间
func cookieExpiry(cs []*http.Cookie) time.Time {
	for _, c := range cs {
		if c.Name != "SESSDATA" {
			continue
		}
		if c.MaxAge > 0 {
			return time.Now().Add(time.Duration(c.MaxAge) * time.Second)
		}
		return c.Expires
	}
	return time.Time{}
}

// expiryFromURL 从扫码成功返回的跨域登录链接里取 Expires（Unix 秒）
func expiryFromURL(raw string) time.Time {
	u, err := url.Parse(raw)
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(u.Query().Get("Expires"), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	if sec < 1e9 {
		// 老的响应里是有效期的秒数，不是时间戳
		return time.Now().Add(time.Duration(sec) * time.Second)
	}
	return time.Unix(sec, 0)
}

// Fingerprint 取一组设备标识（buvid3、buvid4），新会话带上它们更像浏览器
func (c *Client) Fingerprint(ctx context.Context) (map[string]string, error) {
	var d struct {
		B3 string `json:"b_3"`
		B4 string `json:"b_4"`
	}
	if err := c.Get(ctx, "https://api.bilibili.com/x/frontend/finger/spi", nil, &d); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if d.B3 != "" {
		out["buvid3"] = d.B3
	}
	if d.B4 != "" {
		out["buvid4"] = d.B4
	}
	return out, nil
}

// Logout 让 B 站注销这份网页会话
func (c *Client) Logout(ctx context.Context) error {
	return c.Do(ctx, Request{
		URL:    "https://passport.bilibili.com/login/exit/v2",
		Form:   url.Values{"biliCSRF": {c.CSRF()}},
		NoCSRF: true,
	}, nil)
}

/* Cookie 刷新，流程见 bilibili-API-collect 的 docs/login/cookie_refresh.md：
   cookie/info 判断是否需要刷新 → 用公钥加密 refresh_<毫秒时间戳> 得到 correspondPath →
   从 correspond 页面取 refresh_csrf → cookie/refresh 换新 Cookie 和新 refresh_token →
   用新 Cookie 和旧 refresh_token 调 confirm/refresh 让旧 Cookie 失效。 */

const refreshPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDLgd2OAkcGVtoE3ThUREbio0Eg
Uc/prcajMKXvkCKFCWhJYJcLkcM2DKKcSeFpD/j6Boy538YXnR6VhcuUJOhH2x71
nzPjfdTcqMz7djHum0qSZA0AyCBDABUqCrfNgCiJ00Ra7GmRj+YCK1NJEuewlb40
JNrRuoEUXpabUzGB8QIDAQAB
-----END PUBLIC KEY-----`

// refreshPublicKey 测试里会换成自己生成的密钥
var refreshPublicKey = mustParsePublicKey(refreshPublicKeyPEM)

func mustParsePublicKey(s string) *rsa.PublicKey {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		panic("公钥格式不对")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return k.(*rsa.PublicKey)
}

func correspondPath(ts int64) (string, error) {
	b, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, refreshPublicKey, []byte(fmt.Sprintf("refresh_%d", ts)), nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var refreshCSRFRe = regexp.MustCompile(`<div id="1-name">([^<]+)</div>`)

// ErrRefreshMismatch 表示 refresh_token 和 Cookie 对不上，只能重新扫码
var ErrRefreshMismatch = errors.New("refresh_token 和 Cookie 对不上，请重新扫码登录")

// CookieNeedsRefresh 询问 B 站这份 Cookie 是否该刷新了，同时返回服务器的毫秒时间戳
func (c *Client) CookieNeedsRefresh(ctx context.Context) (bool, int64, error) {
	var d struct {
		Refresh   bool  `json:"refresh"`
		Timestamp int64 `json:"timestamp"`
	}
	err := c.Get(ctx, "https://passport.bilibili.com/x/passport-login/web/cookie/info", url.Values{"csrf": {c.CSRF()}}, &d)
	return d.Refresh, d.Timestamp, err
}

// RefreshCookie 换一份新的 Cookie。成功后 c 改用新 Cookie，旧 Cookie 已经在 B 站作废。
func (c *Client) RefreshCookie(ctx context.Context, refreshToken string, ts int64) (*Session, error) {
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}
	p, err := correspondPath(ts)
	if err != nil {
		return nil, err
	}
	page, err := c.Raw(ctx, Request{URL: "https://www.bilibili.com/correspond/1/" + p})
	if err != nil {
		return nil, err
	}
	m := refreshCSRFRe.FindSubmatch(page)
	if m == nil {
		return nil, &Error{Code: CodeBadResponse, Message: "页面里没有 refresh_csrf", Path: "www.bilibili.com/correspond/1"}
	}

	var d struct {
		RefreshToken string `json:"refresh_token"`
	}
	resp, err := c.doResp(ctx, Request{
		URL: "https://passport.bilibili.com/x/passport-login/web/cookie/refresh",
		Form: url.Values{
			"refresh_csrf":  {string(m[1])},
			"source":        {"main_web"},
			"refresh_token": {refreshToken},
		},
	}, &d)
	if err != nil {
		if IsCode(err, 86095) {
			return nil, ErrRefreshMismatch
		}
		return nil, err
	}
	old := c.Cookies()
	s := &Session{Cookies: old, RefreshToken: d.RefreshToken, Expires: cookieExpiry(resp.Cookies())}
	for _, ck := range resp.Cookies() {
		if ck.Value != "" {
			s.Cookies[ck.Name] = ck.Value
		}
	}
	if s.RefreshToken == "" || s.Cookies["SESSDATA"] == c.Cookie("SESSDATA") {
		return nil, &Error{Code: CodeBadResponse, Message: "刷新 Cookie 的响应里没有新的会话", Path: "passport.bilibili.com/x/passport-login/web/cookie/refresh"}
	}
	c.mu.Lock()
	c.cookies = map[string]string{}
	for k, v := range s.Cookies {
		c.cookies[k] = v
	}
	c.mu.Unlock()
	// 旧 Cookie 在确认之后才失效；确认失败不影响新 Cookie 使用，所以不当作错误
	c.PostForm(ctx, "https://passport.bilibili.com/x/passport-login/web/confirm/refresh", url.Values{"refresh_token": {refreshToken}}, nil)
	return s, nil
}
