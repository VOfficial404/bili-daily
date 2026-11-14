package engine

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

func init() {
	register(Task{
		Info:    api.TaskInfo{ID: TaskDaily, Title: "每日任务", Desc: "观看视频、分享和投币，按今天的完成情况补齐经验奖励。", Group: "每日"},
		Enabled: func(s *api.Settings) bool { return s.Tasks.Daily.Enabled },
		Run:     dailyRun,
	})
}

func dailyRun(x *Ctx) {
	opt := x.Set.Tasks.Daily
	reward, rewardErr := x.C.DailyReward(x)
	if rewardErr != nil {
		x.Fail("查询每日奖励", rewardErr)
		if x.Aborted() {
			return
		}
	}
	exp, expErr := x.C.DailyCoinExp(x)
	if expErr != nil {
		x.Fail("查询投币经验", expErr)
		if x.Aborted() {
			return
		}
	}
	if reward == nil {
		reward = &bili.DailyReward{}
	}
	// 说明里只算今天确实拿到的：之前已经完成的，加上这一轮做成的
	given := 0
	defer func() {
		var parts []string
		total := 0
		if reward.Login {
			total += 5
		}
		if reward.Watch {
			total += 5
			parts = append(parts, "观看完成")
		}
		if reward.Share {
			total += 5
			parts = append(parts, "分享完成")
		}
		total += min(50, exp+given*10)
		parts = append(parts, fmt.Sprintf("今天投币 %d 个", exp/10+given), fmt.Sprintf("预计经验 +%d", total))
		if rewardErr != nil || expErr != nil {
			parts = append(parts, "只统计已知奖励")
		}
		x.Note("%s", strings.Join(parts, "，"))
	}()

	// 观看和分享用同一个视频；两样都不用做时不去选
	picker := dailyPicker{x: x, uploads: map[int64]*dailyUploads{}}
	var target bili.Video
	if rewardErr == nil && ((opt.Watch && !reward.Watch) || (opt.Share && !reward.Share)) {
		var err error
		target, err = picker.watch()
		if err != nil {
			x.Fail("选择视频", err)
			if x.Aborted() {
				return
			}
		}
		// 看过的视频投币时还可以再选
		for _, uploads := range picker.uploads {
			clear(uploads.used)
		}
	}

	switch {
	case !opt.Watch:
		x.Skip("观看视频", "设置里没有开启观看")
	case rewardErr != nil:
		x.Skip("观看视频", "没有查到今天的观看状态")
	case reward.Watch:
		x.Skip("观看视频", "今天已经看过")
	case target.Aid <= 0:
		x.Skip("观看视频", "没有选到视频")
	default:
		start := x.Now().Unix()
		err := x.C.VideoHeartbeat(x, target, start, 0)
		if err == nil {
			if !x.Sleep(15 * time.Second) {
				x.Fail("观看视频", context.Canceled)
				return
			}
			err = x.C.VideoHeartbeat(x, target, start, 15)
		}
		if err != nil {
			x.Fail("观看视频", err)
			if x.Aborted() {
				return
			}
		} else {
			reward.Watch = true
			x.OK("观看视频", "已提交两次观看心跳：%s；经验是否到账以 B 站为准", dailyTitle(target))
		}
	}

	switch {
	case !opt.Share:
		x.Skip("分享视频", "设置里没有开启分享")
	case rewardErr != nil:
		x.Skip("分享视频", "没有查到今天的分享状态")
	case reward.Share:
		x.Skip("分享视频", "今天已经分享过")
	case target.Aid <= 0:
		x.Skip("分享视频", "没有选到视频")
	default:
		err := x.C.VideoShare(x, target.Aid)
		switch {
		case bili.IsCode(err, 71000):
			reward.Share = true
			x.Skip("分享视频", "今天已经分享过")
		case err != nil:
			x.Fail("分享视频", err)
			if x.Aborted() {
				return
			}
		default:
			reward.Share = true
			x.OK("分享视频", "%s", dailyTitle(target))
		}
	}

	switch {
	case opt.Coins <= 0:
		x.Skip("投币", "设置里没有开启投币")
	case opt.StopAtLevel > 0 && x.Nav.LevelInfo.Level >= opt.StopAtLevel:
		x.Skip("投币", "账号已到 %d 级，设置为 %d 级起停止投币", x.Nav.LevelInfo.Level, opt.StopAtLevel)
	case expErr != nil:
		x.Skip("投币", "没有查到今天的投币数，不重复花币")
	case exp/10 >= opt.Coins:
		x.Skip("投币", "今天已经投了 %d 个", exp/10)
	default:
		spendable := int(math.Floor(x.Nav.Money)) - opt.KeepCoins
		if spendable <= 0 {
			x.Skip("投币", "余额 %.1f 个，设置保留 %d 个，没有可用硬币", x.Nav.Money, opt.KeepCoins)
			return
		}
		need := opt.Coins - exp/10
		given = dailyCoins(x, &picker, min(need, spendable), need)
	}
}

// dailyCoins 最多投 need 个币，每个视频一次投 1 个，返回投出去几个。
// wanted 是按设置今天还差几个：余额不够时 need 比它小，报告里说明。
func dailyCoins(x *Ctx, picker *dailyPicker, need, wanted int) int {
	if !prepareCoinDevice(x) {
		return 0
	}
	seen := map[int64]bool{}
	var titles []string
	var lastErr error
	insufficient := false
	for range need*4 + 6 {
		if len(titles) >= need || x.Aborted() {
			break
		}
		v, ok, err := picker.next()
		if err != nil {
			lastErr = err
			break
		}
		if !ok {
			break
		}
		if v.Aid <= 0 || seen[v.Aid] {
			continue
		}
		seen[v.Aid] = true
		if v.Owner.Mid == x.Mid {
			continue
		}
		if v.Copyright <= 0 || v.Owner.Mid <= 0 {
			detail, err := x.C.VideoView(x, v.Aid)
			if err != nil {
				if bili.IsCode(err, 10003, -400) {
					continue
				}
				lastErr = err
				break
			}
			v = *detail
		}
		if v.Owner.Mid == x.Mid {
			continue
		}
		coins, err := x.C.VideoCoins(x, v.Aid)
		if err == nil {
			limit := 1
			if v.Copyright == 1 {
				limit = 2
			}
			if coins >= limit {
				continue
			}
			err = x.C.VideoAddCoin(x, v, x.Set.Tasks.Daily.Like)
		}
		switch {
		case err == nil:
			titles = append(titles, dailyTitle(v))
		case bili.IsCode(err, 34005, 34002, 34003, 10003, -400):
			continue
		case bili.IsCode(err, 34004):
			if !x.Sleep(3 * time.Second) {
				lastErr = context.Canceled
			}
		case bili.IsCode(err, -104):
			insufficient = true
		default:
			lastErr = err
		}
		if lastErr != nil || insufficient {
			break
		}
	}
	n := len(titles)
	switch {
	case lastErr != nil:
		x.Fail("投币", fmt.Errorf("%w；本轮已投 %d 个", lastErr, n))
		if x.Aborted() {
			return n
		}
	case insufficient && n == 0:
		x.Failf("投币", bili.ReasonInsufficient, "B 站提示硬币不足，本轮没有投币")
	case n == 0:
		x.Skip("投币", "没有找到可以投币的视频，原计划投 %d 个", wanted)
	default:
		detail := fmt.Sprintf("%d 个：%s", n, dailyShortList(titles))
		if n < wanted {
			detail += fmt.Sprintf("；原计划 %d 个，只投了 %d 个", wanted, n)
		}
		if insufficient {
			detail += "；B 站提示硬币不足，已停止"
		}
		x.OK("投币", "%s", detail)
	}
	return n
}

// prepareCoinDevice 补齐扫码时可能没取到的设备 Cookie；已有标识保持不变。
// 投币接口要求有效的 buvid3，但存在这个值并不等于 B 站一定接受当前会话。
func prepareCoinDevice(x *Ctx) bool {
	if x.C.Cookie("buvid3") != "" {
		return true
	}
	fp, err := x.C.Fingerprint(x)
	if err != nil {
		x.Fail("准备投币设备 Cookie", err)
		return false
	}
	if fp["buvid3"] == "" {
		x.Failf("准备投币设备 Cookie", bili.ReasonChanged, "设备接口没有返回 buvid3，未发出投币请求；请稍后重新扫码")
		return false
	}
	sess, err := x.e.loadSession(x.acc.mid)
	if err != nil {
		x.Failf("准备投币设备 Cookie", bili.ReasonOther, "读取会话失败，未发出投币请求：%v", err)
		return false
	}
	for k, v := range fp {
		if sess.Cookies[k] == "" {
			sess.Cookies[k] = v
		}
	}
	if _, err := x.e.saveSession(sess); err != nil {
		x.Failf("准备投币设备 Cookie", bili.ReasonOther, "设备 Cookie 保存失败，未发出投币请求：%v", err)
		return false
	}
	for k, v := range fp {
		if x.C.Cookie(k) == "" {
			x.C.SetCookie(k, v)
		}
	}
	x.OK("准备投币设备 Cookie", "已补齐并保存缺失的设备 Cookie")
	return true
}

// dailyTitle 是报告里用的视频名字
func dailyTitle(v bili.Video) string {
	if v.Title != "" {
		return v.Title
	}
	if v.Bvid != "" {
		return v.Bvid
	}
	return "视频 av" + strconv.FormatInt(v.Aid, 10)
}

// dailyShortList 把标题用顿号连起来，最多写 5 个
func dailyShortList(titles []string) string {
	const limit = 5
	if len(titles) <= limit {
		return strings.Join(titles, "、")
	}
	return strings.Join(titles[:limit], "、") + "等"
}

// dailyUploads 是一个 UP 主的投稿数，以及这一轮已经取过的第几条
type dailyUploads struct {
	count int
	used  map[int]bool
}

// dailyPicker 按「指定的 UP 主 → 关注 → 排行榜」的顺序给出视频，查过的结果在一轮里只查一次
type dailyPicker struct {
	x           *Ctx
	uploads     map[int64]*dailyUploads
	followed    bool
	followings  []bili.User
	coinStarted bool
	creators    []int64
	ranked      bool
	ranking     []bili.Video
	rankErr     error
}

// following 是按最常访问排序的第一页关注；查不到时为空，改用排行榜
func (p *dailyPicker) following() []bili.User {
	if !p.followed {
		p.followed = true
		p.followings, _, _ = p.x.C.Followings(p.x, 1, true)
	}
	return p.followings
}

// upload 随机取 UP 主的一条还没取过的投稿；没有投稿或查询失败时返回 false
func (p *dailyPicker) upload(mid int64) (bili.Video, bool) {
	if mid <= 0 {
		return bili.Video{}, false
	}
	u := p.uploads[mid]
	if u == nil {
		u = &dailyUploads{used: map[int]bool{}}
		p.uploads[mid] = u
		d, err := p.x.C.VideoUploads(p.x, mid, 1, 30)
		if err != nil {
			return bili.Video{}, false
		}
		u.count = d.Page.Count
	}
	if u.count <= len(u.used) {
		return bili.Video{}, false
	}
	pn := rand.IntN(u.count) + 1
	for u.used[pn] {
		pn = pn%u.count + 1
	}
	u.used[pn] = true
	d, err := p.x.C.VideoUploads(p.x, mid, pn, 1)
	if err != nil || len(d.List.Videos) == 0 {
		u.count = 0
		return bili.Video{}, false
	}
	v := d.List.Videos[0]
	v.Owner.Mid = mid
	return v, v.Aid > 0
}

// ranks 是打乱过的全站排行榜，一轮里只取一次；取不到或者是空的时返回错误
func (p *dailyPicker) ranks() ([]bili.Video, error) {
	if !p.ranked {
		p.ranked = true
		p.ranking, p.rankErr = p.x.C.VideoRanking(p.x)
		if p.rankErr == nil && len(p.ranking) == 0 {
			p.rankErr = &bili.Error{Code: bili.CodeBadResponse, Message: "排行榜没有返回视频", Path: "api.bilibili.com/x/web-interface/ranking/v2"}
		}
		rand.Shuffle(len(p.ranking), func(i, j int) { p.ranking[i], p.ranking[j] = p.ranking[j], p.ranking[i] })
	}
	return p.ranking, p.rankErr
}

// watch 选一个用来观看和分享的视频：每个来源只试一个 UP 主，不行就换下一个来源
func (p *dailyPicker) watch() (bili.Video, error) {
	ids := p.x.Set.Tasks.Daily.UpIDs
	if len(ids) > 0 {
		if v, ok := p.upload(ids[rand.IntN(len(ids))]); ok {
			return v, nil
		}
	}
	users := p.following()
	if len(users) > 0 {
		if v, ok := p.upload(users[rand.IntN(len(users))].Mid); ok {
			return v, nil
		}
	}
	ranks, err := p.ranks()
	if err != nil {
		return bili.Video{}, err
	}
	for _, v := range ranks {
		if v.Aid > 0 {
			return v, nil
		}
	}
	return bili.Video{}, &bili.Error{Code: bili.CodeBadResponse, Message: "排行榜没有有效的视频", Path: "api.bilibili.com/x/web-interface/ranking/v2"}
}

// next 给出下一个可以考虑投币的视频：先是指定的 UP 主，再是关注，他们的投稿都取完之后是排行榜。
// 排行榜也用完时返回 false。
func (p *dailyPicker) next() (bili.Video, bool, error) {
	if !p.coinStarted {
		p.coinStarted = true
		ids := p.x.Set.Tasks.Daily.UpIDs
		for _, i := range rand.Perm(len(ids)) {
			p.creators = append(p.creators, ids[i])
		}
	}
	for {
		for len(p.creators) > 0 {
			mid := p.creators[0]
			p.creators = p.creators[1:]
			if mid == p.x.Mid {
				continue
			}
			if v, ok := p.upload(mid); ok {
				// 放回队尾轮着来，不把币都投给同一个人
				p.creators = append(p.creators, mid)
				return v, true, nil
			}
		}
		// 关注只加进来一次，观看时查过的直接用
		if p.followings != nil || !p.followed {
			users := p.following()
			for _, i := range rand.Perm(len(users)) {
				p.creators = append(p.creators, users[i].Mid)
			}
			p.followings = nil
			continue
		}
		break
	}
	ranks, err := p.ranks()
	if err != nil {
		return bili.Video{}, false, err
	}
	if len(ranks) == 0 {
		return bili.Video{}, false, nil
	}
	v := ranks[0]
	p.ranking = ranks[1:]
	return v, true, nil
}
