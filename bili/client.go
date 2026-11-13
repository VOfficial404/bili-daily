// Package bili 是 B 站接口的客户端：发请求、签名、登录，以及各任务用到的接口。
// 这里只有协议，不保存任何东西；账号、排程和报告在 engine 包里。
package bili

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// WebUA 是网页接口用的浏览器标识
	WebUA   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	maxBody = 8 << 20
)

// Client 以一个账号（或匿名）的身份请求 B 站
type Client struct {
	hc *http.Client

	mu      sync.Mutex
	cookies map[string]string
	wbi     wbiKeys

	// Wait 在每个请求发出前调用，engine 用它控制请求之间的间隔；返回错误时不发请求
	Wait func(ctx context.Context) error
	// Debugf 每个请求记一条日志，可以为 nil
	Debugf func(format string, args ...any)
}

// New 返回用 rt 发请求的客户端。cookies 为 nil 表示匿名。
func New(rt http.RoundTripper, cookies map[string]string) *Client {
	c := &Client{
		hc:      &http.Client{Timeout: 25 * time.Second, Transport: rt},
		cookies: map[string]string{},
	}
	for k, v := range cookies {
		c.cookies[k] = v
	}
	return c
}

// Cookie 返回一个 Cookie 的值，没有时为空
func (c *Client) Cookie(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cookies[name]
}

// Cookies 返回全部 Cookie 的副本
func (c *Client) Cookies() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.cookies))
	for k, v := range c.cookies {
		out[k] = v
	}
	return out
}

// SetCookie 加上或替换一个 Cookie
func (c *Client) SetCookie(name, value string) {
	c.mu.Lock()
	c.cookies[name] = value
	c.mu.Unlock()
}

// CSRF 是 bili_jct，写操作都要带它
func (c *Client) CSRF() string { return c.Cookie("bili_jct") }

// Mid 是账号的 UID
func (c *Client) Mid() string { return c.Cookie("DedeUserID") }

// Request 描述一次接口调用。Form 和 JSON 二选一。
type Request struct {
	Method string // 空表示 GET；有 Form 或 JSON 时默认 POST
	URL    string
	Query  url.Values
	Form   url.Values
	JSON   any
	Header map[string]string // 覆盖默认请求头（User-Agent、Referer 等）

	WBI    bool // 给 Query 加 WBI 签名
	NoCSRF bool // 不自动往 Form 里补 csrf
}

// envelope 是 B 站接口的通用外壳。直播和漫画接口用 msg，其他用 message。
type envelope struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
	Result  json.RawMessage `json:"result"` // 番剧接口把数据放在 result
}

// Get 调用一个 GET 接口，把 data 解析进 out（out 可以为 nil）
func (c *Client) Get(ctx context.Context, rawURL string, query url.Values, out any) error {
	return c.Do(ctx, Request{URL: rawURL, Query: query}, out)
}

// GetWBI 和 Get 一样，另外给参数加 WBI 签名
func (c *Client) GetWBI(ctx context.Context, rawURL string, query url.Values, out any) error {
	return c.Do(ctx, Request{URL: rawURL, Query: query, WBI: true}, out)
}

// PostForm 以表单提交，自动带上 csrf
func (c *Client) PostForm(ctx context.Context, rawURL string, form url.Values, out any) error {
	if form == nil {
		form = url.Values{}
	}
	return c.Do(ctx, Request{URL: rawURL, Form: form}, out)
}

// PostJSON 以 JSON 提交
func (c *Client) PostJSON(ctx context.Context, rawURL string, body any, out any) error {
	return c.Do(ctx, Request{URL: rawURL, JSON: body}, out)
}

// Do 发出请求。接口返回的 code 不是 0 时返回 *Error；out 不为 nil 时把 data（或 result）解析进去。
func (c *Client) Do(ctx context.Context, r Request, out any) error {
	body, err := c.Raw(ctx, r)
	if err != nil {
		return err
	}
	return decodeEnvelope(body, shortURL(r.URL), out)
}

// decodeEnvelope 拆开接口的通用外壳：code 不是 0 时返回 *Error，否则把 data（或 result）解析进 out
func decodeEnvelope(body []byte, path string, out any) error {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return &Error{Code: CodeBadResponse, Message: "返回的不是 JSON", Path: path}
	}
	msg := env.Message
	if msg == "" {
		msg = env.Msg
	}
	code, key := env.code()
	if key != "" {
		return &Error{Code: CodeBadResponse, Key: key, Message: msg, Path: path, Data: env.Data}
	}
	if code != 0 {
		return &Error{Code: code, Message: msg, Path: path, Data: env.Data}
	}
	if out == nil {
		return nil
	}
	data := env.Data
	if len(data) == 0 || string(data) == "null" {
		data = env.Result
	}
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &Error{Code: CodeBadResponse, Message: "返回的数据格式变了：" + err.Error(), Path: path}
	}
	return nil
}

// code 返回数字 code；漫画等接口出错时 code 是字符串，这时返回 key
func (e envelope) code() (n int, key string) {
	if len(e.Code) == 0 || string(e.Code) == "null" {
		return 0, ""
	}
	var num json.Number
	if json.Unmarshal(e.Code, &num) == nil {
		if i, err := num.Int64(); err == nil {
			return int(i), ""
		}
	}
	var s string
	if json.Unmarshal(e.Code, &s) == nil && s != "" {
		if i, err := strconv.Atoi(s); err == nil {
			return i, ""
		}
		return 0, s
	}
	return 0, string(e.Code)
}

// Raw 发出请求并返回响应正文，不解析。HTTP 状态不是 200 时返回 *Error。
func (c *Client) Raw(ctx context.Context, r Request) ([]byte, error) {
	_, body, err := c.send(ctx, r)
	return body, err
}

func (c *Client) send(ctx context.Context, r Request) (*http.Response, []byte, error) {
	if c.Wait != nil {
		if err := c.Wait(ctx); err != nil {
			return nil, nil, err
		}
	}
	req, err := c.build(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	path := req.URL.Host + req.URL.Path
	start := time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		c.debugf("%s %s：%s（%d ms）", req.Method, path, netMessage(err), time.Since(start).Milliseconds())
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, &Error{Code: CodeNetwork, Message: netMessage(err), Path: path}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	c.debugf("%s %s：HTTP %d，%d 字节（%d ms）", req.Method, path, resp.StatusCode, len(body), time.Since(start).Milliseconds())
	if err != nil {
		return nil, nil, &Error{Code: CodeNetwork, Message: netMessage(err), Path: path}
	}
	if resp.StatusCode != http.StatusOK {
		e := &Error{Code: CodeBadResponse, Message: fmt.Sprintf("HTTP %d", resp.StatusCode), Path: path}
		// 412 是被风控拦截，和接口返回 -412 同样处理
		if resp.StatusCode == http.StatusPreconditionFailed {
			e.Code = -412
		}
		// 漫画等 twirp 接口出错时返回 HTTP 400，正文里的 code 是字符串
		var env envelope
		if json.Unmarshal(body, &env) == nil {
			if _, key := env.code(); key != "" {
				e.Key, e.Message = key, env.Msg
			}
		}
		return resp, body, e
	}
	return resp, body, nil
}

func (c *Client) build(ctx context.Context, r Request) (*http.Request, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}
	query := u.Query()
	for k, vs := range r.Query {
		query[k] = vs
	}
	method := r.Method
	var (
		rd    io.Reader
		ctype string
	)
	switch {
	case r.JSON != nil:
		b, err := json.Marshal(r.JSON)
		if err != nil {
			return nil, err
		}
		rd, ctype = bytes.NewReader(b), "application/json"
		if method == "" {
			method = http.MethodPost
		}
	case r.Form != nil:
		form := url.Values{}
		for k, vs := range r.Form {
			form[k] = vs
		}
		if csrf := c.CSRF(); !r.NoCSRF && csrf != "" && form.Get("csrf") == "" {
			form.Set("csrf", csrf)
		}
		rd, ctype = strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"
		if method == "" {
			method = http.MethodPost
		}
	}
	if method == "" {
		method = http.MethodGet
	}
	if r.WBI {
		keys, err := c.wbiKeys(ctx)
		if err != nil {
			return nil, err
		}
		u.RawQuery = wbiSign(query, keys, time.Now())
	} else {
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("User-Agent", WebUA)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	for k, v := range r.Header {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	for k, v := range c.Cookies() {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	return req, nil
}

func (c *Client) debugf(format string, args ...any) {
	if c.Debugf != nil {
		c.Debugf(format, args...)
	}
}

// shortURL 只留主机和路径：查询参数里可能有 access_key 等凭据，不进日志和报告
func shortURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Host + u.Path
}
