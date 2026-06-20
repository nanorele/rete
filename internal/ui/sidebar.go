package ui

import (
	"io"
	"os"
	"path/filepath"

	"tracto/internal/persist"
	"tracto/internal/ui/flow"
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
		DragNodeWinOrig:  &ui.DragNodeWinOrig,
		DragNodeWinPos:   &ui.DragNodeWinPos,

		DragEnvOriginY:  &ui.DragEnvOriginY,
		DragEnvCurrentY: &ui.DragEnvCurrentY,
		DragEnvActive:   &ui.DragEnvActive,

		ColRowH:       &ui.colRowH,
		StickyBandH:   &ui.stickyBandH,
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
		ColsExpandAll:   &ui.ColsExpandAll,
		ColsCollapseAll: &ui.ColsCollapseAll,
		ColsMenuOpen:    &ui.ColsMenuOpen,
		ImportEnvBtn:    &ui.ImportEnvBtn,
		AddEnvBtn:       &ui.AddEnvBtn,
		EnvsMenuBtn:     &ui.EnvsMenuBtn,
		EnvsMenuOpen:    &ui.EnvsMenuOpen,
		SidebarDropTag:  &ui.SidebarDropTag,

		Scripts:            &ui.ScriptRows,
		ScriptList:         &ui.ScriptList,
		ScriptsHeaderClick: &ui.ScriptsHeaderClick,
		ScriptsExpanded:    &ui.ScriptsExpanded,
		AddScriptBtn:       &ui.AddScriptBtn,
		ScriptsMenuBtn:     &ui.ScriptsMenuBtn,
		ScriptsMenuOpen:    &ui.ScriptsMenuOpen,
		ImportScriptBtn:    &ui.ImportScriptBtn,
		ScriptRowH:         &ui.scriptRowH,
		ScriptsHeight:      &ui.SidebarScriptsHeight,
		ScriptsDrag:        &ui.SidebarScriptsDrag,
		ScriptsDragY:       &ui.SidebarScriptsDragY,

		ColsBodyHover:    &ui.ColsBodyHover,
		ScriptsBodyHover: &ui.ScriptsBodyHover,
		EnvsBodyHover:    &ui.EnvsBodyHover,
		ColsBodyFade:     &ui.ColsBodyFade,
		ScriptsBodyFade:  &ui.ScriptsBodyFade,
		EnvsBodyFade:     &ui.EnvsBodyFade,

		ActiveScriptID: func() string {
			if ui.Flow != nil && ui.Flow.Scenario != nil {
				return ui.Flow.Scenario.ID
			}
			return ""
		},
		OpenScript: func(id string) {
			if ui.Flow == nil {
				ui.Flow = flow.NewEditor()
			}
			if ui.Flow.OpenScenario(id) {
				ui.SetSidebarSection("flows")
				ui.saveState()
			}
			ui.Window.Invalidate()
		},
		NewScript: func() {
			if ui.Flow == nil {
				ui.Flow = flow.NewEditor()
			}
			ui.Flow.CreateNew()
			ui.SetSidebarSection("flows")
			ui.saveState()
			ui.Window.Invalidate()
		},
		RenameScript: func(id, name string) {
			if err := flow.RenameScenario(id, name); err == nil {
				if ui.Flow != nil && ui.Flow.Scenario != nil && ui.Flow.Scenario.ID == id {
					ui.Flow.Scenario.NameEd.SetText(name)
				}
				ui.Window.Invalidate()
			}
		},
		DuplicateScript: func(id string) {
			_, _ = flow.DuplicateScenario(id)
			ui.Window.Invalidate()
		},
		DeleteScript: func(id string) {
			_ = flow.DeleteScenario(id)
			ui.Window.Invalidate()
		},
		ImportScript: func(data []byte) {
			if _, err := flow.ImportScenario(data); err == nil {
				ui.Window.Invalidate()
			}
		},

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
		SwitchSection: func(id string) {
			ui.SetSidebarSection(id)
			ui.saveState()
			ui.Window.Invalidate()
		},
		UpdateVisibleCols: ui.updateVisibleCols,
		PushEnvLoaded:     ui.pushEnvLoaded,
		CommitEditingEnv:  ui.commitEditingEnv,
		CloseTab:          ui.closeTab,
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
		DropNodeExternal:      ui.dropNodeOnFlowCanvas,
		LayoutToggleBtn:       ui.layoutSidebarToggleBtn,
		LayoutSectionRequests: ui.layoutSidebarSectionRequestsBtn,
		LayoutSectionFlows:    ui.layoutSidebarSectionFlowsBtn,
		LayoutSectionNetlimit: ui.layoutSidebarSectionNetlimitBtn,
		LayoutNetlimitBody:    ui.layoutNetlimitBody,
		LayoutSectionMITM:     ui.layoutSidebarSectionMITMBtn,
		LayoutMITMRules:       ui.layoutMITMSidebar,
		SidebarSection:        &ui.SidebarSection,
	}
}

func (ui *AppUI) refreshScriptRows() {
	seq := flow.ChangeSeq()
	if ui.scriptSeq == seq && ui.ScriptRows != nil {
		return
	}
	ui.scriptSeq = seq
	infos := flow.ListScenarios()
	old := make(map[string]*sidebar.ScriptRow, len(ui.ScriptRows))
	for _, r := range ui.ScriptRows {
		old[r.ID] = r
	}
	rows := make([]*sidebar.ScriptRow, 0, len(infos))
	for _, inf := range infos {
		name := inf.Name
		if name == "" {
			name = "Untitled"
		}
		if r, ok := old[inf.ID]; ok {
			if !r.IsRenaming {
				r.Name = name
			}
			rows = append(rows, r)
		} else {
			rows = append(rows, &sidebar.ScriptRow{ID: inf.ID, Name: name})
		}
	}
	ui.ScriptRows = rows
}

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	ui.refreshScriptRows()
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
