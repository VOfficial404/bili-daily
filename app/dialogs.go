package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

func (a *app) dialogs(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.qrOpen, func() {
		ui.Column(c).Width(min(340, a.width-64)).Gap(16).AlignItems(ui.Center).Padding(8).Children(func() {
			ui.Text(c, "添加 B 站账号").Bold().FontSize(20)
			muted(c, "打开 B 站 App，用「扫一扫」扫下面的码，再在手机上确认。已经添加过的账号会更新登录。").FontSize(13).TextAlign(ui.Center)
			ui.Box(c).Size(240, 240).Radius(14).Background(ui.Hex("#ffffff")).Center().Children(func() {
				switch {
				case a.qrImage != nil && a.qr.State != "expired":
					ui.Image(c, a.qrImage).Size(216, 216)
				case a.qrBusy:
					ui.Spinner(c)
				default:
					ui.Text(c, "二维码已过期").TextColor(ui.Hex("#71717a"))
				}
			})
			s, col := "正在获取二维码", t.TextMuted
			switch a.qr.State {
			case "waiting":
				s, col = "用 B 站 App 扫一扫", infoColor(c)
			case "scanned":
				s, col = "已扫码，请在手机上确认", t.Success
			case "expired":
				s, col = "二维码已过期", t.Warning
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(col)
				ui.Text(c, s).FontSize(14)
			})
			errorText(c, a.qrErr)
			if a.mobile && a.qr.URL != "" && a.qr.State != "expired" {
				// 扫码的手机就是这一台：直接跳到 B 站 App 确认登录
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					ui.PrimaryButton(c.Key("qr-open"), "在 B 站 App 里打开").OnClick(a.openQR)
					if a.copyText != nil {
						ui.Button(c.Key("qr-copy"), "复制链接").OnClick(func() { a.copyText(a.qr.URL); a.toast("链接已复制") })
					}
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				if a.qr.State == "expired" || a.qrErr != "" {
					ui.PrimaryButton(c, "换一张").Disabled(a.qrBusy).OnClick(a.startQR)
				}
				ui.Button(c, "关闭").OnClick(func() { a.qrOpen = false })
			})
		})
	})
	if ui.AlertDialog(c, &a.confirmOpen, a.confirmTitle, a.confirmText, "取消", "退出账号") == 1 && a.confirmAction != nil {
		a.confirmAction()
	}
	a.runDialog(c)
	a.guardDialog(c)
	a.upPickerDialog(c)
}

func (a *app) runDialog(c *ui.Context) {
	ui.Modal(c, &a.pickOpen, func() {
		ui.Column(c).Width(min(420, a.width-64)).Gap(12).Children(func() {
			ui.Text(c, "运行一次："+a.title(a.pickTask)).Bold().FontSize(18)
			muted(c, "选择账号")
			for _, acc := range a.st.Accounts {
				ui.Button(c.Key("pick-"+acc.Mid), accountName(acc)).Disabled(!acc.LoggedIn || a.groupsBusy).OnClick(func() { a.chooseRunAccount(acc.Mid) })
			}
			if a.pickTask == "unfollow" && a.pickMid != "" {
				if a.groupsBusy {
					ui.Spinner(c)
				} else if len(a.groups) == 0 {
					muted(c, "这个账号还没有关注分组")
				} else {
					labels := make([]string, len(a.groups))
					for i, g := range a.groups {
						labels[i] = fmt.Sprintf("%s（%d 人）", g.Name, g.Count)
					}
					ui.Field(c, "关注分组", func() {
						selected := labels[a.groupIdx]
						if ui.Select(c, &selected, labels).Changed() {
							a.groupIdx = slices.Index(labels, selected)
						}
					})
					ui.Field(c, "最多取关数量（0 表示全部）", func() { ui.TextInput(c, &a.unfollowCount) })
					ui.Field(c, "保留的 UID（逗号分隔）", func() { ui.TextInput(c, &a.unfollowKeep) })
					ui.PrimaryButton(c, "保存选项并运行").Disabled(a.busy["unfollow"]).OnClick(a.confirmUnfollow)
				}
			}
			errorText(c, a.pickErr)
			ui.Button(c, "取消").OnClick(func() { a.pickOpen = false })
		})
	})
}

func (a *app) submitUnfollow() {
	count, err := strconv.Atoi(strings.TrimSpace(a.unfollowCount))
	if err != nil || count < 0 {
		a.pickErr = "取关数量要填大于或等于 0 的整数"
		return
	}
	keep := []int64{}
	for _, s := range strings.FieldsFunc(a.unfollowKeep, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\n' }) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			a.pickErr = "保留列表要填有效的 UID"
			return
		}
		keep = append(keep, id)
	}
	base, s := client.CloneSettings(a.formBase), client.CloneSettings(a.form)
	s.Tasks.Unfollow = api.UnfollowOptions{Group: a.groups[a.groupIdx].Name, Count: count, Keep: keep}
	mid := a.pickMid
	session, ctx, epoch := a.session, a.ctx, a.epoch
	cl := session.Client()
	a.busy["unfollow"] = true
	a.pickErr = ""
	a.async(func() func() {
		saved, err := session.SaveSettings(ctx, base, s)
		if err == nil {
			err = cl.Run(ctx, mid, []string{"unfollow"})
		}
		return func() {
			if epoch != a.epoch {
				return
			}
			a.busy["unfollow"] = false
			if err != nil {
				a.pickErr = err.Error()
				return
			}
			a.form = saved
			a.formBase = client.CloneSettings(saved)
			a.st.Settings = client.CloneSettings(saved)
			a.dirty = false
			a.inputs = map[string]string{}
			a.pickOpen = false
			a.toast("已安排批量取关")
			a.poll()
		}
	})
}

// openQR 把扫码链接交给系统，由 B 站 App 打开确认登录；没装 B 站 App 时提示改用截图
func (a *app) openQR() {
	open, url, gen := a.openURL, a.qr.URL, a.qrGen
	if open == nil || url == "" {
		return
	}
	a.async(func() func() {
		if err := open(url); err != nil {
			return func() {
				if gen == a.qrGen {
					a.qrErr = "无法打开链接。请截图，在 B 站「扫一扫」里从相册选择这张二维码。"
				}
			}
		}
		return nil
	})
}
