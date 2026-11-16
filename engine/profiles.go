package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

// 查到过的 UP 主资料留一天，面板每次打开不再问 B 站
const (
	upCacheTTL = 24 * time.Hour
	upCacheMax = 256
)

type cachedUP struct {
	p  api.UPProfile
	at time.Time
}

func (e *Engine) cachedUP(mid string) (api.UPProfile, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.ups[mid]
	if !ok || e.now().Sub(c.at) > upCacheTTL {
		return api.UPProfile{}, false
	}
	return c.p, true
}

func (e *Engine) cacheUP(p api.UPProfile) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.ups) >= upCacheMax {
		clear(e.ups)
	}
	e.ups[p.Mid] = cachedUP{p: p, at: e.now()}
}

// handleUPProfile 只返回公开资料，账号 Cookie 始终留在服务端。
// 用账号查询时和这个账号的任务共用请求间隔：一次填了几个 UID 就一个个排队查，慢节奏下要等一会儿。
func (e *Engine) handleUPProfile(w http.ResponseWriter, r *http.Request) {
	mid := r.PathValue("mid")
	id, err := strconv.ParseInt(mid, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != mid {
		writeErr(w, http.StatusBadRequest, "UP 主 UID 必须是不含前导零的正整数")
		return
	}
	if p, ok := e.cachedUP(mid); ok {
		writeJSONResp(w, http.StatusOK, p)
		return
	}
	var c *bili.Client
	for _, a := range e.accountList() {
		if !a.snapshot().LoggedIn {
			continue
		}
		c, err = e.accountClient(a.mid)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "读取用于查询 UP 主资料的登录账号失败，请重新登录")
			return
		}
		break
	}
	if c == nil {
		c = e.anonClient()
	}
	p, err := c.UPProfile(r.Context(), id)
	if err != nil {
		switch {
		case bili.IsCode(err, -404):
			writeErr(w, http.StatusNotFound, fmt.Sprintf("UID %s 的 UP 主不存在或已注销", mid))
		case errors.Is(err, context.DeadlineExceeded):
			writeErr(w, http.StatusGatewayTimeout, "查询 UP 主资料超时，请稍后重试")
		case bili.IsCode(err, -101, -403):
			writeErr(w, http.StatusBadGateway, "无法查询 UP 主资料：B 站要求有效的登录账号，请先扫码登录或刷新登录")
		default:
			writeErr(w, http.StatusBadGateway, "查询 UP 主资料失败："+err.Error())
		}
		return
	}
	out := api.UPProfile{Mid: strconv.FormatInt(p.Mid, 10), Uname: p.Uname, Face: p.Face}
	e.cacheUP(out)
	writeJSONResp(w, http.StatusOK, out)
}

// handleFollowings 列出账号的一页关注，q 不为空时在关注里按昵称搜索。选优先 UP 主和充电对象用，
// 和这个账号的任务共用请求间隔；列出来的资料记进 UP 主资料缓存，选上后不用再查一次。
func (e *Engine) handleFollowings(w http.ResponseWriter, r *http.Request) {
	a := e.withAccount(w, r)
	if a == nil {
		return
	}
	q := r.URL.Query()
	pn := 1
	if s := q.Get("pn"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 250 {
			writeErr(w, http.StatusBadRequest, "页码只能是 1～250")
			return
		}
		pn = n
	}
	name := strings.TrimSpace(q.Get("q"))
	if utf8.RuneCountInString(name) > 32 {
		writeErr(w, http.StatusBadRequest, "搜索的昵称最多 32 个字")
		return
	}
	c, err := e.accountClient(a.mid)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	var users []bili.User
	var total int
	if name == "" {
		users, total, err = c.Followings(r.Context(), pn, true)
	} else {
		users, total, err = c.SearchFollowings(r.Context(), name, pn)
	}
	if err != nil {
		writeActionErr(w, err)
		return
	}
	out := api.Followings{List: []api.UPProfile{}, Total: total, More: len(users) > 0 && pn*20 < total}
	for _, u := range users {
		p := api.UPProfile{Mid: strconv.FormatInt(u.Mid, 10), Uname: strings.TrimSpace(u.Uname), Face: u.Face}
		if u.Mid <= 0 || p.Uname == "" {
			continue
		}
		out.List = append(out.List, p)
		if p.Face != "" {
			e.cacheUP(p)
		}
	}
	writeJSONResp(w, http.StatusOK, out)
}
