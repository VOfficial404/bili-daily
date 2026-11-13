package engine

import (
	"slices"
	"strings"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili"
)

func init() {
	register(Task{
		Info: api.TaskInfo{
			ID:     TaskUnfollow,
			Title:  "批量取关",
			Desc:   "取关一个关注分组里的人，例如因为天选时刻抽奖而关注的主播。只在你点运行时执行。",
			Group:  "工具",
			Manual: true,
		},
		Run: runUnfollow,
	})
}

// 一个分组最多读这么多页（每页 20 个）
const unfollowMaxPages = 100

func runUnfollow(x *Ctx) {
	opt := x.Set.Tasks.Unfollow
	if opt.Group == "" {
		x.Failf("查找分组", bili.ReasonIneligible, "还没有选择要取关哪个分组，在设置里选一个")
		return
	}
	tags, err := x.C.Tags(x)
	if err != nil {
		x.Fail("查找分组", err)
		return
	}
	i := slices.IndexFunc(tags, func(t bili.Tag) bool { return t.Name == opt.Group })
	if i < 0 {
		x.Skip("查找分组", "没有叫「%s」的关注分组", opt.Group)
		return
	}
	tag := tags[i]
	if tag.Count == 0 {
		x.Skip("查找分组", "「%s」分组是空的", tag.Name)
		return
	}

	// 接口从新到旧返回；全部读出来之后从最早关注的开始取关
	var members []bili.User
	for pn := 1; pn <= unfollowMaxPages; pn++ {
		page, err := x.C.TagMembers(x, tag.ID, pn)
		if err != nil {
			x.Fail("读取分组", err)
			return
		}
		members = append(members, page...)
		if len(page) < 20 {
			break
		}
	}
	slices.Reverse(members)

	var done, kept []string
	failures := 0 // 连续失败的次数
	var lastErr error
	for _, u := range members {
		if opt.Count > 0 && len(done) >= opt.Count {
			break
		}
		if slices.Contains(opt.Keep, u.Mid) {
			kept = append(kept, u.Uname)
			continue
		}
		if err := x.C.Unfollow(x, u.Mid); err != nil {
			failures, lastErr = failures+1, err
			x.Debugf("取关 %s（%d）失败：%v", u.Uname, u.Mid, err)
			// 登录失效、被中止，或者连着失败：再试下去只会更糟
			if bili.IsNotLogin(err) || x.Err() != nil || failures >= 3 {
				break
			}
			continue
		}
		failures = 0
		done = append(done, u.Uname)
	}

	if len(done) > 0 {
		x.OK("取关", "%d 个：%s", len(done), shortList(done, 10))
	}
	if len(kept) > 0 {
		x.Skip("保留", "%d 个在保留名单里：%s", len(kept), shortList(kept, 10))
	}
	if lastErr != nil {
		x.Fail("取关", lastErr)
	}
	if len(done) == 0 && lastErr == nil && len(kept) == 0 {
		x.Skip("取关", "「%s」分组里没有可以取关的人", tag.Name)
	}
	x.Note("取关 %d 个，「%s」分组里还剩 %d 个", len(done), tag.Name, max(0, len(members)-len(done)))
}

// shortList 把名字用顿号连起来，超过 n 个时只写前 n 个
func shortList(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, "、")
	}
	return strings.Join(names[:n], "、") + " 等"
}
