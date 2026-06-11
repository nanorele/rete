package ui

import (
	"io"
	"os"
	"path/filepath"

	"tracto/internal/persist"
	"tracto/internal/ui/sidebar"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) sidebarHost() *sidebar.Host {
	return &sidebar.Host{
		Theme:    ui.Theme,
		Window:   ui.Window,
		Settings: &ui.Settings,

		Collections:  &ui.Collections,
		VisibleCols:  &ui.VisibleCols,
		Environments: &ui.Environments,
		Tabs:         &ui.Tabs,
		ActiveIdx:    &ui.ActiveIdx,

		RenamingNode:    &ui.RenamingNode,
		EditingEnv:      &ui.EditingEnv,
		PendingEnvClose: &ui.pendingEnvClose,
		DraggedNode:     &ui.DraggedNode,
		DraggedEnv:      &ui.DraggedEnv,
		ActiveEnvID:     &ui.ActiveEnvID,

		DragNodeOriginY:  &ui.DragNodeOriginY,
		DragNodeCurrentY: &ui.DragNodeCurrentY,
		DragNodeOriginX:  &ui.DragNodeOriginX,
		DragNodeCurrentX: &ui.DragNodeCurrentX,
		DragNodeActive:   &ui.DragNodeActive,

		DragEnvOriginY:  &ui.DragEnvOriginY,
		DragEnvCurrentY: &ui.DragEnvCurrentY,
		DragEnvActive:   &ui.DragEnvActive,

		ColRowH:       &ui.colRowH,
		EnvRowH:       &ui.envRowH,
		ColRowYs:      &ui.colRowYs,
		ColAfterLastY: &ui.colAfterLastY,
		WindowSize:    &ui.windowSize,

		SidebarEnvHeight: &ui.SidebarEnvHeight,
		SidebarEnvDrag:   &ui.SidebarEnvDrag,
		SidebarEnvDragY:  &ui.SidebarEnvDragY,

		ColList:         &ui.ColList,
		EnvList:         &ui.EnvList,
		ColsHeaderClick: &ui.ColsHeaderClick,
		EnvsHeaderClick: &ui.EnvsHeaderClick,
		ColsExpanded:    &ui.ColsExpanded,
		EnvsExpanded:    &ui.EnvsExpanded,
		ImportBtn:       &ui.ImportBtn,
		AddColBtn:       &ui.AddColBtn,
		ColsMenuBtn:     &ui.ColsMenuBtn,
		ColsMenuOpen:    &ui.ColsMenuOpen,
		ImportEnvBtn:    &ui.ImportEnvBtn,
		AddEnvBtn:       &ui.AddEnvBtn,
		EnvsMenuBtn:     &ui.EnvsMenuBtn,
		EnvsMenuOpen:    &ui.EnvsMenuOpen,
		SidebarDropTag:  &ui.SidebarDropTag,

		EnvColorPicker: &ui.EnvColorPicker,
		EnvColorEnvID:  &ui.EnvColorEnvID,

		ActiveEnvDirty: &ui.activeEnvDirty,

		ChooseJSONFile: func() ([]byte, error) {
			file, err := ui.Explorer.ChooseFile("json")
			if err != nil || file == nil {
				return nil, err
			}
			defer func() { _ = file.Close() }()
			return io.ReadAll(file)
		},

		SaveState:           ui.saveState,
		PushColLoaded:       ui.pushColLoaded,
		MarkCollectionDirty: ui.markCollectionDirty,
		OpenRequestInTab:    ui.openRequestInTab,
		UpdateVisibleCols:   ui.updateVisibleCols,
		PushEnvLoaded:       ui.pushEnvLoaded,
		CommitEditingEnv:    ui.commitEditingEnv,
		CloseTab:            ui.closeTab,
		DeleteCollection: func(colID string) {
			delete(ui.dirtyCollections, colID)
			ui.collectionSaveMu.Lock()
			if ui.deletedCollections == nil {
				ui.deletedCollections = make(map[string]struct{})
			}
			ui.deletedCollections[colID] = struct{}{}
			_ = os.Remove(filepath.Join(persist.CollectionsDir(), colID+".json"))
			ui.collectionSaveMu.Unlock()
		},
		LayoutToggleBtn:       ui.layoutSidebarToggleBtn,
		LayoutSectionRequests: ui.layoutSidebarSectionRequestsBtn,
		LayoutSectionMITM:     ui.layoutSidebarSectionMITMBtn,
		LayoutMITMRules:       ui.layoutMITMSidebar,
		SidebarSection:        &ui.SidebarSection,
	}
}

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	dims := sidebar.Layout(gtx, ui.sidebarHost())
	if ui.ColList.Position.First != ui.prevColFirst || ui.ColList.Position.Offset != ui.prevColOffset ||
		ui.EnvList.Position.First != ui.prevEnvFirst || ui.EnvList.Position.Offset != ui.prevEnvOffset {
		ui.Window.Invalidate()
	}
	ui.prevColFirst = ui.ColList.Position.First
	ui.prevColOffset = ui.ColList.Position.Offset
	ui.prevEnvFirst = ui.EnvList.Position.First
	ui.prevEnvOffset = ui.EnvList.Position.Offset
	return dims
}
