package engine

import (
	"strings"
	"testing"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const (
	tagsPath   = "api.bilibili.com/x/relation/tags"
	tagPath    = "api.bilibili.com/x/relation/tag"
	modifyPath = "api.bilibili.com/x/relation/modify"
)

// 分组里从新到旧是 C、B、A
func unfollowEnv(t *testing.T) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.b.JSON(tagsPath, []map[string]any{
		{"tagid": -10, "name": "特别关注", "count": 0},
		{"tagid": 77, "name": "天选时刻", "count": 3},
	})
	v.b.Handle(tagPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("tagid") != "77" || c.Query.Get("mid") != "42" {
			t.Errorf("读取分组的参数不对：%v", c.Query)
		}
		if c.Query.Get("pn") != "1" {
			return bilitest.OK([]any{})
		}
		return bilitest.OK([]map[string]any{{"mid": 3, "uname": "C"}, {"mid": 2, "uname": "B"}, {"mid": 1, "uname": "A"}})
	})
	v.b.JSON(modifyPath, nil)
	return v
}

func unfollowed(v *testEnv) string {
	var fids []string
	for _, c := range v.b.Calls(modifyPath) {
		if c.Form.Get("act") != "2" || c.Form.Get("re_src") != "11" || c.Form.Get("csrf") != "csrf-42" {
			v.t.Errorf("取关的参数不对：%v", c.Form)
		}
		fids = append(fids, c.Form.Get("fid"))
	}
	return strings.Join(fids, ",")
}

func TestUnfollowStartsFromOldestAndHonoursCountAndKeep(t *testing.T) {
	v := unfollowEnv(t)
	v.set(func(s *api.Settings) {
		s.Tasks.Unfollow = api.UnfollowOptions{Group: "天选时刻", Count: 1, Keep: []int64{1}}
	})

	r := v.runTask(42, TaskUnfollow)
	if got := unfollowed(v); got != "2" {
		t.Errorf("取关了 %s，应该跳过保留的 A、只取关最早的 B", got)
	}
	if r.Status != api.StatusOK || r.Note != "取关 1 个，「天选时刻」分组里还剩 2 个" {
		t.Errorf("result = %s %q", r.Status, r.Note)
	}
}

func TestUnfollowAllWhenCountIsZero(t *testing.T) {
	v := unfollowEnv(t)
	v.set(func(s *api.Settings) { s.Tasks.Unfollow = api.UnfollowOptions{Group: "天选时刻"} })
	v.runTask(42, TaskUnfollow)
	if got := unfollowed(v); got != "1,2,3" {
		t.Errorf("取关了 %s", got)
	}
}

func TestUnfollowStopsAfterRepeatedFailures(t *testing.T) {
	v := unfollowEnv(t)
	v.set(func(s *api.Settings) { s.Tasks.Unfollow = api.UnfollowOptions{Group: "天选时刻"} })
	v.b.Fail(modifyPath, -352, "风控校验失败")
	r := v.runTask(42, TaskUnfollow)
	if n := v.b.Count(modifyPath); n != 3 {
		t.Errorf("失败后还请求了 %d 次", n)
	}
	if s := step(t, r, "取关"); r.Status != api.StatusFailed || s.Reason != "risk" {
		t.Errorf("result = %+v", r)
	}
}

func TestUnfollowMissingGroupIsSkippedNotFailed(t *testing.T) {
	v := unfollowEnv(t)
	v.set(func(s *api.Settings) { s.Tasks.Unfollow = api.UnfollowOptions{Group: "不存在"} })
	r := v.runTask(42, TaskUnfollow)
	if r.Status != api.StatusSkipped || v.b.Count(modifyPath) != 0 {
		t.Errorf("result = %+v", r)
	}
}

func TestUnfollowIsNotPartOfDailyPlan(t *testing.T) {
	v := unfollowEnv(t)
	run := v.run(42)
	for _, r := range run.Tasks {
		if r.ID == TaskUnfollow {
			t.Fatal("批量取关出现在了每日计划里")
		}
	}
	if v.b.Count(modifyPath) != 0 {
		t.Fatal("每日计划取关了人")
	}
}
