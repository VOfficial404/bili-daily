package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/egoist/mygo/ui"
)

func (a *app) accountsView(c *ui.Context) {
	t := c.Theme()
	if len(a.st.Accounts) == 0 {
		panel(c).Padding(32).AlignItems(ui.Center).Children(func() {
			muted(c, "还没有账号，先扫码添加一个。")
			ui.PrimaryButton(c.Key("first-qr"), "扫码添加账号").OnClick(a.startQR)
		})
		return
	}
	for _, acc := range a.st.Accounts {
		panel(c.Key("acc-" + acc.Mid)).Gap(18).Children(func() {
			ui.Row(c).Gap(16).Children(func() {
				a.ringAvatar(c, acc, 64)
				ui.Column(c).Gap(5).Grow(1).MinWidth(0).Children(func() {
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, accountName(acc)).FontSize(18).Bold().SingleLine()
						levelBadge(c, acc.Level)
					})
					muted(c, "UID "+acc.Mid).FontSize(12).Selectable()
				})
				enabled := acc.Enabled
				ui.Row(c).Gap(8).Children(func() {
					muted(c, "参加每日计划").FontSize(13)
					if ui.Switch(c.Key("plan-"+acc.Mid), &enabled).Label("参加每日计划").Disabled(a.busy[acc.Mid]).Changed() {
						a.action(acc.Mid, "每日计划已更新", func(cl *api.Client, ctx context.Context) error {
							return cl.Patch(ctx, acc.Mid, api.AccountPatch{Enabled: &enabled})
						})
					}
				})
			})
			if !acc.LoggedIn {
				ui.Row(c).Gap(12).Padding(12, 14).Radius(10).Background(t.Danger.Alpha(0.1)).Children(func() {
					ui.Text(c, "登录已失效，任务已暂停。请重新扫码。").TextColor(t.Danger).Grow(1)
					ui.PrimaryButton(c.Key("relogin-"+acc.Mid), "重新扫码").OnClick(a.startQR)
				})
			}

			// 资料：两列
			vip := acc.VIP
			if vip == "" {
				vip = "不是大会员"
			} else {
				if acc.VIPDue != nil {
					vip += "，" + acc.VIPDue.Local().Format("2006-01-02") + " 到期"
				}
			}
			cookie := "有效期未知"
			if acc.CookieExpires != nil {
				days := int(time.Until(*acc.CookieExpires).Hours() / 24)
				cookie = fmt.Sprintf("还有 %d 天（%s），会自动续期", days, acc.CookieExpires.Local().Format("2006-01-02"))
			}
			exp := "已满级"
			if acc.NextExp > 0 {
				exp = thousands(acc.Exp) + " / " + thousands(acc.NextExp)
			}
			ui.Grid(c).Columns(2).GapX(24).GapY(12).Children(func() {
				fact := func(k, v string) {
					ui.Column(c).Gap(3).Children(func() {
						sectionTitle(c, k)
						ui.Text(c, v).FontSize(14).Selectable()
					})
				}
				fact("经验", exp)
				fact("硬币", trimNum(acc.Coins))
				fact("B 币券", trimNum(acc.BCoupon))
				fact("大会员", vip)
				fact("登录", cookie)
				fact("资料更新于", client.HumanTime(acc.UpdatedAt, "暂无"))
			})

			// 这个账号做哪些任务
			ui.Column(c).Gap(8).Children(func() {
				sectionTitle(c, "这个账号做的任务（总开关在「任务」页）")
				ui.Grid(c).Columns(2).GapX(24).GapY(4).Children(func() {
					for _, task := range a.st.Tasks {
						if task.Manual {
							continue
						}
						// 总开关关着时，这里的开关显示为关且不能点
						global := client.TaskOn(&a.st.Settings, task.ID)
						on := global && !slices.Contains(acc.DisabledTasks, task.ID)
						ui.Row(c).Gap(10).Padding(4, 0).Children(func() {
							txt := ui.Text(c, task.Title).FontSize(14).Grow(1)
							if !global {
								txt.TextColor(t.TextMuted).Tooltip("任务页里关着，所有账号都不做")
							}
							if ui.Switch(c.Key(acc.Mid+"-"+task.ID), &on).Label(task.Title).Disabled(a.busy[acc.Mid] || !global).Changed() {
								disabled := slices.Clone(acc.DisabledTasks)
								if on {
									disabled = slices.DeleteFunc(disabled, func(id string) bool { return id == task.ID })
								} else {
									disabled = append(disabled, task.ID)
								}
								a.action(acc.Mid, "账号任务已更新", func(cl *api.Client, ctx context.Context) error {
									return cl.Patch(ctx, acc.Mid, api.AccountPatch{DisabledTasks: &disabled})
								})
							}
						})
					}
				})
			})

			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Button(c.Key("run-"+acc.Mid), "立即运行").Disabled(!acc.LoggedIn || acc.Running != nil || a.busy[acc.Mid]).OnClick(func() { a.run(acc.Mid, nil) })
				ui.Button(c.Key("refresh-"+acc.Mid), "刷新资料").Disabled(a.busy[acc.Mid]).OnClick(func() {
					a.action(acc.Mid, "资料已刷新", func(cl *api.Client, ctx context.Context) error { return cl.Refresh(ctx, acc.Mid) })
				})
				ui.Spacer(c)
				ui.ButtonBase(c.Key("logout-"+acc.Mid)).Padding(6, 10).Radius(6).Label("退出账号").Disabled(a.busy[acc.Mid]).OnClick(func() {
					a.confirmOpen = true
					a.confirmTitle = "退出「" + accountName(acc) + "」？"
					a.confirmText = "这会在 B 站注销这份登录，并删除本机或服务器上这个账号的数据。之后要用需要重新扫码。"
					a.confirmAction = func() {
						a.action(acc.Mid, "账号已退出", func(cl *api.Client, ctx context.Context) error { return cl.Remove(ctx, acc.Mid) })
					}
				}).Children(func() { ui.Text(c, "退出账号").FontSize(13).TextColor(t.Danger) })
			})
		})
	}
}

// accountFilter 是一排按账号筛选的标签，返回是否改了
func (a *app) accountFilter(c *ui.Context, key string, mid *string) bool {
	t := c.Theme()
	changed := false
	chip := func(k, label string, on bool, fn func()) {
		b := ui.ButtonBase(c.Key(k)).Padding(5, 12).Radius(999).Label(label).OnClick(fn)
		if on {
			b.Background(t.Accent.Alpha(0.16)).Border(1, t.Accent.Alpha(0.5))
		} else {
			b.Background(t.Surface).Border(1, t.Border)
		}
		b.Children(func() {
			col := t.TextMuted
			if on {
				col = t.Accent
			}
			ui.Text(c, label).FontSize(12).TextColor(col).SingleLine()
		})
	}
	ui.Row(c.Key(key)).Gap(6).Wrap().Children(func() {
		chip("all", "全部账号", *mid == "", func() { *mid = ""; changed = true })
		for _, acc := range a.st.Accounts {
			chip(acc.Mid, accountName(acc), *mid == acc.Mid, func() { *mid = acc.Mid; changed = true })
		}
	})
	return changed
}
