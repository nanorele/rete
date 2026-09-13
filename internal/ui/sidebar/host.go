package sidebar

import (
	"image"

	"rete/internal/model"
	"rete/internal/ui/collections"
	"rete/internal/ui/colorpicker"
	"rete/internal/ui/environments"
	"rete/internal/ui/widgets"
	"rete/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type Host struct {
	Theme    *material.Theme
	Window   *app.Window
	Settings *model.AppSettings

	Collections  *[]*collections.CollectionUI
	VisibleCols  *[]*collections.CollectionNode
	Environments *[]*environments.EnvironmentUI
	Tabs         *[]*workspace.RequestTab
	ActiveIdx    *int

	RenamingNode    **collections.CollectionNode
	EditingEnv      **environments.EnvironmentUI
	PendingEnvClose **environments.EnvironmentUI
	DraggedNode     **collections.CollectionNode
	DraggedEnv      **environments.EnvironmentUI
	ActiveEnvID     *string

	DragNodeOriginY  *float32
	DragNodeCurrentY *float32
	DragNodeOriginX  *float32
	DragNodeCurrentX *float32
	DragNodeActive   *bool
	DragNodeWinOrig  *f32.Point
	DragNodeWinPos   *f32.Point

	DragEnvOriginY  *float32
	DragEnvCurrentY *float32
	DragEnvActive   *bool

	ColRowH       *int
	EnvRowH       *int
	ColRowYs      *map[int]int
	ColAfterLastY *int
	WindowSize    *image.Point

	StickyRows   []*collections.CollectionNode
	StickyBandH  *int
	StickyScroll *gesture.Scroll

	ColBarScroll    *gesture.Scroll
	EnvBarScroll    *gesture.Scroll
	ScriptBarScroll *gesture.Scroll

	SidebarEnvHeight *int
	SidebarEnvDrag   *gesture.Drag
	SidebarEnvDragY  *float32
	EnvDividerY      *int

	ColList         *widget.List
	EnvList         *widget.List
	ColsHeaderClick *widget.Clickable
	EnvsHeaderClick *widget.Clickable
	ColsExpanded    *bool
	EnvsExpanded    *bool
	ImportBtn       *widget.Clickable
	AddColBtn       *widget.Clickable
	ColsMenuBtn     *widget.Clickable
	ColsExpandAll   *widget.Clickable
	ColsCollapseAll *widget.Clickable
	ColsMenuOpen    *bool
	ImportEnvBtn    *widget.Clickable
	AddEnvBtn       *widget.Clickable
	EnvsMenuBtn     *widget.Clickable
	EnvsMenuOpen    *bool
	SidebarDropTag  *bool

	Scripts            *[]*ScriptRow
	ScriptList         *widget.List
	ScriptsHeaderClick *widget.Clickable
	ScriptsExpanded    *bool
	AddScriptBtn       *widget.Clickable
	ScriptsMenuBtn     *widget.Clickable
	ScriptsMenuOpen    *bool
	ImportScriptBtn    *widget.Clickable
	ScriptRowH         *int
	ScriptsHeight      *int
	ScriptsDrag        *gesture.Drag
	ScriptsDragY       *float32
	ScriptsDividerY    *int
	DraggedScript      **ScriptRow
	DragScriptOriginY  *float32
	DragScriptCurrentY *float32
	DragScriptActive   *bool

	DropZones *[]DropZoneRect

	ColsBodyHover    *widgets.Hover
	ScriptsBodyHover *widgets.Hover
	EnvsBodyHover    *widgets.Hover
	ColsBodyFade     *widgets.Fade
	ScriptsBodyFade  *widgets.Fade
	EnvsBodyFade     *widgets.Fade

	ActiveScriptID  func() string
	OpenScript      func(id string)
	NewScript       func()
	RenameScript    func(id, name string)
	DuplicateScript func(id string)
	DeleteScript    func(id string)
	ImportScript    func(data []byte)
	ReorderScripts  func(ids []string)

	EnvColorPicker *colorpicker.State
	EnvColorEnvID  *string

	ActiveEnvDirty *bool

	ChooseJSONFile func() ([]byte, error)

	SaveState             func()
	PushColLoaded         func(*collections.CollectionUI)
	MarkCollectionDirty   func(*collections.ParsedCollection)
	OpenRequestInTab      func(*collections.CollectionNode)
	SwitchSection         func(string)
	UpdateVisibleCols     func()
	PushEnvLoaded         func(*environments.EnvironmentUI)
	CommitEditingEnv      func()
	CloseTab              func(int)
	DeleteCollection      func(colID string)
	DropNodeExternal      func(*collections.CollectionNode) bool
	LayoutToggleBtn       func(gtx layout.Context) layout.Dimensions
	LayoutSectionRequests func(gtx layout.Context) layout.Dimensions
	LayoutSectionFlows    func(gtx layout.Context) layout.Dimensions
	LayoutSectionNetlimit func(gtx layout.Context) layout.Dimensions
	LayoutNetlimitBody    func(gtx layout.Context) layout.Dimensions
	LayoutSectionMITM     func(gtx layout.Context) layout.Dimensions
	LayoutMITMRules       func(gtx layout.Context) layout.Dimensions
	LayoutSectionHAR      func(gtx layout.Context) layout.Dimensions
	SidebarSection        *string
}

type DropZoneRect struct {
	ID   string
	Rect image.Rectangle
}

func (h *Host) HideSidebar() bool {
	return h.Settings != nil && h.Settings.HideSidebar
}

func (h *Host) ensureScripts() {
	if h.Scripts == nil {
		h.Scripts = new([]*ScriptRow)
	}
	if h.ScriptList == nil {
		h.ScriptList = &widget.List{List: layout.List{Axis: layout.Vertical}}
	}
	if h.ScriptsHeaderClick == nil {
		h.ScriptsHeaderClick = &widget.Clickable{}
	}
	if h.ScriptsExpanded == nil {
		h.ScriptsExpanded = new(bool)
	}
	if h.AddScriptBtn == nil {
		h.AddScriptBtn = &widget.Clickable{}
	}
	if h.ScriptsMenuBtn == nil {
		h.ScriptsMenuBtn = &widget.Clickable{}
	}
	if h.ScriptsMenuOpen == nil {
		h.ScriptsMenuOpen = new(bool)
	}
	if h.ImportScriptBtn == nil {
		h.ImportScriptBtn = &widget.Clickable{}
	}
	if h.ScriptRowH == nil {
		h.ScriptRowH = new(int)
	}
	if h.ScriptsHeight == nil {
		h.ScriptsHeight = new(int)
	}
	if h.ScriptsDrag == nil {
		h.ScriptsDrag = new(gesture.Drag)
	}
	if h.ScriptsDragY == nil {
		h.ScriptsDragY = new(float32)
	}
	if h.ScriptsDividerY == nil {
		h.ScriptsDividerY = new(int)
	}
	if h.DraggedScript == nil {
		h.DraggedScript = new(*ScriptRow)
	}
	if h.DragScriptOriginY == nil {
		h.DragScriptOriginY = new(float32)
	}
	if h.DragScriptCurrentY == nil {
		h.DragScriptCurrentY = new(float32)
	}
	if h.DragScriptActive == nil {
		h.DragScriptActive = new(bool)
	}
	if h.EnvDividerY == nil {
		h.EnvDividerY = new(int)
	}
}
