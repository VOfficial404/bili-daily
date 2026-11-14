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
//   accounts/<UID>/runs/<ID>.json  运行报告，每个账号留最近 keepRuns 份

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
	if err := writeJSON(filepath.Join(e.runsDir(run.Mid), run.ID+".json"), run); err != nil {
		return err
	}
	ids := e.runIDs(run.Mid)
	for _, id := range ids[min(keepRuns, len(ids)):] {
		os.Remove(filepath.Join(e.runsDir(run.Mid), id+".json"))
	}
	return nil
}

// runIDs 返回账号的报告 ID，从新到旧
func (e *Engine) runIDs(mid string) []string {
	entries, err := os.ReadDir(e.runsDir(mid))
	if err != nil {
		return nil
	}
	var ids []string
	for _, ent := range entries {
		if id, ok := strings.CutSuffix(ent.Name(), ".json"); ok && runIDRe.MatchString(id) {
			ids = append(ids, id)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
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
