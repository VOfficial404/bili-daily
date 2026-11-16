package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

const mobileUPInputHint = "填 UID 或 UP 主空间链接，例如 space.bilibili.com/123"

var mobileUPLink = regexp.MustCompile(`(?i)space\.bilibili\.com/([0-9]+)`)
var mobileUPUID = regexp.MustCompile(`(?i)^uid\s*:?\s*([0-9]+)$`)

func mobileParseUP(text string) (int64, error) {
	text = strings.TrimSpace(text)
	if match := mobileUPLink.FindStringSubmatch(text); match != nil {
		text = match[1]
	} else if match := mobileUPUID.FindStringSubmatch(text); match != nil {
		text = match[1]
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%s", mobileUPInputHint)
		}
	}
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s", mobileUPInputHint)
	}
	return id, nil
}

// mobileUPSelection 的修改只进入表单草稿，保存仍交给原来的 readers。
type mobileUPSelection struct {
	ids    *core.Signal[[]int64]
	single bool
}

func (s mobileUPSelection) contains(id int64) bool { return slices.Contains(s.ids.Get(), id) }
func (s mobileUPSelection) add(id int64) {
	if id <= 0 {
		return
	}
	if s.single {
		s.ids.Set([]int64{id})
	} else if !slices.Contains(s.ids.GetUntracked(), id) {
		s.ids.Set(append(slices.Clone(s.ids.GetUntracked()), id))
	}
}
func (s mobileUPSelection) remove(id int64) {
	ids := slices.Clone(s.ids.GetUntracked())
	if i := slices.Index(ids, id); i >= 0 {
		s.ids.Set(slices.Delete(ids, i, i+1))
	}
}
func (s mobileUPSelection) move(id int64, delta int) {
	ids := slices.Clone(s.ids.GetUntracked())
	i := slices.Index(ids, id)
	if s.single || i < 0 || i+delta < 0 || i+delta >= len(ids) {
		return
	}
	ids[i], ids[i+delta] = ids[i+delta], ids[i]
	s.ids.Set(ids)
}
func (s mobileUPSelection) toggle(id int64) {
	if !s.single && s.contains(id) {
		s.remove(id)
	} else {
		s.add(id)
	}
}

func (a *mobileApp) upPicker(cx *core.Scope, f *mobileSettingsForm, initial []int64, single bool, put func([]int64)) core.Node {
	selection := mobileUPSelection{ids: core.NewSignalFunc(cx, []int64(nil), nil), single: single}
	for _, id := range initial {
		selection.add(id)
	}
	f.readers = append(f.readers, func() error { put(slices.Clone(selection.ids.Get())); return nil })
	if a.upProfileCache == nil {
		a.upProfileCache = make(map[string]api.UPProfile)
	}
	cache := a.upProfileCache
	failures := make(map[string]string)
	revision := core.NewSignal(cx, 0)
	paste := core.NewSignal(cx, "")
	inputError := core.NewSignal(cx, "")
	open := core.NewSignal(cx, false)
	cl, session, epoch := a.session.Client(), a.session, a.epoch
	generation := 0
	var cancel context.CancelFunc
	cx.Cleanup(func() {
		generation++
		if cancel != nil {
			cancel()
		}
	})
	cx.Effect(func() {
		ids := selection.ids.Get()
		generation++
		g := generation
		if cancel != nil {
			cancel()
		}
		var missing []string
		for _, id := range ids {
			mid := strconv.FormatInt(id, 10)
			if _, ok := cache[mid]; !ok {
				missing = append(missing, mid)
				delete(failures, mid)
			}
		}
		if len(missing) == 0 {
			return
		}
		ctx, stop := context.WithCancel(cx.Context())
		cancel = stop
		go func() {
			for _, mid := range missing {
				if ctx.Err() != nil {
					return
				}
				profile, err := cl.UPProfile(ctx, mid)
				cx.Post(func() {
					if ctx.Err() != nil || g != generation || epoch != a.epoch || session != a.session {
						return
					}
					if err != nil {
						failures[mid] = err.Error()
					} else {
						cache[mid] = profile
					}
					revision.Set(revision.GetUntracked() + 1)
				})
			}
		}()
	})
	add := func() {
		id, err := mobileParseUP(paste.GetUntracked())
		if err != nil {
			inputError.Set(err.Error())
			return
		}
		selection.add(id)
		paste.Set("")
		inputError.Set("")
	}
	colors := ui.ColorsOf(cx)
	selected := core.Dynamic(func(cx *core.Scope) core.Node {
		_ = revision.Get()
		ids := selection.ids.Get()
		if len(ids) == 0 {
			title := "尚未设置优先 UP 主"
			if single {
				title = "还没有选充电对象"
			}
			return mobileRow{Title: title}
		}
		rows := make([]core.Node, 0, len(ids))
		for i, id := range ids {
			mid := strconv.FormatInt(id, 10)
			profile, known := cache[mid]
			name, detail := profile.Uname, "UID "+mid
			if name == "" {
				name = "未命名 UP 主"
			}
			if !known {
				name = "正在查询…"
				if failures[mid] != "" {
					name = "资料查询失败"
					detail += " · " + failures[mid]
				}
			}
			controls := []core.Node{}
			if !single {
				controls = append(controls,
					mobileButton{Text: "上移", Small: true, Kind: buttonSecondary, Disabled: func() bool { return i == 0 }, OnPress: func() { selection.move(id, -1) }, TestID: "up-up:" + mid},
					mobileButton{Text: "下移", Small: true, Kind: buttonSecondary, Disabled: func() bool { return i == len(ids)-1 }, OnPress: func() { selection.move(id, 1) }, TestID: "up-down:" + mid})
			}
			controls = append(controls, mobileButton{Text: "移除", Small: true, Kind: buttonDanger, OnPress: func() { selection.remove(id) }, TestID: "up-remove:" + mid})
			rows = append(rows, core.View{Style: core.Style{Gap: 2}, Children: []core.Node{
				mobileRow{Title: name, Detail: detail, Leading: mobileAvatar(cx, name, profile.Face, 40)},
				core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd, Gap: 8, Padding: core.Edges{Right: mobileRowPad, Bottom: 12}}, Children: controls},
			}})
		}
		return core.View{Children: rows}
	})
	return core.View{Children: []core.Node{
		selected,
		mobileRow{Title: "从关注里选", Icon: lucide.Users, Tint: colors.Accent, Accent: true, OnPress: func() { open.Set(true) }},
		mobileInput{Label: "粘贴 UID 或空间链接", Placeholder: "space.bilibili.com/123", Value: paste.Get, OnChange: func(v string) { paste.Set(v); inputError.Set("") }, OnSubmit: func(string) { add() }},
		mobileRow{Title: "添加", Icon: lucide.Plus, Tint: colors.Accent, Accent: true, OnPress: add},
		core.Show{When: func() bool { return inputError.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return mobileRow{Title: inputError.Get(), Danger: true}
		}},
		base.Sheet{Label: "从关注里选", Open: open.Get, OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) }, CloseOnSwipe: true,
			BackdropStyle: core.Style{Background: ui.Black(0.4)}, PopupStyle: core.Style{Height: core.Pct(88), MaxHeight: core.Pct(90), CornerRadii: core.Corners{TopLeft: 16, TopRight: 16}},
			PopupStyleFunc: func() core.Style { return core.Style{Background: colors.Background()} },
			Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
				return a.followingsPicker(cx, selection, func(profile api.UPProfile) {
					cache[profile.Mid] = profile
					revision.Set(revision.GetUntracked() + 1)
				}, func() { open.Set(false) })
			})}},
	}}
}

func (a *mobileApp) followingsPicker(cx *core.Scope, selection mobileUPSelection, remember func(api.UPProfile), close func()) core.Node {
	colors := ui.ColorsOf(cx)
	options := []ui.Option[string]{}
	for _, account := range a.state.GetUntracked().Accounts {
		if account.LoggedIn {
			options = append(options, ui.Option[string]{Value: account.Mid, Label: account.Uname + "（" + account.Mid + "）"})
		}
	}
	if len(options) == 0 {
		return mobilePage(cx, nil, nil, mobileSection(cx, "从关注里选", "",
			mobileRow{Title: "先添加并登录一个账号才能从关注里选，也可以直接粘贴 UID"}), mobileButton{Text: "完成", OnPress: close})
	}
	account := core.NewSignal(cx, options[0].Value)
	query := core.NewSignal(cx, "")
	items := core.NewSignalFunc(cx, []api.UPProfile(nil), nil)
	loading := core.NewSignal(cx, false)
	errText := core.NewSignal(cx, "")
	more := core.NewSignal(cx, false)
	page, failedPage, generation := 0, 1, 0
	activeQuery := ""
	cl, session, epoch := a.session.Client(), a.session, a.epoch
	scopeCtx := cx.Context()
	var cancel context.CancelFunc
	cx.Cleanup(func() {
		generation++
		if cancel != nil {
			cancel()
		}
	})
	load := func(pn int) {
		if pn > 1 && loading.GetUntracked() {
			return
		}
		generation++
		g := generation
		if cancel != nil {
			cancel()
		}
		if pn == 1 {
			items.Set(nil)
			more.Set(false)
			page = 0
			activeQuery = strings.TrimSpace(query.GetUntracked())
		}
		mid, q := account.GetUntracked(), activeQuery
		ctx, stop := context.WithCancel(scopeCtx)
		cancel = stop
		failedPage = pn
		loading.Set(true)
		errText.Set("")
		// 请求按账号节奏排队，不设短超时；关闭选择器会取消。
		go func() {
			result, err := cl.Followings(ctx, mid, pn, q)
			cx.Post(func() {
				if ctx.Err() != nil || g != generation || epoch != a.epoch || session != a.session {
					return
				}
				loading.Set(false)
				if err != nil {
					errText.Set(err.Error())
					return
				}
				list := slices.Clone(items.GetUntracked())
				for _, profile := range result.List {
					if !slices.ContainsFunc(list, func(p api.UPProfile) bool { return p.Mid == profile.Mid }) {
						list = append(list, profile)
					}
				}
				items.Set(list)
				page = pn
				more.Set(result.More)
			})
		}()
	}
	cx.Post(func() { load(1) })
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.Edges{Top: 16}}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, Padding: core.Symmetric(0, 16)}, Children: []core.Node{
			mobileLabel(cx, "从关注里选", 18, ui.Semibold, colors.Text, 1), mobileButton{Text: "完成", Small: true, Kind: buttonTonal, OnPress: close},
		}},
		mobilePage(cx, nil, nil,
			mobileSection(cx, "", "搜索关注的昵称，按最常访问排序。",
				mobileRow{Title: "账号", Control: ui.Select[string]{Label: "选择关注账号", Options: options, Value: account.Get, Presentation: base.PresentSheet, OnChange: func(mid string) { account.Set(mid); load(1) }}},
				mobileInput{Label: "搜索昵称", Placeholder: "输入关注的 UP 主昵称", Value: query.Get, OnChange: query.Set, OnSubmit: func(string) { load(1) }},
				mobileRow{Title: "搜索", Icon: lucide.Search, Accent: true, OnPress: func() { load(1) }}),
			core.Dynamic(func(cx *core.Scope) core.Node {
				list := items.Get()
				rows := make([]core.Node, 0, len(list)+2)
				for _, profile := range list {
					id, err := strconv.ParseInt(profile.Mid, 10, 64)
					if err != nil || id <= 0 {
						continue
					}
					state := "选择"
					if selection.contains(id) {
						state = "已选择"
					}
					rows = append(rows, mobileRow{Title: profile.Uname, Detail: "UID " + profile.Mid, Value: state, Leading: mobileAvatar(cx, profile.Uname, profile.Face, 40), TestID: "following:" + profile.Mid,
						OnPress: func() {
							remember(profile)
							selection.toggle(id)
							if selection.single {
								close()
							}
						}})
				}
				switch {
				case loading.Get():
					rows = append(rows, mobileRow{Title: "正在加载关注…", Detail: "请稍候，请求会按账号的节奏排队"})
				case errText.Get() != "":
					rows = append(rows, mobileRow{Title: errText.Get(), Danger: true}, mobileRow{Title: "重试", Icon: lucide.RefreshCw, Accent: true, OnPress: func() { load(failedPage) }})
				case len(list) == 0:
					rows = append(rows, mobileRow{Title: "没有找到关注的 UP 主", Detail: "试试其他昵称或切换账号"})
				case more.Get():
					rows = append(rows, mobileRow{Title: "加载更多", Icon: lucide.Plus, Accent: true, OnPress: func() { load(page + 1) }})
				}
				return mobileSection(cx, "关注", "", rows...)
			}),
		),
	}}
}
