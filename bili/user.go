package bili

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// UPProfile 是指定 UID 的公开资料，不含登录信息。
type UPProfile struct {
	Mid   int64  `json:"mid"`
	Uname string `json:"name"`
	Face  string `json:"face"`
}

// UPProfile 查询用户空间资料。协议见：
// https://goooler.github.io/bilibili-API-collect/docs/user/info.html
// 此接口需要 WBI 签名；使用已登录客户端可满足接口的 Cookie 鉴权。
func (c *Client) UPProfile(ctx context.Context, mid int64) (*UPProfile, error) {
	const path = "api.bilibili.com/x/space/wbi/acc/info"
	if mid <= 0 {
		return nil, &Error{Code: -400, Message: "UP 主 UID 必须是正整数", Path: path}
	}
	var p UPProfile
	err := c.Do(ctx, Request{
		URL:    "https://" + path,
		Query:  url.Values{"mid": {strconv.FormatInt(mid, 10)}},
		Header: map[string]string{"Origin": "https://space.bilibili.com"},
		WBI:    true,
	}, &p)
	if err != nil {
		return nil, err
	}
	p.Uname = strings.TrimSpace(p.Uname)
	if p.Mid != mid || p.Uname == "" {
		return nil, &Error{Code: CodeBadResponse, Message: "UP 主资料不完整或 UID 不匹配，接口可能已变更", Path: path}
	}
	face := strings.TrimSpace(p.Face)
	if strings.HasPrefix(face, "//") {
		face = "https:" + face
	}
	u, err := url.Parse(face)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, &Error{Code: CodeBadResponse, Message: "UP 主头像地址无效，接口可能已变更", Path: path}
	}
	u.Scheme = "https"
	p.Face = u.String()
	return &p, nil
}
