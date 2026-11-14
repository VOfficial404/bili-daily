package client

import (
	"reflect"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
)

func TestTodayPlanKeepsResultsAndAccountOverrides(t *testing.T) {
	state := api.State{Tasks: []api.TaskInfo{
		{ID: "daily", Title: "每日"},
		{ID: "manga", Title: "漫画"},
		{ID: "charge", Title: "充电"},
		{ID: "unfollow", Title: "取关", Manual: true},
		{ID: "vip_point", Title: "大积分"},
	}}
	state.Settings.Tasks.Daily.Enabled = true
	state.Settings.Tasks.Manga.Enabled = true
	state.Settings.Tasks.VipPoint.Enabled = true
	acc := api.Account{LoggedIn: true, DisabledTasks: []string{"manga"}, Today: []api.TaskStatus{
		{ID: "daily", Status: api.StatusOK, Note: "经验 +15"},
		{ID: "charge", Status: api.StatusSkipped},
		{ID: "unfollow", Status: api.StatusPartial},
	}}
	want := []TodayLine{
		{ID: "daily", Title: "每日", Status: api.StatusOK, Detail: "经验 +15"},
		{ID: "charge", Title: "充电", Status: api.StatusSkipped},
		{ID: "unfollow", Title: "取关", Status: api.StatusPartial},
		{ID: "vip_point", Title: "大积分"},
	}
	got := TodayPlan(state, acc)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("今日计划 = %#v，期望 %#v", got, want)
	}
	if total, done, problems := Tally(got); total != 4 || done != 2 || problems != 1 {
		t.Fatalf("计划统计 = %d/%d，问题 %d", done, total, problems)
	}
	acc.LoggedIn = false
	if done, total, problems := AccountTally(acc); done != 2 || total != 3 || problems != 2 {
		t.Fatalf("结果统计 = %d/%d，问题 %d", done, total, problems)
	}
	*TaskEnabled(&state.Settings.Tasks, "daily") = false
	if TaskOn(&state.Settings, "daily") || TaskOn(&state.Settings, "unfollow") || TaskEnabled(&state.Settings.Tasks, "unknown") != nil {
		t.Fatal("开关修改或未知任务处理错误")
	}
}

func TestHumanTimeUsesBeijingDayAcrossYearBoundary(t *testing.T) {
	now := time.Date(2026, 12, 31, 16, 15, 0, 0, time.UTC)
	cases := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 12, 31, 23, 45, 0, 0, time.UTC), "今天 07:45"},
		{time.Date(2027, 1, 1, 16, 30, 0, 0, time.UTC), "明天 00:30"},
		{time.Date(2026, 12, 30, 16, 0, 0, 0, time.UTC), "12-31 00:00"},
	}
	for _, tc := range cases {
		if got := humanTime(&tc.at, "暂无", now); got != tc.want {
			t.Errorf("%s = %q，期望 %q", tc.at, got, tc.want)
		}
	}
	if got := humanTime(nil, "未安排", now); got != "未安排" {
		t.Fatal(got)
	}
}

func TestDurationPreservesSubsecondAndMinuteLabels(t *testing.T) {
	for d, want := range map[time.Duration]string{
		500 * time.Millisecond: "不到 1 秒",
		59 * time.Second:       "59 秒",
		125 * time.Second:      "2 分 5 秒",
	} {
		if got := Duration(d); got != want {
			t.Errorf("%s = %q，期望 %q", d, got, want)
		}
	}
}
