package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

const (
	dailyRewardPath    = "api.bilibili.com/x/member/web/exp/reward"
	dailyExpPath       = "api.bilibili.com/x/web-interface/coin/today/exp"
	dailyFollowingPath = "api.bilibili.com/x/relation/followings"
	dailyUploadsPath   = "api.bilibili.com/x/space/wbi/arc/search"
	dailyRankingPath   = "api.bilibili.com/x/web-interface/ranking/v2"
	dailyViewPath      = "api.bilibili.com/x/web-interface/wbi/view"
	dailyHeartbeatPath = "api.bilibili.com/x/click-interface/web/heartbeat"
	dailySharePath     = "api.bilibili.com/x/web-interface/share/add"
	dailyCoinsPath     = "api.bilibili.com/x/web-interface/archive/coins"
	dailyAddPath       = "api.bilibili.com/x/web-interface/coin/add"
)

func dailyVideo(aid, mid int64, copyright int) map[string]any {
	return map[string]any{"aid": aid, "bvid": fmt.Sprintf("BV%d", aid), "cid": aid + 1000, "title": fmt.Sprintf("视频%d", aid), "copyright": copyright, "duration": 90, "owner": map[string]any{"mid": mid}}
}

func dailyEnv(t *testing.T) *testEnv {
	t.Helper()
	v := newEnv(t)
	v.login(42)
	v.b.JSON(dailyRewardPath, map[string]any{"login": true, "watch": false, "share": false})
	v.b.JSON(dailyExpPath, 0)
	v.b.JSON(dailyFollowingPath, map[string]any{"list": []any{}, "total": 0})
	var list []any
	for i := range 8 {
		list = append(list, dailyVideo(int64(i+1), 100, 1))
	}
	v.b.JSON(dailyRankingPath, map[string]any{"list": list})
	v.b.JSON(dailyHeartbeatPath, nil)
	v.b.JSON(dailySharePath, 1)
	v.b.JSON(dailyCoinsPath, map[string]any{"multiply": 0})
	v.b.JSON(dailyAddPath, map[string]any{"like": true})
	return v
}

func dailyOnlyCoins(v *testEnv, coins int) {
	v.set(func(s *api.Settings) {
		s.Tasks.Daily.Watch = false
		s.Tasks.Daily.Share = false
		s.Tasks.Daily.Coins = coins
	})
}

func dailyNoMutations(t *testing.T, v *testEnv) {
	t.Helper()
	for _, path := range []string{dailyHeartbeatPath, dailySharePath, dailyAddPath} {
		if got := v.b.Count(path); got != 0 {
			t.Errorf("%s 发了 %d 次请求", path, got)
		}
	}
}

func TestDailyHappyPath(t *testing.T) {
	v := dailyEnv(t)
	v.b.JSON(dailyExpPath, 20)
	var waits []time.Duration
	v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		if d == 15*time.Second {
			waits = append(waits, d)
		}
		return ctx.Err()
	}
	r := v.runTask(42, TaskDaily)
	if r.Status != api.StatusOK || !strings.Contains(r.Note, "今天投币 5 个，预计经验 +65") {
		t.Fatalf("结果 = %+v", r)
	}
	heartbeats := v.b.Calls(dailyHeartbeatPath)
	if len(heartbeats) != 2 || len(waits) != 1 {
		t.Fatalf("心跳 %d 次，观看等待 %v", len(heartbeats), waits)
	}
	for i, c := range heartbeats {
		progress := strconv.Itoa(i * 15)
		for key, want := range map[string]string{"played_time": progress, "realtime": progress, "real_played_time": progress, "mid": "42", "csrf": "csrf-42", "type": "3", "dt": "2", "play_type": "3", "start_ts": strconv.FormatInt(v.now.Unix(), 10)} {
			if c.Form.Get(key) != want {
				t.Errorf("心跳%d %s=%q，期望 %q", i, key, c.Form.Get(key), want)
			}
		}
		if c.Method != "POST" || c.Query.Get("played_time") != progress || c.Query.Get("aid") != c.Form.Get("aid") || len(c.Query) != 2 || c.Form.Get("cid") == "" || c.Header.Get("Origin") != "https://www.bilibili.com" {
			t.Errorf("心跳请求 = %+v", c)
		}
	}
	shares := v.b.Calls(dailySharePath)
	if len(shares) != 1 || shares[0].Form.Get("aid") != heartbeats[0].Form.Get("aid") || shares[0].Form.Get("csrf") != "csrf-42" || shares[0].Form.Has("eab_x") || shares[0].Form.Has("ramval") {
		t.Errorf("分享请求 = %+v", shares)
	}
	coins := v.b.Calls(dailyAddPath)
	if len(coins) != 3 {
		t.Fatalf("投币 %d 次，期望 3 次", len(coins))
	}
	seen := map[string]bool{}
	for _, c := range coins {
		aid := c.Form.Get("aid")
		if seen[aid] {
			t.Errorf("重复投了 %s", aid)
		}
		seen[aid] = true
		for key, want := range map[string]string{"multiply": "1", "select_like": "1", "cross_domain": "true", "csrf": "csrf-42", "source": "web_normal", "ga": "1"} {
			if c.Form.Get(key) != want {
				t.Errorf("投币 %s=%q，期望 %q", key, c.Form.Get(key), want)
			}
		}
		if c.Method != "POST" || c.Header.Get("Referer") != "https://www.bilibili.com/video/BV"+aid+"/" || c.Header.Get("Origin") != "https://www.bilibili.com" || len(c.Query) != 0 || c.Form.Has("eab_x") || c.Form.Has("ramval") {
			t.Errorf("投币请求 = %+v", c)
		}
	}
	all := v.b.Calls("")
	firstAdd, shareIndex, heartbeatIndex := -1, -1, -1
	for i, c := range all {
		if c.Path == dailyHeartbeatPath {
			heartbeatIndex = i
		}
		if c.Path == dailySharePath {
			shareIndex = i
		}
		if c.Path == dailyAddPath && firstAdd < 0 {
			firstAdd = i
		}
	}
	if heartbeatIndex >= shareIndex || shareIndex >= firstAdd {
		t.Errorf("请求顺序：心跳 %d 分享 %d 投币 %d", heartbeatIndex, shareIndex, firstAdd)
	}
	if v.b.Calls(dailyRewardPath)[0].Header.Get("Referer") != "https://account.bilibili.com/account/home" || v.b.Calls(dailyRewardPath)[0].Header.Get("Origin") != "https://account.bilibili.com" || v.b.Calls(dailyExpPath)[0].Header.Get("Origin") != "https://www.bilibili.com" {
		t.Error("奖励查询的请求头不对")
	}
}

func TestDailyAlreadyDone(t *testing.T) {
	v := dailyEnv(t)
	v.b.JSON(dailyRewardPath, map[string]any{"login": true, "watch": true, "share": true})
	v.b.JSON(dailyExpPath, 50)
	r := v.runTask(42, TaskDaily)
	dailyNoMutations(t, v)
	if r.Status != api.StatusSkipped || v.b.Count(dailyRankingPath) != 0 || v.b.Count(dailyFollowingPath) != 0 || !strings.Contains(r.Note, "预计经验 +65") {
		t.Fatalf("结果 = %+v", r)
	}
	for _, name := range []string{"观看视频", "分享视频", "投币"} {
		if step(t, r, name).Status != api.StatusSkipped {
			t.Errorf("%s 没有跳过", name)
		}
	}
}

func TestDailyCoinBudgetAndLevel(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		money                                   float64
		reserve, level, stop, exp, target, want int
		status                                  api.Status
		detail                                  string
	}{
		{"保留硬币", 4.9, 3, 4, 6, 0, 5, 1, api.StatusOK, "原计划 5 个，只投了 1 个"},
		{"余额不足", 3.9, 3, 4, 6, 0, 5, 0, api.StatusSkipped, "没有可用硬币"},
		{"停止等级", 100, 0, 6, 6, 0, 5, 0, api.StatusSkipped, "6 级起停止"},
		{"已投部分", 100, 0, 4, 6, 20, 5, 3, api.StatusOK, "3 个"},
		{"达到目标", 100, 0, 4, 6, 30, 3, 0, api.StatusSkipped, "今天已经投了 3 个"},
		{"六级继续", 100, 0, 6, 0, 0, 1, 1, api.StatusOK, "1 个"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := dailyEnv(t)
			dailyOnlyCoins(v, tc.target)
			n := navData(42)
			n["money"] = tc.money
			n["level_info"] = map[string]any{"current_level": tc.level}
			v.b.JSON(navPath, n)
			v.set(func(s *api.Settings) { s.Tasks.Daily.KeepCoins = tc.reserve; s.Tasks.Daily.StopAtLevel = tc.stop })
			v.b.JSON(dailyExpPath, tc.exp)
			r := v.runTask(42, TaskDaily)
			s := step(t, r, "投币")
			if v.b.Count(dailyAddPath) != tc.want || s.Status != tc.status || !strings.Contains(s.Detail, tc.detail) {
				t.Fatalf("投币 %d 次；步骤 %+v", v.b.Count(dailyAddPath), s)
			}
			if tc.want == 0 && v.b.Count(dailyRankingPath) != 0 {
				t.Error("不投币时不应选择视频")
			}
		})
	}
}

func TestDailyEmptyFollowingsUsesRanking(t *testing.T) {
	v := dailyEnv(t)
	dailyOnlyCoins(v, 1)
	r := v.runTask(42, TaskDaily)
	if r.Status != api.StatusOK || v.b.Count(dailyRankingPath) != 1 {
		t.Fatalf("结果 = %+v", r)
	}
	c := v.b.Calls(dailyFollowingPath)[0]
	if c.Query.Get("order_type") != "attention" || c.Query.Get("pn") != "1" || c.Query.Get("vmid") != "42" {
		t.Errorf("关注请求 = %+v", c)
	}
	c = v.b.Calls(dailyRankingPath)[0]
	if c.Query.Get("rid") != "0" || c.Query.Get("type") != "all" || len(c.Query) != 2 || c.Header.Get("Dnt") != "1" {
		t.Errorf("排行榜请求 = %+v", c)
	}
}

func TestDailyExcludesOwnAndFullVideos(t *testing.T) {
	v := dailyEnv(t)
	dailyOnlyCoins(v, 5)
	v.b.JSON(dailyRankingPath, map[string]any{"list": []any{dailyVideo(1, 42, 1), dailyVideo(2, 100, 1), dailyVideo(3, 100, 2), dailyVideo(4, 100, 1), dailyVideo(5, 100, 2)}})
	v.b.Handle(dailyCoinsPath, func(c bilitest.Call) bilitest.Reply {
		amount := 0
		switch c.Query.Get("aid") {
		case "1":
			t.Error("不应查询自己的视频")
		case "2":
			amount = 2
		case "3", "4":
			amount = 1
		}
		return bilitest.OK(map[string]any{"multiply": amount})
	})
	r := v.runTask(42, TaskDaily)
	calls := v.b.Calls(dailyAddPath)
	if len(calls) != 2 {
		t.Fatalf("投币请求 = %+v；结果 = %+v", calls, r)
	}
	for _, c := range calls {
		if id := c.Form.Get("aid"); id != "4" && id != "5" {
			t.Errorf("投给了不合格视频 %s", id)
		}
	}
	if s := step(t, r, "投币"); s.Status != api.StatusOK || !strings.Contains(s.Detail, "原计划 5 个，只投了 2 个") {
		t.Errorf("步骤 = %+v", s)
	}
}

func TestDailyShareRiskStillCoins(t *testing.T) {
	v := dailyEnv(t)
	v.b.Fail(dailySharePath, -352, "风控拦截")
	v.set(func(s *api.Settings) { s.Tasks.Daily.Coins = 2 })
	r := v.runTask(42, TaskDaily)
	s := step(t, r, "分享视频")
	if r.Status != api.StatusPartial || s.Reason != string(bili.ReasonRisk) || s.Code != -352 || v.b.Count(dailyAddPath) != 2 || !strings.Contains(r.Note, "预计经验 +30") {
		t.Fatalf("结果 = %+v", r)
	}
}

func TestDailyInsufficientCoinsStops(t *testing.T) {
	for _, firstOK := range []bool{false, true} {
		t.Run(strconv.FormatBool(firstOK), func(t *testing.T) {
			v := dailyEnv(t)
			dailyOnlyCoins(v, 5)
			n := 0
			v.b.Handle(dailyAddPath, func(c bilitest.Call) bilitest.Reply {
				n++
				if firstOK && n == 1 {
					return bilitest.OK(nil)
				}
				return bilitest.Code(-104, "硬币不足")
			})
			r := v.runTask(42, TaskDaily)
			s := step(t, r, "投币")
			want := 1
			if firstOK {
				want = 2
			}
			if n != want || !strings.Contains(s.Detail, "硬币不足") {
				t.Fatalf("投币 %d 次，步骤 %+v", n, s)
			}
			if firstOK {
				if s.Status != api.StatusOK || !strings.Contains(r.Note, "今天投币 1 个") {
					t.Fatalf("结果 = %+v", r)
				}
			} else if s.Status != api.StatusFailed || s.Reason != string(bili.ReasonInsufficient) {
				t.Fatalf("步骤 = %+v", s)
			}
		})
	}
}

func TestDailyUPSelectionWBIAndFallback(t *testing.T) {
	for _, source := range []string{"指定UP", "关注UP", "没有投稿", "查询失败"} {
		t.Run(source, func(t *testing.T) {
			v := dailyEnv(t)
			v.set(func(s *api.Settings) { s.Tasks.Daily.Coins = 1; s.Tasks.Daily.UpIDs = []int64{100} })
			v.b.JSON(dailyFollowingPath, map[string]any{"list": []any{map[string]any{"mid": 200}}, "total": 1})
			v.b.Handle(dailyUploadsPath, func(c bilitest.Call) bilitest.Reply {
				if c.Query.Get("w_rid") == "" || c.Query.Get("wts") == "" || c.Header.Get("Origin") != "https://space.bilibili.com" || c.Query.Get("order") != "pubdate" || c.Query.Get("web_location") != "1550101" || c.Query.Get("order_avoided") != "true" {
					t.Errorf("投稿请求 = %+v", c)
				}
				mid, _ := strconv.ParseInt(c.Query.Get("mid"), 10, 64)
				if source == "查询失败" || (source == "关注UP" && mid == 100) {
					return bilitest.Code(-400, "请求错误")
				}
				if source == "没有投稿" {
					return bilitest.OK(map[string]any{"page": map[string]any{"count": 0}})
				}
				if c.Query.Get("ps") == "30" {
					if c.Query.Get("pn") != "1" {
						t.Error("查总数应从第一页开始")
					}
					return bilitest.OK(map[string]any{"page": map[string]any{"count": 1}})
				}
				if c.Query.Get("ps") != "1" || c.Query.Get("pn") != "1" {
					t.Errorf("随机单条请求 = %+v", c)
				}
				return bilitest.OK(map[string]any{"list": map[string]any{"vlist": []any{map[string]any{"aid": mid + 1, "bvid": fmt.Sprintf("BV%d", mid+1), "title": "UP投稿"}}}})
			})
			v.b.Handle(dailyViewPath, func(c bilitest.Call) bilitest.Reply {
				if c.Query.Get("w_rid") == "" || c.Query.Get("wts") == "" {
					t.Error("视频详情没有 WBI")
				}
				aid, _ := strconv.ParseInt(c.Query.Get("aid"), 10, 64)
				return bilitest.OK(dailyVideo(aid, aid-1, 1))
			})
			r := v.runTask(42, TaskDaily)
			if r.Status != api.StatusOK || v.b.Count(dailyAddPath) != 1 {
				t.Fatalf("结果 = %+v", r)
			}
			if source == "指定UP" || source == "关注UP" {
				if v.b.Count(dailyRankingPath) != 0 || v.b.Count(dailyViewPath) != 1 {
					t.Fatal("应先投 UP 投稿，不查询排行榜")
				}
				want := "101"
				if source == "关注UP" {
					want = "201"
				}
				if v.b.Calls(dailyAddPath)[0].Form.Get("aid") != want {
					t.Error("没有投给优先 UP")
				}
			} else if v.b.Count(dailyRankingPath) != 1 {
				t.Error("应使用排行榜兜底")
			}
		})
	}
}

func TestDailyCreatorExhaustionUsesRankingAndNoDuplicate(t *testing.T) {
	v := dailyEnv(t)
	dailyOnlyCoins(v, 3)
	v.set(func(s *api.Settings) { s.Tasks.Daily.UpIDs = []int64{100}; s.Tasks.Daily.Like = false })
	v.b.Handle(dailyUploadsPath, func(c bilitest.Call) bilitest.Reply {
		return bilitest.OK(map[string]any{"page": map[string]any{"count": 1}, "list": map[string]any{"vlist": []any{dailyVideo(1, 100, 1)}}})
	})
	v.b.JSON(dailyRankingPath, map[string]any{"list": []any{dailyVideo(1, 100, 1), dailyVideo(2, 100, 1), dailyVideo(3, 100, 1)}})
	r := v.runTask(42, TaskDaily)
	calls := v.b.Calls(dailyAddPath)
	if r.Status != api.StatusOK || len(calls) != 3 || calls[0].Form.Get("aid") != "1" || v.b.Count(dailyUploadsPath) != 2 {
		t.Fatalf("请求 = %+v；结果 = %+v", calls, r)
	}
	seen := map[string]bool{}
	for _, c := range calls {
		id := c.Form.Get("aid")
		if seen[id] || c.Form.Get("select_like") != "0" {
			t.Errorf("请求 = %+v", c)
		}
		seen[id] = true
	}
}

func TestDailyCoinRecoverableAndHardErrors(t *testing.T) {
	for _, code := range []int{34005, 34002, 34003, 10003, -400, 34004, -352, -412, -403, -102, -101} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			v := dailyEnv(t)
			dailyOnlyCoins(v, 1)
			n, slowWait := 0, 0
			v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
				if d == 3*time.Second {
					slowWait++
				}
				return ctx.Err()
			}
			v.b.Handle(dailyAddPath, func(c bilitest.Call) bilitest.Reply {
				n++
				if n == 1 {
					return bilitest.Code(code, "投币失败")
				}
				return bilitest.OK(nil)
			})
			r := v.runTask(42, TaskDaily)
			s := step(t, r, "投币")
			recoverable := code > 0 || code == -400
			if recoverable {
				if n != 2 || s.Status != api.StatusOK {
					t.Fatalf("请求 %d 次；结果 %+v", n, r)
				}
			} else if n != 1 || s.Status != api.StatusFailed {
				t.Fatalf("请求 %d 次；结果 %+v", n, r)
			}
			if code == 34004 && slowWait != 1 {
				t.Errorf("没有等待 3 秒：%d", slowWait)
			}
			if code == -352 || code == -412 || code == -403 {
				if s.Reason != string(bili.ReasonRisk) {
					t.Errorf("步骤 = %+v", s)
				}
			}
		})
	}
}

func TestDailyShareDuplicateAndWatchFailure(t *testing.T) {
	v := dailyEnv(t)
	v.b.Fail(dailyHeartbeatPath, -352, "风控拦截")
	v.b.Fail(dailySharePath, 71000, "已经分享过")
	v.set(func(s *api.Settings) { s.Tasks.Daily.Coins = 1 })
	r := v.runTask(42, TaskDaily)
	if r.Status != api.StatusPartial || step(t, r, "分享视频").Status != api.StatusSkipped || v.b.Count(dailyAddPath) != 1 || !strings.Contains(r.Note, "预计经验 +20") {
		t.Fatalf("结果 = %+v", r)
	}
}

func TestDailyMissingStateDoesNotSpendOrDuplicateWatch(t *testing.T) {
	for _, path := range []string{dailyRewardPath, dailyExpPath} {
		t.Run(path, func(t *testing.T) {
			v := dailyEnv(t)
			v.b.JSON(path, nil)
			r := v.runTask(42, TaskDaily)
			if path == dailyExpPath {
				if v.b.Count(dailyAddPath) != 0 || v.b.Count(dailyHeartbeatPath) != 2 || v.b.Count(dailySharePath) != 1 {
					t.Fatal("投币状态缺失时仍重复花币，或没有执行独立步骤")
				}
			} else if v.b.Count(dailyHeartbeatPath) != 0 || v.b.Count(dailySharePath) != 0 || v.b.Count(dailyAddPath) != 5 {
				t.Fatal("奖励状态缺失时重复观看分享，或没有继续投币")
			}
			if r.Status != api.StatusPartial {
				t.Fatalf("结果 = %+v", r)
			}
		})
	}
}

func TestDailyNoEligibleVideoAndRankingFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(strconv.FormatBool(failed), func(t *testing.T) {
			v := dailyEnv(t)
			dailyOnlyCoins(v, 1)
			if failed {
				v.b.Fail(dailyRankingPath, -352, "风控拦截")
			} else {
				v.b.JSON(dailyRankingPath, map[string]any{"list": []any{dailyVideo(1, 42, 1)}})
			}
			r := v.runTask(42, TaskDaily)
			s := step(t, r, "投币")
			if v.b.Count(dailyAddPath) != 0 {
				t.Error("不应投币")
			}
			if failed {
				if s.Status != api.StatusFailed || s.Reason != string(bili.ReasonRisk) {
					t.Fatalf("步骤 = %+v", s)
				}
			} else if s.Status != api.StatusSkipped {
				t.Fatalf("步骤 = %+v", s)
			}
		})
	}
}
