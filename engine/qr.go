package engine

import (
	"context"
	"errors"
	"time"

	"rsc.io/qr"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili"
)

// B 站的登录二维码 180 秒后过期
const qrTTL = 180 * time.Second

var errNoQR = errors.New("二维码不存在或已经过期，请重新获取")

// qrLogin 是一次进行中的扫码登录
type qrLogin struct {
	q       *bili.WebQR
	c       *bili.Client
	created time.Time
	result  *api.QR // 已经有了最终结果（成功或过期）
}

func qrPNG(content string) ([]byte, error) {
	code, err := qr.Encode(content, qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = 8
	return code.PNG(), nil
}

// StartQR 向 B 站申请一张登录二维码。请求按当前设置走代理，登录 IP 就是出口 IP。
func (e *Engine) StartQR(ctx context.Context) (api.QR, error) {
	c := e.anonClient()
	q, err := c.NewWebQR(ctx)
	if err != nil {
		return api.QR{}, err
	}
	png, err := qrPNG(q.URL)
	if err != nil {
		return api.QR{}, err
	}
	id := randomHex(8)
	now := e.now()
	e.mu.Lock()
	for k, l := range e.qrs {
		if now.Sub(l.created) > qrTTL+time.Minute {
			delete(e.qrs, k)
		}
	}
	e.qrs[id] = &qrLogin{q: q, c: c, created: now}
	e.mu.Unlock()
	return api.QR{ID: id, PNG: png, URL: q.URL, State: string(bili.QRWaiting)}, nil
}

// PollQR 查询一次扫码状态。成功时保存会话，账号开始排程。
func (e *Engine) PollQR(ctx context.Context, id string) (api.QR, error) {
	e.mu.Lock()
	l := e.qrs[id]
	e.mu.Unlock()
	if l == nil {
		return api.QR{}, errNoQR
	}
	if l.result != nil {
		return *l.result, nil
	}
	out := api.QR{ID: id}
	if e.now().Sub(l.created) > qrTTL {
		out.State = string(bili.QRExpired)
		return out, nil
	}
	state, sess, err := l.c.PollWebQR(ctx, l.q)
	if err != nil {
		return out, err
	}
	out.State = string(state)
	if state != bili.QRSuccess {
		if state == bili.QRExpired {
			e.finishQR(l, out)
		}
		return out, nil
	}

	// 带上设备标识，之后的请求更像浏览器；取不到不影响登录
	if fp, err := l.c.Fingerprint(ctx); err == nil {
		for k, v := range fp {
			if sess.Cookies[k] == "" {
				sess.Cookies[k] = v
			}
		}
	}
	mid, err := e.saveSession(session{Cookies: sess.Cookies, RefreshToken: sess.RefreshToken, CookieExpires: sess.Expires})
	if err != nil {
		return out, err
	}
	out.Mid = mid
	e.finishQR(l, out)
	e.accountLoggedIn(mid)
	if a := e.account(mid); a != nil {
		a.update(func(st *accountState) {
			st.CookieExpires, st.CookieDate = nil, ""
			if !sess.Expires.IsZero() {
				st.CookieExpires = &sess.Expires
			}
		})
		// 立刻取一次资料，界面上马上能看到昵称和头像
		if c, err := e.accountClient(mid); err == nil {
			c.Wait = nil
			if n, err := c.Nav(ctx); err == nil {
				a.saveNav(n)
			}
		}
		a.infof("扫码登录成功")
		a.poke()
	}
	return out, nil
}

func (e *Engine) finishQR(l *qrLogin, result api.QR) {
	e.mu.Lock()
	l.result = &result
	e.mu.Unlock()
}
