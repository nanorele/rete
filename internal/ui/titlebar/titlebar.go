package titlebar

import (
	"image"
	"time"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type Bar struct {
	BtnClose     widget.Clickable
	BtnMinimize  widget.Clickable
	BtnMaximize  widget.Clickable
	SettingsBtn  widget.Clickable
	BugReportBtn widget.Clickable

	NetActive    bool
	NetPaused    bool
	BtnNetToggle widget.Clickable
	BtnNetCancel widget.Clickable
	OnNetToggle  func()
	OnNetCancel  func()

	titleTag  struct{}
	lastClick time.Time
	Maximized bool
}

func (b *Bar) layoutBtn(gtx layout.Context, th *material.Theme, btn *widget.Clickable, kind int) layout.Dimensions {
	btnSize := image.Point{X: gtx.Dp(unit.Dp(46)), Y: gtx.Dp(unit.Dp(30))}
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := theme.Bg
		fg := th.Fg

		if btn.Hovered() {
			bg = theme.BgHover
			if kind == 3 {
				bg = theme.CloseHover
				fg = theme.White
			}
		}

		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: btnSize}.Op())
		pointer.CursorPointer.Add(gtx.Ops)

		cx := float32(int(float32(btnSize.X)/2)) + 0.5
		cy := float32(int(float32(btnSize.Y)/2)) + 0.5

		rectPath := func(ops *op.Ops, x, y, s float32) clip.PathSpec {
			var p clip.Path
			p.Begin(ops)
			p.MoveTo(f32.Pt(x, y))
			p.LineTo(f32.Pt(x+s, y))
			p.LineTo(f32.Pt(x+s, y+s))
			p.LineTo(f32.Pt(x, y+s))
			p.Close()
			return p.End()
		}

		switch kind {
		case 0:
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(cx-5, cy))
			p.LineTo(f32.Pt(cx+5, cy))
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: p.End(), Width: 1}.Op())
		case 1:
			s := float32(8)
			x := cx - 4
			y := cy - 4
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: rectPath(gtx.Ops, x, y, s), Width: 1}.Op())
		case 2:
			s := float32(7)
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: rectPath(gtx.Ops, cx-1, cy-4, s), Width: 1}.Op())
			paint.FillShape(gtx.Ops, bg, clip.Rect{
				Min: image.Pt(int(cx-4)-1, int(cy-1)-1),
				Max: image.Pt(int(cx-4+s)+2, int(cy-1+s)+2),
			}.Op())
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: rectPath(gtx.Ops, cx-4, cy-1, s), Width: 1}.Op())
		case 3:
			s := float32(10)
			x := cx - 5
			y := cy - 5
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(x, y))
			p.LineTo(f32.Pt(x+s, y+s))
			p.MoveTo(f32.Pt(x+s, y))
			p.LineTo(f32.Pt(x, y+s))
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: p.End(), Width: 1}.Op())
		}

		return layout.Dimensions{Size: btnSize}
	})
}

func (b *Bar) layoutSettingsBtn(gtx layout.Context, th *material.Theme, win *app.Window, settingsActive bool, onToggle func()) layout.Dimensions {
	btnSize := image.Pt(gtx.Dp(unit.Dp(100)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	for b.SettingsBtn.Clicked(gtx) {
		if onToggle != nil {
			onToggle()
		}
		if win != nil {
			win.Invalidate()
		}
	}

	col := theme.FgMuted
	if settingsActive {
		col = theme.Accent
	}

	return b.SettingsBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {

		areaStack := clip.Rect{Max: btnSize}.Push(gtx.Ops)
		system.ActionInputOp(system.ActionRaise).Add(gtx.Ops)
		areaStack.Pop()

		bg := theme.Bg
		if b.SettingsBtn.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: btnSize}.Op())
		pointer.CursorPointer.Add(gtx.Ops)

		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(16))
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return widgets.IconSettings.Layout(gtx, col)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), "Settings")
					lbl.MaxLines = 1
					lbl.Color = col
					return lbl.Layout(gtx)
				}),
			)
		})

		return layout.Dimensions{Size: btnSize}
	})
}

func (b *Bar) layoutBugBtn(gtx layout.Context, th *material.Theme, bugReportURL string) layout.Dimensions {
	btnSize := image.Pt(gtx.Dp(unit.Dp(100)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints.Min = btnSize
	gtx.Constraints.Max = btnSize

	for b.BugReportBtn.Clicked(gtx) {
		if bugReportURL != "" {
			go workspace.OpenFile(bugReportURL)
		}
	}

	return b.BugReportBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {

		areaStack := clip.Rect{Max: btnSize}.Push(gtx.Ops)
		system.ActionInputOp(system.ActionRaise).Add(gtx.Ops)
		areaStack.Pop()

		bg := theme.Bg
		if b.BugReportBtn.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: btnSize}.Op())
		pointer.CursorPointer.Add(gtx.Ops)

		col := theme.FgMuted
		if b.BugReportBtn.Hovered() {
			col = theme.Danger
		}

		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(16))
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return widgets.IconBug.Layout(gtx, col)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), "Report Bug")
					lbl.MaxLines = 1
					lbl.Color = col
					return lbl.Layout(gtx)
				}),
			)
		})

		return layout.Dimensions{Size: btnSize}
	})
}

func (b *Bar) Layout(gtx layout.Context, th *material.Theme, win *app.Window, title string, bugReportURL string, settingsActive bool, onToggleSettings func()) layout.Dimensions {
	height := gtx.Dp(unit.Dp(30))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	totalW := gtx.Constraints.Max.X

	paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: image.Point{X: totalW, Y: height}}.Op())

	if win != nil {
		if b.BtnClose.Clicked(gtx) {
			win.Perform(system.ActionClose)
		}
		if b.BtnMinimize.Clicked(gtx) {
			win.Perform(system.ActionMinimize)
		}
		if b.BtnMaximize.Clicked(gtx) {
			if b.Maximized {
				win.Perform(system.ActionUnmaximize)
				b.Maximized = false
			} else {
				win.Perform(system.ActionMaximize)
				b.Maximized = true
			}
		}
	}

	if b.BtnNetToggle.Clicked(gtx) && b.OnNetToggle != nil {
		b.OnNetToggle()
	}
	if b.BtnNetCancel.Clicked(gtx) && b.OnNetCancel != nil {
		b.OnNetCancel()
	}

	btnW := gtx.Dp(unit.Dp(46))
	const numBtns = 3
	rowW := btnW * numBtns
	bugBtnW := gtx.Dp(unit.Dp(100))
	minimizeStartX := totalW - rowW
	bugStartX := max(minimizeStartX-bugBtnW, 0)
	leftMaxW := bugStartX

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &b.titleTag,
			Kinds:  pointer.Press | pointer.Drag,
		})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons == pointer.ButtonPrimary {
			if e.Kind == pointer.Press {
				now := time.Now()
				if now.Sub(b.lastClick) <= 300*time.Millisecond && win != nil {
					if b.Maximized {
						win.Perform(system.ActionUnmaximize)
						b.Maximized = false
					} else {
						win.Perform(system.ActionMaximize)
						b.Maximized = true
					}
					b.lastClick = time.Time{}
				} else {
					b.lastClick = now
				}
			} else if e.Kind == pointer.Drag && win != nil {
				win.Perform(system.ActionMove)
			}
		}
	}

	if leftMaxW > 0 {
		labelLeftPad := gtx.Dp(unit.Dp(12))
		gap := gtx.Dp(unit.Dp(8))

		if title == "" {
			title = "Tracto"
		}

		labelMacro := op.Record(gtx.Ops)
		labelGtx := gtx
		labelGtx.Constraints.Min = image.Pt(0, height)
		labelGtx.Constraints.Max = image.Pt(leftMaxW, height)
		labelDim := layout.Center.Layout(labelGtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), title)
			lbl.Font.Typeface = ""
			lbl.MaxLines = 1
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		})
		labelCall := labelMacro.Stop()

		settingsX := labelLeftPad + labelDim.Size.X + gap
		settingsW := gtx.Dp(unit.Dp(100))
		settingsEndX := min(settingsX+settingsW, leftMaxW)

		if settingsX > 0 {
			dragSize := image.Point{X: settingsX, Y: height}
			area := clip.Rect{Max: dragSize}.Push(gtx.Ops)
			event.Op(gtx.Ops, &b.titleTag)
			area.Pop()
		}

		labelOff := op.Offset(image.Pt(labelLeftPad, 0)).Push(gtx.Ops)
		labelCall.Add(gtx.Ops)
		labelOff.Pop()

		settingsOff := op.Offset(image.Pt(settingsX, 0)).Push(gtx.Ops)
		b.layoutSettingsBtn(gtx, th, win, settingsActive, onToggleSettings)
		settingsOff.Pop()

		if leftMaxW > settingsEndX {
			midStart := settingsEndX
			midEnd := leftMaxW

			var badgeCall op.CallOp
			badgeW := 0
			if b.NetActive || b.NetPaused {
				badgeCall, badgeW = b.recordNetBadge(gtx, th, height)
			}

			if badgeW > 0 && midEnd-midStart > badgeW {
				badgeX := midStart + (midEnd-midStart-badgeW)/2
				b.dragZone(gtx, midStart, badgeX-midStart, height)
				bo := op.Offset(image.Pt(badgeX, 0)).Push(gtx.Ops)
				badgeCall.Add(gtx.Ops)
				bo.Pop()
				b.dragZone(gtx, badgeX+badgeW, midEnd-(badgeX+badgeW), height)
			} else {
				b.dragZone(gtx, midStart, midEnd-midStart, height)
			}
		}
	}

	if bugStartX < minimizeStartX {
		bugOff := op.Offset(image.Pt(bugStartX, 0)).Push(gtx.Ops)
		b.layoutBugBtn(gtx, th, bugReportURL)
		bugOff.Pop()
	}

	maxKind := 1
	if b.Maximized {
		maxKind = 2
	}
	btns := [numBtns]struct {
		btn  *widget.Clickable
		kind int
	}{
		{&b.BtnMinimize, 0},
		{&b.BtnMaximize, maxKind},
		{&b.BtnClose, 3},
	}
	for i, bb := range btns {
		off := op.Offset(image.Pt(minimizeStartX+i*btnW, 0)).Push(gtx.Ops)
		b.layoutBtn(gtx, th, bb.btn, bb.kind)
		off.Pop()
	}

	return layout.Dimensions{Size: image.Point{X: totalW, Y: height}}
}

func (b *Bar) dragZone(gtx layout.Context, x, w, h int) {
	if w <= 0 {
		return
	}
	dragOff := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	area := clip.Rect{Max: image.Point{X: w, Y: h}}.Push(gtx.Ops)
	event.Op(gtx.Ops, &b.titleTag)
	area.Pop()
	dragOff.Pop()
}

func (b *Bar) recordNetBadge(gtx layout.Context, th *material.Theme, height int) (op.CallOp, int) {
	macro := op.Record(gtx.Ops)
	g := gtx
	g.Constraints.Min = image.Pt(0, height)
	g.Constraints.Max = image.Pt(1<<20, height)
	dim := b.layoutNetBadge(g, th)
	return macro.Stop(), dim.Size.X
}

func (b *Bar) layoutNetBadge(gtx layout.Context, th *material.Theme) layout.Dimensions {
	dotCol := theme.Accent
	label := "network limit: active"
	toggleIcon := widgets.IconPause
	if b.NetPaused {
		dotCol = theme.FgMuted
		label = "network limit: paused"
		toggleIcon = widgets.IconPlay
	}

	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						d := gtx.Dp(unit.Dp(8))
						sz := image.Pt(d, d)
						paint.FillShape(gtx.Ops, dotCol, clip.Ellipse{Max: sz}.Op(gtx.Ops))
						return layout.Dimensions{Size: sz}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(7)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(12), label)
						lbl.MaxLines = 1
						lbl.Color = theme.Fg
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.netIconBtn(gtx, &b.BtnNetToggle, toggleIcon, false)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.netIconBtn(gtx, &b.BtnNetCancel, widgets.IconClose, true)
					}),
				)
			})
		}),
	)
}

func (b *Bar) netIconBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, danger bool) layout.Dimensions {
	sz := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
	gtx.Constraints.Min = sz
	gtx.Constraints.Max = sz
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if clk.Hovered() {
			bg := theme.BgHover
			if danger {
				bg = theme.CloseHover
			}
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		}
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			d := gtx.Dp(unit.Dp(14))
			gtx.Constraints.Min = image.Pt(d, d)
			gtx.Constraints.Max = gtx.Constraints.Min
			col := theme.FgMuted
			if clk.Hovered() {
				col = theme.Fg
				if danger {
					col = theme.White
				}
			}
			return ic.Layout(gtx, col)
		})
	})
}
