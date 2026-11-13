package engine

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

const (
	fansMedalBootstrapPath = "api.live.bilibili.com/news/v1/notice/recom"
	fansMedalPanelPath     = "api.live.bilibili.com/xlive/app-ucenter/v1/fansMedal/panel"
	fansMedalProgressPath  = "api.live.bilibili.com/xlive/app-ucenter/v1/fansMedal/GetActivatedMedalInfo"
	fansMedalRoomPath      = "api.live.bilibili.com/room/v1/Room/get_info"
	fansMedalLikePath      = "api.live.bilibili.com/xlive/app-ucenter/v1/like_info_v3/like/likeReportV3"
	fansMedalDanmakuPath   = "api.live.bilibili.com/msg/send"
	fansMedalEnterPath     = "live-trace.bilibili.com/xlive/data-interface/v1/x25Kn/E"
	fansMedalHeartPath     = "live-trace.bilibili.com/xlive/data-interface/v1/x25Kn/X"
)

func fansMedalItem(mid int) map[string]any {
	return map[string]any{"medal": map[string]any{"target_id": mid, "medal_id": mid, "medal_name": "粉丝", "level": 25}, "room_info": map[string]any{"room_id": mid + 1000, "living_status": 1}, "anchor_info": map[string]any{"nick_name": fmt.Sprintf("主播%d", mid)}}
}

func fansMedalProgressData(action string, done bool, subtitle string) map[string]any {
	title := "点赞10次"
	switch action {
	case "sendDanmu":
		title = "发送1条弹幕"
	case "watchLive":
		title = "观看1分钟"
	}
	return map[string]any{"is_lighted": true, "reach_free_intimacy_limit": false, "task_info": []any{map[string]any{"title": title, "sub_title": subtitle, "jump_type": action, "is_done": done}}}
}

func fansMedalEnv(t *testing.T, mids ...int) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.set(func(s *api.Settings) {
		s.Tasks.FansMedal.Like = false
		s.Tasks.FansMedal.Watch = false
		s.Tasks.FansMedal.Danmaku = ""
		s.Tasks.FansMedal.Max = 0
	})
	v.b.Handle(fansMedalBootstrapPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("product") != "live" {
			t.Errorf("启动参数：%v", c.Query)
		}
		r := bilitest.OK(nil)
		r.Header = http.Header{"Set-Cookie": {"LIVE_BUVID=live-device; Path=/; Domain=.bilibili.com"}}
		return r
	})
	list := []any{}
	for _, mid := range mids {
		list = append(list, fansMedalItem(mid))
	}
	v.b.JSON(fansMedalPanelPath, map[string]any{"list": list, "special_list": []any{}, "page_info": map[string]any{"total_page": 1, "has_more": false}})
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("like", true, "1/1"))
	v.b.Handle(fansMedalRoomPath, func(c bilitest.Call) bilitest.Reply {
		id, _ := strconv.Atoi(c.Query.Get("room_id"))
		return bilitest.OK(map[string]any{"room_id": id, "uid": id - 1000, "live_status": 1, "area_id": 2, "parent_area_id": 1})
	})
	v.b.JSON(fansMedalLikePath, nil)
	v.b.JSON(fansMedalDanmakuPath, nil)
	return v
}

func TestFansMedalNoneSkipsWithoutInteractions(t *testing.T) {
	v := fansMedalEnv(t)
	v.set(func(s *api.Settings) {
		s.Tasks.FansMedal.Like = true
		s.Tasks.FansMedal.Watch = true
		s.Tasks.FansMedal.Danmaku = "晚上好"
	})
	r := v.runTask(42, TaskFansMedal)
	if r.Status != api.StatusSkipped || r.Note != "没有粉丝牌" {
		t.Fatalf("结果：%+v", r)
	}
	for _, path := range []string{fansMedalProgressPath, fansMedalLikePath, fansMedalDanmakuPath, fansMedalEnterPath, fansMedalHeartPath} {
		if v.b.Count(path) != 0 {
			t.Errorf("没有粉丝牌仍请求了 %s", path)
		}
	}
	if v.b.Calls(fansMedalPanelPath)[0].Cookie["LIVE_BUVID"] != "live-device" {
		t.Fatal("没有保存直播设备 Cookie")
	}
}

func TestFansMedalLikeBatchesStopOnServerProgress(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Like = true })
	reads := 0
	v.b.Handle(fansMedalProgressPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("csrf") != "csrf-42" || c.Query.Get("web_location") != "444.260" || c.Query.Get("target_id") != "1" {
			t.Errorf("进度参数：%v", c.Query)
		}
		reads++
		return bilitest.OK(fansMedalProgressData("like", reads >= 3, fmt.Sprintf("%d/3", reads-1)))
	})
	var waits []time.Duration
	v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return ctx.Err() }
	r := v.runTask(42, TaskFansMedal)
	if r.Status != api.StatusOK || !strings.Contains(r.Note, "点赞 1 个") || v.b.Count(fansMedalLikePath) != 2 {
		t.Fatalf("结果：%+v，点赞%d次", r, v.b.Count(fansMedalLikePath))
	}
	for _, c := range v.b.Calls(fansMedalLikePath) {
		if c.Form.Get("click_time") != "10" || c.Form.Get("room_id") != "1001" || c.Form.Get("anchor_id") != "1" || c.Form.Get("uid") != "42" || c.Form.Get("csrf") != "csrf-42" || c.Form.Get("csrf_token") != "csrf-42" {
			t.Errorf("点赞参数：%v", c.Form)
		}
		if c.Header.Get("Referer") != "https://live.bilibili.com/" || c.Header.Get("Origin") != "https://live.bilibili.com" {
			t.Errorf("点赞请求头：%v", c.Header)
		}
	}
	count := 0
	for _, d := range waits {
		if d == 3*time.Second {
			count++
		}
	}
	if count != 2 {
		t.Errorf("点赞后的等待次数：%d", count)
	}
	v.runTask(42, TaskFansMedal)
	if v.b.Count(fansMedalLikePath) != 2 {
		t.Fatal("已经完成后又点赞")
	}
}

func TestFansMedalOfflineSkipsLikesAndWatchButSendsOneDanmaku(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) {
		s.Tasks.FansMedal.Like = true
		s.Tasks.FansMedal.Watch = true
		s.Tasks.FansMedal.Danmaku = "今天也辛苦啦"
	})
	v.b.JSON(fansMedalRoomPath, map[string]any{"live_status": 0})
	progress := fansMedalProgressData("like", false, "0/1")
	progress["task_info"] = []any{
		map[string]any{"title": "点赞10次", "sub_title": "0/1", "jump_type": "like", "is_done": false},
		map[string]any{"title": "发送1条弹幕", "sub_title": "0/5", "jump_type": "sendDanmu", "is_done": false},
		map[string]any{"title": "观看1分钟", "sub_title": "0/1", "jump_type": "watchLive", "is_done": false},
	}
	v.b.JSON(fansMedalProgressPath, progress)
	r := v.runTask(42, TaskFansMedal)
	if v.b.Count(fansMedalLikePath) != 0 || v.b.Count(fansMedalEnterPath) != 0 || v.b.Count(fansMedalHeartPath) != 0 {
		t.Fatal("离线直播间发生点赞或观看")
	}
	calls := v.b.Calls(fansMedalDanmakuPath)
	if len(calls) != 1 {
		t.Fatalf("弹幕请求%d次", len(calls))
	}
	form := calls[0].Form
	if form.Get("msg") != "今天也辛苦啦" || form.Get("roomid") != "1001" || form.Get("rnd") != strconv.FormatInt(v.now.Unix(), 10) || form.Get("csrf") != "csrf-42" || form.Get("csrf_token") != "csrf-42" || form.Get("bubble") != "0" || form.Get("color") != "16777215" || form.Get("mode") != "1" || form.Get("fontsize") != "25" {
		t.Errorf("弹幕参数：%v", form)
	}
	if step(t, r, "点赞").Status != api.StatusSkipped || step(t, r, "观看").Status != api.StatusSkipped || !strings.Contains(r.Note, "弹幕 1 个") {
		t.Fatalf("结果：%+v", r)
	}
}

func TestFansMedalCompletedAndFullMedalsDoNotInteract(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(strconv.FormatBool(full), func(t *testing.T) {
			v := fansMedalEnv(t, 1)
			v.set(func(s *api.Settings) {
				s.Tasks.FansMedal.Like = true
				s.Tasks.FansMedal.Watch = true
				s.Tasks.FansMedal.Danmaku = "你好"
			})
			p := fansMedalProgressData("like", !full, "0/1")
			p["reach_free_intimacy_limit"] = full
			v.b.JSON(fansMedalProgressPath, p)
			r := v.runTask(42, TaskFansMedal)
			if r.Status != api.StatusSkipped || v.b.Count(fansMedalLikePath)+v.b.Count(fansMedalDanmakuPath)+v.b.Count(fansMedalEnterPath) != 0 {
				t.Fatalf("结果：%+v", r)
			}
		})
	}
}

func TestFansMedalMaxAndPaginationDeduplicateSpecialFirst(t *testing.T) {
	v := fansMedalEnv(t)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Like = true; s.Tasks.FansMedal.Max = 2 })
	v.b.Handle(fansMedalPanelPath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("page_size") != "10" {
			t.Errorf("分页参数：%v", c.Query)
		}
		if c.Query.Get("page") == "1" {
			return bilitest.OK(map[string]any{"special_list": []any{fansMedalItem(2)}, "list": []any{fansMedalItem(1), fansMedalItem(2)}, "page_info": map[string]any{"has_more": true, "total_page": 2}})
		}
		return bilitest.OK(map[string]any{"special_list": []any{fansMedalItem(2)}, "list": []any{fansMedalItem(3)}, "page_info": map[string]any{"has_more": false, "total_page": 2}})
	})
	v.runTask(42, TaskFansMedal)
	calls := v.b.Calls(fansMedalProgressPath)
	if len(calls) != 2 || calls[0].Query.Get("target_id") != "2" || calls[1].Query.Get("target_id") != "1" || v.b.Count(fansMedalPanelPath) != 2 {
		t.Fatalf("没有按特别列表优先去重并限制数量：%v", calls)
	}
}

func fansMedalHeartbeatReply(interval int, key string, timestamp int64) bilitest.Reply {
	return bilitest.OK(map[string]any{"heartbeat_interval": interval, "secret_key": key, "secret_rule": []int{2}, "timestamp": timestamp})
}

func TestFansMedalWatchUsesRotatingSecretsAndReturnedIntervals(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Watch = true; s.Tasks.FansMedal.Minutes = 2 })
	var mu sync.Mutex
	var waits []time.Duration
	v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		return ctx.Err()
	}
	v.b.Handle(fansMedalProgressPath, func(c bilitest.Call) bilitest.Reply {
		return bilitest.OK(fansMedalProgressData("watchLive", v.b.Count(fansMedalHeartPath) >= 2, "0/1"))
	})
	var device string
	v.b.Handle(fansMedalEnterPath, func(c bilitest.Call) bilitest.Reply {
		device = c.Form.Get("device")
		var parts []string
		_ = json.Unmarshal([]byte(device), &parts)
		if len(parts) != 2 || parts[0] != "live-device" || len(parts[1]) != 36 {
			t.Errorf("设备参数：%s", device)
		}
		if c.Form.Get("id") != "[1,2,0,1001]" || c.Form.Get("is_patch") != "0" || c.Form.Get("heart_beat") != "[]" || c.Form.Get("ruid") != "1" || c.Form.Get("web_location") != "444.8" || c.Form.Get("csrf") != "csrf-42" || c.Form.Get("csrf_token") != "csrf-42" || c.Form.Get("ua") == "" || c.Form.Get("wts") == "" || len(c.Form.Get("w_rid")) != 32 {
			t.Errorf("E 参数：%v", c.Form)
		}
		return fansMedalHeartbeatReply(20, "first-secret", 11)
	})
	v.b.Handle(fansMedalHeartPath, func(c bilitest.Call) bilitest.Reply {
		sequence := v.b.Count(fansMedalHeartPath)
		key, interval, timestamp := "first-secret", 20, int64(11)
		if sequence == 2 {
			key, interval, timestamp = "second-secret", 40, 22
		}
		var parts []string
		_ = json.Unmarshal([]byte(device), &parts)
		// 独立拼出签名 JSON，不能调用被测签名函数来计算预期值。
		text := fmt.Sprintf(`{"platform":"web","parent_id":1,"area_id":2,"seq_id":%d,"room_id":1001,"buvid":"live-device","uuid":%q,"ets":%d,"time":%d,"ts":%s}`, sequence, parts[1], timestamp, interval, c.Form.Get("ts"))
		mac := hmac.New(sha256.New, []byte(key))
		_, _ = mac.Write([]byte(text))
		want := hex.EncodeToString(mac.Sum(nil))
		if c.Form.Get("s") != want || c.Form.Get("id") != fmt.Sprintf("[1,2,%d,1001]", sequence) || c.Form.Get("benchmark") != key || c.Form.Get("ets") != strconv.FormatInt(timestamp, 10) || c.Form.Get("time") != strconv.Itoa(interval) || c.Form.Get("device") != device || c.Form.Get("trackid") != "-99998" || c.Form.Get("ruid") != "1" || c.Form.Get("wts") == "" || c.Form.Get("w_rid") == "" {
			t.Errorf("X 参数：%v；预期签名%s", c.Form, want)
		}
		return fansMedalHeartbeatReply(40, "second-secret", 22)
	})
	r := v.runTask(42, TaskFansMedal)
	if r.Status != api.StatusOK || !strings.Contains(r.Note, "观看 1 个") || v.b.Count(fansMedalEnterPath) != 1 || v.b.Count(fansMedalHeartPath) != 2 {
		t.Fatalf("结果：%+v，心跳%d次", r, v.b.Count(fansMedalHeartPath))
	}
	mu.Lock()
	defer mu.Unlock()
	var actual []time.Duration
	for _, d := range waits {
		if d == 20*time.Second || d == 40*time.Second {
			actual = append(actual, d)
		}
	}
	if len(actual) != 2 || actual[0] != 20*time.Second || actual[1] != 40*time.Second {
		t.Errorf("没有按返回间隔累计60秒：%v", actual)
	}
	if v.b.Count(fansMedalProgressPath) != 2 {
		t.Errorf("观看进度查询次数：%d", v.b.Count(fansMedalProgressPath))
	}
}

func TestFansMedalRiskFailureStillRunsIndependentMedals(t *testing.T) {
	v := fansMedalEnv(t, 1, 2, 3)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Danmaku = "你好" })
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("sendDanmu", false, "0/1"))
	v.b.Handle(fansMedalDanmakuPath, func(c bilitest.Call) bilitest.Reply {
		if c.Form.Get("roomid") == "1002" {
			return bilitest.Code(-352, "风控校验失败")
		}
		return bilitest.OK(nil)
	})
	r := v.runTask(42, TaskFansMedal)
	if r.Status != api.StatusPartial || v.b.Count(fansMedalDanmakuPath) != 3 || !strings.Contains(r.Note, "弹幕 2 个") {
		t.Fatalf("结果：%+v", r)
	}
	failures := 0
	for _, s := range r.Steps {
		if s.Name == "弹幕" && s.Status == api.StatusFailed && s.Reason == "risk" {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("没有汇总风控错误：%+v", r.Steps)
	}
}

func TestFansMedalStopsAfterRiskOnThreeDifferentMedals(t *testing.T) {
	v := fansMedalEnv(t, 1, 2, 3, 4)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Danmaku = "你好" })
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("sendDanmu", false, "0/1"))
	v.b.Fail(fansMedalDanmakuPath, -352, "风控校验失败")
	r := v.runTask(42, TaskFansMedal)
	if r.Status != api.StatusFailed || v.b.Count(fansMedalDanmakuPath) != 3 || step(t, r, "弹幕").Reason != "risk" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestFansMedalWatchStopsAfterThreeFailuresWithoutAdvancingSequence(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Watch = true })
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("watchLive", false, "0/1"))
	v.b.Handle(fansMedalEnterPath, func(bilitest.Call) bilitest.Reply { return fansMedalHeartbeatReply(20, "secret", 11) })
	v.b.Fail(fansMedalHeartPath, -352, "风控校验失败")
	r := v.runTask(42, TaskFansMedal)
	if v.b.Count(fansMedalHeartPath) != 3 || step(t, r, "观看").Reason != "risk" || !strings.Contains(r.Note, "观看 0 个") {
		t.Fatalf("结果：%+v", r)
	}
	for _, c := range v.b.Calls(fansMedalHeartPath) {
		if c.Form.Get("id") != "[1,2,1,1001]" || c.Form.Get("ets") != "11" {
			t.Errorf("失败后推进了心跳：%v", c.Form)
		}
	}
}

func TestFansMedalWatchStopsWhenRoomGoesOffline(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Watch = true })
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("watchLive", false, "0/1"))
	v.b.Handle(fansMedalRoomPath, func(bilitest.Call) bilitest.Reply {
		status := 1
		if v.b.Count(fansMedalRoomPath) > 1 {
			status = 0
		}
		return bilitest.OK(map[string]any{"live_status": status, "area_id": 2, "parent_area_id": 1})
	})
	v.b.Handle(fansMedalEnterPath, func(bilitest.Call) bilitest.Reply { return fansMedalHeartbeatReply(20, "secret", 11) })
	r := v.runTask(42, TaskFansMedal)
	if r.Status != api.StatusSkipped || v.b.Count(fansMedalHeartPath) != 0 || !strings.Contains(step(t, r, "观看").Detail, "未开播") {
		t.Fatalf("结果：%+v", r)
	}
}

func TestFansMedalNotLoginStopsImmediately(t *testing.T) {
	v := fansMedalEnv(t, 1, 2)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Danmaku = "你好" })
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("sendDanmu", false, "0/1"))
	v.b.Fail(fansMedalDanmakuPath, -101, "请先登录")
	r := v.runTask(42, TaskFansMedal)
	if v.b.Count(fansMedalDanmakuPath) != 1 || step(t, r, "弹幕").Reason != "not_login" {
		t.Fatalf("结果：%+v", r)
	}
}

func TestFansMedalWatchRunsAtMostFourSessions(t *testing.T) {
	v := fansMedalEnv(t, 1, 2, 3, 4, 5, 6)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Watch = true })
	var mu sync.Mutex
	active, peak, reached := 0, 0, 0
	completed := map[string]bool{}
	release := make(chan struct{})
	v.b.Handle(fansMedalProgressPath, func(c bilitest.Call) bilitest.Reply {
		mu.Lock()
		done := completed[c.Query.Get("target_id")]
		mu.Unlock()
		return bilitest.OK(fansMedalProgressData("watchLive", done, "0/1"))
	})
	v.b.Handle(fansMedalEnterPath, func(bilitest.Call) bilitest.Reply {
		mu.Lock()
		active++
		peak = max(peak, active)
		mu.Unlock()
		return fansMedalHeartbeatReply(60, "secret", 11)
	})
	v.b.Handle(fansMedalHeartPath, func(c bilitest.Call) bilitest.Reply {
		mu.Lock()
		reached++
		if reached == 4 {
			close(release)
		}
		mu.Unlock()
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			t.Error("没有同时开始四个观看会话")
		}
		mu.Lock()
		active--
		completed[c.Form.Get("ruid")] = true
		mu.Unlock()
		return fansMedalHeartbeatReply(60, "secret", 22)
	})
	r := v.runTask(42, TaskFansMedal)
	mu.Lock()
	defer mu.Unlock()
	if peak != 4 || active != 0 || len(completed) != 6 || !strings.Contains(r.Note, "观看 6 个") || r.Status != api.StatusOK {
		t.Fatalf("并发%d，未结束%d，结果：%+v", peak, active, r)
	}
}

func TestFansMedalWatchRespectsMinuteBudgetAndReportsUnconfirmedProgress(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Watch = true; s.Tasks.FansMedal.Minutes = 1 })
	p := fansMedalProgressData("watchLive", false, "0/5")
	v.b.JSON(fansMedalProgressPath, p)
	v.b.Handle(fansMedalEnterPath, func(bilitest.Call) bilitest.Reply { return fansMedalHeartbeatReply(20, "secret", 11) })
	v.b.Handle(fansMedalHeartPath, func(bilitest.Call) bilitest.Reply { return fansMedalHeartbeatReply(20, "secret", 22) })
	r := v.runTask(42, TaskFansMedal)
	if v.b.Count(fansMedalHeartPath) != 3 || !strings.Contains(step(t, r, "观看").Detail, "进度尚未完成") || !strings.Contains(r.Note, "观看 1 个") {
		t.Fatalf("结果：%+v", r)
	}
}

func TestFansMedalLikeUsesPartialBatchAndStopsAtFiveBatches(t *testing.T) {
	for _, quantity := range []int{4, 100} {
		t.Run(strconv.Itoa(quantity), func(t *testing.T) {
			v := fansMedalEnv(t, 1)
			v.set(func(s *api.Settings) { s.Tasks.FansMedal.Like = true })
			v.b.Handle(fansMedalProgressPath, func(bilitest.Call) bilitest.Reply {
				p := fansMedalProgressData("like", quantity == 4 && v.b.Count(fansMedalLikePath) > 0, "0/1")
				p["task_info"].([]any)[0].(map[string]any)["title"] = fmt.Sprintf("点赞%d次", quantity)
				return bilitest.OK(p)
			})
			v.runTask(42, TaskFansMedal)
			calls := v.b.Calls(fansMedalLikePath)
			want := 5
			if quantity == 4 {
				want = 1
			}
			if len(calls) != want {
				t.Fatalf("点赞请求%d次，应为%d", len(calls), want)
			}
			for _, c := range calls {
				if c.Form.Get("click_time") != strconv.Itoa(min(10, quantity)) {
					t.Errorf("点赞批次大小：%v", c.Form)
				}
			}
		})
	}
}

func TestFansMedalUnparseableLitProgressFailsWithoutGuessing(t *testing.T) {
	v := fansMedalEnv(t, 1)
	v.set(func(s *api.Settings) { s.Tasks.FansMedal.Like = true })
	v.b.JSON(fansMedalProgressPath, fansMedalProgressData("like", false, "请完成任务"))
	r := v.runTask(42, TaskFansMedal)
	if v.b.Count(fansMedalLikePath) != 0 || step(t, r, "点赞").Reason != "changed" {
		t.Fatalf("结果：%+v", r)
	}
}
