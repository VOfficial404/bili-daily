package engine

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

func init() {
	register(Task{
		Info:    api.TaskInfo{ID: TaskLiveLottery, Title: "天选时刻", Desc: "寻找免费天选时刻抽奖，按奖品关键词筛选，可把新关注的主播放进分组。", Group: "直播"},
		Enabled: func(s *api.Settings) bool { return s.Tasks.LiveLottery.Enabled },
		Run:     lotteryRun,
	})
}

func lotteryMatches(name string, words []string) bool {
	return slices.ContainsFunc(words, func(word string) bool { return strings.Contains(name, word) })
}

func lotteryRun(x *Ctx) {
	opt := x.Set.Tasks.LiveLottery
	var newest int64
	groupReady := opt.Group
	if opt.Group {
		following, _, err := x.C.Followings(x, 1, false)
		if err != nil {
			x.Fail("分组", err)
			if x.Aborted() {
				return
			}
			groupReady = false
		} else if len(following) > 0 {
			newest = following[0].Mid
		}
	}
	areas, err := x.C.LiveAreas(x)
	if err != nil {
		x.Fail("扫描", err)
		x.Note("参加了 0 个抽奖")
		return
	}
	var rooms []bili.LiveLotteryRoom
	seenRooms := map[int64]bool{}
	var scanErr error
scan:
	for _, area := range areas {
		for page := 1; page <= 3; page++ {
			list, err := x.C.LiveLotteryRooms(x, area.ID, page)
			if err != nil {
				if scanErr == nil || bili.IsNotLogin(err) {
					scanErr = err
				}
				if bili.IsNotLogin(err) || x.Err() != nil {
					break scan
				}
				break
			}
			for _, room := range list {
				if room.ID <= 0 || seenRooms[room.ID] || !room.HasLotteryPendant() {
					continue
				}
				seenRooms[room.ID] = true
				rooms = append(rooms, room)
				if len(rooms) >= 150 {
					break scan
				}
			}
			if len(list) < 99 {
				break
			}
		}
	}
	if len(rooms) > 0 {
		x.OK("扫描", "%d 个直播间在抽奖", len(rooms))
	}
	if scanErr != nil {
		x.Fail("扫描", scanErr)
		if x.Aborted() {
			x.Note("参加了 0 个抽奖")
			return
		}
	}
	if len(rooms) == 0 {
		if scanErr == nil {
			x.Skip("扫描", "现在没有直播间在抽奖")
		}
		x.Note("参加了 0 个抽奖")
		return
	}
	var prizes []string
	joinedHosts := map[int64]bool{}
	seenLotteries := map[int64]bool{}
	excluded, notIncluded, gifts, medals, follows, ended := 0, 0, 0, 0, 0, 0
	joinCalls, consecutiveFailures := 0, 0
	var firstErr, stopErr error
	failed := func(err error) bool {
		x.Debugf("天选时刻请求失败：%v", err)
		if firstErr == nil {
			firstErr = err
		}
		switch bili.Classify(err) {
		case bili.ReasonRisk, bili.ReasonRate, bili.ReasonNetwork:
			consecutiveFailures++
		default:
			consecutiveFailures = 0
		}
		if bili.IsNotLogin(err) || x.Err() != nil || consecutiveFailures >= 3 {
			stopErr = err
			return true
		}
		return false
	}
	for _, room := range rooms {
		if x.Err() != nil {
			stopErr = x.Err()
			break
		}
		lottery, err := x.C.LiveLotteryCheck(x, room.ID)
		if err != nil {
			if failed(err) {
				break
			}
			continue
		}
		if lottery == nil || lottery.Status != 1 || lottery.ID <= 0 {
			consecutiveFailures = 0
			ended++
			continue
		}
		if seenLotteries[lottery.ID] {
			consecutiveFailures = 0
			continue
		}
		seenLotteries[lottery.ID] = true
		if lotteryMatches(lottery.Award, opt.Exclude) {
			consecutiveFailures = 0
			excluded++
			continue
		}
		if len(opt.Include) > 0 && !lotteryMatches(lottery.Award, opt.Include) {
			consecutiveFailures = 0
			notIncluded++
			continue
		}
		if lottery.GiftPrice > 0 {
			consecutiveFailures = 0
			gifts++
			continue
		}
		if lottery.RequireType == nil || *lottery.RequireType < 0 || *lottery.RequireType > 1 {
			consecutiveFailures = 0
			medals++
			continue
		}
		if *lottery.RequireType == 1 && (!opt.Follow || opt.Group && !groupReady) {
			consecutiveFailures = 0
			follows++
			continue
		}
		if !x.Sleep(3 * time.Second) {
			stopErr = x.Err()
			break
		}
		joinCalls++
		err = x.C.LiveLotteryJoin(x, *lottery)
		if err != nil {
			if failed(err) {
				break
			}
		} else {
			consecutiveFailures = 0
			prizes = append(prizes, lottery.Award)
			if *lottery.RequireType == 1 && room.UID > 0 {
				joinedHosts[room.UID] = true
			}
		}
		if joinCalls >= 30 {
			break
		}
	}
	if len(prizes) > 0 {
		x.OK("参加", "%d 个：%s", len(prizes), shortList(prizes, 5))
	}
	if stopErr != nil {
		x.Fail("参加", stopErr)
		x.Note("参加了 %d 个抽奖", len(prizes))
		if x.Aborted() {
			return
		}
	} else if len(prizes) == 0 && firstErr != nil {
		x.Fail("参加", firstErr)
		if x.Aborted() {
			x.Note("参加了 0 个抽奖")
			return
		}
	} else if len(prizes) == 0 {
		x.Skip("参加", "没有符合条件的抽奖：排除关键词 %d 个、未匹配关键词 %d 个、需要礼物 %d 个、需要粉丝牌或舰长 %d 个、需要关注 %d 个、已结束 %d 个", excluded, notIncluded, gifts, medals, follows, ended)
	}
	if groupReady {
		if len(joinedHosts) == 0 {
			x.Skip("分组", "没有因为抽奖新关注的主播")
		} else {
			count, err := lotteryGroupHosts(x, newest, joinedHosts)
			if err != nil {
				x.Fail("分组", err)
				if x.Aborted() {
					x.Note("参加了 %d 个抽奖", len(prizes))
					return
				}
			} else if count == 0 {
				x.Skip("分组", "没有查到因为抽奖新关注的主播；平台是否自动关注以关注列表为准")
			} else {
				x.OK("分组", "%d 个新关注的主播已加入「%s」", count, lotteryGroup)
			}
		}
	}
	x.Note("参加了 %d 个抽奖", len(prizes))
}

func lotteryGroupHosts(x *Ctx, newest int64, joined map[int64]bool) (int, error) {
	var mids []int64
	seen := map[int64]bool{}
	stop := false
	for page := 1; page <= 5 && !stop; page++ {
		list, _, err := x.C.Followings(x, page, false)
		if err != nil {
			return 0, err
		}
		for _, user := range list {
			if user.Mid == newest {
				stop = true
				break
			}
			if joined[user.Mid] && !seen[user.Mid] {
				mids = append(mids, user.Mid)
				seen[user.Mid] = true
			}
		}
		if len(list) < 20 {
			break
		}
	}
	if len(mids) == 0 {
		return 0, nil
	}
	tags, err := x.C.Tags(x)
	if err != nil {
		return 0, err
	}
	find := func(tags []bili.Tag) int64 {
		for _, tag := range tags {
			if tag.Name == lotteryGroup && tag.ID > 0 {
				return tag.ID
			}
		}
		return 0
	}
	tagID := find(tags)
	if tagID == 0 {
		tagID, err = x.C.CreateTag(x, lotteryGroup)
		if bili.IsCode(err, 22106) {
			tags, err = x.C.Tags(x)
			if err == nil {
				tagID = find(tags)
			}
		}
		if err != nil {
			return 0, err
		}
		if tagID <= 0 {
			return 0, &bili.Error{Code: bili.CodeBadResponse, Message: fmt.Sprintf("没有取得「%s」分组的编号", lotteryGroup)}
		}
	}
	if err := x.C.AddToTag(x, mids, tagID); err != nil {
		return 0, err
	}
	return len(mids), nil
}
