package engine

import (
	"errors"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

func init() {
	register(Task{
		Info: api.TaskInfo{
			ID:    TaskSilver2Coin,
			Title: "银瓜子换硬币",
			Desc:  "每天用直播间的银瓜子换一次硬币。",
			Group: "直播",
		},
		Enabled: func(s *api.Settings) bool { return s.Tasks.Silver2Coin.Enabled },
		Run:     runSilver2Coin,
	})
}

func runSilver2Coin(x *Ctx) {
	w, err := x.C.LiveWallet(x)
	if err != nil {
		x.Fail("查询钱包", err)
		return
	}
	if w.Silver2CoinLeft <= 0 {
		x.Skip("兑换", "今天已经换过")
		return
	}
	coin, silver, err := x.C.Silver2Coin(x)
	var be *bili.Error
	switch {
	case err == nil:
		x.OK("兑换", "换到 %d 个硬币，还剩 %d 个银瓜子", coin, silver)
		x.Note("换到 %d 个硬币", coin)
	case errors.As(err, &be) && strings.Contains(be.Message, "不足"):
		// 银瓜子不够时的 code 没有资料可查，按 B 站返回的说明判断
		x.Skip("兑换", "%s（现在有 %d 个银瓜子）", be.Message, w.Silver)
	default:
		x.Fail("兑换", err)
	}
}
