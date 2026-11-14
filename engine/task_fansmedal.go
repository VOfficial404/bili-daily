package engine

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

func init() {
	register(Task{
		Info:    api.TaskInfo{ID: TaskFansMedal, Title: "粉丝牌", Desc: "给粉丝牌点赞、发弹幕和观看直播，按直播间进度补足每天的互动。", Group: "直播"},
		Enabled: func(s *api.Settings) bool { return s.Tasks.FansMedal.Enabled },
		Run:     fansMedalRun,
	})
}

var fansMedalNumber = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)
var fansMedalFraction = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*/\s*([0-9]+(?:\.[0-9]+)?)`)

// 任务文字给出每轮的数量，副标题给出已完成的轮数。
func fansMedalRemaining(p *bili.LiveMedalProgress, action string) (int, error) {
	if p.Lighted && p.Full {
		return 0, nil
	}
	for _, task := range p.Tasks {
		if task.Type != action {
			continue
		}
		if task.Done {
			return 0, nil
		}
		quantity, err := strconv.ParseFloat(fansMedalNumber.FindString(task.Title), 64)
		if err != nil || quantity <= 0 || quantity > 1440 {
			return 0, &bili.Error{Code: bili.CodeBadResponse, Message: "粉丝牌任务没有有效的互动数量：" + task.Title}
		}
		if action == "watchLive" {
			quantity *= 60
		}
		fraction := fansMedalFraction.FindStringSubmatch(task.Subtitle)
		if fraction == nil {
			if !p.Lighted {
				return int(math.Ceil(quantity)), nil
			}
			return 0, &bili.Error{Code: bili.CodeBadResponse, Message: "粉丝牌任务没有可识别的进度：" + task.Subtitle}
		}
		current, _ := strconv.ParseFloat(fraction[1], 64)
		limit, _ := strconv.ParseFloat(fraction[2], 64)
		if limit <= 0 || limit > 1000 {
			return 0, &bili.Error{Code: bili.CodeBadResponse, Message: "粉丝牌任务的进度上限不对：" + task.Subtitle}
		}
		return int(math.Ceil(max(0, limit-current) * quantity)), nil
	}
	return 0, nil
}

type fansMedalAction struct {
	name    string
	enabled bool
	done    []string
	err     error
	offline int
	capped  int
}

func (a *fansMedalAction) fail(err error) {
	if a.err == nil || bili.IsNotLogin(err) {
		a.err = err
	}
}

func (a *fansMedalAction) report(x *Ctx) {
	if len(a.done) > 0 {
		detail := fmt.Sprintf("%d 个粉丝牌：%s", len(a.done), shortList(a.done, 5))
		if a.name == "观看" {
			detail += "（按成功心跳累计时长，亲密度以平台进度为准）"
		}
		if a.capped > 0 {
			detail += fmt.Sprintf("；%d 个达到本轮上限，任务进度尚未完成", a.capped)
		}
		x.OK(a.name, "%s", detail)
	}
	if a.err != nil {
		x.Fail(a.name, a.err)
		if x.Aborted() {
			return
		}
	}
	if len(a.done) == 0 && a.err == nil {
		switch {
		case !a.enabled:
			x.Skip(a.name, "设置里没有开启")
		case a.offline > 0:
			x.Skip(a.name, "%d 个直播间未开播，其余已经完成或免费亲密度已满", a.offline)
		case a.capped > 0:
			x.Skip(a.name, "剩余观看预算不足一个服务端心跳间隔")
		default:
			x.Skip(a.name, "全部已经完成、没有对应任务或免费亲密度已满")
		}
	}
}

func fansMedalList(x *Ctx) ([]bili.LiveMedal, error) {
	var medals []bili.LiveMedal
	seen := map[int64]bool{}
	for page := 1; page <= 1000; page++ {
		panel, err := x.C.LiveMedalPanel(x, page)
		if err != nil {
			return nil, err
		}
		for _, list := range [][]bili.LiveMedal{panel.Special, panel.List} {
			for _, medal := range list {
				if medal.Medal.TargetID <= 0 || medal.Room.ID <= 0 || seen[medal.Medal.TargetID] {
					continue
				}
				seen[medal.Medal.TargetID] = true
				medals = append(medals, medal)
			}
		}
		if !panel.Page.More && page >= panel.Page.Total {
			return medals, nil
		}
		if !x.Sleep(500 * time.Millisecond) {
			return nil, x.Err()
		}
	}
	return nil, &bili.Error{Code: bili.CodeBadResponse, Message: "粉丝牌超过 1000 页，未能完整读取"}
}

type fansMedalWatchJob struct {
	medal     bili.LiveMedal
	remaining int
}

type fansMedalWatchResult struct {
	medal    bili.LiveMedal
	seconds  int
	capped   bool
	offline  bool
	err      error
	firstErr error
	riskErr  error
}

func fansMedalName(m bili.LiveMedal) string {
	if m.Anchor.Name != "" {
		return m.Anchor.Name
	}
	if m.Medal.Name != "" {
		return m.Medal.Name
	}
	return strconv.FormatInt(m.Medal.TargetID, 10)
}

func fansMedalRun(x *Ctx) {
	if x.C.Cookie("LIVE_BUVID") == "" {
		cookies, err := x.C.LiveBootstrap(x)
		if err != nil {
			x.Fail("准备直播 Cookie", err)
			return
		}
		for name, value := range cookies {
			x.C.SetCookie(name, value)
		}
		if x.C.Cookie("LIVE_BUVID") == "" {
			x.Failf("准备直播 Cookie", bili.ReasonChanged, "直播接口没有返回 LIVE_BUVID")
			return
		}
	}
	medals, err := fansMedalList(x)
	if err != nil {
		x.Fail("读取粉丝牌", err)
		return
	}
	if len(medals) == 0 {
		x.Skip("粉丝牌", "没有粉丝牌")
		return
	}
	opt := x.Set.Tasks.FansMedal
	if opt.Max > 0 && len(medals) > opt.Max {
		medals = medals[:opt.Max]
	}
	like := fansMedalAction{name: "点赞", enabled: opt.Like}
	danmaku := fansMedalAction{name: "弹幕", enabled: opt.Danmaku != ""}
	watch := fansMedalAction{name: "观看", enabled: opt.Watch}
	actions := []*fansMedalAction{&like, &danmaku, &watch}
	defer func() {
		for _, a := range actions {
			a.report(x)
		}
		x.Note("点赞 %d 个、弹幕 %d 个、观看 %d 个粉丝牌", len(like.done), len(danmaku.done), len(watch.done))
	}()
	risks := map[int64]bool{}
	stop := func(mid int64, err error) bool {
		if bili.IsCode(err, -352, -412, -403) {
			risks[mid] = true
		}
		return bili.IsNotLogin(err) || x.Err() != nil || len(risks) >= 3
	}
	var jobs []fansMedalWatchJob
	for _, medal := range medals {
		if x.Err() != nil {
			return
		}
		p, err := x.C.LiveMedalProgress(x, medal.Medal.TargetID)
		if err != nil {
			for _, a := range actions {
				if a.enabled {
					a.fail(err)
				}
			}
			if stop(medal.Medal.TargetID, err) {
				return
			}
			continue
		}
		if like.enabled {
			result := fansMedalLike(x, medal, p)
			p = result.progress
			if result.did {
				like.done = append(like.done, fansMedalName(medal))
			}
			if result.offline {
				like.offline++
			}
			if result.capped {
				like.capped++
			}
			if result.err != nil {
				like.fail(result.err)
				if stop(medal.Medal.TargetID, result.err) {
					return
				}
			}
		}
		if p == nil {
			// 点赞后的查询失败不影响独立的后续动作，再取一次当前进度。
			p, err = x.C.LiveMedalProgress(x, medal.Medal.TargetID)
			if err != nil {
				if danmaku.enabled {
					danmaku.fail(err)
				}
				if watch.enabled {
					watch.fail(err)
				}
				if stop(medal.Medal.TargetID, err) {
					return
				}
				continue
			}
		}
		if danmaku.enabled {
			remaining, err := fansMedalRemaining(p, "sendDanmu")
			if err != nil {
				danmaku.fail(err)
			}
			if remaining > 0 {
				err = x.C.LiveDanmaku(x, medal.Room.ID, opt.Danmaku, x.Now())
				waited := x.Sleep(7 * time.Second)
				if err != nil {
					danmaku.fail(err)
					if stop(medal.Medal.TargetID, err) {
						return
					}
				} else {
					danmaku.done = append(danmaku.done, fansMedalName(medal))
				}
				if !waited {
					return
				}
				p, err = x.C.LiveMedalProgress(x, medal.Medal.TargetID)
				if err != nil {
					if watch.enabled {
						watch.fail(err)
					}
					if stop(medal.Medal.TargetID, err) {
						return
					}
					continue
				}
			}
		}
		if watch.enabled {
			remaining, err := fansMedalRemaining(p, "watchLive")
			if err != nil {
				watch.fail(err)
			}
			if remaining > 0 {
				jobs = append(jobs, fansMedalWatchJob{medal: medal, remaining: remaining})
			}
		}
	}
	if len(jobs) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(x)
	defer cancel()
	queue := make(chan fansMedalWatchJob)
	results := make(chan fansMedalWatchResult, 4)
	var workers sync.WaitGroup
	for range min(4, len(jobs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range queue {
				if ctx.Err() != nil {
					return
				}
				results <- fansMedalWatch(ctx, x, job, opt.Minutes)
			}
		}()
	}
	go func() {
		defer close(queue)
		for _, job := range jobs {
			select {
			case queue <- job:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	stopped := false
	for result := range results {
		if result.seconds > 0 {
			watch.done = append(watch.done, fansMedalName(result.medal))
		}
		if result.capped {
			watch.capped++
		}
		if result.offline {
			watch.offline++
		}
		if result.firstErr != nil {
			watch.fail(result.firstErr)
		}
		if result.riskErr != nil && stop(result.medal.Medal.TargetID, result.riskErr) {
			stopped = true
			cancel()
		}
		if result.err != nil && !(stopped && result.err == context.Canceled) {
			watch.fail(result.err)
			if stop(result.medal.Medal.TargetID, result.err) {
				stopped = true
				cancel()
			}
		}
	}
}

// 工作者只做请求和等待，不写报告，也不读写 Ctx.stop。
func fansMedalWatch(ctx context.Context, x *Ctx, job fansMedalWatchJob, minutes int) (out fansMedalWatchResult) {
	out.medal = job.medal
	room, err := x.C.LiveRoomInfo(ctx, job.medal.Room.ID)
	if err != nil {
		out.err = err
		return
	}
	if room.Status != 1 {
		out.offline = true
		return
	}
	if room.ParentID <= 0 {
		out.err = &bili.Error{Code: bili.CodeBadResponse, Message: "直播间没有父分区，不能开始观看"}
		return
	}
	uuid, err := bili.LiveWatchUUID()
	if err != nil {
		out.err = err
		return
	}
	in := bili.LiveHeartbeatInput{ParentID: room.ParentID, AreaID: room.AreaID, RoomID: job.medal.Room.ID, Buvid: x.C.Cookie("LIVE_BUVID"), UUID: uuid, TS: x.Now().UnixMilli()}
	state, err := x.C.LiveEnter(ctx, job.medal.Medal.TargetID, in)
	if err != nil {
		out.err = err
		return
	}
	target := min(job.remaining, minutes*60)
	failures, sinceRefresh := 0, 0
	for out.seconds < target {
		interval := state.Interval
		// 不发送超过本轮预算的心跳；入场 E 本身不计时。
		if out.seconds+interval > minutes*60 {
			out.capped = true
			break
		}
		if err := x.e.cfg.Sleep(ctx, time.Duration(interval)*time.Second); err != nil {
			out.err = err
			return
		}
		room, err = x.C.LiveRoomInfo(ctx, job.medal.Room.ID)
		if err != nil {
			out.err = err
			return
		}
		if room.Status != 1 {
			out.offline = true
			return
		}
		in.Sequence = 1 + in.Sequence
		in.ETS, in.Interval, in.TS = state.Timestamp, interval, x.Now().UnixMilli()
		next, err := x.C.LiveHeartbeat(ctx, job.medal.Medal.TargetID, in, *state)
		if err != nil {
			in.Sequence--
			failures++
			if out.firstErr == nil {
				out.firstErr = err
			}
			if bili.IsCode(err, -352, -412, -403) {
				out.riskErr = err
			}
			if bili.IsNotLogin(err) || ctx.Err() != nil || failures >= 3 {
				out.err = err
				return
			}
			continue
		}
		state, failures = next, 0
		out.seconds += interval
		sinceRefresh += interval
		if sinceRefresh >= 60 || out.seconds >= target {
			p, err := x.C.LiveMedalProgress(ctx, job.medal.Medal.TargetID)
			if err != nil {
				out.err = err
				return
			}
			remaining, err := fansMedalRemaining(p, "watchLive")
			if err != nil {
				out.err = err
				return
			}
			if remaining == 0 {
				return
			}
			sinceRefresh = 0
			if out.seconds >= target {
				if target < job.remaining {
					out.capped = true
					return
				}
				if err := x.e.cfg.Sleep(ctx, 3*time.Second); err != nil {
					out.err = err
					return
				}
				p, err = x.C.LiveMedalProgress(ctx, job.medal.Medal.TargetID)
				if err != nil {
					out.err = err
					return
				}
				remaining, err = fansMedalRemaining(p, "watchLive")
				if err != nil {
					out.err = err
					return
				}
				if remaining > 0 {
					out.err = &bili.Error{Code: bili.CodeBadResponse, Message: "观看心跳已完成，但粉丝牌进度没有确认完成"}
				}
			}
		}
	}
	return
}

type fansMedalLikeResult struct {
	progress *bili.LiveMedalProgress
	did      bool
	offline  bool
	capped   bool
	err      error
}

func fansMedalLike(x *Ctx, medal bili.LiveMedal, p *bili.LiveMedalProgress) (out fansMedalLikeResult) {
	out.progress = p
	remaining, err := fansMedalRemaining(p, "like")
	if err != nil {
		out.err = err
		return
	}
	if remaining == 0 {
		return
	}
	room, err := x.C.LiveRoomInfo(x, medal.Room.ID)
	if err != nil {
		out.err = err
		return
	}
	if room.Status != 1 {
		out.offline = true
		return
	}
	for batch := 0; batch < 5 && remaining > 0; batch++ {
		if batch > 0 {
			room, err = x.C.LiveRoomInfo(x, medal.Room.ID)
			if err != nil {
				out.err = err
				return
			}
			if room.Status != 1 {
				out.offline = true
				return
			}
		}
		err = x.C.LiveLike(x, medal.Room.ID, medal.Medal.TargetID, min(10, remaining))
		waited := x.Sleep(3 * time.Second)
		if err != nil {
			out.err = err
			return
		}
		out.did = true
		if !waited {
			out.err = x.Err()
			return
		}
		out.progress, err = x.C.LiveMedalProgress(x, medal.Medal.TargetID)
		if err != nil {
			out.err = err
			return
		}
		remaining, err = fansMedalRemaining(out.progress, "like")
		if err != nil {
			out.err = err
			return
		}
	}
	out.capped = remaining > 0
	return
}
