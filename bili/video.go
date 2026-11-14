package bili

import (
	"context"
	"net/url"
	"strconv"
)

// 网页端写操作（心跳、分享、投币）都带的来源
var videoOrigin = map[string]string{"Origin": "https://www.bilibili.com"}

// DailyReward 是今天已经拿到的每日经验奖励，每项 5 经验
type DailyReward struct {
	Login bool `json:"login"`
	Watch bool `json:"watch"`
	Share bool `json:"share"`
}

// DailyReward 查询今天的登录、观看、分享奖励是否已经拿到
func (c *Client) DailyReward(ctx context.Context) (*DailyReward, error) {
	var d *DailyReward
	err := c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/member/web/exp/reward",
		Header: map[string]string{
			"Origin":  "https://account.bilibili.com",
			"Referer": "https://account.bilibili.com/account/home",
		},
	}, &d)
	if err == nil && d == nil {
		err = &Error{Code: CodeBadResponse, Message: "没有返回每日奖励状态", Path: "api.bilibili.com/x/member/web/exp/reward"}
	}
	return d, err
}

// DailyCoinExp 是今天投币已经拿到的经验：每个币 10，最多 50
func (c *Client) DailyCoinExp(ctx context.Context) (int, error) {
	var exp *int
	err := c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/x/web-interface/coin/today/exp",
		Header: videoOrigin,
	}, &exp)
	if err != nil {
		return 0, err
	}
	if exp == nil || *exp < 0 {
		return 0, &Error{Code: CodeBadResponse, Message: "没有返回有效的投币经验", Path: "api.bilibili.com/x/web-interface/coin/today/exp"}
	}
	return *exp, nil
}

// Video 是一个视频。投稿列表只保留 Aid、Bvid 和 Title，其余资料要用 VideoView 补。
type Video struct {
	Aid       int64  `json:"aid"`
	Bvid      string `json:"bvid"`
	CID       int64  `json:"cid"`
	Title     string `json:"title"`
	Copyright int    `json:"copyright"` // 1 原创，2 转载
	Duration  int    `json:"duration"`  // 秒
	Owner     struct {
		Mid int64 `json:"mid"`
	} `json:"owner"`
}

// VideoUploads 是 UP 主投稿列表的一页
type VideoUploads struct {
	Page struct {
		Count int `json:"count"` // 投稿总数
	} `json:"page"`
	List struct {
		Videos []Video `json:"vlist"`
	} `json:"list"`
}

// VideoUploads 取 UP 主投稿列表的一页，从新到旧；pn 从 1 开始，ps 是每页几个
func (c *Client) VideoUploads(ctx context.Context, mid int64, pn, ps int) (VideoUploads, error) {
	var d struct {
		Page struct {
			Count int `json:"count"`
		} `json:"page"`
		List struct {
			Videos []struct {
				Aid   int64  `json:"aid"`
				Bvid  string `json:"bvid"`
				Title string `json:"title"`
			} `json:"vlist"`
		} `json:"list"`
	}
	err := c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/space/wbi/arc/search",
		Query: url.Values{
			"mid":           {strconv.FormatInt(mid, 10)},
			"pn":            {strconv.Itoa(pn)},
			"ps":            {strconv.Itoa(ps)},
			"tid":           {"0"},
			"keyword":       {""},
			"order":         {"pubdate"},
			"platform":      {"web"},
			"web_location":  {"1550101"},
			"order_avoided": {"true"},
		},
		Header: map[string]string{"Origin": "https://space.bilibili.com"},
		WBI:    true,
	}, &d)
	if err != nil {
		return VideoUploads{}, err
	}
	var uploads VideoUploads
	uploads.Page.Count = d.Page.Count
	uploads.List.Videos = make([]Video, len(d.List.Videos))
	for i, v := range d.List.Videos {
		uploads.List.Videos[i] = Video{Aid: v.Aid, Bvid: v.Bvid, Title: v.Title}
	}
	return uploads, nil
}

// VideoRanking 是全站排行榜，条目里带着 CID、Copyright 和 Owner
func (c *Client) VideoRanking(ctx context.Context) ([]Video, error) {
	var d struct {
		List []Video `json:"list"`
	}
	err := c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/x/web-interface/ranking/v2",
		Query:  url.Values{"rid": {"0"}, "type": {"all"}},
		Header: map[string]string{"Origin": "https://www.bilibili.com", "Dnt": "1"},
	}, &d)
	return d.List, err
}

// VideoView 取一个视频的资料
func (c *Client) VideoView(ctx context.Context, aid int64) (*Video, error) {
	var d *Video
	err := c.GetWBI(ctx, "https://api.bilibili.com/x/web-interface/wbi/view", url.Values{"aid": {strconv.FormatInt(aid, 10)}}, &d)
	if err == nil && (d == nil || d.Aid != aid || d.Owner.Mid <= 0 || d.Copyright <= 0) {
		err = &Error{Code: CodeBadResponse, Message: "没有返回完整的视频资料", Path: "api.bilibili.com/x/web-interface/wbi/view"}
	}
	return d, err
}

// VideoHeartbeat 上报观看进度。start 是开始观看的时间（秒），played 是已经看了多少秒。
func (c *Client) VideoHeartbeat(ctx context.Context, v Video, start int64, played int) error {
	aid, progress := strconv.FormatInt(v.Aid, 10), strconv.Itoa(played)
	form := url.Values{
		"aid":              {aid},
		"bvid":             {v.Bvid},
		"mid":              {c.Mid()},
		"played_time":      {progress},
		"realtime":         {progress},
		"real_played_time": {progress},
		"start_ts":         {strconv.FormatInt(start, 10)},
		"type":             {"3"},
		"dt":               {"2"},
		"play_type":        {"3"},
	}
	if v.CID > 0 {
		form.Set("cid", strconv.FormatInt(v.CID, 10))
	}
	return c.Do(ctx, Request{
		URL:   "https://api.bilibili.com/x/click-interface/web/heartbeat",
		Query: url.Values{"aid": {aid}, "played_time": {progress}},
		Form:  form,
		Header: map[string]string{
			"Origin":       "https://www.bilibili.com",
			"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
		},
	}, nil)
}

// VideoShare 分享一个视频。今天已经分享过时 B 站返回 71000。
func (c *Client) VideoShare(ctx context.Context, aid int64) error {
	return c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/web-interface/share/add",
		Form: url.Values{
			"aid":    {strconv.FormatInt(aid, 10)},
			"source": {"web_normal"},
			"ga":     {"1"},
		},
		Header: videoOrigin,
	}, nil)
}

// VideoCoins 是账号已经给这个视频投了几个币
func (c *Client) VideoCoins(ctx context.Context, aid int64) (int, error) {
	var d *struct {
		Multiply int `json:"multiply"`
	}
	err := c.Get(ctx, "https://api.bilibili.com/x/web-interface/archive/coins", url.Values{"aid": {strconv.FormatInt(aid, 10)}}, &d)
	if err != nil {
		return 0, err
	}
	if d == nil || d.Multiply < 0 {
		return 0, &Error{Code: CodeBadResponse, Message: "没有返回视频的已投币数", Path: "api.bilibili.com/x/web-interface/archive/coins"}
	}
	return d.Multiply, nil
}

// VideoAddCoin 给视频投 1 个币，like 为 true 时顺便点赞。
// 34005 这个视频已经投满，34002 是自己的视频，34004 投得太快，-104 硬币不够。
func (c *Client) VideoAddCoin(ctx context.Context, v Video, like bool) error {
	selectLike := "0"
	if like {
		selectLike = "1"
	}
	id := v.Bvid
	if id == "" {
		id = "av" + strconv.FormatInt(v.Aid, 10)
	}
	return c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/web-interface/coin/add",
		Form: url.Values{
			"aid":          {strconv.FormatInt(v.Aid, 10)},
			"multiply":     {"1"},
			"select_like":  {selectLike},
			"cross_domain": {"true"},
			"source":       {"web_normal"},
			"ga":           {"1"},
		},
		Header: map[string]string{
			"Origin":  "https://www.bilibili.com",
			"Referer": "https://www.bilibili.com/video/" + id + "/",
		},
	}, nil)
}
