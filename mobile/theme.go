package main

import (
	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// mobileTheme 用 B 站粉作强调色：深色用 #fb7299，浅色用更深的 #d63e72 保证白底上的文字对比度。
// 深色的粉色按钮上用深色文字（Material 3 的做法），白字在 #fb7299 上对比度不足。
func mobileTheme() *core.Theme {
	t := core.DefaultTheme()
	l, d := &t.Light, &t.Dark
	l.Background, l.Surface = core.RGB(0xf2f2f6), core.RGB(0xffffff)
	l.Text, l.TextMuted = core.RGB(0x18181b), core.RGB(0x6b6b74)
	l.Border = core.RGBA(0x18181b, 0.12)
	l.Accent, l.AccentHover, l.AccentPressed, l.AccentText = core.RGB(0xd63e72), core.RGB(0xc63566), core.RGB(0xb02d5a), core.RGB(0xffffff)
	l.Success, l.Warning, l.Danger = core.RGB(0x16803c), core.RGB(0xb45309), core.RGB(0xc8333f)
	l.Selection, l.Focus = core.RGBA(0xd63e72, 0.22), core.RGBA(0xd63e72, 0.55)
	d.Background, d.Surface = core.RGB(0x0f0f11), core.RGB(0x1c1c1f)
	d.Text, d.TextMuted = core.RGB(0xf2f2f4), core.RGB(0xa1a1aa)
	d.Border = core.RGBA(0xffffff, 0.10)
	d.Accent, d.AccentHover, d.AccentPressed, d.AccentText = core.RGB(0xfb7299), core.RGB(0xfc85a7), core.RGB(0xe8638a), core.RGB(0x2b0a17)
	d.Success, d.Warning, d.Danger = core.RGB(0x4ade80), core.RGB(0xfbbf24), core.RGB(0xf87171)
	d.Selection, d.Focus = core.RGBA(0xfb7299, 0.26), core.RGBA(0xfb7299, 0.7)
	for name, c := range map[string]core.Color{"accentFill": l.Accent, "field": core.RGB(0xf4f4f7), "fieldBorder": l.Border, "inset": core.RGB(0xeaeaef), "line": core.RGBA(0x18181b, 0.09), "faint": core.RGBA(0x18181b, 0.36)} {
		l.SetColor(name, c)
	}
	for name, c := range map[string]core.Color{"accentFill": d.Accent, "field": core.RGB(0x26262a), "fieldBorder": d.Border, "inset": core.RGB(0x2a2a2e), "line": core.RGBA(0xffffff, 0.08), "faint": core.RGBA(0xffffff, 0.36)} {
		d.SetColor(name, c)
	}
	t.Radius = 10
	return t
}

// 尺寸：触控目标至少 44pt。
const (
	mobileRowHeight core.Points = 50
	mobileRowPad    core.Points = 16
	mobileRadius    core.Points = 14
)

// mobileTint 是强调色或状态色的浅底。
func mobileTint(cx *core.Scope, c func() core.Color, amount float32) func() core.Color {
	colors := ui.ColorsOf(cx)
	return func() core.Color { return ui.Mix(colors.Surface(), c(), amount) }
}

func mobileLabel(cx *core.Scope, text string, size core.Points, weight core.FontWeight, color func() core.Color, lines int) core.Text {
	t := ui.Text(text, size, weight, color)
	t.Style.LineHeight = size + 5
	t.Style.FlexShrink, t.Style.MinWidth = 1, core.Pt(0)
	if lines > 0 {
		t.MaxLines, t.EllipsizeMode = lines, core.EllipsizeTail
	}
	return t
}

func mobileText(cx *core.Scope, text string) core.Node {
	return mobileLabel(cx, text, 14, ui.Regular, ui.ColorsOf(cx).Muted, 0)
}

// mobilePage 是可滚动的页面内容：两侧留白、底部让开键盘和悬浮的 iOS 标签栏。
func mobilePage(cx *core.Scope, refresh func(), refreshing func() bool, children ...core.Node) core.Node {
	content := core.View{StyleFunc: func() core.Style {
		return core.Style{Padding: core.Edges{Top: 12, Right: 16, Bottom: 32 + ui.TabBarInset(cx), Left: 16}, Gap: 20,
			PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}}
	}, Children: children}
	return core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, OnRefresh: refresh, Refreshing: refreshing,
		Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissOnDrag, KeyboardShouldPersistTaps: core.KeyboardTapsHandled},
		Children: []core.Node{content}}
}

func mobileColumn(children ...core.Node) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node { return mobilePage(cx, nil, nil, children...) })
}

// mobileCard 是一张圆角卡片。
func mobileCard(cx *core.Scope, style core.Style, children ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{CornerRadius: mobileRadius, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden}.Merge(style),
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }, Children: children}
}

// mobileSection 是 iOS 设置式的分组：标题、卡片里的行和细分隔线、脚注。
func mobileSection(cx *core.Scope, title, footer string, rows ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	var body []core.Node
	for _, r := range rows {
		if r == nil {
			continue
		}
		if len(body) > 0 {
			body = append(body, core.View{Style: core.Style{Height: core.Pt(1), Margin: core.Edges{Left: mobileRowPad}}, StyleFunc: ui.Filled(colors.Line)})
		}
		body = append(body, r)
	}
	var nodes []core.Node
	if title != "" {
		t := mobileLabel(cx, title, 13, ui.Medium, colors.Muted, 1)
		t.Role = core.RoleHeader
		t.Style.Padding = core.Symmetric(0, mobileRowPad)
		nodes = append(nodes, t)
	}
	nodes = append(nodes, mobileCard(cx, core.Style{}, body...))
	if footer != "" {
		f := mobileLabel(cx, footer, 12, ui.Regular, colors.Muted, 0)
		f.Style.Padding = core.Symmetric(0, mobileRowPad)
		nodes = append(nodes, f)
	}
	return core.View{Style: core.Style{Gap: 7}, Children: nodes}
}

// mobileRow 是分组里的一行：可选的图标、标题和说明、右侧的值或控件；有 OnPress 时可点。
type mobileRow struct {
	Icon     func() *core.SVG
	Tint     func() core.Color
	Title    string
	Detail   string
	Lines    int // 说明的最多行数，0 为不限
	Value    string
	Control  core.Node
	Leading  core.Node
	OnPress  func()
	Disabled func() bool
	Accent   bool
	Danger   bool
	Chevron  bool
	Label    string
	TestID   string
}

func (r mobileRow) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	titleColor := colors.Text
	switch {
	case r.Danger:
		titleColor = colors.Danger
	case r.Accent:
		titleColor = colors.Accent
	}
	var children []core.Node
	if r.Leading != nil {
		children = append(children, r.Leading)
	} else if r.Icon != nil {
		tint := r.Tint
		if tint == nil {
			tint = titleColor
		}
		children = append(children, core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), CornerRadius: 8, BorderCurve: core.BorderCurveContinuous,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
			StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), tint(), 0.14)} },
			Children:  []core.Node{ui.Icon(r.Icon, 17, tint)}})
	}
	texts := []core.Node{mobileLabel(cx, r.Title, 16, ui.Regular, titleColor, 2)}
	if r.Detail != "" {
		texts = append(texts, mobileLabel(cx, r.Detail, 13, ui.Regular, colors.Muted, r.Lines))
	}
	children = append(children, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts})
	if r.Value != "" {
		v := mobileLabel(cx, r.Value, 15, ui.Regular, colors.Muted, 1)
		v.Style.MaxWidth = core.Pct(55)
		v.Style.TextAlign = core.TextAlignRight
		children = append(children, v)
	}
	if r.Control != nil {
		children = append(children, core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{r.Control}})
	}
	if r.OnPress != nil && (r.Chevron || !r.Accent && !r.Danger) {
		children = append(children, ui.Icon(lucide.ChevronRight, 18, colors.Faint))
	}
	style := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(mobileRowHeight),
		Padding: core.Symmetric(10, mobileRowPad)}
	if r.OnPress == nil {
		return core.View{TestID: r.TestID, Style: style, Children: children}
	}
	label := r.Label
	if label == "" {
		label = r.Title
	}
	style.Disabled = &core.Style{Opacity: core.Alpha(0.45)}
	return base.Button{Label: label, TestID: r.TestID, Disabled: r.Disabled, OnPress: func(core.PressEvent) { r.OnPress() }, Style: style,
		StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} }, Children: children}
}

// mobileButtonKind 是按钮的样式：主要（粉底）、浅粉、灰底、危险。
type mobileButtonKind uint8

const (
	buttonPrimary mobileButtonKind = iota
	buttonTonal
	buttonSecondary
	buttonDanger
)

// mobileInput 是分组里的输入行：短值（数字、时间）在标题右侧，长值在标题下方。
type mobileInput struct {
	Label       string
	Detail      string
	Placeholder string
	TestID      string
	Value       func() string
	OnChange    func(string)
	OnSubmit    func(string)
	Keyboard    core.KeyboardType
	Masked      bool
	Inline      bool
	Disabled    func() bool
}

func (f mobileInput) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	focused := core.NewSignal(cx, false)
	field := core.Input{Label: f.Label, TestID: f.TestID, Value: f.Value, OnChange: f.OnChange, Placeholder: f.Placeholder, KeyboardType: f.Keyboard,
		Masked: f.Masked, Disabled: f.Disabled, NoAutoCorrect: true, AutoCapitalize: core.AutoCapitalizeNone,
		OnSubmit: f.OnSubmit,
		OnFocus:  func() { focused.Set(true) }, OnBlur: func() { focused.Set(false) },
		StyleFunc: func() core.Style {
			s := core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 12), FontSize: 16, Color: colors.Text(), PlaceholderColor: colors.Faint(),
				CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection, Background: colors.Field(), CornerRadius: 10,
				BorderWidth: 1, BorderColor: core.Transparent}
			if focused.Get() {
				s.BorderColor = colors.Accent()
			}
			if f.Inline {
				s.Width, s.TextAlign = core.Pt(120), core.TextAlignRight
			}
			return s
		}}
	texts := []core.Node{mobileLabel(cx, f.Label, 16, ui.Regular, colors.Text, 2)}
	if f.Detail != "" {
		texts = append(texts, mobileLabel(cx, f.Detail, 13, ui.Regular, colors.Muted, 0))
	}
	head := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts}
	if f.Inline {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(mobileRowHeight), Padding: core.Symmetric(8, mobileRowPad)},
			Children: []core.Node{head, field}}
	}
	return core.View{Style: core.Style{Gap: 8, Padding: core.Symmetric(12, mobileRowPad)}, Children: []core.Node{head, field}}
}

// mobileButton 是手机尺寸的按钮：默认 48pt 高，Small 为 36pt 胶囊。
type mobileButton struct {
	Text     string
	Icon     func() *core.SVG
	Kind     mobileButtonKind
	Small    bool
	Flex     float32
	Disabled func() bool
	OnPress  func()
	TestID   string
}

func (b mobileButton) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	fg, bg := colors.OnAccent, colors.Accent
	switch b.Kind {
	case buttonTonal:
		fg, bg = colors.Accent, func() core.Color { return ui.Mix(colors.Surface(), colors.Accent(), 0.15) }
	case buttonSecondary:
		fg, bg = colors.Text, func() core.Color { return ui.Mix(colors.Surface(), colors.Text(), 0.08) }
	case buttonDanger:
		fg, bg = colors.Danger, func() core.Color { return ui.Mix(colors.Surface(), colors.Danger(), 0.13) }
	}
	height, size, radius := core.Points(48), core.Points(16), core.Points(12)
	if b.Small {
		height, size, radius = 36, 14, 18
	}
	var children []core.Node
	if b.Icon != nil {
		children = append(children, ui.Icon(b.Icon, size+1, fg))
	}
	text := ui.Text(b.Text, size, ui.Semibold, fg)
	text.MaxLines, text.EllipsizeMode = 1, core.EllipsizeTail
	text.Style.FlexShrink = 1
	children = append(children, text)
	return base.Button{Label: b.Text, TestID: b.TestID, Disabled: b.Disabled, OnPress: func(core.PressEvent) {
		if b.OnPress != nil {
			b.OnPress()
		}
	},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6,
			Height: core.Pt(height), Padding: core.Symmetric(0, 16), CornerRadius: radius, BorderCurve: core.BorderCurveContinuous,
			FlexGrow: b.Flex, FlexShrink: b.Flex, Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
		StyleFunc: func() core.Style {
			return core.Style{Background: bg(), Active: &core.Style{Opacity: core.Alpha(0.8)}}
		}, Children: children}
}

func (a *mobileApp) button(text string, fn func()) core.Node {
	return mobileButton{Text: text, OnPress: fn, Disabled: a.isBusy}
}

// mobileBadge 是一个小胶囊标签。
func mobileBadge(cx *core.Scope, text string, color func() core.Color) core.Node {
	return core.View{Style: core.Style{Padding: core.Symmetric(2, 7), CornerRadius: 6, FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(ui.ColorsOf(cx).Surface(), color(), 0.16)} },
		Children:  []core.Node{ui.Text(text, 11, ui.Semibold, color)}}
}

// mobileAvatar 是头像：有图片时显示图片，否则显示名字的首字。
func mobileAvatar(cx *core.Scope, name, face string, size core.Points) core.Node {
	colors := ui.ColorsOf(cx)
	initial := "?"
	for _, r := range name {
		initial = string(r)
		break
	}
	children := []core.Node{ui.Text(initial, size*0.42, ui.Semibold, colors.Accent)}
	if face != "" {
		children = append(children, core.Image{Source: core.Src(face), ResizeMode: core.Cover,
			Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2}})
	}
	return core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, FlexShrink: 0, Overflow: core.Hidden,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), colors.Accent(), 0.16)} }, Children: children}
}

// mobileBar 是细进度条。
func mobileBar(cx *core.Scope, value float32, color func() core.Color) core.Node {
	colors := ui.ColorsOf(cx)
	value = min(max(value, 0), 1)
	return core.View{Style: core.Style{Height: core.Pt(4), CornerRadius: 2, Overflow: core.Hidden, FlexGrow: 1},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), colors.Text(), 0.1)} },
		Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(4), CornerRadius: 2, Width: core.Pct(value * 100)},
			StyleFunc: func() core.Style { return core.Style{Background: color()} }}}}
}

// mobileStatusLook 是状态的图标和颜色。
func mobileStatusLook(cx *core.Scope, status api.Status) (func() *core.SVG, func() core.Color) {
	colors := ui.ColorsOf(cx)
	switch status {
	case api.StatusOK:
		return lucide.CircleCheck, colors.Up
	case api.StatusPartial:
		return lucide.CircleAlert, colors.Warn
	case api.StatusFailed:
		return lucide.CircleX, colors.Danger
	case api.StatusSkipped:
		return lucide.CircleMinus, colors.Muted
	case api.StatusRunning:
		return lucide.LoaderCircle, colors.Accent
	}
	return lucide.CircleDashed, colors.Faint
}

func mobileStatusIcon(cx *core.Scope, status api.Status, size core.Points) core.Node {
	icon, color := mobileStatusLook(cx, status)
	return ui.Icon(icon, size, color)
}

// mobileStatusChip 是图标加文字的状态标签。
func mobileStatusChip(cx *core.Scope, status api.Status) core.Node {
	icon, color := mobileStatusLook(cx, status)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Padding: core.Symmetric(3, 8), CornerRadius: 10, FlexShrink: 0, AlignSelf: core.AlignStart},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(ui.ColorsOf(cx).Surface(), color(), 0.14)} },
		Children:  []core.Node{ui.Icon(icon, 13, color), ui.Text(client.StatusShort(status), 12, ui.Semibold, color)}}
}

// mobileBanner 显示连接或刷新错误。
func (a *mobileApp) banner(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return a.errorText.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		text := ui.Textf(a.errorText.Get, 14, ui.Medium, colors.Danger)
		text.Style.FlexShrink, text.Style.MinWidth, text.Style.LineHeight = 1, core.Pt(0), 20
		return core.View{Style: core.Style{Direction: core.Row, Gap: 10, Padding: core.Symmetric(12, 14), CornerRadius: 12, AlignItems: core.AlignStart},
			StyleFunc: func() core.Style { return core.Style{Background: ui.Mix(colors.Surface(), colors.Danger(), 0.12)} },
			Children:  []core.Node{ui.Icon(lucide.TriangleAlert, 18, colors.Danger), text}}
	}}
}
