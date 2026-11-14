// Package client 提供桌面和手机共用的连接、设置与展示逻辑，不依赖界面库。
package client

import (
	"fmt"
	"slices"
	"time"

	"github.com/VOfficial404/bili-daily/api"
)

// TaskEnabled 返回每日任务开关；只能手动运行或未知的任务返回 nil。
func TaskEnabled(t *api.TaskSettings, id string) *bool {
	switch id {
	case "daily":
		return &t.Daily.Enabled
	case "vip_point":
		return &t.VipPoint.Enabled
	case "vip_privilege":
		return &t.VipPrivilege.Enabled
	case "charge":
		return &t.Charge.Enabled
	case "manga":
		return &t.Manga.Enabled
	case "manga_privilege":
		return &t.MangaPrivilege.Enabled
	case "silver2coin":
		return &t.Silver2Coin.Enabled
	case "fans_medal":
		return &t.FansMedal.Enabled
	case "live_lottery":
		return &t.LiveLottery.Enabled
	}
	return nil
}

// TaskOn 表示每日任务已开启，手动任务不算。
func TaskOn(s *api.Settings, id string) bool {
	on := TaskEnabled(&s.Tasks, id)
	return on != nil && *on
}

// TodayLine 是账号今天计划里的一项，尚未运行时 Status 为空。
type TodayLine struct {
	ID, Title string
	Status    api.Status
	Detail    string
}

// TodayPlan 保留任务顺序，包含启用的计划和今天已经运行过的任务。
func TodayPlan(state api.State, acc api.Account) []TodayLine {
	var out []TodayLine
	for _, task := range state.Tasks {
		var result api.TaskStatus
		ran := false
		for _, today := range acc.Today {
			if today.ID == task.ID {
				result, ran = today, true
			}
		}
		planned := !task.Manual && TaskOn(&state.Settings, task.ID) && !slices.Contains(acc.DisabledTasks, task.ID)
		if planned || ran {
			out = append(out, TodayLine{ID: task.ID, Title: task.Title, Status: result.Status, Detail: result.Note})
		}
	}
	return out
}

// Finished 包括已完成和无需执行。
func Finished(status api.Status) bool {
	return status == api.StatusOK || status == api.StatusSkipped
}

// Tally 统计计划总数、完成数与失败或部分完成的项数。
func Tally(lines []TodayLine) (total, done, problems int) {
	for _, line := range lines {
		total++
		if Finished(line.Status) {
			done++
		} else if line.Status == api.StatusFailed || line.Status == api.StatusPartial {
			problems++
		}
	}
	return
}

// AccountTally 统计已有的今日结果，登录失效另计一个问题。
func AccountTally(acc api.Account) (done, total, problems int) {
	for _, task := range acc.Today {
		total++
		if Finished(task.Status) {
			done++
		} else if task.Status == api.StatusFailed || task.Status == api.StatusPartial {
			problems++
		}
	}
	if !acc.LoggedIn {
		problems++
	}
	return
}

// StatusText 是完整的状态标签。
func StatusText(status api.Status) string {
	switch status {
	case api.StatusOK:
		return "已完成"
	case api.StatusPartial:
		return "部分完成"
	case api.StatusFailed:
		return "失败"
	case api.StatusSkipped:
		return "已跳过"
	case api.StatusRunning:
		return "运行中"
	}
	return "还没运行"
}

// StatusShort 保留手机界面的简短状态标签。
func StatusShort(status api.Status) string {
	switch status {
	case api.StatusOK:
		return "完成"
	case api.StatusSkipped:
		return "无需执行"
	case api.StatusRunning:
		return "正在运行"
	case "":
		return "等待运行"
	case api.StatusPartial, api.StatusFailed:
		return StatusText(status)
	}
	return "等待运行"
}

// TriggerText 是运行来源的中文标签。
func TriggerText(trigger string) string {
	switch trigger {
	case "schedule":
		return "定时"
	case "catchup":
		return "补跑"
	case "retry":
		return "重试"
	case "manual":
		return "手动"
	}
	return trigger
}

// ReasonText 是报告中失败原因的中文标签。
func ReasonText(reason string) string {
	switch reason {
	case "not_login":
		return "登录失效"
	case "risk":
		return "被风控拦截"
	case "rate":
		return "请求太频繁"
	case "ineligible":
		return "不满足条件"
	case "insufficient":
		return "余额不足"
	case "network":
		return "网络错误"
	case "changed":
		return "B 站接口变化"
	case "canceled":
		return "已取消"
	}
	return "其他原因"
}

// Duration 把运行用时写成中文。
func Duration(d time.Duration) string {
	if d < time.Second {
		return "不到 1 秒"
	}
	seconds := int(d / time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", seconds)
	}
	return fmt.Sprintf("%d 分 %d 秒", seconds/60, seconds%60)
}

var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

// HumanTime 使用北京时间；没有时间时显示调用方提供的文字。
func HumanTime(t *time.Time, empty string) string {
	return humanTime(t, empty, time.Now())
}

func humanTime(t *time.Time, empty string, now time.Time) string {
	if t == nil {
		return empty
	}
	local := t.In(beijing)
	now = now.In(beijing)
	switch local.Format("2006-01-02") {
	case now.Format("2006-01-02"):
		return "今天 " + local.Format("15:04")
	case now.AddDate(0, 0, 1).Format("2006-01-02"):
		return "明天 " + local.Format("15:04")
	}
	return local.Format("01-02 15:04")
}
