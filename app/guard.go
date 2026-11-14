package main

import (
	"fmt"
	"strings"

	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

// 会花钱或者撤不回的操作，提交之前先说清楚要做什么。

// needsChargeConfirm 在打开 B 币券充电或者换了充电对象时要求确认；需要确认时打开对话框并返回 true
func (a *app) needsChargeConfirm() bool {
	f := a.form.Tasks.Charge
	if a.chargeConfirmed || !client.NeedsChargeConfirm(a.formBase, a.form) {
		return false
	}
	recipient := fmt.Sprintf("UID %d", f.UpID)
	if preview := a.upProfiles[fmt.Sprint(f.UpID)]; preview != nil && preview.err == "" && preview.profile.Uname != "" {
		recipient = preview.profile.Uname + "（" + recipient + "）"
	}
	a.guardOpen = true
	a.guardTitle = "确认 B 币券充电？"
	a.guardText = fmt.Sprintf("保存之后，每月 %d 号起会把每个账号里的全部整数 B 币券充给 %s。充进去的 B 币拿不回来，先确认这个 UID 是你想支持的 UP 主。", f.Day, recipient)
	a.guardOK = "确认并保存"
	a.guardAction = func() {
		a.chargeConfirmed = true
		a.saveSettings()
	}
	return true
}

// confirmUnfollow 在提交批量取关之前确认账号、分组和数量
func (a *app) confirmUnfollow() {
	if len(a.groups) == 0 {
		return
	}
	count := strings.TrimSpace(a.unfollowCount)
	if count == "" || count == "0" {
		count = "全部"
	} else {
		count += " 个"
	}
	a.guardOpen = true
	a.guardTitle = "确认批量取关？"
	a.guardText = fmt.Sprintf("账号：%s\n分组：%s\n最多取关：%s\n保留名单里的人不会被取关。取关之后不能撤销。", a.name(a.pickMid), a.groups[a.groupIdx].Name, count)
	a.guardOK = "确认取关"
	a.guardAction = a.submitUnfollow
}

func (a *app) guardDialog(c *ui.Context) {
	if ui.AlertDialog(c, &a.guardOpen, a.guardTitle, a.guardText, "取消", a.guardOK) == 1 && a.guardAction != nil {
		fn := a.guardAction
		a.guardAction = nil
		fn()
	}
}
