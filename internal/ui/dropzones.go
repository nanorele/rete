package ui

import (
	"bytes"
	"image"
	"image/color"
	"path/filepath"
	"sync"

	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/flow"
	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type dndState struct {
	mu     sync.Mutex
	active bool
	pos    f32.Point
	topY   int
	zones  []dropZone
}

type dropZone struct {
	id    string
	label string
	rect  image.Rectangle
}

func (ui *AppUI) onOSFilesDragged(pos f32.Point, active bool) {
	ui.dnd.mu.Lock()
	ui.dnd.active = active
	ui.dnd.pos = pos
	ui.dnd.mu.Unlock()
	if ui.Window != nil {
		ui.Window.Invalidate()
	}
}

func (ui *AppUI) rebuildDropZones(gtx layout.Context) {
	ui.dnd.zones = ui.dnd.zones[:0]
	if ui.SettingsOpen || ui.EditingEnv != nil {
		return
	}
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	topY := ui.dnd.topY
	if topY <= 0 || topY >= winH || winW <= 0 {
		return
	}

	switch ui.SidebarSection {
	case "har":
		ui.dnd.zones = append(ui.dnd.zones, dropZone{
			id: "har", label: "Drop a .har file to import",
			rect: image.Rect(0, topY, winW, winH),
		})
	case "mitm", "netlimit":
		return
	default:
		if ui.hideSidebar() {
			return
		}
		for _, z := range ui.sidebarZones {
			ui.dnd.zones = append(ui.dnd.zones, dropZone{
				id:    z.ID,
				label: dropZoneLabel(z.ID),
				rect:  z.Rect.Add(image.Pt(0, topY)),
			})
		}
	}
}

func dropZoneLabel(id string) string {
	switch id {
	case "collections":
		return "Collections"
	case "scripts":
		return "Scripts"
	case "variables":
		return "Variables"
	default:
		return id
	}
}

func (ui *AppUI) zoneAt(pos f32.Point) string {
	p := image.Pt(int(pos.X), int(pos.Y))
	for _, z := range ui.dnd.zones {
		if p.In(z.rect) {
			return z.id
		}
	}
	return ""
}

func (ui *AppUI) layoutDropOverlay(gtx layout.Context) {
	ui.dnd.mu.Lock()
	active := ui.dnd.active
	pos := ui.dnd.pos
	ui.dnd.mu.Unlock()
	if !active || len(ui.dnd.zones) == 0 {
		return
	}
	hovered := ui.zoneAt(pos)
	border := gtx.Dp(unit.Dp(2))
	for _, z := range ui.dnd.zones {
		isHover := z.id == hovered
		fill := theme.WithAlpha(theme.Accent, 36)
		borderCol := theme.WithAlpha(theme.Accent, 120)
		labelCol := theme.FgMuted
		if isHover {
			fill = theme.WithAlpha(theme.Accent, 110)
			borderCol = theme.AccentHover
			labelCol = theme.White
		}
		paint.FillShape(gtx.Ops, fill, clip.Rect(z.rect).Op())
		strokeRect(gtx, z.rect, borderCol, border)
		ui.drawZoneLabel(gtx, z.rect, z.label, labelCol)
	}
}

func strokeRect(gtx layout.Context, r image.Rectangle, col color.NRGBA, w int) {
	if w <= 0 {
		w = 1
	}
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: r.Min, Max: image.Pt(r.Max.X, r.Min.Y+w)}.Op())
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(r.Min.X, r.Max.Y-w), Max: r.Max}.Op())
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: r.Min, Max: image.Pt(r.Min.X+w, r.Max.Y)}.Op())
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(r.Max.X-w, r.Min.Y), Max: r.Max}.Op())
}

func (ui *AppUI) drawZoneLabel(gtx layout.Context, rect image.Rectangle, label string, col color.NRGBA) {
	off := op.Offset(rect.Min).Push(gtx.Ops)
	cgtx := gtx
	cgtx.Constraints = layout.Exact(rect.Size())
	layout.Center.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(ui.Theme, unit.Sp(13), label)
		lbl.Color = col
		lbl.Font.Weight = font.Bold
		lbl.Alignment = text.Middle
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
	off.Pop()
}

type importKind int

const (
	importKindAuto importKind = iota
	importKindCollection
	importKindEnvironment
	importKindScript
)

func zoneImportKind(zone string) importKind {
	switch zone {
	case "collections":
		return importKindCollection
	case "variables":
		return importKindEnvironment
	case "scripts":
		return importKindScript
	default:
		return importKindAuto
	}
}

func (ui *AppUI) importDataAs(data []byte, kind importKind) {
	switch kind {
	case importKindCollection:
		ui.importCollectionData(data)
	case importKindEnvironment:
		ui.importEnvironmentData(data)
	case importKindScript:
		if _, err := flow.ImportScenario(data); err == nil && ui.Window != nil {
			ui.Window.Invalidate()
		}
	default:
		ui.importDroppedData(data)
	}
}

func (ui *AppUI) importCollectionData(data []byte) {
	go func() {
		id := persist.NewRandomID()
		col, err := collections.ParseCollection(bytes.NewReader(data), id)
		if err != nil || col == nil || col.Name == "" {
			return
		}
		if werr := persist.AtomicWriteFile(filepath.Join(persist.CollectionsDir(), id+".json"), data); werr == nil {
			ui.pushColLoaded(&collections.CollectionUI{Data: col})
		}
	}()
}

func (ui *AppUI) importEnvironmentData(data []byte) {
	go func() {
		id := persist.NewRandomID()
		env, err := environments.ParseEnvironment(bytes.NewReader(data), id)
		if err != nil || env == nil || env.Name == "" {
			return
		}
		if werr := persist.AtomicWriteFile(filepath.Join(persist.EnvironmentsDir(), id+".json"), data); werr == nil {
			ui.pushEnvLoaded(&environments.EnvironmentUI{Data: env})
		}
	}()
}
