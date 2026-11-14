package engine

import (
	"context"
	"hash/fnv"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

// account 是一个账号：保存的状态，加上它自己的排程循环
type account struct {
	e   *Engine
	mid string

	mu          sync.Mutex
	st          accountState
	running     *api.Progress
	queued      bool // 手动运行已经排上，还没开始
	cancel      context.CancelFunc
	runDone     chan struct{}
	closing     bool
	paused      bool
	lifecycleMu sync.Mutex // 串行化重新登录与删除，等待运行时不持有 mu
	retryAt     time.Time  // 每日计划因为网络问题没跑成时，什么时候再试
	retryDay    string
	retries     int

	wake      chan struct{}
	manual    chan []string
	done      chan struct{}
	closeOnce sync.Once
}

func (e *Engine) newAccount(mid string) *account {
	return &account{
		e:      e,
		mid:    mid,
		st:     e.loadState(mid),
		wake:   make(chan struct{}, 1),
		manual: make(chan []string, 1),
		done:   make(chan struct{}),
	}
}

func (a *account) name() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.st.Uname != "" {
		return a.st.Uname
	}
	return "UID " + a.mid
}

func (a *account) infof(format string, args ...any) { a.e.logf(levelInfo, a.mid, format, args...) }
func (a *account) warnf(format string, args ...any) { a.e.logf(levelWarn, a.mid, format, args...) }

// snapshot 返回状态的副本
func (a *account) snapshot() accountState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneAccountState(a.st)
}

// update 修改状态并保存
func (a *account) update(fn func(st *accountState)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errNoAccount
	}
	st := cloneAccountState(a.st)
	fn(&st)
	if err := writeJSON(a.e.stateFile(a.mid), st); err != nil {
		a.e.Errorf("保存账号 %s 的状态失败：%v", a.mid, err)
		return err
	}
	a.st = st
	return nil
}

func cloneAccountState(st accountState) accountState {
	st.DisabledTasks = slices.Clone(st.DisabledTasks)
	st.Today = maps.Clone(st.Today)
	st.Marks = maps.Clone(st.Marks)
	return st
}

// saveNav 把 nav 返回的资料记到状态里
func (a *account) saveNav(n *bili.Nav) {
	now := a.e.now()
	a.update(func(st *accountState) {
		st.Uname, st.Face = n.Uname, n.Face
		st.Level, st.Exp, st.NextExp = n.LevelInfo.Level, n.LevelInfo.Exp, n.NextExp()
		st.Coins, st.BCoupon = n.Money, n.Wallet.Coupon
		st.VIP, st.VIPDue = n.VIPName(), nil
		if due := n.VIPDue(); !due.IsZero() {
			st.VIPDue = &due
		}
		st.UpdatedAt = &now
		st.LoggedIn = true
	})
}

// poke 让排程循环重新算下一次运行（设置或开关变了）
func (a *account) poke() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// close 停下排程循环
func (a *account) close() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closing = true
	a.closeOnce.Do(func() { close(a.done) })
	if a.cancel != nil {
		a.cancel()
	}
	return a.runDone
}

// cancelRun 中止正在进行的一轮
func (a *account) cancelRun() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel == nil {
		return false
	}
	a.cancel()
	return true
}

// runNow 让排程循环立即跑一轮；正在运行或已经排上时返回 false
func (a *account) runNow(ids []string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing || a.paused || a.e.ctx.Err() != nil || a.running != nil || a.queued {
		return false
	}
	select {
	case a.manual <- ids:
		a.queued = true
		return true
	default:
		return false
	}
}

// dailyTime 是账号在 date 那天（北京时间）开始每日计划的时间：
// 设置的开始时间加上 0~Jitter 分钟。推迟多少由账号和日期决定，所以每天不同、重启后不变。
func dailyTime(set *api.Settings, mid string, date time.Time) time.Time {
	at, _ := time.ParseInLocation("15:04", set.RunAt, Beijing)
	y, m, d := date.In(Beijing).Date()
	t := time.Date(y, m, d, at.Hour(), at.Minute(), 0, 0, Beijing)
	if set.Jitter > 0 {
		h := fnv.New32a()
		h.Write([]byte(mid + t.Format("2006-01-02")))
		// 随机推迟只在同一个北京日期内采样，避免午夜后重新排程漏掉前一天。
		last := time.Date(y, m, d, 23, 59, 59, 0, Beijing)
		seconds := min(set.Jitter*60, int(last.Sub(t)/time.Second))
		t = t.Add(time.Duration(h.Sum32()%uint32(seconds+1)) * time.Second)
	}
	return t
}

// nextDue 算出下一次定时运行：什么时候、跑哪些任务（nil 表示每日计划的全部）、因为什么。
// 账号停用或登录失效时 ok 为 false。
func (a *account) nextDue(now time.Time) (when time.Time, ids []string, trigger string, ok bool) {
	set := a.e.Settings()
	a.mu.Lock()
	st, retryAt := a.st, a.retryAt
	stopped := a.closing || a.paused || a.e.ctx.Err() != nil
	a.mu.Unlock()
	if stopped || !st.Enabled || !st.LoggedIn {
		return time.Time{}, nil, "", false
	}
	today := day(now)
	trigger = "schedule"
	switch {
	case st.DailyDate != today:
		when = dailyTime(&set, a.mid, now)
		if now.Sub(when) > 10*time.Minute {
			trigger = "catchup" // 到点的时候程序没在运行，现在补上
		}
	case !retryAt.IsZero():
		when, trigger = retryAt, "retry"
	default:
		when = dailyTime(&set, a.mid, now.In(Beijing).AddDate(0, 0, 1))
	}

	l := set.Tasks.LiveLottery
	if l.Enabled && l.EveryHours > 0 && !st.LotteryAt.IsZero() && !slices.Contains(st.DisabledTasks, TaskLiveLottery) {
		if _, has := registry[TaskLiveLottery]; has {
			if next := st.LotteryAt.Add(time.Duration(l.EveryHours) * time.Hour); next.Before(when) {
				return next, []string{TaskLiveLottery}, "schedule", true
			}
		}
	}
	return when, nil, trigger, true
}

// loop 是账号的排程循环：等到下一次定时运行或者手动运行
func (a *account) loop() {
	for {
		when, ids, trigger, ok := a.nextDue(a.e.now())
		var (
			timer *time.Timer
			fire  <-chan time.Time
		)
		if ok {
			// 至多等一分钟就重新看墙上时钟：手机、电脑睡眠时 Go 的单调时钟可能不走，
			// 醒来后不能还等着睡前算出的那几个小时
			timer = time.NewTimer(min(time.Minute, max(0, when.Sub(a.e.now()))))
			fire = timer.C
		}
		var run *api.Run
		ran := false
		select {
		case <-a.done:
		case <-a.e.ctx.Done():
		case <-a.wake:
		case ids := <-a.manual:
			run, ran = a.execute(a.e.ctx, "manual", ids), true
			a.mu.Lock()
			a.queued = false
			a.mu.Unlock()
		case <-fire:
			if !a.e.now().Before(when) {
				run, ran = a.execute(a.e.ctx, trigger, ids), true
			}
		}
		if timer != nil {
			timer.Stop()
		}
		select {
		case <-a.done:
			return
		case <-a.e.ctx.Done():
			return
		default:
		}
		if ran && run == nil {
			// 别处（RunNow）正在跑这个账号，等一会儿再看
			sleepCtx(a.e.ctx, 5*time.Second)
		}
	}
}

// execute 跑一轮并返回报告；这个账号已经在运行时返回 nil
func (a *account) execute(parent context.Context, trigger string, ids []string) *api.Run {
	if !a.e.beginWork() {
		return nil
	}
	defer a.e.workers.Done()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stopEngineCancel := context.AfterFunc(a.e.ctx, cancel)
	defer stopEngineCancel()
	started := a.e.now()
	a.mu.Lock()
	if a.closing || a.paused || a.e.ctx.Err() != nil || a.running != nil {
		a.mu.Unlock()
		return nil
	}
	a.running = &api.Progress{RunID: runID(started, a.mid), Started: started, Task: "登录检查"}
	a.cancel = cancel
	done := make(chan struct{})
	a.runDone = done
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.running, a.cancel, a.runDone = nil, nil, nil
		close(done)
		a.mu.Unlock()
	}()
	return a.e.runAccount(ctx, a, trigger, ids, started)
}

// progress 更新正在进行的一轮的进度
func (a *account) progress(task string, done, total int) {
	a.mu.Lock()
	if a.running != nil {
		a.running.Task, a.running.Done, a.running.Total = task, done, total
	}
	a.mu.Unlock()
}

// plan 决定这一轮跑哪些任务。ids 为空时是每日计划：设置里开着、这个账号没有关掉的任务。
func (a *account) plan(set *api.Settings, ids []string) []Task {
	disabled := a.snapshot().DisabledTasks
	var out []Task
	for _, t := range allTasks() {
		if len(ids) > 0 {
			if slices.Contains(ids, t.Info.ID) {
				out = append(out, t)
			}
			continue
		}
		if t.Info.Manual || t.Enabled == nil || !t.Enabled(set) || slices.Contains(disabled, t.Info.ID) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// status 是给客户端看的账号状态
func (a *account) status() api.Account {
	now := a.e.now()
	when, _, _, due := a.nextDue(now)

	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.st
	acc := api.Account{
		Mid:           a.mid,
		Uname:         st.Uname,
		Face:          st.Face,
		Enabled:       st.Enabled,
		DisabledTasks: append([]string{}, st.DisabledTasks...),
		LoggedIn:      st.LoggedIn,
		Level:         st.Level,
		Exp:           st.Exp,
		NextExp:       st.NextExp,
		Coins:         st.Coins,
		BCoupon:       st.BCoupon,
		VIP:           st.VIP,
		VIPDue:        st.VIPDue,
		CookieExpires: st.CookieExpires,
		UpdatedAt:     st.UpdatedAt,
		LastRun:       st.LastRun,
		Today:         []api.TaskStatus{},
	}
	if acc.Uname == "" {
		acc.Uname = "UID " + a.mid
	}
	switch {
	case a.running != nil:
		p := *a.running
		acc.Running = &p
	case a.queued:
		acc.Running = &api.Progress{Started: now, Task: "准备中"}
	}
	if due {
		acc.NextRun = &when
	}
	if st.TodayDate == day(now) {
		for _, t := range allTasks() {
			if s, ok := st.Today[t.Info.ID]; ok {
				acc.Today = append(acc.Today, s)
			}
		}
	}
	return acc
}
