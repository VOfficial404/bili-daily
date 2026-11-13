package bili

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Nav 是 x/web-interface/nav 返回的账号资料
type Nav struct {
	IsLogin    bool    `json:"isLogin"`
	Mid        int64   `json:"mid"`
	Uname      string  `json:"uname"`
	Face       string  `json:"face"`
	Money      float64 `json:"money"` // 硬币
	VipStatus  int     `json:"vipStatus"`
	VipType    int     `json:"vipType"`
	VipDueDate int64   `json:"vipDueDate"` // 毫秒
	LevelInfo  struct {
		Level   int             `json:"current_level"`
		Exp     int             `json:"current_exp"`
		NextExp json.RawMessage `json:"next_exp"` // 6 级时是字符串 "--"
	} `json:"level_info"`
	Wallet struct {
		BCoin  float64 `json:"bcoin_balance"`
		Coupon float64 `json:"coupon_balance"` // B 币券
	} `json:"wallet"`
}

// VIP 类型，只在大会员有效时才算数
const (
	VIPNone    = 0
	VIPMonthly = 1
	VIPAnnual  = 2
)

// VIP 返回当前有效的大会员类型
func (n *Nav) VIP() int {
	if n.VipStatus != 1 {
		return VIPNone
	}
	return n.VipType
}

// VIPName 是大会员类型的名字，不是大会员时为空
func (n *Nav) VIPName() string {
	switch n.VIP() {
	case VIPMonthly:
		return "月度大会员"
	case VIPAnnual:
		return "年度大会员"
	}
	return ""
}

// VIPDue 是大会员到期时间，不是大会员时为零值
func (n *Nav) VIPDue() time.Time {
	if n.VIP() == VIPNone || n.VipDueDate <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(n.VipDueDate)
}

// NextExp 是升到下一级需要的总经验，6 级时为 0
func (n *Nav) NextExp() int {
	v, _ := strconv.Atoi(strings.Trim(string(n.LevelInfo.NextExp), `"`))
	return v
}

// Nav 确认登录状态并取账号资料。登录失效时返回的错误 IsNotLogin 为 true。
func (c *Client) Nav(ctx context.Context) (*Nav, error) {
	var n Nav
	if err := c.Get(ctx, "https://api.bilibili.com/x/web-interface/nav", nil, &n); err != nil {
		return nil, err
	}
	if !n.IsLogin {
		return nil, &Error{Code: -101, Message: "账号未登录", Path: "api.bilibili.com/x/web-interface/nav"}
	}
	return &n, nil
}

// Zone 是 B 站看到的出口 IP 和它的归属
type Zone struct {
	Addr     string `json:"addr"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

// Place 是给人看的地区，例如「广东 深圳」
func (z *Zone) Place() string {
	parts := []string{z.Country, z.Province}
	if z.Country == "中国" {
		parts = []string{z.Province, z.City}
	}
	var out []string
	for _, p := range parts {
		if p != "" && (len(out) == 0 || out[len(out)-1] != p) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return "未知地区"
	}
	return strings.Join(out, " ")
}

// Zone 查询 B 站把出口 IP 识别为哪里，不需要登录
func (c *Client) Zone(ctx context.Context) (*Zone, error) {
	var z Zone
	if err := c.Get(ctx, "https://api.bilibili.com/x/web-interface/zone", nil, &z); err != nil {
		return nil, err
	}
	if z.Addr == "" {
		return nil, &Error{Code: CodeBadResponse, Message: "没有返回出口 IP", Path: "api.bilibili.com/x/web-interface/zone"}
	}
	return &z, nil
}
