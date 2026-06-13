package flow

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

var ops = []string{">", ">=", "==", "!=", "<=", "<"}

var valueOps = []string{"==", "!=", "contains", ">", ">=", "<", "<="}

type paletteItem struct {
	kind  NodeKind
	icon  *widget.Icon
	title string
	desc  string
}

func (ed *Editor) layoutPanel(gtx layout.Context, th *material.Theme, host *Host) layout.Dimensions {
	paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	ed.panelCompact = gtx.Constraints.Max.X < gtx.Dp(unit.Dp(170))

	for ed.BtnWidgets.Clicked(gtx) {
		ed.mode = modeWidgets
	}
	for ed.BtnProps.Clicked(gtx) {
		ed.mode = modeProps
	}
	for ed.BtnHistory.Clicked(gtx) {
		ed.mode = modeHistory
	}
	for ed.BtnRun.Clicked(gtx) {
		ed.ToggleRun(host)
	}
	for ed.BtnStep.Clicked(gtx) {
		ed.Runner.Step()
	}
	for ed.BtnStepMode.Clicked(gtx) {
		ed.Runner.SetStepMode(!ed.Runner.StepMode())
	}
	for ed.BtnSave.Clicked(gtx) {
		ed.SaveScenario()
	}
	for ed.BtnNew.Clicked(gtx) {
		ed.CreateNew()
	}

	divider := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(unit.Dp(1))
		paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, h)}.Op())
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
	})
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			pad := unit.Dp(10)
			if ed.panelCompact {
				pad = unit.Dp(6)
			}
			return layout.UniformInset(pad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ed.layoutRunBlock(gtx, th)
			})
		}),
		divider,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ed.layoutModeToggle(gtx, th)
		}),
		divider,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, &ed.panelList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				inset := layout.Inset{Left: unit.Dp(10), Right: unit.Dp(14), Bottom: unit.Dp(12)}
				if ed.panelCompact {
					inset = layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Bottom: unit.Dp(12)}
				}
				return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					switch ed.mode {
					case modeWidgets:
						return ed.layoutPalette(gtx, th)
					case modeHistory:
						return ed.layoutHistory(gtx, th)
					default:
						return ed.layoutProps(gtx, th)
					}
				})
			})
		}),
	)
}

func (ed *Editor) layoutModeToggle(gtx layout.Context, th *material.Theme) layout.Dimensions {
	tab := func(gtx layout.Context, clk *widget.Clickable, title string, ic *widget.Icon, active bool) layout.Dimensions {
		return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := theme.Bg
			if active || clk.Hovered() {
				bg = theme.BgHover
			}
			sz := image.Pt(gtx.Constraints.Min.X, gtx.Dp(unit.Dp(34)))
			gtx.Constraints.Min = sz
			paint.FillShape(gtx.Ops, bg, clip.Rect{Max: sz}.Op())
			if active {
				ih := gtx.Dp(unit.Dp(2))
				paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Min: image.Pt(0, sz.Y-ih), Max: sz}.Op())
			}
			col := theme.FgMuted
			if active {
				col = theme.Fg
			}
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if ed.panelCompact {
					s := gtx.Dp(unit.Dp(16))
					gtx.Constraints.Min = image.Pt(s, s)
					gtx.Constraints.Max = gtx.Constraints.Min
					return ic.Layout(gtx, col)
				}
				lbl := material.Label(th, unit.Sp(12), title)
				lbl.Color = col
				return lbl.Layout(gtx)
			})
		})
	}
	labels := [3]string{"Widgets", "Properties", "History"}
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(210)) {
		labels = [3]string{"Add", "Props", "Runs"}
	}
	third := gtx.Constraints.Max.X / 3
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = third
			gtx.Constraints.Max.X = third
			return tab(gtx, &ed.BtnWidgets, labels[0], widgets.IconAdd, ed.mode == modeWidgets)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = third
			gtx.Constraints.Max.X = third
			return tab(gtx, &ed.BtnProps, labels[1], widgets.IconTune, ed.mode == modeProps)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return tab(gtx, &ed.BtnHistory, labels[2], widgets.IconHistory, ed.mode == modeHistory)
		}),
	)
}

var statusShortener = strings.NewReplacer(
	"Finished with errors", "errors",
	"Finished", "done",
	"Stopped: step limit reached", "step limit",
	"Stopped", "stopped",
	"Running...", "running",
	"Paused · ", "paused ",
	" ok", "✓",
	" failed", "✗",
	" · ", " ",
)

func (ed *Editor) statusLine(gtx layout.Context, th *material.Theme) layout.Dimensions {
	status := ed.Runner.Status()
	note := ed.note
	sp := unit.Sp(11)
	if ed.panelCompact {
		status = statusShortener.Replace(status)
		sp = unit.Sp(10)
	}
	var children []layout.FlexChild
	add := func(txt string, col color.NRGBA) {
		if txt == "" {
			return
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, sp, txt)
				lbl.Color = col
				return lbl.Layout(gtx)
			})
		}))
	}
	add(status, theme.FgMuted)
	noteCol := theme.FgMuted
	if strings.HasPrefix(note, "⚠") {
		noteCol = color.NRGBA{R: 235, G: 180, B: 60, A: 255}
	}
	add(note, noteCol)
	if len(children) == 0 {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ed *Editor) squareIconBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, s int, bg, fg color.NRGBA) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if clk.Hovered() {
			bg = theme.Mix(bg, theme.Fg, 0.12)
		}
		rect := image.Rectangle{Max: image.Pt(s, s)}
		rr := gtx.Dp(unit.Dp(5))
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, rr).Op(gtx.Ops))
		isz := s * 3 / 5
		off := (s - isz) / 2
		defer op.Offset(image.Pt(off, off)).Push(gtx.Ops).Pop()
		gtx.Constraints.Min = image.Pt(isz, isz)
		gtx.Constraints.Max = gtx.Constraints.Min
		ic.Layout(gtx, fg)
		return layout.Dimensions{Size: rect.Max}
	})
}

func (ed *Editor) layoutRunBlockCompact(gtx layout.Context, th *material.Theme, running bool) layout.Dimensions {
	gap := gtx.Dp(unit.Dp(6))
	avail := gtx.Constraints.Max.X
	count := 4
	if running && ed.Runner.StepMode() {
		count = 5
	}
	s := (avail - gap*(count-1)) / count
	axis := layout.Horizontal
	if s < gtx.Dp(unit.Dp(22)) {
		axis = layout.Vertical
		s = avail
	}
	if max := gtx.Dp(unit.Dp(38)); s > max {
		s = max
	}
	runIc := widgets.IconPlay
	runBg := color.NRGBA{R: 46, G: 140, B: 80, A: 255}
	if running {
		runIc = widgets.IconStop
		runBg = theme.Danger
	}
	stepMode := ed.Runner.StepMode()
	paused := ed.Runner.Paused()
	var btns []layout.FlexChild
	addBtn := func(clk *widget.Clickable, ic *widget.Icon, bg, fg color.NRGBA) {
		if len(btns) > 0 {
			btns = append(btns, layout.Rigid(layout.Spacer{Width: unit.Dp(6), Height: unit.Dp(6)}.Layout))
		}
		btns = append(btns, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ed.squareIconBtn(gtx, clk, ic, s, bg, fg)
		}))
	}
	addBtn(&ed.BtnRun, runIc, runBg, theme.White)
	if running && stepMode {
		bg := theme.BgSecondary
		fg := theme.Fg
		if paused {
			bg = theme.Accent
			fg = theme.AccentFg
		}
		addBtn(&ed.BtnStep, widgets.IconNext, bg, fg)
	}
	if stepMode {
		addBtn(&ed.BtnStepMode, widgets.IconPause, theme.Accent, theme.AccentFg)
	} else {
		addBtn(&ed.BtnStepMode, widgets.IconPause, theme.BgSecondary, theme.FgMuted)
	}
	addBtn(&ed.BtnSave, widgets.IconSave, theme.Accent, theme.AccentFg)
	addBtn(&ed.BtnNew, widgets.IconAddReq, theme.BgSecondary, theme.Fg)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: axis}.Layout(gtx, btns...)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ed.statusLine(gtx, th)
		}),
	)
}

func (ed *Editor) layoutRunBlock(gtx layout.Context, th *material.Theme) layout.Dimensions {
	running := ed.Runner.Running()
	if ed.panelCompact {
		return ed.layoutRunBlockCompact(gtx, th, running)
	}
	stepMode := ed.Runner.StepMode()
	paused := ed.Runner.Paused()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(34))
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					label := "Run scenario"
					bg := color.NRGBA{R: 46, G: 140, B: 80, A: 255}
					if running {
						label = "Stop"
						bg = theme.Danger
					}
					btn := material.Button(th, &ed.BtnRun, label)
					btn.Background = bg
					btn.Color = theme.White
					btn.TextSize = unit.Sp(13)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return btn.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !running || !stepMode {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						bg := theme.BgSecondary
						fg := theme.Fg
						if paused {
							bg = theme.Accent
							fg = theme.AccentFg
						}
						return ed.squareIconBtn(gtx, &ed.BtnStep, widgets.IconNext, s, bg, fg)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						bg := theme.BgSecondary
						fg := theme.FgMuted
						if stepMode {
							bg = theme.Accent
							fg = theme.AccentFg
						}
						return ed.squareIconBtn(gtx, &ed.BtnStepMode, widgets.IconPause, s, bg, fg)
					})
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			saveBtn := func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(th, &ed.BtnSave, "Save")
				btn.Background = theme.Accent
				btn.Color = theme.AccentFg
				btn.TextSize = unit.Sp(12)
				btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
				return btn.Layout(gtx)
			}
			newBtn := func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(th, &ed.BtnNew, "New")
				btn.Background = theme.BgSecondary
				btn.Color = theme.Fg
				btn.TextSize = unit.Sp(12)
				btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
				return btn.Layout(gtx)
			}
			if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(230)) {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ed.borderedEditor(gtx, th, &ed.Scenario.NameEd, "New scenario")
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, saveBtn),
							layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
							layout.Flexed(1, newBtn),
						)
					}),
				)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ed.borderedEditor(gtx, th, &ed.Scenario.NameEd, "New scenario")
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(saveBtn),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(newBtn),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ed.statusLine(gtx, th)
		}),
	)
}

func (ed *Editor) paletteItems() []paletteItem {
	return []paletteItem{
		{KindRequest, widgets.IconRequests, "HTTP Request", "send a request"},
		{KindCondition, widgets.IconSplit, "Condition", "branch by arrow rules"},
		{KindLoop, widgets.IconRefresh, "Loop", "container, repeats its content"},
		{KindDelay, widgets.IconDelay, "Delay", "wait before next step"},
		{KindSetVar, widgets.IconRename, "Set Variable", "store value or response field"},
		{KindNote, widgets.IconBatch, "Note", "annotation, not executed"},
	}
}

func (ed *Editor) handlePaletteItemEvents(gtx layout.Context, i int, it paletteItem) {
	for ed.addBtns[i].Clicked(gtx) {
		if !ed.palDragActive {
			ed.addNode(it.kind)
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ed.palDragTags[i],
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			ed.palDragKind = it.kind
			ed.palDragOn = true
			ed.palDragActive = false
		case pointer.Drag:
			if ed.palDragOn && ed.palDragKind == it.kind {
				if _, over := ed.windowToCanvas(widgets.GlobalPointerPos); over {
					ed.palDragActive = true
				}
			}
		case pointer.Release:
			if ed.palDragOn && ed.palDragKind == it.kind && ed.palDragActive {
				ed.dropKindAtWindow(it.kind, widgets.GlobalPointerPos)
			}
			ed.palDragOn = false
			ed.palDragActive = false
		case pointer.Cancel:
			ed.palDragOn = false
			ed.palDragActive = false
		}
	}
}

func (ed *Editor) paletteGrid(gtx layout.Context, items []paletteItem) []layout.FlexChild {
	gap := gtx.Dp(unit.Dp(6))
	tile := gtx.Dp(unit.Dp(40))
	cols := (gtx.Constraints.Max.X + gap) / (tile + gap)
	if cols < 1 {
		cols = 1
	}
	var rows []layout.FlexChild
	for start := 0; start < len(items); start += cols {
		end := start + cols
		if end > len(items) {
			end = len(items)
		}
		row := make([]layout.FlexChild, 0, (end-start)*2)
		for i := start; i < end; i++ {
			i := i
			it := items[i]
			if i > start {
				row = append(row, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
			}
			row = append(row, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.addBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					bg := theme.BgField
					if ed.addBtns[i].Hovered() {
						bg = theme.BgHover
					}
					rect := image.Rectangle{Max: image.Pt(tile, tile)}
					rr := gtx.Dp(unit.Dp(6))
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, rr).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, theme.Border, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
					stripe := image.Rect(0, rr, gtx.Dp(unit.Dp(3)), tile-rr)
					paint.FillShape(gtx.Ops, kindColor(it.kind), clip.Rect(stripe).Op())
					defer clip.Rect(rect).Push(gtx.Ops).Pop()
					event.Op(gtx.Ops, &ed.palDragTags[i])
					isz := gtx.Dp(unit.Dp(20))
					off := image.Pt((tile-isz)/2, (tile-isz)/2)
					defer op.Offset(off).Push(gtx.Ops).Pop()
					gtx.Constraints.Min = image.Pt(isz, isz)
					gtx.Constraints.Max = gtx.Constraints.Min
					it.icon.Layout(gtx, kindColor(it.kind))
					return layout.Dimensions{Size: rect.Max}
				})
			}))
		}
		rowChildren := row
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, rowChildren...)
			})
		}))
	}
	return rows
}

func (ed *Editor) shortcutsSection(gtx layout.Context, th *material.Theme) []layout.FlexChild {
	lines := []string{
		"Drag from right port — connect nodes",
		"Double-click node — rename",
		"RMB / MMB drag — pan canvas",
		"Scroll — zoom · Ctrl+scroll — zoom ×3",
		"Ctrl+Enter — run · Ctrl+S — save",
		"Pause button — step-by-step run",
		"Del — delete · Esc — cancel",
		"Ctrl+C / V / D — copy / paste / duplicate",
		"Ctrl+Z / Ctrl+Y — undo / redo",
		"Ctrl+A — select all",
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Shortcuts") }),
	}
	for _, line := range lines {
		line := line
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(10), line)
				lbl.Color = theme.FgDim
				return lbl.Layout(gtx)
			})
		}))
	}
	return children
}

func (ed *Editor) layoutPalette(gtx layout.Context, th *material.Theme) layout.Dimensions {
	items := ed.paletteItems()
	for i, it := range items {
		ed.handlePaletteItemEvents(gtx, i, it)
	}
	if ed.panelCompact {
		children := []layout.FlexChild{
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		}
		children = append(children, ed.paletteGrid(gtx, items)...)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(10), "Click to add, or drag onto the canvas. Drag from a right port to connect nodes.")
				lbl.Color = theme.FgDim
				return lbl.Layout(gtx)
			})
		}),
	}
	for i, it := range items {
		i, it := i, it
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ed.addBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					bg := theme.BgField
					if ed.addBtns[i].Hovered() {
						bg = theme.BgHover
					}
					rect := image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(38)))}
					rr := gtx.Dp(unit.Dp(5))
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, rr).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, theme.Border, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
					stripe := image.Rect(0, rr, gtx.Dp(unit.Dp(3)), rect.Max.Y-rr)
					paint.FillShape(gtx.Ops, kindColor(it.kind), clip.Rect(stripe).Op())
					defer clip.Rect(rect).Push(gtx.Ops).Pop()
					event.Op(gtx.Ops, &ed.palDragTags[i])
					return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = rect.Max.Y
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								s := gtx.Dp(unit.Dp(16))
								gtx.Constraints.Min = image.Pt(s, s)
								gtx.Constraints.Max = gtx.Constraints.Min
								return it.icon.Layout(gtx, kindColor(it.kind))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(th, unit.Sp(12), it.title)
										lbl.LineHeightScale = 1.0
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(th, unit.Sp(9), it.desc)
										lbl.Color = theme.FgMuted
										lbl.LineHeightScale = 1.0
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
								)
							}),
						)
					})
				})
			})
		}))
	}
	children = append(children, ed.shortcutsSection(gtx, th)...)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ed *Editor) sectionLabel(gtx layout.Context, th *material.Theme, txt string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), txt)
		lbl.Color = theme.FgMuted
		return lbl.Layout(gtx)
	})
}

func (ed *Editor) borderedEditor(gtx layout.Context, th *material.Theme, e *widget.Editor, hint string) layout.Dimensions {
	rr := gtx.Dp(unit.Dp(4))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(rect, rr).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, theme.Border, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(7)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				me := material.Editor(th, e, hint)
				me.TextSize = unit.Sp(12)
				me.HintColor = theme.FgDim
				return me.Layout(gtx)
			})
		},
	)
}

func (ed *Editor) chipRow(gtx layout.Context, th *material.Theme, btns []*widget.Clickable, labels []string, selected string, onClick func(string)) layout.Dimensions {
	avail := gtx.Constraints.Max.X
	chip := func(i int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return btns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					active := labels[i] == selected
					bg := theme.BgField
					fg := theme.FgMuted
					if active {
						bg = theme.Accent
						fg = theme.AccentFg
					} else if btns[i].Hovered() {
						bg = theme.BgHover
					}
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(th, unit.Sp(11), labels[i])
								lbl.Color = fg
								return lbl.Layout(gtx)
							})
						},
					)
				})
			})
		})
	}
	var rows []layout.FlexChild
	var cur []layout.FlexChild
	used := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		row := cur
		cur = nil
		used = 0
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, row...)
		}))
	}
	for i := range labels {
		i := i
		for btns[i].Clicked(gtx) {
			onClick(labels[i])
		}
		cw := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(11), font.Font{}, labels[i]) + gtx.Dp(unit.Dp(20))
		if used > 0 && used+cw > avail {
			flush()
		}
		cur = append(cur, chip(i))
		used += cw
	}
	flush()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (ed *Editor) radioRow(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		bg := theme.Transparent
		if clk.Hovered() {
			bg = theme.BgHover
		}
		rect := image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(26)))}
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
		gtx.Constraints.Min.Y = rect.Max.Y
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				r := gtx.Dp(unit.Dp(5))
				c := image.Pt(gtx.Dp(unit.Dp(10)), rect.Max.Y/2)
				circ := image.Rect(c.X-r, c.Y-r, c.X+r, c.Y+r)
				col := theme.FgDim
				if active {
					col = theme.Accent
					paint.FillShape(gtx.Ops, col, clip.Ellipse(circ).Op(gtx.Ops))
				}
				paint.FillShape(gtx.Ops, col, clip.Stroke{Path: clip.Ellipse(circ).Path(gtx.Ops), Width: 1.5}.Op())
				return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(20)), rect.Max.Y)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				lbl := material.Label(th, unit.Sp(12), title)
				if active {
					lbl.Color = theme.Fg
				} else {
					lbl.Color = theme.FgMuted
				}
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (ed *Editor) warnBlock(gtx layout.Context, th *material.Theme, n *Node) layout.Dimensions {
	missing := ed.missingVars(n)
	if len(missing) == 0 {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), "⚠ Unknown variables: "+joinComma(missing))
		lbl.Color = color.NRGBA{R: 235, G: 180, B: 60, A: 255}
		return lbl.Layout(gtx)
	})
}

func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += "{{" + s + "}}"
	}
	return out
}

func (ed *Editor) envDropdown(gtx layout.Context, th *material.Theme, n *Node) []layout.FlexChild {
	opts := ed.envOpts
	if len(ed.envBtns) < len(opts) {
		ed.envBtns = make([]widget.Clickable, len(opts))
	}
	for ed.envDropBtn.Clicked(gtx) {
		ed.envDropOpen = !ed.envDropOpen
		if ed.envDropOpen {
			ed.envDropAtY = widgets.GlobalPointerPos.Y
		}
	}
	for i, o := range opts {
		i, o := i, o
		for ed.envBtns[i].Clicked(gtx) {
			if n.EnvID != o.ID {
				ed.pushHistory()
				n.EnvID = o.ID
			}
			ed.envDropOpen = false
		}
	}
	optName := func(o EnvOption) string {
		if o.ID == "" {
			return "Active environment"
		}
		return o.Name
	}
	return []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := ed.envDropBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				rr := gtx.Dp(unit.Dp(4))
				bg := theme.BgField
				if ed.envDropBtn.Hovered() {
					bg = theme.BgHover
				}
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						rect := image.Rectangle{Max: gtx.Constraints.Min}
						bcol := theme.Border
						if ed.envDropOpen {
							bcol = theme.Accent
						}
						paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, rr).Op(gtx.Ops))
						paint.FillShape(gtx.Ops, bcol, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									name := ed.envName(n.EnvID)
									if n.EnvID == "" {
										name = "Active environment"
									}
									lbl := material.Label(th, unit.Sp(12), name)
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									s := gtx.Dp(unit.Dp(16))
									gtx.Constraints.Min = image.Pt(s, s)
									gtx.Constraints.Max = gtx.Constraints.Min
									ic := widgets.IconExpandMore
									if ed.envDropOpen {
										ic = widgets.IconExpandLess
									}
									return ic.Layout(gtx, theme.FgMuted)
								}),
							)
						})
					},
				)
			})
			if ed.envDropOpen && len(opts) > 0 {
				rowH := gtx.Dp(unit.Dp(26))
				menuH := rowH * len(opts)
				if maxH := gtx.Dp(unit.Dp(280)); menuH > maxH {
					menuH = maxH
				}
				offY := dims.Size.Y + gtx.Dp(unit.Dp(2))
				if ed.winH > 0 {
					below := ed.winH - int(ed.envDropAtY) - gtx.Dp(unit.Dp(40))
					if menuH > below && int(ed.envDropAtY) > menuH+gtx.Dp(unit.Dp(48)) {
						offY = -menuH - gtx.Dp(unit.Dp(2))
					}
				}
				macro := op.Record(gtx.Ops)
				op.Offset(image.Pt(0, offY)).Add(gtx.Ops)

				rec := op.Record(gtx.Ops)
				menuGtx := gtx
				menuGtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
				menuGtx.Constraints.Max.Y = menuH
				var rows []layout.FlexChild
				for i, o := range opts {
					i, o := i, o
					rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ed.envBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							sz := image.Pt(gtx.Constraints.Max.X, rowH)
							bg := theme.BgPopup
							switch {
							case n.EnvID == o.ID:
								bg = theme.Mix(theme.BgPopup, theme.Accent, 0.25)
							case ed.envBtns[i].Hovered():
								bg = theme.BgHover
							}
							paint.FillShape(gtx.Ops, bg, clip.Rect{Max: sz}.Op())
							gtx.Constraints.Min.Y = rowH
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									lbl := material.Label(th, unit.Sp(12), optName(o))
									if n.EnvID == o.ID {
										lbl.Color = theme.Fg
									} else {
										lbl.Color = theme.FgMuted
									}
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
							)
						})
					}))
				}
				menuDims := layout.Flex{Axis: layout.Vertical}.Layout(menuGtx, rows...)
				menuCall := rec.Stop()

				sz := menuDims.Size
				rr := gtx.Dp(unit.Dp(5))
				paint.FillShape(gtx.Ops, theme.BorderLight, clip.UniformRRect(image.Rect(-1, -1, sz.X+1, sz.Y+1), rr).Op(gtx.Ops))
				paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: sz}, rr).Op(gtx.Ops))
				cl := clip.UniformRRect(image.Rectangle{Max: sz}, rr).Push(gtx.Ops)
				menuCall.Add(gtx.Ops)
				cl.Pop()

				op.Defer(gtx.Ops, macro.Stop())
			}
			return dims
		}),
	}
}

func (ed *Editor) layoutProps(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if n := ed.selectedNode(); n != nil {
		return ed.layoutNodeProps(gtx, th, n)
	}
	if e := ed.selectedEdge(); e != nil {
		return ed.layoutEdgeProps(gtx, th, e)
	}
	return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(12), "Select a node or an arrow on the canvas")
		lbl.Color = theme.FgDim
		return lbl.Layout(gtx)
	})
}

func (ed *Editor) layoutNodeProps(gtx layout.Context, th *material.Theme, n *Node) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				title := n.Kind.Title()
				if len(ed.selected) > 1 {
					title += " · " + strconv.Itoa(len(ed.selected)) + " selected"
				}
				lbl := material.Label(th, unit.Sp(13), title)
				lbl.Color = kindColor(n.Kind)
				return lbl.Layout(gtx)
			})
		}),
	}

	short := func(full, sh string) string {
		if ed.panelCompact {
			return sh
		}
		return full
	}
	hint := func(txt string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ed.panelCompact {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), txt)
				lbl.Color = theme.FgDim
				return lbl.Layout(gtx)
			})
		})
	}

	if n.Kind != KindStart {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Name") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if ed.focusNameID == n.ID {
					ed.focusNameID = ""
					gtx.Execute(key.FocusCmd{Tag: &n.NameEd})
				}
				return ed.borderedEditor(gtx, th, &n.NameEd, n.Kind.Title())
			}),
		)
	}

	switch n.Kind {
	case KindRequest:
		btns := make([]*widget.Clickable, len(methods))
		for i := range methods {
			btns[i] = &ed.methodBtn[i]
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Method") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.chipRow(gtx, th, btns, methods, n.Method, func(m string) {
					ed.pushHistory()
					n.Method = m
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "URL") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.URLEd, "https://example.com/api or {{base_url}}/path")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.sectionLabel(gtx, th, short("Headers (Key: Value per line)", "Headers"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.HeadersEd, "Content-Type: application/json")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Body") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(90))
				return ed.borderedEditor(gtx, th, &n.BodyEd, "{ }")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.warnBlock(gtx, th, n) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Environment") }),
		)
		children = append(children, ed.envDropdown(gtx, th, n)...)
	case KindLoop:
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Iterations") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.CountEd, "3")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.sectionLabel(gtx, th, short("For each array (overrides iterations)", "For each"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.LoopSrcEd, "$.data.items")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.sectionLabel(gtx, th, short("Delay between iterations, ms", "Delay, ms"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.DelayEd, "0")
			}),
			hint("Loop is a container: nodes placed inside run on every iteration. Arrows from the loop itself fire after all iterations."),
			hint("With an array path, the loop walks the last response's array; inside use {{loop.index}}, {{loop.item}} and {{loop.item.field}}."),
		)
	case KindDelay:
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Delay, ms") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.DelayEd, "1000")
			}),
		)
	case KindSetVar:
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.sectionLabel(gtx, th, short("Variable name", "Name"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.VarNameEd, "token")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.sectionLabel(gtx, th, short("Value ($.path / $header.Name / $status)", "Value"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &n.VarValueEd, "$.data.token, $header.Set-Cookie, $status or literal")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.warnBlock(gtx, th, n) }),
			hint("Stored variables are available as {{name}} in URLs, headers, bodies and conditions of next nodes. $.path reads the last response body, $header.Name a response header, $status the code."),
		)
	case KindNote:
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Text") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(70))
				return ed.borderedEditor(gtx, th, &n.BodyEd, "Note text")
			}),
		)
	case KindCondition:
		children = append(children,
			hint("Routing rules are configured on outgoing arrows: select an arrow and set its condition."),
		)
	}

	if n.Kind != KindStart {
		for ed.BtnDelete.Clicked(gtx) {
			ed.pushHistory()
			if len(ed.selected) > 1 {
				for id := range ed.selected {
					ed.Scenario.RemoveNode(id)
				}
			} else {
				ed.Scenario.RemoveNode(n.ID)
			}
			ed.clearSelection()
		}
		label := "Delete node"
		if len(ed.selected) > 1 {
			label = "Delete " + strconv.Itoa(len(ed.selected)) + " nodes"
		}
		if ed.panelCompact {
			label = "Delete"
			if len(ed.selected) > 1 {
				label = "Delete " + strconv.Itoa(len(ed.selected))
			}
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(th, &ed.BtnDelete, label)
				btn.Background = theme.Danger
				btn.Color = theme.DangerFg
				btn.TextSize = unit.Sp(12)
				btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return btn.Layout(gtx)
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ed *Editor) layoutEdgeProps(gtx layout.Context, th *material.Theme, e *Edge) layout.Dimensions {
	from := ed.Scenario.NodeByID(e.From)
	to := ed.Scenario.NodeByID(e.To)
	title := "Arrow"
	if from != nil && to != nil {
		title = from.DisplayName() + " → " + to.DisplayName()
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(13), title)
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Condition") }),
	}

	for i, ck := range CondKinds {
		i, ck := i, ck
		for ed.condBtn[i].Clicked(gtx) {
			if e.Cond != ck {
				ed.pushHistory()
				e.Cond = ck
			}
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ed.radioRow(gtx, th, &ed.condBtn[i], ck.Title(), e.Cond == ck)
		}))
	}

	switch e.Cond {
	case CondStatus:
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Status pattern") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &e.ValueEd, "200, 2xx, 4xx ...")
			}),
		)
	case CondBodyField:
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Field path") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &e.ValueEd, "data.items.0.id")
			}),
		)
	case CondArrayCount:
		btns := make([]*widget.Clickable, len(ops))
		for i := range ops {
			btns[i] = &ed.opBtn[i]
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := "Array field path"
				if ed.panelCompact {
					lbl = "Array path"
				}
				return ed.sectionLabel(gtx, th, lbl)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &e.ValueEd, "data.items")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Comparison") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.chipRow(gtx, th, btns, ops, e.Op, func(o string) {
					ed.pushHistory()
					e.Op = o
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Count") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &e.CountEd, "0")
			}),
		)
	case CondBodyValue:
		btns := make([]*widget.Clickable, len(valueOps))
		for i := range valueOps {
			btns[i] = &ed.valOpBtn[i]
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Field path") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &e.ValueEd, "data.status")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Comparison") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.chipRow(gtx, th, btns, valueOps, e.Op, func(o string) {
					ed.pushHistory()
					e.Op = o
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := "Expected value ({{vars}} allowed)"
				if ed.panelCompact {
					lbl = "Expected"
				}
				return ed.sectionLabel(gtx, th, lbl)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ed.borderedEditor(gtx, th, &e.Val2Ed, "ok")
			}),
		)
	}

	for ed.BtnDelete.Clicked(gtx) {
		ed.pushHistory()
		ed.Scenario.RemoveEdge(e.ID)
		ed.selEdgeID = ""
	}
	delLbl := "Delete arrow"
	if ed.panelCompact {
		delLbl = "Delete"
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, &ed.BtnDelete, delLbl)
			btn.Background = theme.Danger
			btn.Color = theme.DangerFg
			btn.TextSize = unit.Sp(12)
			btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return btn.Layout(gtx)
		})
	}))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func statusCodeColor(code int, ok bool) color.NRGBA {
	switch {
	case !ok:
		return theme.Danger
	case code >= 300 && code < 400:
		return color.NRGBA{R: 235, G: 180, B: 60, A: 255}
	default:
		return color.NRGBA{R: 70, G: 190, B: 100, A: 255}
	}
}

func fmtDur(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return itoa(int(d.Milliseconds())) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

func (ed *Editor) layoutHistory(gtx layout.Context, th *material.Theme) layout.Dimensions {
	runs := ed.Runner.Runs()
	if len(runs) == 0 {
		msg := "No runs yet. Press \"Run scenario\"."
		if ed.panelCompact {
			msg = "No runs yet"
		}
		return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), msg)
			lbl.Color = theme.FgDim
			return lbl.Layout(gtx)
		})
	}

	selected := ed.histRun
	found := false
	for _, r := range runs {
		if r == selected {
			found = true
			break
		}
	}
	if !found {
		selected = runs[len(runs)-1]
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ed.sectionLabel(gtx, th, "Runs") }),
	}

	for i := len(runs) - 1; i >= 0; i-- {
		rec := runs[i]
		for rec.SelBtn.Clicked(gtx) {
			ed.histRun = rec
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return rec.SelBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				bg := theme.Transparent
				if rec == selected {
					bg = theme.BgHover
				} else if rec.SelBtn.Hovered() {
					bg = theme.BgHover
				}
				rect := image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(24)))}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
				gtx.Constraints.Min.Y = rect.Max.Y
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						r := gtx.Dp(unit.Dp(4))
						c := image.Pt(gtx.Dp(unit.Dp(9)), rect.Max.Y/2)
						circ := image.Rect(c.X-r, c.Y-r, c.X+r, c.Y+r)
						col := color.NRGBA{R: 235, G: 180, B: 60, A: 255}
						if rec.Done {
							if rec.Failed || rec.Stopped {
								col = theme.Danger
							} else {
								col = color.NRGBA{R: 70, G: 190, B: 100, A: 255}
							}
						}
						paint.FillShape(gtx.Ops, col, clip.Ellipse(circ).Op(gtx.Ops))
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(18)), rect.Max.Y)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						txt := rec.Label
						if ed.panelCompact {
							clock := rec.Clock
							if len(clock) > 5 {
								clock = clock[:5]
							}
							txt = itoa(rec.Seq) + " · " + clock
						} else if d := fmtDur(rec.Dur); rec.Done && d != "" {
							txt += " · " + d
						}
						lbl := material.Label(th, unit.Sp(11), txt)
						if rec == selected {
							lbl.Color = theme.Fg
						} else {
							lbl.Color = theme.FgMuted
						}
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
				)
			})
		}))
	}

	entries := ed.Runner.Entries(selected)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return ed.sectionLabel(gtx, th, "Requests")
	}))
	if len(entries) == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), "No requests executed")
			lbl.Color = theme.FgDim
			return lbl.Layout(gtx)
		}))
	}
	for _, ent := range entries {
		ent := ent
		for ent.Click.Clicked(gtx) {
			ent.Expanded = !ent.Expanded
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ent.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							bg := theme.BgField
							if ent.Click.Hovered() {
								bg = theme.BgHover
							}
							rr := gtx.Dp(unit.Dp(4))
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									rect := image.Rectangle{Max: gtx.Constraints.Min}
									paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, rr).Op(gtx.Ops))
									return layout.Dimensions{Size: gtx.Constraints.Min}
								},
								func(gtx layout.Context) layout.Dimensions {
									return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
										status := ent.Status
										if ed.panelCompact {
											if ent.Code > 0 {
												status = strconv.Itoa(ent.Code)
											} else {
												status = "ERR"
											}
										}
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														lbl := material.Label(th, unit.Sp(11), status)
														lbl.Color = statusCodeColor(ent.Code, ent.OK)
														lbl.Font.Weight = 600
														lbl.MaxLines = 1
														return lbl.Layout(gtx)
													}),
													layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min.Y = 0
														lbl := material.Label(th, unit.Sp(11), ent.Node)
														lbl.MaxLines = 1
														return lbl.Layout(gtx)
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														d := fmtDur(ent.Dur)
														if d == "" {
															return layout.Dimensions{}
														}
														lbl := material.Label(th, unit.Sp(10), d)
														lbl.Color = theme.FgMuted
														return lbl.Layout(gtx)
													}),
												)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if ed.panelCompact {
													return layout.Dimensions{}
												}
												lbl := material.Label(th, unit.Sp(10), ent.Detail)
												lbl.Color = theme.FgMuted
												lbl.MaxLines = 1
												return lbl.Layout(gtx)
											}),
										)
									})
								},
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !ent.Expanded {
							return layout.Dimensions{}
						}
						body := ent.Body
						if body == "" {
							body = "(empty body)"
						}
						return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							rr := gtx.Dp(unit.Dp(4))
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									rect := image.Rectangle{Max: gtx.Constraints.Min}
									paint.FillShape(gtx.Ops, theme.BgDark, clip.UniformRRect(rect, rr).Op(gtx.Ops))
									paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
									return layout.Dimensions{Size: gtx.Constraints.Min}
								},
								func(gtx layout.Context) layout.Dimensions {
									return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min.X = gtx.Constraints.Max.X
												lbl := widgets.MonoLabel(th, unit.Sp(10), body)
												lbl.Color = theme.FgMuted
												return lbl.Layout(gtx)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if ent.BodyLen <= maxHistoryBody {
													return layout.Dimensions{}
												}
												txt := "… truncated · " + itoa(ent.BodyLen/1024) + " KB total"
												return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													lbl := material.Label(th, unit.Sp(10), txt)
													lbl.Color = theme.FgDim
													return lbl.Layout(gtx)
												})
											}),
										)
									})
								},
							)
						})
					}),
				)
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
