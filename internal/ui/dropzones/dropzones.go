package dropzones

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
	"tracto/internal/ui/sidebar"
	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/app"
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

type Host struct {
	Theme  *material.Theme
	Window *app.Window

	Blocked        bool
	SidebarSection string
	SidebarHidden  bool
	SidebarZones   []sidebar.DropZoneRect

	LoadHAR       func(path string)
	ImportData    func(data []byte)
	PushColLoaded func(*collections.CollectionUI)
	PushEnvLoaded func(*environments.EnvironmentUI)
}

type State struct {
	host *Host

	mu     sync.Mutex
	active bool
	pos    f32.Point
	TopY   int
	zones  []dropZone

	dropped chan droppedPayload
}

type dropZone struct {
	id    string
	label string
	rect  image.Rectangle
}

func (s *State) Init() {
	s.dropped = make(chan droppedPayload, 8)
}

func (s *State) Dragged(host *Host, pos f32.Point, active bool) {
	s.host = host
	s.mu.Lock()
	s.active = active
	s.pos = pos
	s.mu.Unlock()
	if host.Window != nil {
		host.Window.Invalidate()
	}
}

func (s *State) RebuildZones(gtx layout.Context, host *Host) {
	s.host = host
	s.zones = s.zones[:0]
	if host.Blocked {
		return
	}
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	topY := s.TopY
	if topY <= 0 || topY >= winH || winW <= 0 {
		return
	}

	switch host.SidebarSection {
	case "har":
		s.zones = append(s.zones, dropZone{
			id: "har", label: "Drop a .har file to import",
			rect: image.Rect(0, topY, winW, winH),
		})
	case "mitm", "netlimit":
		return
	default:
		if host.SidebarHidden {
			return
		}
		for _, z := range host.SidebarZones {
			s.zones = append(s.zones, dropZone{
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

func (s *State) zoneAt(pos f32.Point) string {
	p := image.Pt(int(pos.X), int(pos.Y))
	for _, z := range s.zones {
		if p.In(z.rect) {
			return z.id
		}
	}
	return ""
}

func (s *State) LayoutOverlay(gtx layout.Context, host *Host) {
	s.host = host
	s.mu.Lock()
	active := s.active
	pos := s.pos
	s.mu.Unlock()
	if !active || len(s.zones) == 0 {
		return
	}
	hovered := s.zoneAt(pos)
	border := gtx.Dp(unit.Dp(2))
	for _, z := range s.zones {
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
		s.drawZoneLabel(gtx, z.rect, z.label, labelCol)
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

func (s *State) drawZoneLabel(gtx layout.Context, rect image.Rectangle, label string, col color.NRGBA) {
	off := op.Offset(rect.Min).Push(gtx.Ops)
	cgtx := gtx
	cgtx.Constraints = layout.Exact(rect.Size())
	layout.Center.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(s.host.Theme, unit.Sp(13), label)
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

func importDataAs(host *Host, data []byte, kind importKind) {
	switch kind {
	case importKindCollection:
		importCollectionData(host, data)
	case importKindEnvironment:
		importEnvironmentData(host, data)
	case importKindScript:
		if _, err := flow.ImportScenario(data); err == nil && host.Window != nil {
			host.Window.Invalidate()
		}
	default:
		host.ImportData(data)
	}
}

func importCollectionData(host *Host, data []byte) {
	go func() {
		id := persist.NewRandomID()
		col, err := collections.ParseCollection(bytes.NewReader(data), id)
		if err != nil || col == nil || col.Name == "" {
			return
		}
		if werr := persist.AtomicWriteFile(filepath.Join(persist.CollectionsDir(), id+".json"), data); werr == nil {
			host.PushColLoaded(&collections.CollectionUI{Data: col})
		}
	}()
}

func importEnvironmentData(host *Host, data []byte) {
	go func() {
		id := persist.NewRandomID()
		env, err := environments.ParseEnvironment(bytes.NewReader(data), id)
		if err != nil || env == nil || env.Name == "" {
			return
		}
		if werr := persist.AtomicWriteFile(filepath.Join(persist.EnvironmentsDir(), id+".json"), data); werr == nil {
			host.PushEnvLoaded(&environments.EnvironmentUI{Data: env})
		}
	}()
}
