package bili

import (
	"context"
	"net/url"
	"strconv"
)

// mangaPost 调用漫画的 twirp 接口。参数在地址和表单里各带一份：
// 手头的两份资料各记了一种写法（见 docs/research/api-vip-manga.md 第 4 节）。
// 出错时这些接口返回 HTTP 400，code 是字符串，在 Error.Key 里。
func (c *Client) mangaPost(ctx context.Context, method string, params url.Values, out any) error {
	return c.Do(ctx, Request{
		URL:    "https://manga.bilibili.com/twirp/" + method,
		Query:  params,
		Form:   params,
		NoCSRF: true,
		Header: map[string]string{
			"Origin":  "https://manga.bilibili.com",
			"Referer": "https://manga.bilibili.com/",
		},
	}, out)
}

// MangaClockIn 漫画签到。今天签过时返回的错误 Key 是 invalid_argument，Message 里有 duplicate。
func (c *Client) MangaClockIn(ctx context.Context) error {
	return c.mangaPost(ctx, "activity.v1.Activity/ClockIn", url.Values{"platform": {"android"}}, nil)
}

// MangaAddHistory 记一次阅读：把某部漫画的某一话加进阅读历史
func (c *Client) MangaAddHistory(ctx context.Context, comicID, epID int64) error {
	return c.mangaPost(ctx, "bookshelf.v1.Bookshelf/AddHistory", url.Values{
		"platform": {"android"},
		"comic_id": {strconv.FormatInt(comicID, 10)},
		"ep_id":    {strconv.FormatInt(epID, 10)},
	}, nil)
}

// MangaVipReward 领大会员每月的漫读券，返回领到几张
func (c *Client) MangaVipReward(ctx context.Context) (int, error) {
	var d *struct {
		Amount int `json:"amount"`
	}
	const method = "user.v1.User/GetVipReward"
	// reason_id=1 是漫读券
	if err := c.mangaPost(ctx, method, url.Values{"reason_id": {"1"}}, &d); err != nil {
		return 0, err
	}
	if d == nil {
		return 0, &Error{Code: CodeBadResponse, Message: "没有返回领取结果", Path: "manga.bilibili.com/twirp/" + method}
	}
	return d.Amount, nil
}
