package engine

import (
	"strings"
	"testing"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const (
	mangaClockPath   = "manga.bilibili.com/twirp/activity.v1.Activity/ClockIn"
	mangaHistoryPath = "manga.bilibili.com/twirp/bookshelf.v1.Bookshelf/AddHistory"
	mangaRewardPath  = "manga.bilibili.com/twirp/user.v1.User/GetVipReward"
)

func mangaEnv(t *testing.T) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.set(func(s *api.Settings) {
		s.Tasks.Manga.Read = true
		s.Tasks.Manga.ComicID = 123
		s.Tasks.Manga.EpID = 456
	})
	v.b.JSON(mangaClockPath, nil)
	v.b.JSON(mangaHistoryPath, nil)
	return v
}

func TestMangaClockAndReading(t *testing.T) {
	v := mangaEnv(t)
	r := v.runTask(42, TaskManga)
	if r.Status != api.StatusOK || r.Note != "签到成功，记了一次阅读" {
		t.Fatalf("结果不对：%+v", r)
	}
	for _, path := range []string{mangaClockPath, mangaHistoryPath} {
		calls := v.b.Calls(path)
		if len(calls) != 1 {
			t.Fatalf("请求数不对：%s %d", path, len(calls))
		}
		c := calls[0]
		if c.Method != "POST" || c.Form.Get("platform") != "android" || c.Header.Get("Origin") != "https://manga.bilibili.com" || c.Header.Get("Referer") != "https://manga.bilibili.com/" {
			t.Errorf("请求不对：%+v", c)
		}
	}
	c := v.b.Calls(mangaHistoryPath)[0]
	if c.Form.Get("comic_id") != "123" || c.Form.Get("ep_id") != "456" {
		t.Errorf("阅读参数不对：%v", c.Form)
	}
	var order []string
	for _, c := range v.b.Calls("") {
		if c.Path == mangaClockPath || c.Path == mangaHistoryPath {
			order = append(order, c.Path)
		}
	}
	if strings.Join(order, ",") != mangaClockPath+","+mangaHistoryPath {
		t.Fatalf("顺序不对：%v", order)
	}
}

func TestMangaDuplicateStillReads(t *testing.T) {
	v := mangaEnv(t)
	v.b.Handle(mangaClockPath, func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Status: 400, Body: `{"code":"invalid_argument","msg":"clockin clockin is duplicate"}`}
	})
	r := v.runTask(42, TaskManga)
	if step(t, r, "签到").Status != api.StatusSkipped || step(t, r, "阅读").Status != api.StatusOK || r.Status != api.StatusOK || v.b.Count(mangaHistoryPath) != 1 {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestMangaReadDisabled(t *testing.T) {
	v := mangaEnv(t)
	v.set(func(s *api.Settings) { s.Tasks.Manga.Read = false })
	r := v.runTask(42, TaskManga)
	if v.b.Count(mangaHistoryPath) != 0 || len(r.Steps) != 1 || r.Status != api.StatusOK {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestMangaRiskStillReads(t *testing.T) {
	v := mangaEnv(t)
	v.b.Fail(mangaClockPath, -352, "风控校验失败")
	r := v.runTask(42, TaskManga)
	if r.Status != api.StatusPartial || step(t, r, "签到").Reason != "risk" || step(t, r, "阅读").Status != api.StatusOK || v.b.Count(mangaHistoryPath) != 1 {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestMangaLoginLostStopsReading(t *testing.T) {
	v := mangaEnv(t)
	v.b.Fail(mangaClockPath, -101, "未登录")
	r := v.runTask(42, TaskManga)
	if r.Status != api.StatusFailed || v.b.Count(mangaHistoryPath) != 0 {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestMangaPrivilegeNonVIP(t *testing.T) {
	v := mangaEnv(t)
	r := v.runTask(42, TaskMangaPrivilege)
	if r.Status != api.StatusSkipped || v.b.Count(mangaRewardPath) != 0 {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestMangaPrivilegeMonthMark(t *testing.T) {
	v := mangaEnv(t)
	v.vip(42, 1, 0)
	v.b.Handle(mangaRewardPath, func(c bilitest.Call) bilitest.Reply {
		if c.Method != "POST" || c.Query.Get("reason_id") != "1" || c.Header.Get("Origin") != "https://manga.bilibili.com" {
			t.Errorf("领取参数不对：%+v", c)
		}
		return bilitest.OK(map[string]any{"amount": 5})
	})
	r := v.runTask(42, TaskMangaPrivilege)
	if r.Status != api.StatusOK || r.Note != "领到 5 张漫读券" || v.e.account("42").snapshot().Marks["manga_privilege.month"] != v.now.Format("2006-01") {
		t.Fatalf("结果不对：%+v", r)
	}
	r = v.runTask(42, TaskMangaPrivilege)
	if r.Status != api.StatusSkipped || v.b.Count(mangaRewardPath) != 1 {
		t.Fatalf("本月重复领取：%+v", r)
	}
	v.now = v.now.AddDate(0, 1, 0)
	r = v.runTask(42, TaskMangaPrivilege)
	if r.Status != api.StatusOK || v.b.Count(mangaRewardPath) != 2 {
		t.Fatalf("新月份没有领取：%+v", r)
	}
}
func TestMangaPrivilegeAlreadyClaimedMarksMonth(t *testing.T) {
	v := mangaEnv(t)
	v.vip(42, 2, 0)
	v.b.Fail(mangaRewardPath, 1, "已经领取")
	r := v.runTask(42, TaskMangaPrivilege)
	v.runTask(42, TaskMangaPrivilege)
	if r.Status != api.StatusSkipped || v.b.Count(mangaRewardPath) != 1 {
		t.Fatalf("重复领取：%+v", r)
	}
}
func TestMangaPrivilegeFailureDoesNotMark(t *testing.T) {
	for _, data := range []bool{false, true} {
		t.Run(map[bool]string{false: "风控", true: "缺少结果"}[data], func(t *testing.T) {
			v := mangaEnv(t)
			v.vip(42, 1, 0)
			if data {
				v.b.JSON(mangaRewardPath, nil)
			} else {
				v.b.Fail(mangaRewardPath, -352, "风控校验失败")
			}
			r := v.runTask(42, TaskMangaPrivilege)
			if r.Status != api.StatusFailed || v.e.account("42").snapshot().Marks["manga_privilege.month"] != "" {
				t.Fatalf("失败被记成领取：%+v", r)
			}
		})
	}
}
