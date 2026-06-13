package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
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

	inEd    widget.Editor
	outEd   widget.Editor
	totalEd widget.Editor
	unitMB  bool
	unitBtn widget.Clickable

	startBtn  widget.Clickable
	stopBtn   widget.Clickable
	resumeBtn widget.Clickable
	cancelBtn widget.Clickable
	relaunch  widget.Clickable

	pickBtn    widget.Clickable
	pickerOpen bool
	searchEd   widget.Editor
	procList   widget.List
	procClicks []widget.Clickable
	selApp     netlimit.ProcInfo
	hasApp     bool

	orphan         bool
	clearOrphanBtn widget.Clickable

	mu           sync.Mutex
	procs        []netlimit.ProcInfo
	procsLoading bool
	lastErr      string
}

type netConfig struct {
	Scope   int    `json:"scope"`
	AppPath string `json:"app_path,omitempty"`
	AppName string `json:"app_name,omitempty"`
	In      string `json:"in,omitempty"`
	Out     string `json:"out,omitempty"`
	Total   string `json:"total,omitempty"`
	UnitMB  bool   `json:"unit_mb"`
}

func (ui *AppUI) initNetlimit() {
	ui.NetMgr = netlimit.New()
	ui.NetMgr.SetMarkerPath(persist.NetlimitMarkerPath())
	ui.Net.caps = ui.NetMgr.Caps()
	ui.Net.inEd.SingleLine = true
	ui.Net.outEd.SingleLine = true
	ui.Net.totalEd.SingleLine = true
	ui.Net.searchEd.SingleLine = true
	ui.Net.unitMB = true
	ui.Net.procList.Axis = layout.Vertical
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
	ui.Net.unitMB = c.UnitMB
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
		UnitMB:  ui.Net.unitMB,
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
	mul := int64(1024)
	if ui.Net.unitMB {
		mul = 1024 * 1024
	}
	parse := func(ed *widget.Editor) int64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(ed.Text()), 64)
		if err != nil || v <= 0 {
			return 0
		}
		return int64(v * float64(mul))
	}
	spec := netlimit.LimitSpec{
		Scope:    ui.Net.scope,
		InBps:    parse(&ui.Net.inEd),
		OutBps:   parse(&ui.Net.outEd),
		TotalBps: parse(&ui.Net.totalEd),
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
	for ui.Net.unitBtn.Clicked(gtx) {
		ui.Net.unitMB = !ui.Net.unitMB
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
			unitLabel := "KB/s"
			if ui.Net.unitMB {
				unitLabel = "MB/s"
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return netSectionLabel(gtx, th, "LIMITS")
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return netButton(gtx, th, &ui.Net.unitBtn, unitLabel, theme.BgSecondary, theme.Fg, true)
				}),
			)
		})
		gap(6)
		add(func(gtx layout.Context) layout.Dimensions {
			return ui.netField(gtx, &ui.Net.inEd, "Download (in)")
		})
		gap(6)
		add(func(gtx layout.Context) layout.Dimensions {
			return ui.netField(gtx, &ui.Net.outEd, "Upload (out)")
		})
		gap(6)
		add(func(gtx layout.Context) layout.Dimensions {
			return ui.netField(gtx, &ui.Net.totalEd, "Total")
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
			return netButton(gtx, th, &ui.Net.relaunch, "Restart as administrator", theme.Accent, theme.AccentFg, true)
		})
		return
	}

	switch state {
	case netlimit.StateIdle:
		add(func(gtx layout.Context) layout.Dimensions {
			return netButton(gtx, th, &ui.Net.startBtn, "Start limiting", theme.Accent, theme.AccentFg, ui.Net.caps.Available)
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
					return netButton(gtx, th, &ui.Net.resumeBtn, "Resume", theme.Accent, theme.AccentFg, true)
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
				return material.List(th, &ui.Net.procList).Layout(gtx, len(visible), func(gtx layout.Context, i int) layout.Dimensions {
					r := visible[i]
					clk := &ui.Net.procClicks[r.idx]
					return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						bg := theme.Transparent
						if clk.Hovered() {
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
			}),
		)
	})
}

func (ui *AppUI) netField(gtx layout.Context, ed *widget.Editor, hint string) layout.Dimensions {
	th := ui.Theme
	return netBox(gtx, theme.BgField, theme.BorderLight, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(7)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			me := material.Editor(th, ed, hint)
			me.TextSize = unit.Sp(13)
			me.Color = theme.Fg
			me.HintColor = theme.FgMuted
			return me.Layout(gtx)
		})
	})
}

func (ui *AppUI) layoutNetlimitSection(gtx layout.Context) layout.Dimensions {
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(500 * time.Millisecond)})
	th := ui.Theme
	paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	sys := ui.NetMgr.SystemSpeed()
	state := ui.NetMgr.State()

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(13), "Current traffic")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return netSpeedRow(gtx, th, widgets.IconDownload, theme.MethodGet, formatRate(sys.InBps))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return netSpeedRow(gtx, th, widgets.IconUpload, theme.MethodPost, formatRate(sys.OutBps))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.netAppSpeed(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return netStateBadge(gtx, th, state, ui.NetMgr.Spec())
			}),
		)
	})
}

func (ui *AppUI) netAppSpeed(gtx layout.Context) layout.Dimensions {
	th := ui.Theme
	if ui.Net.scope != netlimit.ScopeApp || !ui.Net.hasApp || !ui.Net.caps.PerAppSpeed {
		return layout.Dimensions{}
	}
	app := ui.NetMgr.AppSpeed()
	lbl := material.Label(th, unit.Sp(12),
		fmt.Sprintf("%s — ↓ %s   ↑ %s", ui.Net.selApp.Name, formatRate(app.InBps), formatRate(app.OutBps)))
	lbl.Color = theme.FgHint
	return lbl.Layout(gtx)
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
		bg = theme.Accent
		fg = theme.AccentFg
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
