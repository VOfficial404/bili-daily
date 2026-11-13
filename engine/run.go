package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili"
)

// 每日计划因为网络问题连登录检查都没过时，隔一会儿再试，一天最多 maxRetries 次
const (
	retryAfter = 30 * time.Minute
	maxRetries = 3
)

var triggerNames = map[string]string{
	"schedule": "定时",
	"catchup":  "补跑",
	"retry":    "重试",
	"manual":   "手动",
}

// runAccount 给账号跑一轮：检查登录，依次执行任务，保存报告并推送
func (e *Engine) runAccount(ctx context.Context, a *account, trigger string, ids []string, started time.Time) *api.Run {
	set := e.Settings()
	today := day(started)
	plan := a.plan(&set, ids)
	run := &api.Run{
		RunSummary: api.RunSummary{ID: runID(started, a.mid), Mid: a.mid, Trigger: trigger, Started: started},
		Tasks:      []api.TaskResult{},
	}
	a.progress("登录检查", 0, len(plan)+1)
	a.infof("开始运行（%s）：%d 个任务", triggerNames[trigger], len(plan))

	// 登录检查也按一个任务记在报告里
	login := api.TaskResult{ID: "login", Title: "登录检查", Started: started, Steps: []api.Step{}}
	x := &Ctx{Context: ctx, Set: set, Day: today, e: e, acc: a, res: &login}
	var before, after *bili.Nav
	c, err := e.accountClient(a.mid)
	if err != nil {
		x.Failf("登录状态", bili.ReasonNotLogin, "%v", err)
	} else if before, err = c.Nav(ctx); err != nil {
		x.Fail("登录状态", err)
	} else {
		a.saveNav(before)
		x.Mid, x.C, x.Nav = before.Mid, c, before
		x.OK("登录状态", "%s，Lv%d，硬币 %s", before.Uname, before.LevelInfo.Level, trimFloat(before.Money))
		e.checkCookie(x)
	}
	finish(&login)
	login.Millis = e.now().Sub(started).Milliseconds()
	run.Tasks = append(run.Tasks, login)
	loggedOut := x.stop

	if before != nil {
		for i, t := range plan {
			if ctx.Err() != nil || loggedOut {
				break
			}
			a.progress(t.Info.Title, i+1, len(plan)+1)
			res, stop := e.runTask(ctx, a, c, before, set, today, t)
			run.Tasks = append(run.Tasks, res)
			loggedOut = stop
		}
		if !loggedOut && ctx.Err() == nil {
			// 再取一次资料：界面上的硬币和经验是跑完之后的
			if after, err = c.Nav(ctx); err == nil {
				a.saveNav(after)
			}
		}
	}
	run.Ended = e.now()
	run.Uname = a.name()
	summarize(run, before, after, ctx.Err() != nil)

	networkDown := before == nil && len(login.Steps) > 0 && login.Steps[0].Reason == string(bili.ReasonNetwork)
	ranLottery := slices.ContainsFunc(run.Tasks, func(r api.TaskResult) bool { return r.ID == TaskLiveLottery })
	a.mu.Lock()
	if trigger != "manual" && len(ids) == 0 {
		if a.retryDay != today {
			a.retryDay, a.retries = today, 0
		}
		a.retryAt = time.Time{}
		if networkDown && a.retries < maxRetries {
			a.retries++
			a.retryAt = run.Ended.Add(retryAfter)
		}
	}
	a.mu.Unlock()
	a.update(func(st *accountState) {
		if loggedOut {
			st.LoggedIn = false
		}
		if len(ids) == 0 {
			st.DailyDate = today
		}
		if ranLottery {
			st.LotteryAt = run.Ended
		}
		if st.TodayDate != today {
			st.TodayDate, st.Today = today, nil
		}
		if st.Today == nil {
			st.Today = map[string]api.TaskStatus{}
		}
		for _, r := range run.Tasks[1:] {
			st.Today[r.ID] = api.TaskStatus{ID: r.ID, Status: r.Status, Note: r.Note}
		}
		sum := run.RunSummary
		st.LastRun = &sum
	})

	lv := levelInfo
	if run.Status != api.StatusOK {
		lv = levelWarn
	}
	e.logf(lv, a.mid, "运行结束：%s", run.Summary)
	if err := e.saveRun(run); err != nil {
		e.Errorf("保存运行报告失败：%v", err)
	}
	if trigger != "manual" {
		e.notifyRun(a, run, loggedOut)
	}
	return run
}

// runTask 执行一个任务。任务里的 panic 记成一步失败，不影响其他任务。
func (e *Engine) runTask(ctx context.Context, a *account, c *bili.Client, nav *bili.Nav, set api.Settings, today string, t Task) (api.TaskResult, bool) {
	res := api.TaskResult{ID: t.Info.ID, Title: t.Info.Title, Started: e.now(), Steps: []api.Step{}}
	x := &Ctx{Context: ctx, C: c, Nav: nav, Set: set, Mid: nav.Mid, Day: today, e: e, acc: a, res: &res}
	func() {
		defer func() {
			if r := recover(); r != nil {
				e.Errorf("任务 %s 出错：%v\n%s", t.Info.ID, r, debug.Stack())
				x.Failf("运行", bili.ReasonOther, "程序出错：%v。请把日志反馈给开发者", r)
			}
		}()
		t.Run(x)
	}()
	finish(&res)
	res.Millis = e.now().Sub(res.Started).Milliseconds()
	return res, x.stop
}

// checkCookie 每天问一次 B 站 Cookie 是否该刷新了，需要时换一份新的
func (e *Engine) checkCookie(x *Ctx) {
	a := x.acc
	if a.snapshot().CookieDate == x.Day {
		return
	}
	need, ts, err := x.C.CookieNeedsRefresh(x)
	if err != nil {
		if bili.IsNotLogin(err) {
			x.Fail("Cookie", err)
		} else {
			a.warnf("检查 Cookie 是否要刷新失败：%v", err)
		}
		return
	}
	a.update(func(st *accountState) { st.CookieDate = x.Day })
	if !need {
		return
	}
	sess, err := e.loadSession(a.mid)
	if err != nil || sess.RefreshToken == "" {
		x.Skip("Cookie 刷新", "B 站提示 Cookie 该刷新了，但这个账号没有 refresh_token，过期后需要重新扫码")
		return
	}
	oldToken := sess.RefreshToken
	s, err := x.C.RefreshCookie(x, sess.RefreshToken, ts)
	if err != nil {
		if errors.Is(err, bili.ErrRefreshMismatch) {
			x.Failf("Cookie 刷新", bili.ReasonNotLogin, "%v", err)
			x.stop = false // 旧 Cookie 现在还能用，这一轮继续
		} else {
			x.Fail("Cookie 刷新", err)
		}
		return
	}
	if !s.Expires.IsZero() {
		sess.CookieExpires = s.Expires
	}
	sess.Cookies, sess.RefreshToken = s.Cookies, s.RefreshToken
	if _, err := e.saveSession(sess); err != nil {
		x.Failf("Cookie 刷新", bili.ReasonOther, "新 Cookie 保存失败：%v", err)
		return
	}
	if err := x.C.ConfirmRefresh(x, oldToken); err != nil {
		a.warnf("新 Cookie 已保存，但作废旧会话失败：%v", err)
	}
	if s.Expires.IsZero() {
		x.OK("Cookie 刷新", "已换成新的 Cookie")
		return
	}
	a.update(func(st *accountState) { st.CookieExpires = &s.Expires })
	x.OK("Cookie 刷新", "已换成新的 Cookie，有效期到 %s", s.Expires.In(Beijing).Format("2006-01-02"))
}

// summarize 算出一轮的结果和一句话总结
func summarize(run *api.Run, before, after *bili.Nav, canceled bool) {
	login := run.Tasks[0]
	if before == nil {
		run.Status, run.Summary = api.StatusFailed, login.Note
		return
	}
	var ok, partial, failed, skipped int
	for _, r := range run.Tasks[1:] {
		switch r.Status {
		case api.StatusOK:
			ok++
		case api.StatusPartial:
			partial++
		case api.StatusFailed:
			failed++
		default:
			skipped++
		}
	}
	if login.Status == api.StatusFailed || login.Status == api.StatusPartial {
		partial++ // Cookie 刷新失败也算有问题
	}
	switch {
	case failed+partial == 0:
		run.Status = api.StatusOK
	case ok+partial == 0:
		run.Status = api.StatusFailed
	default:
		run.Status = api.StatusPartial
	}

	var parts []string
	add := func(n int, format string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf(format, n))
		}
	}
	add(ok, "%d 个任务完成")
	add(partial, "%d 个部分完成")
	add(failed, "%d 个失败")
	add(skipped, "%d 个不需要做")
	if len(parts) == 0 {
		parts = append(parts, "没有要做的任务")
	}
	if canceled {
		parts = append([]string{"已中止"}, parts...)
		if run.Status == api.StatusOK {
			run.Status = api.StatusPartial
		}
	}
	s := strings.Join(parts, "，")
	if after != nil {
		var gains []string
		if d := after.LevelInfo.Exp - before.LevelInfo.Exp; d > 0 {
			gains = append(gains, fmt.Sprintf("经验 +%d", d))
		}
		if d := after.Money - before.Money; d > 0.001 || d < -0.001 {
			gains = append(gains, "硬币 "+signed(d))
		}
		if len(gains) > 0 {
			s += "；" + strings.Join(gains, "，")
		}
	}
	run.Summary = s
}

// trimFloat 把 12.0 写成 12，12.5 写成 12.5
func trimFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func signed(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	if f > 0 {
		return "+" + s
	}
	return s
}
