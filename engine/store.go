package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/VOfficial404/bili-daily/api"
)

// 数据目录的结构：
//   token                      接口令牌
//   settings.json              设置
//   accounts/<UID>/session.json  登录会话（Cookie 和 refresh_token），等同于账号的登录权限
//   accounts/<UID>/state.json    资料、开关和每日计划的进度
//   accounts/<UID>/runs/<ID>.json  运行报告，按设置的天数保留，每个账号最多 keepRuns 份

const keepRuns = 60

var (
	midRe        = regexp.MustCompile(`^[0-9]{1,20}$`)
	runIDRe      = regexp.MustCompile(`^[0-9]{8}T[0-9]{9}-([0-9]{1,20})$`)
	jsonRenameMu sync.Mutex // Windows 不允许同时替换同一个目标；仅串行化最后的 rename
)

var errNoSession = errors.New("还没有登录会话，请先扫码登录")

func (e *Engine) accountsDir() string          { return filepath.Join(e.cfg.DataDir, "accounts") }
func (e *Engine) accountDir(mid string) string { return filepath.Join(e.accountsDir(), mid) }
func (e *Engine) sessionFile(mid string) string {
	return filepath.Join(e.accountDir(mid), "session.json")
}
func (e *Engine) stateFile(mid string) string { return filepath.Join(e.accountDir(mid), "state.json") }
func (e *Engine) runsDir(mid string) string   { return filepath.Join(e.accountDir(mid), "runs") }

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	jsonRenameMu.Lock()
	defer jsonRenameMu.Unlock()
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// session 是账号的登录会话
type session struct {
	Cookies       map[string]string `json:"cookies"`
	RefreshToken  string            `json:"refresh_token,omitempty"`
	CookieExpires time.Time         `json:"cookie_expires,omitempty"`
}

func (e *Engine) loadSession(mid string) (session, error) {
	var s session
	err := readJSON(e.sessionFile(mid), &s)
	if errors.Is(err, os.ErrNotExist) {
		return s, errNoSession
	}
	if err != nil {
		return s, fmt.Errorf("读取 %s 失败：%w", e.sessionFile(mid), err)
	}
	if s.Cookies["SESSDATA"] == "" {
		return s, errNoSession
	}
	return s, nil
}

// saveSession 按 Cookie 里的 DedeUserID（即 UID）保存会话，返回 UID
func (e *Engine) saveSession(s session) (string, error) {
	mid := s.Cookies["DedeUserID"]
	if s.Cookies["SESSDATA"] == "" || s.Cookies["bili_jct"] == "" || !midRe.MatchString(mid) {
		return "", errors.New("会话里缺少 SESSDATA、bili_jct 或 DedeUserID，B 站接口可能已变化")
	}
	return mid, writeJSON(e.sessionFile(mid), s)
}

// listAccounts 返回有会话文件的账号 UID
func (e *Engine) listAccounts() []string {
	entries, err := os.ReadDir(e.accountsDir())
	if err != nil {
		return nil
	}
	var mids []string
	for _, ent := range entries {
		if !ent.IsDir() || !midRe.MatchString(ent.Name()) {
			continue
		}
		if _, err := os.Stat(e.sessionFile(ent.Name())); err == nil {
			mids = append(mids, ent.Name())
		}
	}
	return mids
}

// accountState 是账号保存在 state.json 里的全部内容
type accountState struct {
	Uname string `json:"uname"`
	Face  string `json:"face"`

	Enabled       bool     `json:"enabled"`
	DisabledTasks []string `json:"disabled_tasks,omitempty"`
	LoggedIn      bool     `json:"logged_in"`

	Level     int        `json:"level"`
	Exp       int        `json:"exp"`
	NextExp   int        `json:"next_exp"`
	Coins     float64    `json:"coins"`
	BCoupon   float64    `json:"b_coupon"`
	VIP       string     `json:"vip,omitempty"`
	VIPDue    *time.Time `json:"vip_due,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`

	CookieExpires *time.Time `json:"cookie_expires,omitempty"`

	DailyDate   string                    `json:"daily_date,omitempty"`  // 最近一次跑每日计划的日期（北京时间）
	CookieDate  string                    `json:"cookie_date,omitempty"` // 最近一次询问 Cookie 是否要刷新的日期
	LotteryAt   time.Time                 `json:"lottery_at,omitempty"`  // 最近一次找天选时刻的时间
	TodayDate   string                    `json:"today_date,omitempty"`
	Today       map[string]api.TaskStatus `json:"today,omitempty"` // TodayDate 那天各任务的结果
	LastRun     *api.RunSummary           `json:"last_run,omitempty"`
	Marks       map[string]string         `json:"marks,omitempty"` // 任务自己记的东西，键以任务 ID 开头
	NotifiedOut string                    `json:"notified_out,omitempty"`
}

func (e *Engine) loadState(mid string) accountState {
	st := accountState{Enabled: true, LoggedIn: true}
	if err := readJSON(e.stateFile(mid), &st); err != nil && !errors.Is(err, os.ErrNotExist) {
		e.Warnf("读取 %s 失败，按新账号处理：%v", e.stateFile(mid), err)
	}
	return st
}

/* 运行报告 */

// runID 由开始时间和账号组成，同时也是文件名
func runID(started time.Time, mid string) string {
	return strings.Replace(started.In(Beijing).Format("20060102T150405.000"), ".", "", 1) + "-" + mid
}

func (e *Engine) saveRun(run *api.Run) error {
	e.runsMu.Lock()
	defer e.runsMu.Unlock()
	if _, ok := reportStarted(run.ID, run.Mid); !ok {
		return errors.New("运行报告 ID 与账号不匹配")
	}
	if err := writeJSON(filepath.Join(e.runsDir(run.Mid), run.ID+".json"), run); err != nil {
		return err
	}
	return e.pruneAccountReports(run.Mid, e.now().Add(-time.Duration(e.Settings().ReportRetentionDays)*24*time.Hour))
}

// runIDs 返回账号的报告 ID，从新到旧
func (e *Engine) runIDs(mid string) []string {
	if !e.reportDirectory(mid) {
		return nil
	}
	entries, err := os.ReadDir(e.runsDir(mid))
	if err != nil {
		return nil
	}
	var ids []string
	for _, ent := range entries {
		info, err := ent.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if id, ok := strings.CutSuffix(ent.Name(), ".json"); ok {
			if _, valid := reportStarted(id, mid); !valid {
				continue
			}
			ids = append(ids, id)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

// reportStarted 只识别当前账号的真实报告文件名；时间戳使用北京时间。
func reportStarted(id, mid string) (time.Time, bool) {
	m := runIDRe.FindStringSubmatch(id)
	if m == nil || m[1] != mid {
		return time.Time{}, false
	}
	started, err := time.ParseInLocation("20060102T150405.000", id[:15]+"."+id[15:18], Beijing)
	return started, err == nil
}

// reportDirectory 不跟随 accounts、账号目录或 runs 的符号链接。
func (e *Engine) reportDirectory(mid string) bool {
	if !midRe.MatchString(mid) {
		return false
	}
	for _, path := range []string{e.accountsDir(), e.accountDir(mid), e.runsDir(mid)} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && path == e.runsDir(mid) {
			return true // runs 尚不存在时，仍可清掉状态里残留的报告引用
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

// pruneReports 在打开目录、缩短保留期及每小时运行，即使没有新运行也清理。
func (e *Engine) pruneReports() {
	e.runsMu.Lock()
	defer e.runsMu.Unlock()
	cutoff := e.now().Add(-time.Duration(e.Settings().ReportRetentionDays) * 24 * time.Hour)
	entries, err := os.ReadDir(e.accountsDir())
	if err != nil {
		return
	}
	for _, ent := range entries {
		if !ent.IsDir() || !midRe.MatchString(ent.Name()) {
			continue
		}
		if err := e.pruneAccountReports(ent.Name(), cutoff); err != nil {
			e.Warnf("清理账号 %s 的运行报告失败：%v", ent.Name(), err)
		}
	}
}

func (e *Engine) reportRetentionLoop() {
	// 不使用 cfg.Sleep：测试中的替代函数可能立即返回。
	for sleepCtx(e.ctx, time.Hour) == nil {
		e.pruneReports()
	}
}

// 调用方持有 runsMu。账号 mu 同时保护状态写入和关闭，避免清理后写回旧 LastRun，
// 或在账号删除后重新创建目录。未注册账号在 mu 下处理，阻止注册读到旧状态。
func (e *Engine) pruneAccountReports(mid string, cutoff time.Time) error {
	e.mu.Lock()
	a := e.accounts[mid]
	if a == nil {
		defer e.mu.Unlock()
	} else {
		e.mu.Unlock()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.closing {
			return nil
		}
	}
	if !e.reportDirectory(mid) {
		return nil
	}
	var st accountState
	if a != nil {
		st = a.st
	} else if err := readJSON(e.stateFile(mid), &st); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	removedLast := false
	kept := 0
	for _, id := range e.runIDs(mid) {
		started, _ := reportStarted(id, mid)
		// 文件名只有毫秒精度；报告内时间保留完整精度，边界相等时仍保留。
		var run api.RunSummary
		if readJSON(filepath.Join(e.runsDir(mid), id+".json"), &run) == nil && !run.Started.IsZero() && runID(run.Started, mid) == id {
			started = run.Started
		}
		if !started.Before(cutoff) && kept < keepRuns {
			kept++
			continue
		}
		if err := os.Remove(filepath.Join(e.runsDir(mid), id+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if st.LastRun != nil && st.LastRun.ID == id {
			removedLast = true
		}
	}
	if st.LastRun == nil {
		return nil
	}
	// 旧版本或中断留下的缺失报告也清掉；运行结束正在保存的新报告不能误清。
	if !removedLast && (a == nil || a.running == nil) {
		if _, valid := reportStarted(st.LastRun.ID, mid); valid {
			_, err := os.Lstat(filepath.Join(e.runsDir(mid), st.LastRun.ID+".json"))
			removedLast = errors.Is(err, os.ErrNotExist)
		}
	}
	if !removedLast {
		return nil
	}
	st.LastRun = nil
	if err := writeJSON(e.stateFile(mid), st); err != nil {
		return err
	}
	if a != nil {
		a.st = st
	}
	return nil
}

func (e *Engine) loadRun(id string) (*api.Run, error) {
	m := runIDRe.FindStringSubmatch(id)
	if m == nil {
		return nil, os.ErrNotExist
	}
	var run api.Run
	if err := readJSON(filepath.Join(e.runsDir(m[1]), id+".json"), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// listRuns 返回最近的报告摘要，从新到旧。mid 为空时是所有账号的。
func (e *Engine) listRuns(mid string, limit int) []api.RunSummary {
	mids := []string{mid}
	if mid == "" {
		mids = e.Accounts()
	}
	var ids []string
	for _, m := range mids {
		if !midRe.MatchString(m) {
			continue
		}
		list := e.runIDs(m)
		ids = append(ids, list[:min(limit, len(list))]...)
	}
	// ID 以开始时间开头，按它排序就是按时间排序
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	out := []api.RunSummary{}
	for _, id := range ids[:min(limit, len(ids))] {
		if run, err := e.loadRun(id); err == nil {
			out = append(out, run.RunSummary)
		}
	}
	return out
}
