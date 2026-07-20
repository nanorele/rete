package netlimit

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"strings"
	"time"

	netlim "tracto/internal/netlimit"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
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

type Host struct {
	Theme  *material.Theme
	Window *app.Window
}

func (s *Section) handleClicks(gtx layout.Context) {
	for s.scopeSys.Clicked(gtx) {
		s.scope = netlim.ScopeSystem
		s.mgr.SetWatchPID(0)
	}
	for s.scopeApp.Clicked(gtx) {
		s.scope = netlim.ScopeApp
		if s.hasApp {
			s.mgr.SetWatchPID(s.selApp.PID)
		}
	}
	for _, u := range []*unitSel{&s.inUnit, &s.outUnit, &s.totalUnit} {
		u.ensure()
		for i := range u.clicks {
			for u.clicks[i].Clicked(gtx) {
				u.idx = i
			}
		}
	}
	for s.pickBtn.Clicked(gtx) {
		s.pickerOpen = !s.pickerOpen
		if s.pickerOpen {
			s.loadProcs()
		}
	}
	for i := range s.procClicks {
		for s.procClicks[i].Clicked(gtx) {
			procs := s.getProcs()
			if i < len(procs) {
				s.selApp = procs[i]
				s.hasApp = true
				s.pickerOpen = false
				if s.scope == netlim.ScopeApp {
					s.mgr.SetWatchPID(procs[i].PID)
				}
			}
		}
	}
	for s.startBtn.Clicked(gtx) {
		spec := s.buildSpec()
		if spec.Unlimited() {
			s.setErr(fmt.Errorf("set at least one rate limit"))
			continue
		}
		if spec.Scope == netlim.ScopeApp && !s.hasApp {
			s.setErr(fmt.Errorf("select an application first"))
			continue
		}
		s.setErr(nil)
		s.saveConfig()
		go func() {
			s.setErr(s.mgr.Apply(spec))
			s.host.Window.Invalidate()
		}()
	}
	for s.clearOrphanBtn.Clicked(gtx) {
		s.orphan = false
		go func() {
			s.setErr(s.mgr.ClearOrphan())
			s.host.Window.Invalidate()
		}()
	}
	for s.stopBtn.Clicked(gtx) {
		go func() {
			s.setErr(s.mgr.Pause())
			s.host.Window.Invalidate()
		}()
	}
	for s.resumeBtn.Clicked(gtx) {
		go func() {
			s.setErr(s.mgr.Resume())
			s.host.Window.Invalidate()
		}()
	}
	for s.cancelBtn.Clicked(gtx) {
		go func() {
			s.setErr(s.mgr.Cancel())
			s.host.Window.Invalidate()
		}()
	}
	for s.relaunch.Clicked(gtx) {
		if err := netlim.RelaunchElevated(); err != nil {
			s.setErr(err)
		} else {
			s.host.Window.Perform(system.ActionClose)
		}
	}
}

func (s *Section) LayoutBody(gtx layout.Context, host *Host) layout.Dimensions {
	s.host = host
	s.handleClicks(gtx)

	th := host.Theme
	inset := layout.UniformInset(unit.Dp(10))
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var rows []layout.FlexChild
		add := func(w layout.Widget) {
			rows = append(rows, layout.Rigid(w))
		}
		gap := func(dp int) {
			rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(float32(dp))}.Layout))
		}

		add(func(gtx layout.Context) layout.Dimensions {
			return sectionLabel(gtx, th, "NETWORK LIMIT")
		})
		gap(8)

		if s.orphan {
			add(func(gtx layout.Context) layout.Dimensions {
				return box(gtx, theme.VarMissing, theme.Danger, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(th, unit.Sp(11), "Leftover limiting rules from a previous session were detected.")
								lbl.Color = theme.Fg
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return button(gtx, th, &s.clearOrphanBtn, "Clear leftover rules", theme.Danger, theme.DangerFg, true)
							}),
						)
					})
				})
			})
			gap(10)
		}

		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return toggle(gtx, th, &s.scopeSys, "System", s.scope == netlim.ScopeSystem)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return toggle(gtx, th, &s.scopeApp, "Application", s.scope == netlim.ScopeApp)
				}),
			)
		})

		if s.scope == netlim.ScopeApp {
			gap(8)
			add(func(gtx layout.Context) layout.Dimensions {
				label := "Choose application…"
				if s.hasApp {
					label = s.selApp.Name
				}
				return button(gtx, th, &s.pickBtn, label, theme.BgField, theme.Fg, true)
			})
			if s.pickerOpen {
				gap(4)
				add(func(gtx layout.Context) layout.Dimensions {
					return s.procPicker(gtx)
				})
			}
		}

		gap(12)
		add(func(gtx layout.Context) layout.Dimensions {
			return sectionLabel(gtx, th, "LIMITS")
		})
		gap(8)
		add(func(gtx layout.Context) layout.Dimensions {
			return s.limitRow(gtx, &s.inEd, &s.inUnit, "Download")
		})
		gap(8)
		add(func(gtx layout.Context) layout.Dimensions {
			return s.limitRow(gtx, &s.outEd, &s.outUnit, "Upload")
		})
		gap(8)
		add(func(gtx layout.Context) layout.Dimensions {
			return s.limitRow(gtx, &s.totalEd, &s.totalUnit, "Total")
		})

		gap(12)
		s.controlButtons(&rows, gap, add)

		if note := s.statusNote(); note != "" {
			gap(10)
			add(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), note)
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			})
		}
		if e := s.getErr(); e != "" {
			gap(8)
			add(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), e)
				lbl.Color = theme.Danger
				return lbl.Layout(gtx)
			})
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

func (s *Section) controlButtons(rows *[]layout.FlexChild, gap func(int), add func(layout.Widget)) {
	th := s.host.Theme
	state := s.mgr.State()
	needsElev := s.caps.NeedsElevation && !netlim.IsElevated()

	if runtime.GOOS == "windows" && needsElev {
		add(func(gtx layout.Context) layout.Dimensions {
			return button(gtx, th, &s.relaunch, "Restart as administrator", theme.BtnPrimary, theme.BtnPrimaryFg, true)
		})
		return
	}

	switch state {
	case netlim.StateIdle:
		add(func(gtx layout.Context) layout.Dimensions {
			return button(gtx, th, &s.startBtn, "Start limiting", theme.BtnPrimary, theme.BtnPrimaryFg, s.caps.Available)
		})
	case netlim.StateActive:
		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return button(gtx, th, &s.stopBtn, "Pause", theme.BgSecondary, theme.Fg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return button(gtx, th, &s.cancelBtn, "Cancel", theme.Danger, theme.DangerFg, true)
				}),
			)
		})
	case netlim.StatePaused:
		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return button(gtx, th, &s.resumeBtn, "Resume", theme.BtnPrimary, theme.BtnPrimaryFg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return button(gtx, th, &s.cancelBtn, "Cancel", theme.Danger, theme.DangerFg, true)
				}),
			)
		})
	}
}

func (s *Section) statusNote() string {
	if !s.caps.Available {
		if s.caps.Note != "" {
			return s.caps.Note
		}
		return "Network limiting is not available on this system."
	}
	notes := []string{}
	if s.scope == netlim.ScopeApp && !s.caps.AppLimit {
		notes = append(notes, "Per-application limiting is not supported here.")
	}
	if s.caps.Note != "" {
		notes = append(notes, s.caps.Note)
	}
	if s.caps.NeedsElevation && !netlim.IsElevated() && runtime.GOOS != "windows" {
		notes = append(notes, "Administrator/root privileges are required; you may be prompted.")
	}
	return strings.Join(notes, " ")
}

func (s *Section) procPicker(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	h := gtx.Dp(unit.Dp(220))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	procs := s.getProcs()
	if n := len(procs); n > len(s.procClicks) {
		s.procClicks = make([]widget.Clickable, n)
	}

	filter := strings.ToLower(strings.TrimSpace(s.searchEd.Text()))
	type row struct {
		idx int
		p   netlim.ProcInfo
	}
	visible := make([]row, 0, len(procs))
	for i, p := range procs {
		if filter == "" || strings.Contains(strings.ToLower(p.Name), filter) {
			visible = append(visible, row{idx: i, p: p})
		}
	}

	return box(gtx, theme.BgField, theme.BorderLight, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, &s.searchEd, "Search…")
					ed.TextSize = unit.Sp(12)
					ed.Color = theme.Fg
					ed.HintColor = theme.FgMuted
					return ed.Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if s.procsLoading && len(procs) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(12), "Loading…")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					})
				}
				s.procListHover.Update(gtx.Source)
				rowH := gtx.Dp(unit.Dp(24))
				hoveredIdx := -1
				if s.procListHover.Hovered() && rowH > 0 {
					rel := s.procListHover.Pos().Y + float32(s.procList.Position.Offset)
					if rel >= 0 {
						if idx := s.procList.Position.First + int(rel)/rowH; idx >= 0 && idx < len(visible) {
							hoveredIdx = idx
						}
					}
				}
				dim := material.List(th, &s.procList).Layout(gtx, len(visible), func(gtx layout.Context, i int) layout.Dimensions {
					r := visible[i]
					clk := &s.procClicks[r.idx]
					return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						bg := theme.Transparent
						if i == hoveredIdx {
							bg = theme.BgHover
						}
						if s.hasApp && s.selApp.PID == r.p.PID {
							bg = theme.AccentDim
						}
						paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(24)))}.Op())
						return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th, unit.Sp(12), fmt.Sprintf("%s  (%d)", r.p.Name, r.p.PID))
							lbl.MaxLines = 1
							lbl.Color = theme.Fg
							return lbl.Layout(gtx)
						})
					})
				})
				pass := pointer.PassOp{}.Push(gtx.Ops)
				cl := clip.Rect{Max: dim.Size}.Push(gtx.Ops)
				s.procListHover.Add(gtx.Ops)
				cl.Pop()
				pass.Pop()
				return dim
			}),
		)
	})
}

func (s *Section) limitRow(gtx layout.Context, ed *widget.Editor, u *unitSel, label string) layout.Dimensions {
	th := s.host.Theme
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(10), label)
			lbl.Color = theme.FgMuted
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.field(gtx, ed, "0")
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.unitChips(gtx, u)
				}),
			)
		}),
	)
}

func (s *Section) unitChips(gtx layout.Context, u *unitSel) layout.Dimensions {
	th := s.host.Theme
	u.ensure()
	children := make([]layout.FlexChild, 0, len(units)*2)
	for i := range units {
		i := i
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(3)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chip(gtx, th, &u.clicks[i], units[i].label, u.idx == i)
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (s *Section) field(gtx layout.Context, ed *widget.Editor, hint string) layout.Dimensions {
	return widgets.TextField(gtx, s.host.Theme, ed, hint, true, nil, 0, unit.Sp(13))
}

func (s *Section) LayoutSection(gtx layout.Context, host *Host) layout.Dimensions {
	s.host = host
	s.handleSectionClicks(gtx)
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(400 * time.Millisecond)})
	th := host.Theme
	paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	if s.graphWindow == 0 {
		s.graphWindow = time.Minute
	}

	cards := []layout.Widget{
		s.graphCard,
		layout.Spacer{Height: unit.Dp(12)}.Layout,
		s.diagCard,
	}
	inset := layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(14), Left: unit.Dp(14), Right: unit.Dp(14)}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(th, &s.secList).Layout(gtx, len(cards), func(gtx layout.Context, i int) layout.Dimensions {
			return cards[i](gtx)
		})
	})
}

func (s *Section) handleSectionClicks(gtx layout.Context) {
	for s.win30Btn.Clicked(gtx) {
		s.graphWindow = 30 * time.Second
	}
	for s.win1mBtn.Clicked(gtx) {
		s.graphWindow = time.Minute
	}
	for s.win5mBtn.Clicked(gtx) {
		s.graphWindow = 5 * time.Minute
	}
	for s.diagBtn.Clicked(gtx) {
		s.mu.Lock()
		if s.diagRunning {
			s.mu.Unlock()
			continue
		}
		s.diagRunning = true
		s.mu.Unlock()
		go func() {
			lines := s.buildDiagnostics()
			s.mu.Lock()
			s.diagLines = lines
			s.diagRunning = false
			s.mu.Unlock()
			s.host.Window.Invalidate()
		}()
	}
}

func (s *Section) graphCard(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	interval := s.mgr.Interval()
	if interval <= 0 {
		interval = 700 * time.Millisecond
	}
	slots := int(s.graphWindow / interval)
	if slots < 2 {
		slots = 2
	}
	hist := s.mgr.History()
	vis := hist
	if len(vis) > slots {
		vis = vis[len(vis)-slots:]
	}

	var curIn, curOut, peakIn, peakOut int64
	if n := len(vis); n > 0 {
		curIn = vis[n-1].InBps
		curOut = vis[n-1].OutBps
	}
	for _, p := range vis {
		if p.InBps > peakIn {
			peakIn = p.InBps
		}
		if p.OutBps > peakOut {
			peakOut = p.OutBps
		}
	}

	return box(gtx, theme.BgDark, theme.Border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return sectionLabel(gtx, th, "CURRENT TRAFFIC")
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.intervalSelector(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return trafficGraph(gtx, th, vis, slots, peakIn, peakOut)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.currentNumbers(gtx, curIn, curOut, peakIn, peakOut)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return stateBadge(gtx, th, s.mgr.State(), s.mgr.Spec())
				}),
			)
		})
	})
}

func (s *Section) intervalSelector(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	w := s.graphWindow
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chip(gtx, th, &s.win30Btn, "30s", w == 30*time.Second)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chip(gtx, th, &s.win1mBtn, "1m", w == time.Minute)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chip(gtx, th, &s.win5mBtn, "5m", w == 5*time.Minute)
		}),
	)
}

func (s *Section) currentNumbers(gtx layout.Context, in, out, peakIn, peakOut int64) layout.Dimensions {
	th := s.host.Theme
	w := numbersColWidth(gtx, th)
	gtx.Constraints.Min.X = w
	gtx.Constraints.Max.X = w
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return speedRow(gtx, th, widgets.IconDownload, theme.MethodGet, formatRate(in))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return speedRow(gtx, th, widgets.IconUpload, theme.MethodPost, formatRate(out))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11),
				fmt.Sprintf("Peak  ↓ %s   ↑ %s", formatRate(peakIn), formatRate(peakOut)))
			lbl.Color = theme.FgMuted
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
	}
	if s.scope == netlim.ScopeApp && s.hasApp && s.caps.PerAppSpeed {
		app := s.mgr.AppSpeed()
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11),
					fmt.Sprintf("%s  ↓ %s   ↑ %s", s.selApp.Name, formatRate(app.InBps), formatRate(app.OutBps)))
				lbl.Color = theme.FgHint
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func numbersColWidth(gtx layout.Context, th *material.Theme) int {
	measure := func(w layout.Widget) int {
		m := op.Record(gtx.Ops)
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
		d := w(g)
		m.Stop()
		return d.Size.X
	}
	rowW := measure(func(g layout.Context) layout.Dimensions {
		return speedRow(g, th, widgets.IconDownload, theme.MethodGet, "1020.90 MB/s")
	})
	peakW := measure(func(g layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), "Peak  ↓ 1020.90 MB/s   ↑ 1020.90 MB/s")
		return lbl.Layout(g)
	})
	w := rowW
	if peakW > w {
		w = peakW
	}
	return w + gtx.Dp(unit.Dp(4))
}

func trafficGraph(gtx layout.Context, th *material.Theme, vis []netlim.TrafficPoint, slots int, peakIn, peakOut int64) layout.Dimensions {
	w := gtx.Constraints.Max.X
	if w < gtx.Dp(unit.Dp(120)) {
		w = gtx.Dp(unit.Dp(120))
	}
	h := gtx.Dp(unit.Dp(150))
	size := image.Pt(w, h)
	rect := image.Rectangle{Max: size}
	rr := gtx.Dp(unit.Dp(4))

	paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(rect, rr).Op(gtx.Ops))
	widgets.PaintBorder1px(gtx, size, theme.BorderLight)

	maxVal := peakIn
	if peakOut > maxVal {
		maxVal = peakOut
	}
	floor := int64(64 * 1024)
	if maxVal < floor {
		maxVal = floor
	}
	maxVal = niceCeil(maxVal)

	pad := gtx.Dp(unit.Dp(4))
	left := float32(pad)
	right := float32(w - pad)
	top := float32(gtx.Dp(unit.Dp(16)))
	bottom := float32(h - pad)

	func() {
		defer clip.Rect(rect).Push(gtx.Ops).Pop()

		grid := theme.BorderSubtle
		for i := 1; i < 4; i++ {
			y := top + (bottom-top)*float32(i)/4
			var gp clip.Path
			gp.Begin(gtx.Ops)
			gp.MoveTo(f32.Pt(left, y))
			gp.LineTo(f32.Pt(right, y))
			paint.FillShape(gtx.Ops, grid, clip.Stroke{Path: gp.End(), Width: 1}.Op())
		}

		xAt := func(idx int) float32 {
			if slots <= 1 {
				return right
			}
			return left + (right-left)*float32(idx)/float32(slots-1)
		}
		yAt := func(v int64) float32 {
			if maxVal <= 0 {
				return bottom
			}
			frac := float32(v) / float32(maxVal)
			if frac > 1 {
				frac = 1
			}
			return bottom - (bottom-top)*frac
		}

		drawSeries := func(get func(netlim.TrafficPoint) int64, line, fill color.NRGBA) {
			if len(vis) < 2 {
				return
			}
			off := slots - len(vis)
			var area clip.Path
			area.Begin(gtx.Ops)
			area.MoveTo(f32.Pt(xAt(off), bottom))
			for j, p := range vis {
				area.LineTo(f32.Pt(xAt(off+j), yAt(get(p))))
			}
			area.LineTo(f32.Pt(xAt(off+len(vis)-1), bottom))
			area.Close()
			paint.FillShape(gtx.Ops, fill, clip.Outline{Path: area.End()}.Op())

			var ln clip.Path
			ln.Begin(gtx.Ops)
			ln.MoveTo(f32.Pt(xAt(off), yAt(get(vis[0]))))
			for j, p := range vis {
				ln.LineTo(f32.Pt(xAt(off+j), yAt(get(p))))
			}
			paint.FillShape(gtx.Ops, line, clip.Stroke{Path: ln.End(), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())
		}

		inFill := theme.MethodGet
		inFill.A = 48
		outFill := theme.MethodPost
		outFill.A = 48
		drawSeries(func(p netlim.TrafficPoint) int64 { return p.OutBps }, theme.MethodPost, outFill)
		drawSeries(func(p netlim.TrafficPoint) int64 { return p.InBps }, theme.MethodGet, inFill)
	}()

	func() {
		defer op.Offset(image.Pt(pad+gtx.Dp(unit.Dp(2)), gtx.Dp(unit.Dp(2)))).Push(gtx.Ops).Pop()
		lbl := material.Label(th, unit.Sp(10), formatRate(maxVal))
		lbl.Color = theme.FgMuted
		lbl.Layout(gtx)
	}()

	return layout.Dimensions{Size: size}
}

func niceCeil(v int64) int64 {
	if v <= 0 {
		return 1
	}
	mag := int64(1)
	for mag*10 <= v {
		mag *= 10
	}
	for _, f := range []int64{1, 2, 5, 10} {
		if f*mag >= v {
			return f * mag
		}
	}
	return 10 * mag
}

func (s *Section) diagCard(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	s.mu.Lock()
	running := s.diagRunning
	lines := s.diagLines
	s.mu.Unlock()

	return box(gtx, theme.BgDark, theme.Border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			rows := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return sectionLabel(gtx, th, "DIAGNOSTICS")
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := "Run test"
							if running {
								label = "Testing…"
							}
							return chip(gtx, th, &s.diagBtn, label, running)
						}),
					)
				}),
			}

			if len(lines) == 0 {
				rows = append(rows,
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), "Run a connectivity test to check the link, privileges and backend.")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
				)
			}
			for _, ln := range lines {
				ln := ln
				rows = append(rows,
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return diagRow(gtx, th, ln)
					}),
				)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	})
}

func diagRow(gtx layout.Context, th *material.Theme, ln diagLine) layout.Dimensions {
	col := theme.Fg
	switch ln.ok {
	case 1:
		col = theme.MethodGet
	case -1:
		col = theme.Danger
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), ln.label)
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), ln.value)
			lbl.Color = col
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
	)
}

func speedRow(gtx layout.Context, th *material.Theme, ic *widget.Icon, col color.NRGBA, value string) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(26))
			gtx.Constraints.Min = image.Pt(s, s)
			gtx.Constraints.Max = gtx.Constraints.Min
			return ic.Layout(gtx, col)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(28), value)
			lbl.Color = theme.Fg
			return lbl.Layout(gtx)
		}),
	)
}

func stateBadge(gtx layout.Context, th *material.Theme, state netlim.State, spec netlim.LimitSpec) layout.Dimensions {
	var text string
	var col color.NRGBA
	switch state {
	case netlim.StateActive:
		text = "Limit active — " + specSummary(spec)
		col = theme.Accent
	case netlim.StatePaused:
		text = "Limit paused — " + specSummary(spec)
		col = theme.FgMuted
	default:
		text = "No active limit"
		col = theme.FgMuted
	}
	lbl := material.Label(th, unit.Sp(12), text)
	lbl.Color = col
	return lbl.Layout(gtx)
}

func specSummary(spec netlim.LimitSpec) string {
	parts := []string{}
	if spec.Scope == netlim.ScopeApp && spec.AppName != "" {
		parts = append(parts, spec.AppName)
	}
	if spec.InBps > 0 {
		parts = append(parts, "↓"+formatRate(spec.InBps))
	}
	if spec.OutBps > 0 {
		parts = append(parts, "↑"+formatRate(spec.OutBps))
	}
	if spec.TotalBps > 0 {
		parts = append(parts, "Σ"+formatRate(spec.TotalBps))
	}
	return strings.Join(parts, "  ")
}

func formatRate(bps int64) string {
	f := float64(bps)
	switch {
	case f >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", f/(1024*1024))
	case f >= 1024:
		return fmt.Sprintf("%.1f KB/s", f/1024)
	default:
		return fmt.Sprintf("%d B/s", bps)
	}
}

func sectionLabel(gtx layout.Context, th *material.Theme, txt string) layout.Dimensions {
	lbl := material.Label(th, unit.Sp(11), txt)
	lbl.Color = theme.FgMuted
	return lbl.Layout(gtx)
}

func toggle(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	bg := theme.BgField
	fg := theme.FgMuted
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return button(gtx, th, clk, label, bg, fg, true)
}

func button(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, bg, fg color.NRGBA, enabled bool) layout.Dimensions {
	if !enabled {
		bg = theme.BgSecondary
		fg = theme.FgDisabled
	}
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if enabled && clk.Hovered() {
			bg = lighten(bg)
		}
		h := gtx.Dp(unit.Dp(30))
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				w := gtx.Constraints.Min.X
				if w < gtx.Constraints.Max.X {
					w = gtx.Constraints.Max.X
				}
				sz := image.Pt(w, h)
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
				return layout.Dimensions{Size: sz}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = h
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(12), label)
						lbl.Color = fg
						lbl.MaxLines = 1
						lbl.Alignment = 1
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	})
}

func chip(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	bg := theme.BgField
	fg := theme.FgMuted
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if !active && clk.Hovered() {
			bg = lighten(bg)
		}
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Constraints.Min
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
				return layout.Dimensions{Size: sz}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), label)
					lbl.Color = fg
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

func box(gtx layout.Context, bg, border color.NRGBA, w layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			r := gtx.Dp(unit.Dp(4))
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, r).Op(gtx.Ops))
			widgets.PaintBorder1px(gtx, sz, border)
			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(w),
	)
}

func lighten(c color.NRGBA) color.NRGBA {
	add := func(v uint8) uint8 {
		n := int(v) + 18
		if n > 255 {
			n = 255
		}
		return uint8(n)
	}
	return color.NRGBA{R: add(c.R), G: add(c.G), B: add(c.B), A: c.A}
}
