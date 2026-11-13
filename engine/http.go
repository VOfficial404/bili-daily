package engine

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili"
)

// 接口的说明见 docs/DESIGN.md。除了 /api/ping，都要带 Authorization: Bearer <令牌>。

var authFailMu sync.Mutex // 令牌不对时串行等待，拖慢暴力尝试

// Handler 返回给客户端用的接口
func (e *Engine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", e.handlePing)
	mux.HandleFunc("GET /api/state", e.auth(e.handleState))
	mux.HandleFunc("POST /api/qr", e.auth(e.handleQRStart))
	mux.HandleFunc("GET /api/qr/{id}", e.auth(e.handleQRPoll))
	mux.HandleFunc("POST /api/run", e.auth(e.handleRunAll))
	mux.HandleFunc("POST /api/accounts/{mid}/run", e.auth(e.handleRun))
	mux.HandleFunc("DELETE /api/accounts/{mid}/run", e.auth(e.handleRunCancel))
	mux.HandleFunc("POST /api/accounts/{mid}/refresh", e.auth(e.handleRefresh))
	mux.HandleFunc("PATCH /api/accounts/{mid}", e.auth(e.handlePatch))
	mux.HandleFunc("DELETE /api/accounts/{mid}", e.auth(e.handleRemove))
	mux.HandleFunc("GET /api/accounts/{mid}/groups", e.auth(e.handleGroups))
	mux.HandleFunc("GET /api/runs", e.auth(e.handleRuns))
	mux.HandleFunc("GET /api/runs/{id}", e.auth(e.handleRunDetail))
	mux.HandleFunc("PUT /api/settings", e.auth(e.handleSettings))
	mux.HandleFunc("POST /api/notify/test", e.auth(e.handleNotifyTest))
	mux.HandleFunc("POST /api/proxy/test", e.auth(e.handleProxyTest))
	mux.HandleFunc("GET /api/logs", e.auth(e.handleLogs))
	return mux
}

func writeJSONResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSONResp(w, code, api.Error{Message: msg})
}

func writeOK(w http.ResponseWriter) { writeJSONResp(w, http.StatusOK, struct{}{}) }

// writeActionErr 把操作的错误写成合适的状态码
func writeActionErr(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		writeOK(w)
	case errors.Is(err, errBusy):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, errNoAccount), errors.Is(err, errNoQR), errors.Is(err, os.ErrNotExist):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

// readBody 把请求正文解析进 v，不对时写出 400 并返回 false
func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式不对")
		return false
	}
	return true
}

func (e *Engine) auth(next http.HandlerFunc) http.HandlerFunc {
	want := sha256.Sum256([]byte(e.token))
	return func(w http.ResponseWriter, r *http.Request) {
		given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		got := sha256.Sum256([]byte(strings.TrimSpace(given)))
		if !ok || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			authFailMu.Lock()
			e.cfg.Sleep(r.Context(), time.Second)
			authFailMu.Unlock()
			e.Warnf("接口令牌不对（来自 %s）", r.RemoteAddr)
			writeErr(w, http.StatusUnauthorized, "令牌不对")
			return
		}
		next(w, r)
	}
}

func (e *Engine) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSONResp(w, http.StatusOK, api.Ping{Name: "bili-tools", Version: e.cfg.Version})
}

func (e *Engine) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSONResp(w, http.StatusOK, e.State())
}

func (e *Engine) handleQRStart(w http.ResponseWriter, r *http.Request) {
	q, err := e.StartQR(r.Context())
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeJSONResp(w, http.StatusOK, q)
}

func (e *Engine) handleQRPoll(w http.ResponseWriter, r *http.Request) {
	q, err := e.PollQR(r.Context(), r.PathValue("id"))
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeJSONResp(w, http.StatusOK, q)
}

func (e *Engine) handleRunAll(w http.ResponseWriter, r *http.Request) {
	if e.RunAll() == 0 {
		writeErr(w, http.StatusConflict, "没有可以立即运行的账号：可能都在运行中，或者都没有参加每日计划")
		return
	}
	writeOK(w)
}

// withAccount 取出路径里的账号，找不到时写出 404
func (e *Engine) withAccount(w http.ResponseWriter, r *http.Request) *account {
	a := e.account(r.PathValue("mid"))
	if a == nil {
		writeErr(w, http.StatusNotFound, errNoAccount.Error())
	}
	return a
}

func (e *Engine) handleRun(w http.ResponseWriter, r *http.Request) {
	a := e.withAccount(w, r)
	if a == nil {
		return
	}
	var req api.RunRequest
	if r.ContentLength != 0 && !readBody(w, r, &req) {
		return
	}
	for _, id := range req.Tasks {
		if _, ok := registry[id]; !ok {
			writeErr(w, http.StatusBadRequest, "没有这个任务："+id)
			return
		}
	}
	if !a.runNow(req.Tasks) {
		writeErr(w, http.StatusConflict, errBusy.Error())
		return
	}
	writeOK(w)
}

func (e *Engine) handleRunCancel(w http.ResponseWriter, r *http.Request) {
	a := e.withAccount(w, r)
	if a == nil {
		return
	}
	if !a.cancelRun() {
		writeErr(w, http.StatusConflict, "这个账号现在没有在运行")
		return
	}
	writeOK(w)
}

// handleRefresh 重新取账号资料，顺便确认登录还有效
func (e *Engine) handleRefresh(w http.ResponseWriter, r *http.Request) {
	a := e.withAccount(w, r)
	if a == nil {
		return
	}
	c, err := e.accountClient(a.mid)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	c.Wait = nil
	n, err := c.Nav(r.Context())
	if err != nil {
		if bili.IsNotLogin(err) {
			a.update(func(st *accountState) { st.LoggedIn = false })
			a.poke()
		}
		writeActionErr(w, err)
		return
	}
	a.saveNav(n)
	a.poke()
	writeOK(w)
}

func (e *Engine) handlePatch(w http.ResponseWriter, r *http.Request) {
	a := e.withAccount(w, r)
	if a == nil {
		return
	}
	var p api.AccountPatch
	if !readBody(w, r, &p) {
		return
	}
	if p.DisabledTasks != nil {
		for _, id := range *p.DisabledTasks {
			if _, ok := registry[id]; !ok {
				writeErr(w, http.StatusBadRequest, "没有这个任务："+id)
				return
			}
		}
	}
	a.update(func(st *accountState) {
		if p.Enabled != nil {
			st.Enabled = *p.Enabled
		}
		if p.DisabledTasks != nil {
			st.DisabledTasks = slices.Compact(slices.Sorted(slices.Values(*p.DisabledTasks)))
		}
	})
	a.poke()
	writeOK(w)
}

func (e *Engine) handleRemove(w http.ResponseWriter, r *http.Request) {
	writeActionErr(w, e.removeAccount(r.Context(), r.PathValue("mid")))
}

// handleGroups 列出账号的关注分组，批量取关时选
func (e *Engine) handleGroups(w http.ResponseWriter, r *http.Request) {
	a := e.withAccount(w, r)
	if a == nil {
		return
	}
	c, err := e.accountClient(a.mid)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	c.Wait = nil
	tags, err := c.Tags(r.Context())
	if err != nil {
		writeActionErr(w, err)
		return
	}
	out := []api.Group{}
	for _, t := range tags {
		out = append(out, api.Group{ID: t.ID, Name: t.Name, Count: t.Count})
	}
	writeJSONResp(w, http.StatusOK, out)
}

func (e *Engine) handleRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	writeJSONResp(w, http.StatusOK, e.listRuns(q.Get("mid"), limit))
}

func (e *Engine) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	run, err := e.loadRun(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "找不到这份运行报告")
		return
	}
	writeJSONResp(w, http.StatusOK, run)
}

func (e *Engine) handleSettings(w http.ResponseWriter, r *http.Request) {
	// 从当前设置开始解析：请求里没有的项保持不变
	s := e.Settings()
	if !readBody(w, r, &s) {
		return
	}
	s, err := e.UpdateSettings(s)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	e.Infof("设置已保存")
	writeJSONResp(w, http.StatusOK, s)
}

// handleNotifyTest 用请求里的推送设置（不一定保存过）发一条测试消息
func (e *Engine) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	s := e.Settings()
	if !readBody(w, r, &s.Notify) {
		return
	}
	if err := checkSettings(&s); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := e.push(r.Context(), s.Notify, "bili-tools 测试消息", "收到这条消息说明推送设置是对的。"); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeOK(w)
}

// handleProxyTest 查询经过请求里的代理（不一定保存过）时 B 站看到的出口 IP
func (e *Engine) handleProxyTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Proxy string `json:"proxy"`
	}
	if !readBody(w, r, &body) {
		return
	}
	eg, err := e.probe(r.Context(), strings.TrimSpace(body.Proxy))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSONResp(w, http.StatusOK, eg)
}

// handleLogs 返回日志。after 是上次拿到的最后一条的序号，只返回更新的；level 是最低级别；mid 只看一个账号。
func (e *Engine) handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	min := levelDebug
	if s := q.Get("level"); s != "" {
		if !validLevel(s) {
			writeErr(w, http.StatusBadRequest, "日志级别只能是 debug、info、warn 或 error")
			return
		}
		min = parseLevel(s)
	}
	writeJSONResp(w, http.StatusOK, e.logs.list(after, min, q.Get("mid")))
}
