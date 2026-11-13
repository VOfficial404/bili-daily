package engine

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

const (
	lotteryAreasPath     = "api.live.bilibili.com/xlive/web-interface/v1/index/getWebAreaList"
	lotteryRoomsPath     = "api.live.bilibili.com/room/v3/area/getRoomList"
	lotteryCheckPath     = "api.live.bilibili.com/xlive/lottery-interface/v1/Anchor/Check"
	lotteryJoinPath      = "api.live.bilibili.com/xlive/lottery-interface/v1/Anchor/Join"
	lotteryFollowingPath = "api.bilibili.com/x/relation/followings"
	lotteryTagsPath      = "api.bilibili.com/x/relation/tags"
	lotteryCreatePath    = "api.bilibili.com/x/relation/tag/create"
	lotteryCopyPath      = "api.bilibili.com/x/relation/tags/copyUsers"
)

func lotteryRoom(id int, pendant any) map[string]any {
	return map[string]any{"roomid": id, "uid": id + 1000, "uname": fmt.Sprintf("主播%d", id), "pendant_info": pendant}
}

func lotteryPendant(id int) any { return map[string]any{"2": map[string]any{"pendent_id": id}} }

func lotteryDetail(id, require int) map[string]any {
	return map[string]any{"id": id, "room_id": id, "status": 1, "award_name": fmt.Sprintf("奖品%d", id), "require_type": require, "gift_id": 5, "gift_num": 2, "gift_price": 0}
}

func lotteryEnv(t *testing.T, count int) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.set(func(s *api.Settings) {
		s.Tasks.LiveLottery.Include = nil
		s.Tasks.LiveLottery.Exclude = nil
		s.Tasks.LiveLottery.Follow = false
		s.Tasks.LiveLottery.Group = false
	})
	v.b.Handle(lotteryAreasPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("source_id") != "2" {
			t.Errorf("分区参数：%v", c.Query)
		}
		return bilitest.OK(map[string]any{"data": []any{map[string]any{"id": 1, "name": "网游"}}})
	})
	rooms := []any{}
	for id := 1; id <= count; id++ {
		rooms = append(rooms, lotteryRoom(id, lotteryPendant(504)))
	}
	v.b.Handle(lotteryRoomsPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("platform") != "web" || c.Query.Get("parent_area_id") != "1" || c.Query.Get("area_id") != "0" || c.Query.Get("sort_type") != "online" || c.Query.Get("page_size") != "99" || c.Query.Get("w_rid") != "" {
			t.Errorf("房间列表参数：%v", c.Query)
		}
		if c.Header.Get("Referer") != "https://live.bilibili.com/" || c.Header.Get("Origin") != "https://live.bilibili.com" {
			t.Errorf("列表请求头：%v", c.Header)
		}
		return bilitest.OK(map[string]any{"list": rooms})
	})
	v.b.Handle(lotteryCheckPath, func(c bilitest.Call) bilitest.Reply {
		id, _ := strconv.Atoi(c.Query.Get("roomid"))
		return bilitest.OK(lotteryDetail(id, 0))
	})
	v.b.JSON(lotteryJoinPath, nil)
	v.b.JSON(lotteryFollowingPath, map[string]any{"list": []any{}, "total": 0})
	return v
}

func TestLotteryFiltersPendantStatusKeywordsGiftsAndRequirements(t *testing.T) {
	v := lotteryEnv(t, 0)
	v.set(func(s *api.Settings) {
		s.Tasks.LiveLottery.Include = []string{"键盘"}
		s.Tasks.LiveLottery.Exclude = []string{"照片"}
		s.Tasks.LiveLottery.Follow = true
	})
	rooms := []any{
		lotteryRoom(1, lotteryPendant(504)), lotteryRoom(2, lotteryPendant(1432)),
		lotteryRoom(3, lotteryPendant(504)), lotteryRoom(4, lotteryPendant(504)),
		lotteryRoom(5, lotteryPendant(504)), lotteryRoom(6, lotteryPendant(504)),
		lotteryRoom(7, lotteryPendant(504)), lotteryRoom(8, lotteryPendant(504)),
		lotteryRoom(9, lotteryPendant(1)), lotteryRoom(10, nil), lotteryRoom(11, []any{1, 2}),
		lotteryRoom(12, "文字"), lotteryRoom(13, map[string]any{"2": 123}),
		lotteryRoom(14, map[string]any{"2": map[string]any{"pendent_id": 504.5}}),
		lotteryRoom(15, lotteryPendant(504)),
	}
	v.b.JSON(lotteryRoomsPath, map[string]any{"list": rooms})
	v.b.Handle(lotteryCheckPath, func(c bilitest.Call) bilitest.Reply {
		id, _ := strconv.Atoi(c.Query.Get("roomid"))
		d := lotteryDetail(id, 0)
		d["award_name"] = "键盘"
		switch id {
		case 2:
			d["require_type"] = 1
		case 3:
			d["status"] = 2
		case 4:
			d["award_name"] = "键盘照片"
		case 5:
			d["award_name"] = "鼠标"
		case 6:
			d["gift_price"] = 1
		case 7:
			d["require_type"] = 2
		case 8:
			d["require_type"] = 3
		case 15:
			d["require_type"] = nil
		}
		return bilitest.OK(d)
	})
	r := v.runTask(42, TaskLiveLottery)
	calls := v.b.Calls(lotteryJoinPath)
	if len(calls) != 2 || calls[0].Form.Get("id") != "1" || calls[1].Form.Get("id") != "2" || v.b.Count(lotteryCheckPath) != 9 {
		t.Fatalf("参加请求：%v，检查%d次", calls, v.b.Count(lotteryCheckPath))
	}
	visit := ""
	for _, c := range calls {
		if c.Form.Get("gift_id") != "5" || c.Form.Get("gift_num") != "2" || c.Form.Get("platform") != "pc" || c.Form.Get("csrf") != "csrf-42" || c.Form.Get("csrf_token") != "csrf-42" || c.Form.Has("roomid") || c.Form.Has("room_id") {
			t.Errorf("参加参数：%v", c.Form)
		}
		if visit != "" && visit != c.Form.Get("visit_id") {
			t.Fatal("visit_id 没有复用")
		}
		visit = c.Form.Get("visit_id")
		if len(visit) != 12 || visit[0] < '1' || visit[0] > '9' || visit[11] != '0' {
			t.Errorf("visit_id 格式：%q", visit)
		}
	}
	if r.Status != api.StatusOK || r.Note != "参加了 2 个抽奖" || step(t, r, "扫描").Detail != "9 个直播间在抽奖" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryFollowDisabledSkipsFollowRequired(t *testing.T) {
	v := lotteryEnv(t, 1)
	v.b.JSON(lotteryCheckPath, lotteryDetail(1, 1))
	r := v.runTask(42, TaskLiveLottery)
	if v.b.Count(lotteryJoinPath) != 0 || step(t, r, "参加").Status != api.StatusSkipped || !strings.Contains(step(t, r, "参加").Detail, "需要关注 1 个") || r.Note != "参加了 0 个抽奖" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryNoRoomsAndEndedLotteryNeverJoin(t *testing.T) {
	for _, count := range []int{0, 1} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			v := lotteryEnv(t, count)
			d := lotteryDetail(1, 0)
			d["status"] = 2
			v.b.JSON(lotteryCheckPath, d)
			r := v.runTask(42, TaskLiveLottery)
			if v.b.Count(lotteryJoinPath) != 0 {
				t.Fatal("参加了没有开放的抽奖")
			}
			if count == 0 && (r.Status != api.StatusSkipped || step(t, r, "扫描").Detail != "现在没有直播间在抽奖") {
				t.Fatalf("结果：%+v", r)
			}
		})
	}
}

func TestLotteryGroupsOnlyNewlyFollowedJoinedHostsAndCreatesTag(t *testing.T) {
	v := lotteryEnv(t, 4)
	v.set(func(s *api.Settings) { s.Tasks.LiveLottery.Follow = true; s.Tasks.LiveLottery.Group = true })
	v.b.Handle(lotteryCheckPath, func(c bilitest.Call) bilitest.Reply {
		id, _ := strconv.Atoi(c.Query.Get("roomid"))
		return bilitest.OK(lotteryDetail(id, 1))
	})
	v.b.Handle(lotteryJoinPath, func(c bilitest.Call) bilitest.Reply {
		if c.Form.Get("id") == "3" {
			return bilitest.Code(-400, "本场不能参加")
		}
		return bilitest.OK(nil)
	})
	v.b.Handle(lotteryFollowingPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("order_type") != "" || c.Query.Get("order") != "desc" || c.Query.Get("ps") != "20" || c.Query.Get("vmid") != "42" {
			t.Errorf("关注列表参数：%v", c.Query)
		}
		if v.b.Count(lotteryFollowingPath) == 1 {
			return bilitest.OK(map[string]any{"list": []any{map[string]any{"mid": 99}}, "total": 2})
		}
		// 1004 位于旧关注 99 之后；1003 的抽奖失败；888 是无关的新关注。
		return bilitest.OK(map[string]any{"list": []any{map[string]any{"mid": 1002}, map[string]any{"mid": 1003}, map[string]any{"mid": 888}, map[string]any{"mid": 1001}, map[string]any{"mid": 99}, map[string]any{"mid": 1004}}, "total": 6})
	})
	v.b.JSON(lotteryTagsPath, []any{})
	v.b.Handle(lotteryCreatePath, func(c bilitest.Call) bilitest.Reply {
		if c.Form.Get("tag") != "天选时刻" || c.Form.Get("csrf") != "csrf-42" {
			t.Errorf("建组参数：%v", c.Form)
		}
		return bilitest.OK(map[string]any{"tagid": 77})
	})
	v.b.JSON(lotteryCopyPath, nil)
	r := v.runTask(42, TaskLiveLottery)
	calls := v.b.Calls(lotteryCopyPath)
	if len(calls) != 1 || calls[0].Form.Get("fids") != "1002,1001" || calls[0].Form.Get("tagids") != "77" || v.b.Count(lotteryCreatePath) != 1 {
		t.Fatalf("分组请求：%v", calls)
	}
	if r.Note != "参加了 3 个抽奖" || step(t, r, "分组").Status != api.StatusOK {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryGroupingPaginatesAndRereadsExistingTag(t *testing.T) {
	v := lotteryEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.LiveLottery.Follow = true; s.Tasks.LiveLottery.Group = true })
	v.b.JSON(lotteryCheckPath, lotteryDetail(1, 1))
	v.b.Handle(lotteryFollowingPath, func(c bilitest.Call) bilitest.Reply {
		if v.b.Count(lotteryFollowingPath) == 1 {
			return bilitest.OK(map[string]any{"list": []any{map[string]any{"mid": 99}}, "total": 30})
		}
		if c.Query.Get("pn") == "1" {
			list := []any{}
			for i := range 20 {
				list = append(list, map[string]any{"mid": 2000 + i})
			}
			return bilitest.OK(map[string]any{"list": list, "total": 30})
		}
		return bilitest.OK(map[string]any{"list": []any{map[string]any{"mid": 1001}, map[string]any{"mid": 99}}, "total": 30})
	})
	v.b.Handle(lotteryTagsPath, func(bilitest.Call) bilitest.Reply {
		if v.b.Count(lotteryTagsPath) == 1 {
			return bilitest.OK([]any{})
		}
		return bilitest.OK([]any{map[string]any{"tagid": 78, "name": "天选时刻"}})
	})
	v.b.Fail(lotteryCreatePath, 22106, "分组已存在")
	v.b.JSON(lotteryCopyPath, nil)
	r := v.runTask(42, TaskLiveLottery)
	if v.b.Count(lotteryFollowingPath) != 3 || v.b.Count(lotteryTagsPath) != 2 || v.b.Calls(lotteryCopyPath)[0].Form.Get("tagids") != "78" || step(t, r, "分组").Status != api.StatusOK {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryGroupingFailurePreservesJoinedResult(t *testing.T) {
	v := lotteryEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.LiveLottery.Follow = true; s.Tasks.LiveLottery.Group = true })
	v.b.JSON(lotteryCheckPath, lotteryDetail(1, 1))
	v.b.Handle(lotteryFollowingPath, func(bilitest.Call) bilitest.Reply {
		list := []any{}
		if v.b.Count(lotteryFollowingPath) > 1 {
			list = append(list, map[string]any{"mid": 1001})
		}
		return bilitest.OK(map[string]any{"list": list, "total": len(list)})
	})
	v.b.JSON(lotteryTagsPath, []any{})
	v.b.Fail(lotteryCreatePath, -352, "风控校验失败")
	r := v.runTask(42, TaskLiveLottery)
	if r.Status != api.StatusPartial || step(t, r, "参加").Status != api.StatusOK || step(t, r, "分组").Reason != "risk" || !strings.Contains(r.Note, "参加了 1 个抽奖") || v.b.Count(lotteryCopyPath) != 0 {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryRiskInMiddleContinuesAndRepeatedRiskStops(t *testing.T) {
	for _, path := range []string{lotteryCheckPath, lotteryJoinPath} {
		t.Run(path, func(t *testing.T) {
			v := lotteryEnv(t, 8)
			v.b.Handle(path, func(c bilitest.Call) bilitest.Reply {
				id, _ := strconv.Atoi(c.Form.Get("id"))
				if path == lotteryCheckPath {
					id, _ = strconv.Atoi(c.Query.Get("roomid"))
				}
				if id >= 2 && id <= 4 {
					return bilitest.Code(-352, "风控校验失败")
				}
				if path == lotteryCheckPath {
					return bilitest.OK(lotteryDetail(id, 0))
				}
				return bilitest.OK(nil)
			})
			r := v.runTask(42, TaskLiveLottery)
			if v.b.Count(path) != 4 || v.b.Count(lotteryCheckPath) != 4 || r.Status != api.StatusPartial || !strings.Contains(r.Note, "参加了 1 个抽奖") {
				t.Fatalf("结果：%+v，出错端点请求%d次", r, v.b.Count(path))
			}
			found := false
			for _, s := range r.Steps {
				if s.Name == "参加" && s.Reason == "risk" {
					found = true
				}
			}
			if !found {
				t.Fatal("没有报告连续风控")
			}
		})
	}
}

func TestLotterySingleJoinFailureDoesNotStopLaterRooms(t *testing.T) {
	v := lotteryEnv(t, 3)
	v.b.Handle(lotteryJoinPath, func(c bilitest.Call) bilitest.Reply {
		if c.Form.Get("id") == "2" {
			return bilitest.Code(-352, "风控校验失败")
		}
		return bilitest.OK(nil)
	})
	r := v.runTask(42, TaskLiveLottery)
	if v.b.Count(lotteryJoinPath) != 3 || r.Note != "参加了 2 个抽奖" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryNotLoginStopsWithoutGrouping(t *testing.T) {
	v := lotteryEnv(t, 3)
	v.b.Fail(lotteryJoinPath, -101, "请先登录")
	r := v.runTask(42, TaskLiveLottery)
	if v.b.Count(lotteryJoinPath) != 1 || v.b.Count(lotteryCheckPath) != 1 || step(t, r, "参加").Reason != "not_login" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryBoundsScanAt150CandidatesAnd30Joins(t *testing.T) {
	v := lotteryEnv(t, 0)
	v.b.Handle(lotteryRoomsPath, func(c bilitest.Call) bilitest.Reply {
		page, _ := strconv.Atoi(c.Query.Get("page"))
		list := []any{}
		for i := range 99 {
			list = append(list, lotteryRoom((page-1)*99+i+1, lotteryPendant(504)))
		}
		return bilitest.OK(map[string]any{"list": list})
	})
	r := v.runTask(42, TaskLiveLottery)
	if v.b.Count(lotteryRoomsPath) != 2 || v.b.Count(lotteryJoinPath) != 30 || v.b.Count(lotteryCheckPath) != 30 || step(t, r, "扫描").Detail != "150 个直播间在抽奖" || r.Note != "参加了 30 个抽奖" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestLotteryScansAtMostThreePagesPerAreaAndStopsShortPage(t *testing.T) {
	v := lotteryEnv(t, 0)
	v.b.JSON(lotteryAreasPath, map[string]any{"data": []any{map[string]any{"id": 1}, map[string]any{"id": 2}}})
	v.b.Handle(lotteryRoomsPath, func(c bilitest.Call) bilitest.Reply {
		list := []any{}
		if c.Query.Get("parent_area_id") == "1" {
			for i := range 99 {
				list = append(list, lotteryRoom(i+1, nil))
			}
		}
		return bilitest.OK(map[string]any{"list": list})
	})
	r := v.runTask(42, TaskLiveLottery)
	if v.b.Count(lotteryRoomsPath) != 4 || r.Status != api.StatusSkipped || v.b.Count(lotteryCheckPath) != 0 {
		t.Fatalf("结果：%+v", r)
	}
}
