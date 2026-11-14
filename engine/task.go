package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

// 任务的 ID，也是设置和报告里用的名字
const (
	TaskDaily          = "daily"
	TaskVipPoint       = "vip_point"
	TaskVipPrivilege   = "vip_privilege"
	TaskCharge         = "charge"
	TaskManga          = "manga"
	TaskMangaPrivilege = "manga_privilege"
	TaskSilver2Coin    = "silver2coin"
	TaskFansMedal      = "fans_medal"
	TaskLiveLottery    = "live_lottery"
	TaskUnfollow       = "unfollow"
)

// taskOrder 是一轮里任务的先后顺序，也是界面上的顺序
var taskOrder = []string{
	TaskDaily, TaskVipPoint, TaskVipPrivilege, TaskCharge, TaskManga, TaskMangaPrivilege,
	TaskSilver2Coin, TaskFansMedal, TaskLiveLottery, TaskUnfollow,
}

// Task 是一个任务。Run 把做了什么一步步记到 Ctx 里，不返回错误：
// 一步失败不影响后面的步骤，也不影响其他任务。
type Task struct {
	Info api.TaskInfo
	// Enabled 表示设置里这个任务开着。只能手动运行的任务（Info.Manual）不需要它。
	Enabled func(s *api.Settings) bool
	Run     func(x *Ctx)
}

var registry = map[string]Task{}

// register 登记一个任务，各任务在自己文件的 init 里调用
func register(t Task) {
	if _, dup := registry[t.Info.ID]; dup || t.Info.ID == "" || t.Run == nil {
		panic("任务登记不对：" + t.Info.ID)
	}
	registry[t.Info.ID] = t
}

// allTasks 按 taskOrder 返回登记过的任务
func allTasks() []Task {
	var out []Task
	for _, id := range taskOrder {
		if t, ok := registry[id]; ok {
			out = append(out, t)
		}
	}
	var rest []string
	for id := range registry {
		if !slices.Contains(taskOrder, id) {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	for _, id := range rest {
		out = append(out, registry[id])
	}
	return out
}

// Ctx 是任务运行时拿到的东西：账号的客户端、资料、设置，以及记录结果的方法。
// 它也是一个 context：运行被中止时取消。
type Ctx struct {
	context.Context
	C   *bili.Client
	Nav *bili.Nav    // 这一轮开始时的账号资料
	Set api.Settings // 这一轮开始时的设置
	Mid int64
	Day string // 今天的日期，北京时间，2006-01-02

	e    *Engine
	acc  *account
	res  *api.TaskResult
	stop bool // 登录失效：这一轮后面的任务都不用做了
}

// Now 是当前时间（测试里可以换）
func (x *Ctx) Now() time.Time { return x.e.now() }

func (x *Ctx) step(s api.Step) {
	x.res.Steps = append(x.res.Steps, s)
	lv, word := levelInfo, "成功"
	switch s.Status {
	case api.StatusSkipped:
		word = "跳过"
	case api.StatusFailed:
		lv, word = levelWarn, "失败"
	}
	text := fmt.Sprintf("%s · %s：%s", x.res.Title, s.Name, word)
	if s.Detail != "" {
		text += "，" + s.Detail
	}
	x.e.logf(lv, x.acc.mid, "%s", text)
}

// OK 记一步成功。name 是这一步的名字（例如「观看视频」），后面是结果的说明。
func (x *Ctx) OK(name, format string, args ...any) {
	x.step(api.Step{Name: name, Status: api.StatusOK, Detail: fmt.Sprintf(format, args...)})
}

// Skip 记一步不需要做：今天已经做过、设置里关着、账号不满足条件。
func (x *Ctx) Skip(name, format string, args ...any) {
	x.step(api.Step{Name: name, Status: api.StatusSkipped, Detail: fmt.Sprintf(format, args...)})
}

// Fail 记一步失败，原因和建议按 err 归类。登录失效或运行被中止时，Aborted 之后返回 true。
func (x *Ctx) Fail(name string, err error) {
	reason := bili.Classify(err)
	s := api.Step{Name: name, Status: api.StatusFailed, Detail: err.Error(), Reason: string(reason), Hint: reason.Hint()}
	var be *bili.Error
	if errors.As(err, &be) && be.Code != bili.CodeNetwork && be.Code != bili.CodeBadResponse {
		s.Code = be.Code
		if be.Code == -403 && be.Path == "api.bilibili.com/x/web-interface/coin/add" {
			s.Hint = "B 站将这次投币判为账号异常，本轮已停止投币，不会自动重试。先暂停投币并在官方 App 检查账号提示；若官方 App 正常，请检查服务器是否使用稳定出口，并在同一出口重新扫码更新会话。此错误本身不能确定是频率、IP 还是设备会话问题。"
		}
	}
	if reason == bili.ReasonCanceled {
		s.Detail = "运行被中止"
	}
	if reason == bili.ReasonNotLogin {
		x.stop = true
	}
	x.step(s)
}

// Failf 记一步失败，原因由任务自己判断（例如 bili.ReasonIneligible、bili.ReasonInsufficient）。
func (x *Ctx) Failf(name string, reason bili.Reason, format string, args ...any) {
	if reason == bili.ReasonNotLogin {
		x.stop = true
	}
	x.step(api.Step{Name: name, Status: api.StatusFailed, Detail: fmt.Sprintf(format, args...), Reason: string(reason), Hint: reason.Hint()})
}

// Note 设置任务的一句话结果，显示在任务列表和推送里，例如「经验 +15，投币 5 个」。
// 不设置时按各步的结果生成。
func (x *Ctx) Note(format string, args ...any) { x.res.Note = fmt.Sprintf(format, args...) }

// Aborted 表示任务应该尽快返回：运行被中止，或者登录失效了
func (x *Ctx) Aborted() bool { return x.stop || x.Err() != nil }

// Sleep 等 d；运行被中止时提前返回 false
func (x *Ctx) Sleep(d time.Duration) bool { return x.e.cfg.Sleep(x, d) == nil }

// Mark 读任务自己记在账号上的东西（例如这个月是否充过电），没有时为空。
// key 以任务 ID 开头，例如 "charge.month"。
func (x *Ctx) Mark(key string) string { return x.acc.snapshot().Marks[key] }

// SetMark 记下一个值，保存在账号的数据里；value 为空时删除
func (x *Ctx) SetMark(key, value string) {
	x.acc.update(func(st *accountState) {
		if value == "" {
			delete(st.Marks, key)
			return
		}
		if st.Marks == nil {
			st.Marks = map[string]string{}
		}
		st.Marks[key] = value
	})
}

// Infof 记一条这个账号的日志（不进报告）
func (x *Ctx) Infof(format string, args ...any) { x.e.logf(levelInfo, x.acc.mid, format, args...) }

// Debugf 记一条这个账号的调试日志
func (x *Ctx) Debugf(format string, args ...any) { x.e.logf(levelDebug, x.acc.mid, format, args...) }

// finish 按各步的结果算出任务的结果和一句话说明
func finish(res *api.TaskResult) {
	var ok, failed, skipped int
	var firstFail, lastSkip string
	for _, s := range res.Steps {
		switch s.Status {
		case api.StatusOK:
			ok++
		case api.StatusFailed:
			failed++
			if firstFail == "" {
				firstFail = s.Name + "失败"
				if s.Hint != "" {
					firstFail += "：" + s.Hint
				} else if s.Detail != "" {
					firstFail += "：" + s.Detail
				}
			}
		case api.StatusSkipped:
			skipped++
			lastSkip = s.Detail
		}
	}
	switch {
	case failed > 0 && ok > 0:
		res.Status = api.StatusPartial
	case failed > 0:
		res.Status = api.StatusFailed
	case ok > 0:
		res.Status = api.StatusOK
	default:
		res.Status = api.StatusSkipped
	}
	switch {
	case failed > 0:
		// 失败时说明以失败的原因为准，任务自己设的说明接在后面
		if res.Note != "" {
			res.Note = firstFail + "；" + res.Note
		} else {
			res.Note = firstFail
		}
	case res.Note != "":
	case ok > 0:
		res.Note = fmt.Sprintf("%d 步完成", ok)
	case skipped > 0:
		res.Note = lastSkip
	default:
		res.Note = "没有要做的"
	}
}
