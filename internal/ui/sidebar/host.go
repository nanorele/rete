package sidebar

import (
	"image"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
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
	ImportEnvBtn    *widget.Clickable
	AddEnvBtn       *widget.Clickable
	SidebarDropTag  *bool

	EnvColorPicker *colorpicker.State
	EnvColorEnvID  *string

	ActiveEnvDirty *bool

	ChooseJSONFile func() ([]byte, error)

	SaveState           func()
	PushColLoaded       func(*collections.CollectionUI)
	MarkCollectionDirty func(*collections.ParsedCollection)
	OpenRequestInTab    func(*collections.CollectionNode)
	UpdateVisibleCols   func()
	PushEnvLoaded       func(*environments.EnvironmentUI)
	CommitEditingEnv    func()
	CloseTab            func(int)
	DeleteCollection    func(colID string)
	LayoutToggleBtn     func(gtx layout.Context) layout.Dimensions
}
