package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"tracto/internal/netlimit"
	"tracto/internal/persist"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

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

type netLimitState struct {
	caps netlimit.Caps

	scope    netlimit.Scope
	scopeSys widget.Clickable
	scopeApp widget.Clickable

	inEd      widget.Editor
	outEd     widget.Editor
	totalEd   widget.Editor
	inUnit    netUnitSel
	outUnit   netUnitSel
	totalUnit netUnitSel

	startBtn  widget.Clickable
	stopBtn   widget.Clickable
	resumeBtn widget.Clickable
	cancelBtn widget.Clickable
	relaunch  widget.Clickable

	pickBtn       widget.Clickable
	pickerOpen    bool
	searchEd      widget.Editor
	procList      widget.List
	procClicks    []widget.Clickable
	procListHover widgets.Hover
	selApp        netlimit.ProcInfo
	hasApp        bool

	orphan         bool
	clearOrphanBtn widget.Clickable

	secList     widget.List
	graphWindow time.Duration
	win30Btn    widget.Clickable
	win1mBtn    widget.Clickable
	win5mBtn    widget.Clickable

	diagBtn     widget.Clickable
	diagRunning bool
	diagLines   []netDiagLine

	mu           sync.Mutex
	procs        []netlimit.ProcInfo
	procsLoading bool
	lastErr      string
}

type netDiagLine struct {
	label string
	value string
	ok    int8
}

var netUnits = []struct {
	label string
	mul   int64
}{
	{"KB/s", 1024},
	{"MB/s", 1024 * 1024},
	{"GB/s", 1024 * 1024 * 1024},
}

type netUnitSel struct {
	idx    int
	clicks []widget.Clickable
}

func (u *netUnitSel) ensure() {
	if len(u.clicks) < len(netUnits) {
		u.clicks = make([]widget.Clickable, len(netUnits))
	}
}

func (u *netUnitSel) mul() int64 {
	if u.idx < 0 || u.idx >= len(netUnits) {
		return netUnits[0].mul
	}
	return netUnits[u.idx].mul
}

type netConfig struct {
	Scope   int    `json:"scope"`
	AppPath string `json:"app_path,omitempty"`
	AppName string `json:"app_name,omitempty"`
	In      string `json:"in,omitempty"`
	Out     string `json:"out,omitempty"`
	Total   string `json:"total,omitempty"`
	UnitMB  bool   `json:"unit_mb"`
	Unit    int    `json:"unit"`
	InUnit  int    `json:"in_unit"`
	OutUnit int    `json:"out_unit"`
	TotUnit int    `json:"total_unit"`
}

func (ui *AppUI) initNetlimit() {
	ui.NetMgr = netlimit.New()
	ui.NetMgr.SetMarkerPath(persist.NetlimitMarkerPath())
	ui.Net.caps = ui.NetMgr.Caps()
	ui.Net.inEd.SingleLine = true
	ui.Net.outEd.SingleLine = true
	ui.Net.totalEd.SingleLine = true
	ui.Net.searchEd.SingleLine = true
	ui.Net.inUnit.idx = 1
	ui.Net.outUnit.idx = 1
	ui.Net.totalUnit.idx = 1
	ui.Net.inUnit.ensure()
	ui.Net.outUnit.ensure()
	ui.Net.totalUnit.ensure()
	ui.Net.procList.Axis = layout.Vertical
	ui.Net.secList.Axis = layout.Vertical
	ui.Net.graphWindow = time.Minute
	ui.loadNetConfig()
	ui.Net.orphan = ui.NetMgr.HasOrphan()
	ui.NetMgr.Start()
}

func (ui *AppUI) loadNetConfig() {
	data, err := os.ReadFile(persist.NetlimitConfigPath())
	if err != nil {
		return
	}
	var c netConfig
	if json.Unmarshal(data, &c) != nil {
		return
	}
	ui.Net.scope = netlimit.Scope(c.Scope)
	clampUnit := func(v int) int {
		if v < 0 || v >= len(netUnits) {
			return 0
		}
		return v
	}
	if c.InUnit == 0 && c.OutUnit == 0 && c.TotUnit == 0 {
		shared := 0
		if c.Unit > 0 && c.Unit < len(netUnits) {
			shared = c.Unit
		} else if c.UnitMB {
			shared = 1
		}
		ui.Net.inUnit.idx = shared
		ui.Net.outUnit.idx = shared
		ui.Net.totalUnit.idx = shared
	} else {
		ui.Net.inUnit.idx = clampUnit(c.InUnit)
		ui.Net.outUnit.idx = clampUnit(c.OutUnit)
		ui.Net.totalUnit.idx = clampUnit(c.TotUnit)
	}
	ui.Net.inEd.SetText(c.In)
	ui.Net.outEd.SetText(c.Out)
	ui.Net.totalEd.SetText(c.Total)
	if c.AppName != "" {
		ui.Net.selApp = netlimit.ProcInfo{Name: c.AppName, Exe: c.AppPath}
		ui.Net.hasApp = true
	}
}

func (ui *AppUI) saveNetConfig() {
	c := netConfig{
		Scope:   int(ui.Net.scope),
		AppPath: ui.Net.selApp.Exe,
		AppName: ui.Net.selApp.Name,
		In:      ui.Net.inEd.Text(),
		Out:     ui.Net.outEd.Text(),
		Total:   ui.Net.totalEd.Text(),
		InUnit:  ui.Net.inUnit.idx,
		OutUnit: ui.Net.outUnit.idx,
		TotUnit: ui.Net.totalUnit.idx,
		Unit:    ui.Net.inUnit.idx,
		UnitMB:  ui.Net.inUnit.idx >= 1,
	}
	if data, err := json.Marshal(c); err == nil {
		_ = persist.AtomicWriteFile(persist.NetlimitConfigPath(), data)
	}
}

func (ui *AppUI) closeNetlimit() {
	if ui.NetMgr != nil {
		_ = ui.NetMgr.Close()
	}
}

func (ui *AppUI) wireNetTitlebar() {
	if ui.NetMgr == nil {
		return
	}
	state := ui.NetMgr.State()
	ui.TitleBar.NetActive = state == netlimit.StateActive
	ui.TitleBar.NetPaused = state == netlimit.StatePaused
	ui.TitleBar.OnNetToggle = func() {
		go func() {
			if ui.NetMgr.State() == netlimit.StatePaused {
				ui.Net.setErr(ui.NetMgr.Resume())
			} else {
				ui.Net.setErr(ui.NetMgr.Pause())
			}
			ui.Window.Invalidate()
		}()
	}
	ui.TitleBar.OnNetCancel = func() {
		go func() {
			ui.Net.setErr(ui.NetMgr.Cancel())
			ui.Window.Invalidate()
		}()
	}
}

func (ui *AppUI) layoutSidebarSectionNetlimitBtn(gtx layout.Context) layout.Dimensions {
	return ui.layoutSidebarSectionBtn(gtx, &ui.BtnSecNetlimit, widgets.IconNetlimit, "netlimit")
}

func (n *netLimitState) setErr(err error) {
	n.mu.Lock()
	if err != nil {
		n.lastErr = err.Error()
	} else {
		n.lastErr = ""
	}
	n.mu.Unlock()
}

func (n *netLimitState) getErr() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastErr
}

func (n *netLimitState) setProcs(p []netlimit.ProcInfo) {
	n.mu.Lock()
	n.procs = p
	n.procsLoading = false
	n.mu.Unlock()
}

func (n *netLimitState) getProcs() []netlimit.ProcInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.procs
}

func (ui *AppUI) netBuildSpec() netlimit.LimitSpec {
	parse := func(ed *widget.Editor, u *netUnitSel) int64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(ed.Text()), 64)
		if err != nil || v <= 0 {
			return 0
		}
		return int64(v * float64(u.mul()))
	}
	spec := netlimit.LimitSpec{
		Scope:    ui.Net.scope,
		InBps:    parse(&ui.Net.inEd, &ui.Net.inUnit),
		OutBps:   parse(&ui.Net.outEd, &ui.Net.outUnit),
		TotalBps: parse(&ui.Net.totalEd, &ui.Net.totalUnit),
	}
	if ui.Net.scope == netlimit.ScopeApp && ui.Net.hasApp {
		spec.AppPID = ui.Net.selApp.PID
		spec.AppName = ui.Net.selApp.Name
		spec.AppPath = ui.Net.selApp.Exe
	}
	return spec
}

func (ui *AppUI) netHandleClicks(gtx layout.Context) {
	for ui.Net.scopeSys.Clicked(gtx) {
		ui.Net.scope = netlimit.ScopeSystem
		ui.NetMgr.SetWatchPID(0)
	}
	for ui.Net.scopeApp.Clicked(gtx) {
		ui.Net.scope = netlimit.ScopeApp
		if ui.Net.hasApp {
			ui.NetMgr.SetWatchPID(ui.Net.selApp.PID)
		}
	}
	for _, u := range []*netUnitSel{&ui.Net.inUnit, &ui.Net.outUnit, &ui.Net.totalUnit} {
		u.ensure()
		for i := range u.clicks {
			for u.clicks[i].Clicked(gtx) {
				u.idx = i
			}
		}
	}
	for ui.Net.pickBtn.Clicked(gtx) {
		ui.Net.pickerOpen = !ui.Net.pickerOpen
		if ui.Net.pickerOpen {
			ui.netLoadProcs()
		}
	}
	for i := range ui.Net.procClicks {
		for ui.Net.procClicks[i].Clicked(gtx) {
			procs := ui.Net.getProcs()
			if i < len(procs) {
				ui.Net.selApp = procs[i]
				ui.Net.hasApp = true
				ui.Net.pickerOpen = false
				if ui.Net.scope == netlimit.ScopeApp {
					ui.NetMgr.SetWatchPID(procs[i].PID)
				}
			}
		}
	}
	for ui.Net.startBtn.Clicked(gtx) {
		spec := ui.netBuildSpec()
		if spec.Unlimited() {
			ui.Net.setErr(fmt.Errorf("set at least one rate limit"))
			continue
		}
		if spec.Scope == netlimit.ScopeApp && !ui.Net.hasApp {
			ui.Net.setErr(fmt.Errorf("select an application first"))
			continue
		}
		ui.Net.setErr(nil)
		ui.saveNetConfig()
		go func() {
			ui.Net.setErr(ui.NetMgr.Apply(spec))
			ui.Window.Invalidate()
		}()
	}
	for ui.Net.clearOrphanBtn.Clicked(gtx) {
		ui.Net.orphan = false
		go func() {
			ui.Net.setErr(ui.NetMgr.ClearOrphan())
			ui.Window.Invalidate()
		}()
	}
	for ui.Net.stopBtn.Clicked(gtx) {
		go func() {
			ui.Net.setErr(ui.NetMgr.Pause())
			ui.Window.Invalidate()
		}()
	}
	for ui.Net.resumeBtn.Clicked(gtx) {
		go func() {
			ui.Net.setErr(ui.NetMgr.Resume())
			ui.Window.Invalidate()
		}()
	}
	for ui.Net.cancelBtn.Clicked(gtx) {
		go func() {
			ui.Net.setErr(ui.NetMgr.Cancel())
			ui.Window.Invalidate()
		}()
	}
	for ui.Net.relaunch.Clicked(gtx) {
		if err := netlimit.RelaunchElevated(); err != nil {
			ui.Net.setErr(err)
		} else {
			ui.Window.Perform(system.ActionClose)
		}
	}
}

func (ui *AppUI) netLoadProcs() {
	ui.Net.mu.Lock()
	if ui.Net.procsLoading {
		ui.Net.mu.Unlock()
		return
	}
	ui.Net.procsLoading = true
	ui.Net.mu.Unlock()
	go func() {
		procs, _ := ui.NetMgr.ListProcs()
		ui.Net.setProcs(procs)
		ui.Window.Invalidate()
	}()
}

func (ui *AppUI) layoutNetlimitBody(gtx layout.Context) layout.Dimensions {
	ui.netHandleClicks(gtx)

	th := ui.Theme
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
			return netSectionLabel(gtx, th, "NETWORK LIMIT")
		})
		gap(8)

		if ui.Net.orphan {
			add(func(gtx layout.Context) layout.Dimensions {
				return netBox(gtx, theme.VarMissing, theme.Danger, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(th, unit.Sp(11), "Leftover limiting rules from a previous session were detected.")
								lbl.Color = theme.Fg
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return netButton(gtx, th, &ui.Net.clearOrphanBtn, "Clear leftover rules", theme.Danger, theme.DangerFg, true)
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
					return netToggle(gtx, th, &ui.Net.scopeSys, "System", ui.Net.scope == netlimit.ScopeSystem)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return netToggle(gtx, th, &ui.Net.scopeApp, "Application", ui.Net.scope == netlimit.ScopeApp)
				}),
			)
		})

		if ui.Net.scope == netlimit.ScopeApp {
			gap(8)
			add(func(gtx layout.Context) layout.Dimensions {
				label := "Choose application…"
				if ui.Net.hasApp {
					label = ui.Net.selApp.Name
				}
				return netButton(gtx, th, &ui.Net.pickBtn, label, theme.BgField, theme.Fg, true)
			})
			if ui.Net.pickerOpen {
				gap(4)
				add(func(gtx layout.Context) layout.Dimensions {
					return ui.netProcPicker(gtx)
				})
			}
		}

		gap(12)
		add(func(gtx layout.Context) layout.Dimensions {
			return netSectionLabel(gtx, th, "LIMITS")
		})
		gap(8)
		add(func(gtx layout.Context) layout.Dimensions {
			return ui.netLimitRow(gtx, &ui.Net.inEd, &ui.Net.inUnit, "Download")
		})
		gap(8)
		add(func(gtx layout.Context) layout.Dimensions {
			return ui.netLimitRow(gtx, &ui.Net.outEd, &ui.Net.outUnit, "Upload")
		})
		gap(8)
		add(func(gtx layout.Context) layout.Dimensions {
			return ui.netLimitRow(gtx, &ui.Net.totalEd, &ui.Net.totalUnit, "Total")
		})

		gap(12)
		ui.netControlButtons(&rows, gap, add)

		if note := ui.netStatusNote(); note != "" {
			gap(10)
			add(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), note)
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			})
		}
		if e := ui.Net.getErr(); e != "" {
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

func (ui *AppUI) netControlButtons(rows *[]layout.FlexChild, gap func(int), add func(layout.Widget)) {
	th := ui.Theme
	state := ui.NetMgr.State()
	needsElev := ui.Net.caps.NeedsElevation && !netlimit.IsElevated()

	if runtime.GOOS == "windows" && needsElev {
		add(func(gtx layout.Context) layout.Dimensions {
			return netButton(gtx, th, &ui.Net.relaunch, "Restart as administrator", theme.BtnPrimary, theme.BtnPrimaryFg, true)
		})
		return
	}

	switch state {
	case netlimit.StateIdle:
		add(func(gtx layout.Context) layout.Dimensions {
			return netButton(gtx, th, &ui.Net.startBtn, "Start limiting", theme.BtnPrimary, theme.BtnPrimaryFg, ui.Net.caps.Available)
		})
	case netlimit.StateActive:
		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return netButton(gtx, th, &ui.Net.stopBtn, "Pause", theme.BgSecondary, theme.Fg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return netButton(gtx, th, &ui.Net.cancelBtn, "Cancel", theme.Danger, theme.DangerFg, true)
				}),
			)
		})
	case netlimit.StatePaused:
		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return netButton(gtx, th, &ui.Net.resumeBtn, "Resume", theme.BtnPrimary, theme.BtnPrimaryFg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return netButton(gtx, th, &ui.Net.cancelBtn, "Cancel", theme.Danger, theme.DangerFg, true)
				}),
			)
		})
	}
}

func (ui *AppUI) netStatusNote() string {
	if !ui.Net.caps.Available {
		if ui.Net.caps.Note != "" {
			return ui.Net.caps.Note
		}
		return "Network limiting is not available on this system."
	}
	notes := []string{}
	if ui.Net.scope == netlimit.ScopeApp && !ui.Net.caps.AppLimit {
		notes = append(notes, "Per-application limiting is not supported here.")
	}
	if ui.Net.caps.Note != "" {
		notes = append(notes, ui.Net.caps.Note)
	}
	if ui.Net.caps.NeedsElevation && !netlimit.IsElevated() && runtime.GOOS != "windows" {
		notes = append(notes, "Administrator/root privileges are required; you may be prompted.")
	}
	return strings.Join(notes, " ")
}

func (ui *AppUI) netProcPicker(gtx layout.Context) layout.Dimensions {
	th := ui.Theme
	h := gtx.Dp(unit.Dp(220))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	procs := ui.Net.getProcs()
	if n := len(procs); n > len(ui.Net.procClicks) {
		ui.Net.procClicks = make([]widget.Clickable, n)
	}

	filter := strings.ToLower(strings.TrimSpace(ui.Net.searchEd.Text()))
	type row struct {
		idx int
		p   netlimit.ProcInfo
	}
	visible := make([]row, 0, len(procs))
	for i, p := range procs {
		if filter == "" || strings.Contains(strings.ToLower(p.Name), filter) {
			visible = append(visible, row{idx: i, p: p})
		}
	}

	return netBox(gtx, theme.BgField, theme.BorderLight, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, &ui.Net.searchEd, "Search…")
					ed.TextSize = unit.Sp(12)
					ed.Color = theme.Fg
					ed.HintColor = theme.FgMuted
					return ed.Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if ui.Net.procsLoading && len(procs) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(12), "Loading…")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					})
				}
				ui.Net.procListHover.Update(gtx.Source)
				rowH := gtx.Dp(unit.Dp(24))
				hoveredIdx := -1
				if ui.Net.procListHover.Hovered() && rowH > 0 {
					rel := ui.Net.procListHover.Pos().Y + float32(ui.Net.procList.Position.Offset)
					if rel >= 0 {
						if idx := ui.Net.procList.Position.First + int(rel)/rowH; idx >= 0 && idx < len(visible) {
							hoveredIdx = idx
						}
					}
				}
				dim := material.List(th, &ui.Net.procList).Layout(gtx, len(visible), func(gtx layout.Context, i int) layout.Dimensions {
					r := visible[i]
					clk := &ui.Net.procClicks[r.idx]
					return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						bg := theme.Transparent
						if i == hoveredIdx {
							bg = theme.BgHover
						}
						if ui.Net.hasApp && ui.Net.selApp.PID == r.p.PID {
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
				ui.Net.procListHover.Add(gtx.Ops)
				cl.Pop()
				pass.Pop()
				return dim
			}),
		)
	})
}

func (ui *AppUI) netLimitRow(gtx layout.Context, ed *widget.Editor, u *netUnitSel, label string) layout.Dimensions {
	th := ui.Theme
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
					return ui.netField(gtx, ed, "0")
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.netUnitChips(gtx, u)
				}),
			)
		}),
	)
}

func (ui *AppUI) netUnitChips(gtx layout.Context, u *netUnitSel) layout.Dimensions {
	th := ui.Theme
	u.ensure()
	children := make([]layout.FlexChild, 0, len(netUnits)*2)
	for i := range netUnits {
		i := i
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(3)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return netChip(gtx, th, &u.clicks[i], netUnits[i].label, u.idx == i)
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (ui *AppUI) netField(gtx layout.Context, ed *widget.Editor, hint string) layout.Dimensions {
	return widgets.TextField(gtx, ui.Theme, ed, hint, true, nil, 0, unit.Sp(13))
}

func (ui *AppUI) layoutNetlimitSection(gtx layout.Context) layout.Dimensions {
	ui.netHandleSectionClicks(gtx)
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(400 * time.Millisecond)})
	th := ui.Theme
	paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	if ui.Net.graphWindow == 0 {
		ui.Net.graphWindow = time.Minute
	}

	cards := []layout.Widget{
		ui.netGraphCard,
		layout.Spacer{Height: unit.Dp(12)}.Layout,
		ui.netDiagCard,
	}
	inset := layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(14), Left: unit.Dp(14), Right: unit.Dp(14)}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(th, &ui.Net.secList).Layout(gtx, len(cards), func(gtx layout.Context, i int) layout.Dimensions {
			return cards[i](gtx)
		})
	})
}

func (ui *AppUI) netHandleSectionClicks(gtx layout.Context) {
	for ui.Net.win30Btn.Clicked(gtx) {
		ui.Net.graphWindow = 30 * time.Second
	}
	for ui.Net.win1mBtn.Clicked(gtx) {
		ui.Net.graphWindow = time.Minute
	}
	for ui.Net.win5mBtn.Clicked(gtx) {
		ui.Net.graphWindow = 5 * time.Minute
	}
	for ui.Net.diagBtn.Clicked(gtx) {
		ui.Net.mu.Lock()
		if ui.Net.diagRunning {
			ui.Net.mu.Unlock()
			continue
		}
		ui.Net.diagRunning = true
		ui.Net.mu.Unlock()
		go func() {
			lines := ui.buildDiagnostics()
			ui.Net.mu.Lock()
			ui.Net.diagLines = lines
			ui.Net.diagRunning = false
			ui.Net.mu.Unlock()
			ui.Window.Invalidate()
		}()
	}
}

func (ui *AppUI) netGraphCard(gtx layout.Context) layout.Dimensions {
	th := ui.Theme
	interval := ui.NetMgr.Interval()
	if interval <= 0 {
		interval = 700 * time.Millisecond
	}
	slots := int(ui.Net.graphWindow / interval)
	if slots < 2 {
		slots = 2
	}
	hist := ui.NetMgr.History()
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

	return netBox(gtx, theme.BgDark, theme.Border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return netSectionLabel(gtx, th, "CURRENT TRAFFIC")
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.netIntervalSelector(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return netTrafficGraph(gtx, th, vis, slots, peakIn, peakOut)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.netCurrentNumbers(gtx, curIn, curOut, peakIn, peakOut)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return netStateBadge(gtx, th, ui.NetMgr.State(), ui.NetMgr.Spec())
				}),
			)
		})
	})
}

func (ui *AppUI) netIntervalSelector(gtx layout.Context) layout.Dimensions {
	th := ui.Theme
	w := ui.Net.graphWindow
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return netChip(gtx, th, &ui.Net.win30Btn, "30s", w == 30*time.Second)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return netChip(gtx, th, &ui.Net.win1mBtn, "1m", w == time.Minute)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return netChip(gtx, th, &ui.Net.win5mBtn, "5m", w == 5*time.Minute)
		}),
	)
}

func (ui *AppUI) netCurrentNumbers(gtx layout.Context, in, out, peakIn, peakOut int64) layout.Dimensions {
	th := ui.Theme
	w := netNumbersColWidth(gtx, th)
	gtx.Constraints.Min.X = w
	gtx.Constraints.Max.X = w
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return netSpeedRow(gtx, th, widgets.IconDownload, theme.MethodGet, formatRate(in))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return netSpeedRow(gtx, th, widgets.IconUpload, theme.MethodPost, formatRate(out))
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
	if ui.Net.scope == netlimit.ScopeApp && ui.Net.hasApp && ui.Net.caps.PerAppSpeed {
		app := ui.NetMgr.AppSpeed()
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11),
					fmt.Sprintf("%s  ↓ %s   ↑ %s", ui.Net.selApp.Name, formatRate(app.InBps), formatRate(app.OutBps)))
				lbl.Color = theme.FgHint
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func netNumbersColWidth(gtx layout.Context, th *material.Theme) int {
	measure := func(w layout.Widget) int {
		m := op.Record(gtx.Ops)
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
		d := w(g)
		m.Stop()
		return d.Size.X
	}
	rowW := measure(func(g layout.Context) layout.Dimensions {
		return netSpeedRow(g, th, widgets.IconDownload, theme.MethodGet, "1020.90 MB/s")
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

func netTrafficGraph(gtx layout.Context, th *material.Theme, vis []netlimit.TrafficPoint, slots int, peakIn, peakOut int64) layout.Dimensions {
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

		drawSeries := func(get func(netlimit.TrafficPoint) int64, line, fill color.NRGBA) {
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
		drawSeries(func(p netlimit.TrafficPoint) int64 { return p.OutBps }, theme.MethodPost, outFill)
		drawSeries(func(p netlimit.TrafficPoint) int64 { return p.InBps }, theme.MethodGet, inFill)
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

func (ui *AppUI) netDiagCard(gtx layout.Context) layout.Dimensions {
	th := ui.Theme
	ui.Net.mu.Lock()
	running := ui.Net.diagRunning
	lines := ui.Net.diagLines
	ui.Net.mu.Unlock()

	return netBox(gtx, theme.BgDark, theme.Border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			rows := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return netSectionLabel(gtx, th, "DIAGNOSTICS")
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := "Run test"
							if running {
								label = "Testing…"
							}
							return netChip(gtx, th, &ui.Net.diagBtn, label, running)
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
						return netDiagRow(gtx, th, ln)
					}),
				)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	})
}

func netDiagRow(gtx layout.Context, th *material.Theme, ln netDiagLine) layout.Dimensions {
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

func (ui *AppUI) buildDiagnostics() []netDiagLine {
	caps := ui.Net.caps
	var out []netDiagLine

	if caps.Available {
		out = append(out, netDiagLine{"Limiter backend", "available", 1})
	} else {
		out = append(out, netDiagLine{"Limiter backend", "unavailable", -1})
	}

	if caps.NeedsElevation {
		if netlimit.IsElevated() {
			out = append(out, netDiagLine{"Privileges", "elevated", 1})
		} else {
			out = append(out, netDiagLine{"Privileges", "not elevated", -1})
		}
	}

	if caps.PerAppSpeed {
		out = append(out, netDiagLine{"Per-app monitoring", "supported", 1})
	} else {
		out = append(out, netDiagLine{"Per-app monitoring", "unsupported", 0})
	}

	for _, t := range []string{"1.1.1.1:443", "8.8.8.8:53"} {
		r := netlimit.TCPPing(t, 3*time.Second)
		if r.OK {
			out = append(out, netDiagLine{"Ping " + t, fmt.Sprintf("%d ms", r.Latency.Milliseconds()), 1})
		} else {
			out = append(out, netDiagLine{"Ping " + t, "no response", -1})
		}
	}

	if ifaces, err := net.Interfaces(); err == nil {
		active := 0
		for _, in := range ifaces {
			if in.Flags&net.FlagUp != 0 && in.Flags&net.FlagLoopback == 0 {
				if addrs, _ := in.Addrs(); len(addrs) > 0 {
					active++
				}
			}
		}
		out = append(out, netDiagLine{"Active interfaces", strconv.Itoa(active), 0})
	}

	hist := ui.NetMgr.History()
	var pIn, pOut int64
	for _, p := range hist {
		if p.InBps > pIn {
			pIn = p.InBps
		}
		if p.OutBps > pOut {
			pOut = p.OutBps
		}
	}
	out = append(out, netDiagLine{"Session peak ↓", formatRate(pIn), 0})
	out = append(out, netDiagLine{"Session peak ↑", formatRate(pOut), 0})

	return out
}

func netSpeedRow(gtx layout.Context, th *material.Theme, ic *widget.Icon, col color.NRGBA, value string) layout.Dimensions {
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

func netStateBadge(gtx layout.Context, th *material.Theme, state netlimit.State, spec netlimit.LimitSpec) layout.Dimensions {
	var text string
	var col color.NRGBA
	switch state {
	case netlimit.StateActive:
		text = "Limit active — " + netSpecSummary(spec)
		col = theme.Accent
	case netlimit.StatePaused:
		text = "Limit paused — " + netSpecSummary(spec)
		col = theme.FgMuted
	default:
		text = "No active limit"
		col = theme.FgMuted
	}
	lbl := material.Label(th, unit.Sp(12), text)
	lbl.Color = col
	return lbl.Layout(gtx)
}

func netSpecSummary(spec netlimit.LimitSpec) string {
	parts := []string{}
	if spec.Scope == netlimit.ScopeApp && spec.AppName != "" {
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

func netSectionLabel(gtx layout.Context, th *material.Theme, txt string) layout.Dimensions {
	lbl := material.Label(th, unit.Sp(11), txt)
	lbl.Color = theme.FgMuted
	return lbl.Layout(gtx)
}

func netToggle(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	bg := theme.BgField
	fg := theme.FgMuted
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return netButton(gtx, th, clk, label, bg, fg, true)
}

func netButton(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, bg, fg color.NRGBA, enabled bool) layout.Dimensions {
	if !enabled {
		bg = theme.BgSecondary
		fg = theme.FgDisabled
	}
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if enabled && clk.Hovered() {
			bg = netLighten(bg)
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

func netChip(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	bg := theme.BgField
	fg := theme.FgMuted
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if !active && clk.Hovered() {
			bg = netLighten(bg)
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

func netBox(gtx layout.Context, bg, border color.NRGBA, w layout.Widget) layout.Dimensions {
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

func netLighten(c color.NRGBA) color.NRGBA {
	add := func(v uint8) uint8 {
		n := int(v) + 18
		if n > 255 {
			n = 255
		}
		return uint8(n)
	}
	return color.NRGBA{R: add(c.R), G: add(c.G), B: add(c.B), A: c.A}
}
