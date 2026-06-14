package sidebar

import (
	"image"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"

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

	SidebarEnvHeight *int
	SidebarEnvDrag   *gesture.Drag
	SidebarEnvDragY  *float32

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
	SidebarSection        *string
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
}
