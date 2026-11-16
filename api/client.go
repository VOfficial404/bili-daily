package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client 是服务端接口的客户端，桌面客户端和手机 App 用它。
// 连服务器和本机运行用的是同一套接口，区别只是地址。
type Client struct {
	base  string
	token string
	hc    *http.Client
}

// NormalizeBase 整理用户填的服务器地址：补上 http://，去掉末尾的斜杠
func NormalizeBase(s string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return "", errors.New("请填写服务器地址")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("服务器地址不对，应写成 http://192.168.1.10:8686 或 https://bili.example.com")
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
}

// NewClient 返回连接 base（NormalizeBase 整理过的地址）的客户端
func NewClient(base, token string) *Client {
	return &Client{base: base, token: strings.TrimSpace(token), hc: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Base() string { return c.base }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			if ue.Timeout() {
				return errors.New("连接服务器超时")
			}
			err = ue.Err
		}
		return fmt.Errorf("连不上服务器：%v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("读取服务器的响应失败：%v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e := &Error{Status: resp.StatusCode}
		if json.Unmarshal(data, e) != nil || e.Message == "" {
			e.Message = fmt.Sprintf("服务器返回 HTTP %d，这个地址可能不是 bili-daily", resp.StatusCode)
		}
		return e
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("服务器的响应看不懂，这个地址可能不是 bili-daily，或者版本不一致")
	}
	return nil
}

// IsUnauthorized 表示令牌不对
func IsUnauthorized(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusUnauthorized
}

// Ping 确认地址是一个 bili-daily 服务端，不需要令牌
func (c *Client) Ping(ctx context.Context) (Ping, error) {
	var p Ping
	err := c.do(ctx, http.MethodGet, "/api/ping", nil, &p)
	if err == nil && p.Name != "bili-daily" {
		err = errors.New("这个地址不是 bili-daily 的服务端")
	}
	return p, err
}

// 下面的方法都用具名返回值：`return v, c.do(…, &v)` 里读 v 和调用 do 的先后 Go 没有规定，
// 可能返回的是 do 填充之前的值。

func (c *Client) State(ctx context.Context) (s State, err error) {
	err = c.do(ctx, http.MethodGet, "/api/state", nil, &s)
	return s, err
}

// StartQR 申请一张登录二维码
func (c *Client) StartQR(ctx context.Context) (q QR, err error) {
	err = c.do(ctx, http.MethodPost, "/api/qr", nil, &q)
	return q, err
}

// PollQR 查询扫码状态
func (c *Client) PollQR(ctx context.Context, id string) (q QR, err error) {
	err = c.do(ctx, http.MethodGet, "/api/qr/"+url.PathEscape(id), nil, &q)
	return q, err
}

// RunAll 让所有参加每日计划的账号立即跑一轮
func (c *Client) RunAll(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/run", nil, nil)
}

// Run 让一个账号立即跑一轮；tasks 为空表示每日计划里的全部任务
func (c *Client) Run(ctx context.Context, mid string, tasks []string) error {
	return c.do(ctx, http.MethodPost, "/api/accounts/"+mid+"/run", RunRequest{Tasks: tasks}, nil)
}

// Cancel 中止账号正在进行的一轮
func (c *Client) Cancel(ctx context.Context, mid string) error {
	return c.do(ctx, http.MethodDelete, "/api/accounts/"+mid+"/run", nil, nil)
}

// Refresh 重新取账号资料
func (c *Client) Refresh(ctx context.Context, mid string) error {
	return c.do(ctx, http.MethodPost, "/api/accounts/"+mid+"/refresh", nil, nil)
}

func (c *Client) Patch(ctx context.Context, mid string, p AccountPatch) error {
	return c.do(ctx, http.MethodPatch, "/api/accounts/"+mid, p, nil)
}

// Remove 退出账号：在 B 站注销会话并删除服务端保存的数据
func (c *Client) Remove(ctx context.Context, mid string) error {
	return c.do(ctx, http.MethodDelete, "/api/accounts/"+mid, nil, nil)
}

// Groups 列出账号的关注分组
func (c *Client) Groups(ctx context.Context, mid string) (g []Group, err error) {
	err = c.do(ctx, http.MethodGet, "/api/accounts/"+mid+"/groups", nil, &g)
	return g, err
}

// Followings 取账号关注的第 pn 页（从 1 开始，每页 20 个）；q 不为空时在关注里按昵称搜索
func (c *Client) Followings(ctx context.Context, mid string, pn int, q string) (f Followings, err error) {
	v := url.Values{"pn": {strconv.Itoa(pn)}}
	if q != "" {
		v.Set("q", q)
	}
	err = c.do(ctx, http.MethodGet, "/api/accounts/"+url.PathEscape(mid)+"/followings?"+v.Encode(), nil, &f)
	return f, err
}

// UPProfile 查询指定 UID 的 UP 主昵称和头像。
func (c *Client) UPProfile(ctx context.Context, mid string) (p UPProfile, err error) {
	err = c.do(ctx, http.MethodGet, "/api/ups/"+url.PathEscape(mid), nil, &p)
	return p, err
}

// Runs 返回最近的运行报告摘要，从新到旧；mid 为空时是所有账号的
func (c *Client) Runs(ctx context.Context, mid string, limit int) (r []RunSummary, err error) {
	q := url.Values{"mid": {mid}, "limit": {strconv.Itoa(limit)}}
	err = c.do(ctx, http.MethodGet, "/api/runs?"+q.Encode(), nil, &r)
	return r, err
}

// RunDetail 返回一份完整的运行报告
func (c *Client) RunDetail(ctx context.Context, id string) (r Run, err error) {
	err = c.do(ctx, http.MethodGet, "/api/runs/"+url.PathEscape(id), nil, &r)
	return r, err
}

// SaveSettings 保存设置，返回服务端整理过的设置
func (c *Client) SaveSettings(ctx context.Context, s Settings) (out Settings, err error) {
	err = c.do(ctx, http.MethodPut, "/api/settings", s, &out)
	return out, err
}

// TestNotify 用 n（不一定保存过）发一条测试消息
func (c *Client) TestNotify(ctx context.Context, n Notify) error {
	return c.do(ctx, http.MethodPost, "/api/notify/test", n, nil)
}

// TestProxy 查询经过 proxy 时 B 站看到的出口 IP
func (c *Client) TestProxy(ctx context.Context, proxy string) (e Egress, err error) {
	err = c.do(ctx, http.MethodPost, "/api/proxy/test", map[string]string{"proxy": proxy}, &e)
	return e, err
}

// Logs 返回序号大于 after 的日志；level 是最低级别（空表示全部），mid 不为空时只看一个账号
func (c *Client) Logs(ctx context.Context, after int64, level, mid string) (l []LogEntry, err error) {
	q := url.Values{"after": {strconv.FormatInt(after, 10)}}
	if level != "" {
		q.Set("level", level)
	}
	if mid != "" {
		q.Set("mid", mid)
	}
	err = c.do(ctx, http.MethodGet, "/api/logs?"+q.Encode(), nil, &l)
	return l, err
}
