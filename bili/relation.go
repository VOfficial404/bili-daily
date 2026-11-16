package bili

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// 关注分组里固定的两个：特别关注和默认分组
const (
	TagSpecial = -10
	TagDefault = 0
)

// Tag 是一个关注分组
type Tag struct {
	ID    int64  `json:"tagid"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// User 是关注列表里的一个人。Face 已经换成 https 地址，地址不对时为空。
type User struct {
	Mid   int64  `json:"mid"`
	Uname string `json:"uname"`
	Face  string `json:"face"`
}

func (c *Client) spaceHeader(tail string) map[string]string {
	return map[string]string{
		"Origin":  "https://space.bilibili.com",
		"Referer": "https://space.bilibili.com/" + c.Mid() + tail,
	}
}

// Tags 列出账号的关注分组
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	var tags []Tag
	err := c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/x/relation/tags",
		Header: c.spaceHeader("/fans/follow"),
	}, &tags)
	return tags, err
}

// TagMembers 取分组里的一页关注，pn 从 1 开始，每页最多 20 个
func (c *Client) TagMembers(ctx context.Context, tagID int64, pn int) ([]User, error) {
	var users []User
	err := c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/relation/tag",
		Query: url.Values{
			"mid":   {c.Mid()},
			"tagid": {strconv.FormatInt(tagID, 10)},
			"pn":    {strconv.Itoa(pn)},
			"ps":    {"20"},
		},
		Header: c.spaceHeader("/fans/follow"),
	}, &users)
	return users, err
}

// Followings 取一页关注，pn 从 1 开始，每页 20 个。
// byAttention 为 true 时按最常访问排序，否则按关注时间从新到旧。
func (c *Client) Followings(ctx context.Context, pn int, byAttention bool) (list []User, total int, err error) {
	order := ""
	if byAttention {
		order = "attention"
	}
	var d struct {
		List  []User `json:"list"`
		Total int    `json:"total"`
	}
	err = c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/relation/followings",
		Query: url.Values{
			"vmid":       {c.Mid()},
			"order_type": {order},
			"order":      {"desc"},
			"pn":         {strconv.Itoa(pn)},
			"ps":         {"20"},
		},
		Header: c.spaceHeader("/fans/follow"),
	}, &d)
	for i := range d.List {
		d.List[i].Face = httpsFace(d.List[i].Face)
	}
	return d.List, d.Total, err
}

// SearchFollowings 在账号的关注里按昵称搜索一页，pn 从 1 开始，每页 20 个
func (c *Client) SearchFollowings(ctx context.Context, name string, pn int) (list []User, total int, err error) {
	var d struct {
		List  []User `json:"list"`
		Total int    `json:"total"`
	}
	err = c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/relation/followings/search",
		Query: url.Values{
			"vmid": {c.Mid()},
			"name": {name},
			"pn":   {strconv.Itoa(pn)},
			"ps":   {"20"},
		},
		Header: c.spaceHeader("/fans/follow"),
	}, &d)
	for i := range d.List {
		d.List[i].Face = httpsFace(d.List[i].Face)
	}
	return d.List, d.Total, err
}

// CreateTag 新建关注分组，返回它的 ID。同名分组已存在时 B 站返回 22106。
func (c *Client) CreateTag(ctx context.Context, name string) (int64, error) {
	var d struct {
		ID int64 `json:"tagid"`
	}
	err := c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/x/relation/tag/create",
		Form:   url.Values{"tag": {name}},
		Header: c.spaceHeader("/fans/follow"),
	}, &d)
	return d.ID, err
}

// AddToTag 把已经关注的人加进一个分组（原来所在的分组不变）
func (c *Client) AddToTag(ctx context.Context, mids []int64, tagID int64) error {
	fids := make([]string, len(mids))
	for i, m := range mids {
		fids[i] = strconv.FormatInt(m, 10)
	}
	return c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/relation/tags/copyUsers",
		Form: url.Values{
			"fids":   {strings.Join(fids, ",")},
			"tagids": {strconv.FormatInt(tagID, 10)},
		},
		Header: c.spaceHeader("/fans/follow?tagid=-1"),
	}, nil)
}

// Unfollow 取消关注一个人
func (c *Client) Unfollow(ctx context.Context, mid int64) error {
	return c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/relation/modify",
		Form: url.Values{
			"fid":    {strconv.FormatInt(mid, 10)},
			"act":    {"2"},
			"re_src": {"11"},
		},
		Header: c.spaceHeader("/fans/follow"),
	}, nil)
}
