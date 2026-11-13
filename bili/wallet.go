package bili

import (
	"context"
	"math/rand/v2"
	"net/url"
)

const liveWalletBase = "https://api.live.bilibili.com/xlive/revenue/v1/wallet/"

var liveWalletHeader = map[string]string{
	"Origin":  "https://link.bilibili.com",
	"Referer": "https://link.bilibili.com/",
}

// LiveWallet 是直播钱包的状态
type LiveWallet struct {
	Silver          int64 `json:"silver"`             // 银瓜子
	Silver2CoinLeft int   `json:"silver_2_coin_left"` // 今天还能用银瓜子换几次硬币
}

// LiveWallet 查询直播钱包
func (c *Client) LiveWallet(ctx context.Context) (*LiveWallet, error) {
	var w *LiveWallet
	if err := c.Do(ctx, Request{URL: liveWalletBase + "getStatus", Header: liveWalletHeader}, &w); err != nil {
		return nil, err
	}
	if w == nil {
		return nil, &Error{Code: CodeBadResponse, Message: "没有返回钱包状态", Path: "api.live.bilibili.com/xlive/revenue/v1/wallet/getStatus"}
	}
	return w, nil
}

// Silver2Coin 用银瓜子换一次硬币，返回换到的硬币数和剩下的银瓜子
func (c *Client) Silver2Coin(ctx context.Context) (coin int, silver int64, err error) {
	var d *struct {
		Coin   int   `json:"coin"`
		Silver int64 `json:"silver"`
	}
	err = c.Do(ctx, Request{
		URL:    liveWalletBase + "silver2coin",
		Form:   url.Values{"csrf_token": {c.CSRF()}, "visit_id": {liveVisitID()}},
		Header: liveWalletHeader,
	}, &d)
	if err != nil {
		return 0, 0, err
	}
	if d == nil {
		return 0, 0, &Error{Code: CodeBadResponse, Message: "没有返回兑换结果", Path: "api.live.bilibili.com/xlive/revenue/v1/wallet/silver2coin"}
	}
	return d.Coin, d.Silver, nil
}

// liveVisitID 是直播接口要的 visit_id：一位 1~9 的数字，十个小写字母，最后是 0
func liveVisitID() string {
	b := make([]byte, 12)
	b[0], b[11] = byte('1'+rand.IntN(9)), '0'
	for i := 1; i <= 10; i++ {
		b[i] = byte('a' + rand.IntN(26))
	}
	return string(b)
}
