package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/egoist/mygo/ui"
)

const upPasteHint = "填 UID 或 UP 主空间链接，例如 space.bilibili.com/123"

var (
	upDigits    = regexp.MustCompile(`^[0-9]+$`)
	upSpaceLink = regexp.MustCompile(`(?i)space\.bilibili\.com/([0-9]+)`)
	upUIDLabel  = regexp.MustCompile(`(?i)^uid(?:\s*:\s*|\s+)([0-9]+)$`)
)

type upPasteInput struct {
	text, err string
}

type upPickerState struct {
	open, single, loading   bool
	mid, search, query, err string
	list                    []api.UPProfile
	page                    int
	more                    bool
}

func parseUPInput(raw string) (int64, bool) {
	s := strings.TrimSpace(raw)
	if m := upSpaceLink.FindStringSubmatch(s); m != nil {
		s = m[1]
	} else if m := upUIDLabel.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if !upDigits.MatchString(s) {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n > 0
}

func (a *app) selectedUPs(single bool) []int64 {
	if single {
		if a.form.Tasks.Charge.UpID > 0 {
			return []int64{a.form.Tasks.Charge.UpID}
		}
		return nil
	}
	return a.form.Tasks.Daily.UpIDs
}

func (a *app) addUP(single bool, id int64) {
	if id <= 0 || slices.Contains(a.selectedUPs(single), id) {
		return
	}
	if single {
		a.form.Tasks.Charge.UpID = id
	} else {
		a.form.Tasks.Daily.UpIDs = append(a.form.Tasks.Daily.UpIDs, id)
	}
	a.dirty = true
	a.chargeConfirmed = false
}

func (a *app) removeUP(single bool, id int64) {
	if single {
		if a.form.Tasks.Charge.UpID != id {
			return
		}
		a.form.Tasks.Charge.UpID = 0
	} else {
		ids := a.form.Tasks.Daily.UpIDs
		i := slices.Index(ids, id)
		if i < 0 {
			return
		}
		a.form.Tasks.Daily.UpIDs = slices.Delete(ids, i, i+1)
	}
	a.dirty = true
	a.chargeConfirmed = false
}

func (a *app) moveUP(id int64, offset int) {
	ids := a.form.Tasks.Daily.UpIDs
	i := slices.Index(ids, id)
	j := i + offset
	if i < 0 || j < 0 || j >= len(ids) || i == j {
		return
	}
	ids[i], ids[j] = ids[j], ids[i]
	a.dirty = true
}

func (a *app) pasteUP(single bool, input *upPasteInput) {
	id, ok := parseUPInput(input.text)
	if !ok {
		input.err = upPasteHint
		return
	}
	a.addUP(single, id)
	input.text, input.err = "", ""
	mid := strconv.FormatInt(id, 10)
	if a.upProfiles[mid] == nil {
		a.loadUPProfile(mid)
	}
}

func (a *app) chooseUP(profile api.UPProfile) {
	p := a.upPicker
	id, ok := parseUPInput(profile.Mid)
	if p == nil || !p.open || !ok {
		return
	}
	profile.Mid = strconv.FormatInt(id, 10)
	if a.upProfiles == nil {
		a.upProfiles = make(map[string]*upProfilePreview)
	}
	a.upProfiles[profile.Mid] = &upProfilePreview{profile: profile}
	a.loadFace(profile.Face)
	if !p.single && slices.Contains(a.selectedUPs(false), id) {
		a.removeUP(false, id)
	} else {
		a.addUP(p.single, id)
	}
	if p.single {
		p.open = false
	}
}

func (a *app) openUPPicker(single bool) {
	p := &upPickerState{open: true, single: single}
	a.upPicker = p
	if a.st != nil {
		for _, acc := range a.st.Accounts {
			if acc.LoggedIn {
				p.mid = acc.Mid
				break
			}
		}
	}
	a.loadFollowings(true)
}

// 只有打开、提交搜索、切换账号和点击加载更多会请求关注列表。
func (a *app) loadFollowings(reset bool) {
	p := a.upPicker
	if p == nil || !p.open || p.loading || p.mid == "" || a.session.Client() == nil {
		return
	}
	if reset {
		p.page, p.more = 0, false
		p.list = nil
		p.query = strings.TrimSpace(p.search)
	}
	page, mid, query := p.page+1, p.mid, p.query
	p.loading, p.err = true, ""
	session, ctx, epoch := a.session, a.ctx, a.epoch
	cl := session.Client()
	a.async(func() func() {
		result, err := cl.Followings(ctx, mid, page, query)
		return func() {
			if a.upPicker != p || !p.open || a.session != session || a.epoch != epoch {
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				return
			}
			for _, profile := range result.List {
				if !slices.ContainsFunc(p.list, func(up api.UPProfile) bool { return up.Mid == profile.Mid }) {
					p.list = append(p.list, profile)
				}
				a.loadFace(profile.Face)
			}
			p.page, p.more = page, result.More
		}
	})
}

func (a *app) upProfileRow(c *ui.Context, profile api.UPProfile, status string) {
	a.avatar(c, api.Account{Mid: profile.Mid, Uname: profile.Uname, Face: profile.Face}, 36)
	ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
		if profile.Uname != "" {
			ui.Text(c, profile.Uname).Bold().SingleLine()
		}
		muted(c, "UID "+profile.Mid).FontSize(12).Selectable()
		if status != "" {
			muted(c, status).FontSize(12)
		}
	})
}

func (a *app) settingsUPPicker(c *ui.Context, single bool) {
	key, label, hint, index := "daily", "优先观看和投币的 UP 主", "按列表顺序优先选择，可上下移动调整。留空自动选择。", 0
	if single {
		key, label, hint, index = "charge", "充电对象", "会消耗 B 币券，启用前请确认对象。", 1
	}
	ui.Field(c, label, func() {
		ui.Column(c.Key("up-" + key)).Gap(8).Grow(1).Children(func() {
			ids := a.selectedUPs(single)
			if len(ids) == 0 {
				if single {
					muted(c, "还没有选充电对象")
				} else {
					muted(c, "还没有选优先 UP 主，任务会自动选择")
				}
			}
			for i, id := range ids {
				mid := strconv.FormatInt(id, 10)
				preview := a.upProfiles[mid]
				profile := api.UPProfile{Mid: mid}
				status, button, loading := "正在查询…", "获取资料", false
				if preview != nil {
					loading = preview.loading
					switch {
					case loading:
						button = "正在查询"
					case preview.err != "":
						status, button = preview.err+" 可直接保存 UID。", "重试"
					default:
						profile, status = preview.profile, ""
					}
				}
				panel(c.Key("selected-" + mid)).Padding(10).Gap(8).Children(func() {
					ui.Row(c).Gap(10).Children(func() { a.upProfileRow(c, profile, status) })
					ui.Row(c).Gap(6).Wrap().Children(func() {
						if status != "" {
							ui.Button(c.Key("lookup"), button).Disabled(loading).OnClick(func() { a.loadUPProfile(mid) })
						}
						if !single {
							ui.Button(c.Key("up"), "上移").Disabled(i == 0).OnClick(func() { a.moveUP(id, -1) })
							ui.Button(c.Key("down"), "下移").Disabled(i == len(ids)-1).OnClick(func() { a.moveUP(id, 1) })
						}
						ui.Button(c.Key("remove"), "移除").OnClick(func() { a.removeUP(single, id) })
					})
				})
			}
			ui.Button(c.Key("followings"), "从关注里选").OnClick(func() { a.openUPPicker(single) })
			input := &a.upPaste[index]
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c.Key("paste"), &input.text).Placeholder("UID 或 UP 主空间链接").Grow(1).OnSubmit(func() { a.pasteUP(single, input) })
				ui.Button(c.Key("add"), "添加").OnClick(func() { a.pasteUP(single, input) })
			})
			errorText(c, input.err)
		})
	}).Description(hint)
}

func (a *app) upPickerDialog(c *ui.Context) {
	p := a.upPicker
	if p == nil {
		return
	}
	ui.Modal(c.Key("up-picker"), &p.open, func() {
		ui.Column(c).Width(min(560, a.width-64)).Gap(12).Children(func() {
			ui.Text(c, "从关注里选").FontSize(18).Bold()
			labels, mids := []string{}, []string{}
			selected := ""
			if a.st != nil {
				for _, acc := range a.st.Accounts {
					if !acc.LoggedIn {
						continue
					}
					label := accountName(acc) + "（UID " + acc.Mid + "）"
					labels, mids = append(labels, label), append(mids, acc.Mid)
					if p.mid == acc.Mid {
						selected = label
					}
				}
			}
			if len(mids) == 0 {
				muted(c, "先添加并登录一个账号才能从关注里选，也可以直接粘贴 UID")
			} else {
				ui.Field(c, "选择账号", func() {
					if ui.Select(c.Key("account"), &selected, labels).Disabled(p.loading).Changed() {
						p.mid = mids[slices.Index(labels, selected)]
						a.loadFollowings(true)
					}
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("search"), &p.search).Label("搜索关注的昵称").Placeholder("搜索关注的昵称").Grow(1).Disabled(p.loading).OnSubmit(func() { a.loadFollowings(true) })
					ui.Button(c.Key("submit"), "搜索").Disabled(p.loading).OnClick(func() { a.loadFollowings(true) })
				})
				ui.Scroll(c.Key("followings-list")).Height(min(360, max(160, a.height-340))).FillWidth().Children(func() {
					ui.Column(c).Gap(8).FillWidth().Children(func() {
						for _, profile := range p.list {
							id, _ := parseUPInput(profile.Mid)
							on := slices.Contains(a.selectedUPs(p.single), id)
							b := ui.ButtonBase(c.Key("following-"+profile.Mid)).Padding(10).Radius(10).FillWidth().Border(1, c.Theme().Border).
								Label(profile.Uname + " UID " + profile.Mid).OnClick(func() { a.chooseUP(profile) })
							if on {
								b.Background(c.Theme().Accent.Alpha(0.12))
							}
							b.Children(func() {
								ui.Row(c).Gap(10).FillWidth().Children(func() {
									a.upProfileRow(c, profile, "")
									if on {
										pill(c, "已选", c.Theme().Accent)
									}
								})
							})
						}
						if !p.loading && p.err == "" && len(p.list) == 0 {
							muted(c, "没有找到关注的 UP 主，可以换个昵称搜索或直接粘贴 UID")
						}
					})
				})
				if p.loading {
					ui.Row(c).Gap(8).Children(func() {
						ui.Spinner(c)
						muted(c, "正在加载关注，请稍候…")
					})
				} else if p.err != "" {
					errorText(c, p.err)
					ui.Button(c.Key("retry"), "重试").OnClick(func() { a.loadFollowings(false) })
				} else if p.more {
					ui.Button(c.Key("more"), "加载更多").OnClick(func() { a.loadFollowings(false) })
				}
			}
			ui.PrimaryButton(c.Key("done"), "完成").OnClick(func() { p.open = false })
		})
	})
}
