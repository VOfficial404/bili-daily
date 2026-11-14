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
			ID:    TaskManga,
			Title: "漫画",
			Desc:  "每天在哔哩哔哩漫画签到，再记一次阅读。",
			Group: "漫画",
		},
		Enabled: func(s *api.Settings) bool { return s.Tasks.Manga.Enabled },
		Run:     runManga,
	})
	register(Task{
		Info: api.TaskInfo{
			ID:    TaskMangaPrivilege,
			Title: "大会员漫画权益",
			Desc:  "领大会员每月送的漫读券。",
			Group: "漫画",
			Needs: "大会员",
		},
		Enabled: func(s *api.Settings) bool { return s.Tasks.MangaPrivilege.Enabled },
		Run:     runMangaPrivilege,
	})
}

func runManga(x *Ctx) {
	var done []string
	err := x.C.MangaClockIn(x)
	var be *bili.Error
	switch {
	case err == nil:
		x.OK("签到", "签到成功")
		done = append(done, "签到成功")
	case errors.As(err, &be) && be.Key == "invalid_argument" && strings.Contains(be.Message, "duplicate"):
		x.Skip("签到", "今天已签到")
	default:
		x.Fail("签到", err)
		if x.Aborted() {
			return
		}
	}

	if opt := x.Set.Tasks.Manga; opt.Read {
		if err := x.C.MangaAddHistory(x, opt.ComicID, opt.EpID); err != nil {
			x.Fail("阅读", err)
		} else {
			// 这个接口只是把这一话加进阅读历史；B 站会不会因此发阅读奖励，没有查到依据
			x.OK("阅读", "记了一次阅读（漫画 %d 第 %d 话），不知道有没有阅读奖励", opt.ComicID, opt.EpID)
			done = append(done, "记了一次阅读")
		}
	}
	if len(done) > 0 {
		x.Note("%s", strings.Join(done, "，"))
	}
}

// mangaPrivilegeMark 记着哪个月已经领过漫读券：B 站没有接口能查，领过之后再领会报错
const mangaPrivilegeMark = "manga_privilege.month"

func runMangaPrivilege(x *Ctx) {
	if x.Nav.VIP() == bili.VIPNone {
		x.Skip("领漫读券", "不是大会员")
		return
	}
	month := x.Day[:7]
	if x.Mark(mangaPrivilegeMark) == month {
		x.Skip("领漫读券", "这个月已经领过")
		return
	}
	amount, err := x.C.MangaVipReward(x)
	var be *bili.Error
	switch {
	case err == nil:
		x.OK("领漫读券", "领到 %d 张漫读券", amount)
		x.SetMark(mangaPrivilegeMark, month)
		x.Note("领到 %d 张漫读券", amount)
	case errors.As(err, &be) && strings.Contains(be.Message, "已") && strings.Contains(be.Message, "领"):
		// 「已领取」的 code 没有资料可查，按 B 站返回的说明判断
		x.Skip("领漫读券", "这个月已经领过（%s）", be.Message)
		x.SetMark(mangaPrivilegeMark, month)
	default:
		x.Fail("领漫读券", err)
	}
}
