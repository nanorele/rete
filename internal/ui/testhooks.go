package ui

import (
	"tracto/internal/har"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/sidebar"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) LayoutApp(gtx layout.Context) layout.Dimensions {
	return ui.layoutApp(gtx)
}

func (ui *AppUI) LayoutContent(gtx layout.Context) layout.Dimensions {
	return ui.layoutContent(gtx)
}

func (ui *AppUI) LayoutSidebar(gtx layout.Context) layout.Dimensions {
	return ui.layoutSidebar(gtx)
}

func (ui *AppUI) LayoutTabBar(gtx layout.Context) layout.Dimensions {
	return ui.layoutTabBar(gtx)
}

func (ui *AppUI) LayoutTitleBar(gtx layout.Context) layout.Dimensions {
	return ui.layoutTitleBar(gtx)
}

func (ui *AppUI) LayoutHARSection(gtx layout.Context) layout.Dimensions {
	return ui.layoutHARSection(gtx)
}

func (ui *AppUI) LayoutMITMSection(gtx layout.Context) layout.Dimensions {
	return ui.layoutMITMSection(gtx)
}

func (ui *AppUI) LayoutMITMSidebar(gtx layout.Context) layout.Dimensions {
	return ui.layoutMITMSidebar(gtx)
}

func (ui *AppUI) LayoutNetlimitBody(gtx layout.Context) layout.Dimensions {
	return ui.layoutNetlimitBody(gtx)
}

func (ui *AppUI) LayoutNetlimitSection(gtx layout.Context) layout.Dimensions {
	return ui.layoutNetlimitSection(gtx)
}

func (ui *AppUI) LayoutEnvEditor(gtx layout.Context) layout.Dimensions {
	return ui.layoutEnvEditor(gtx)
}

func (ui *AppUI) WireNetTitlebar() {
	ui.wireNetTitlebar()
}

func (ui *AppUI) UpdateVisibleCols() {
	ui.updateVisibleCols()
}

func (ui *AppUI) RefreshActiveEnv() {
	ui.refreshActiveEnv()
}

func (ui *AppUI) CloseTab(idx int) {
	ui.closeTab(idx)
}

func (ui *AppUI) OpenRequestInTab(node *collections.CollectionNode) {
	ui.openRequestInTab(node)
}

func (ui *AppUI) RevealLinkedNode(tab *workspace.RequestTab) {
	ui.revealLinkedNode(tab)
}

func (ui *AppUI) RelinkTabs() {
	ui.relinkTabs()
}

func (ui *AppUI) SaveStateSync() {
	ui.saveStateSync()
}

func (ui *AppUI) FlushSaveState() {
	ui.flushSaveState()
}

func (ui *AppUI) BuildStateSnapshot() persist.AppState {
	return ui.buildStateSnapshot()
}

func (ui *AppUI) MarkCollectionDirty(col *collections.ParsedCollection) {
	ui.markCollectionDirty(col)
}

func (ui *AppUI) FlushCollectionSavesSync() {
	ui.flushCollectionSavesSync()
}

func (ui *AppUI) ImportDroppedData(data []byte) {
	ui.importDroppedData(data)
}

func (ui *AppUI) InheritActiveTabLayout(rt *workspace.RequestTab) {
	ui.inheritActiveTabLayout(rt)
}

func (ui *AppUI) CommitEditingEnv() {
	ui.commitEditingEnv()
}

func (ui *AppUI) SaveVarPopup() {
	ui.saveVarPopup()
}

func (ui *AppUI) HARRunEntry(e *har.Entry) {
	ui.harRunEntry(e)
}

func (ui *AppUI) OnOSFilesDropped(paths []string, pos f32.Point) {
	ui.onOSFilesDropped(paths, pos)
}

func (ui *AppUI) DrainDroppedFiles() {
	ui.drainDroppedFiles()
}

func (ui *AppUI) RebuildDropZones(gtx layout.Context) {
	ui.rebuildDropZones(gtx)
}

func (ui *AppUI) HideSidebar() bool {
	return ui.hideSidebar()
}

func (ui *AppUI) CloseAllSidebarMenus() {
	ui.closeAllSidebarMenus()
}

func (ui *AppUI) ContentKeyFilters() []event.Filter {
	return ui.contentKeyFilters()
}

func (ui *AppUI) ActiveEnvSnapshot() map[string]string {
	return ui.activeEnvSnapshot()
}

func (ui *AppUI) ActiveEnvVarsMap() map[string]string {
	return ui.activeEnvVars
}

func (ui *AppUI) SetActiveEnvVars(m map[string]string) {
	ui.activeEnvVars = m
}

func (ui *AppUI) SetActiveEnvDirty(v bool) {
	ui.activeEnvDirty = v
}

func (ui *AppUI) SaveNeeded() bool {
	return ui.saveNeeded
}

func (ui *AppUI) SetSaveNeeded(v bool) {
	ui.saveNeeded = v
}

func (ui *AppUI) DirtyCollectionsMap() map[string]*dirtyCollection {
	return ui.dirtyCollections
}

func (ui *AppUI) SetSidebarZones(zones []sidebar.DropZoneRect) {
	ui.sidebarZones = zones
}

func (ui *AppUI) EnvDivY() int {
	return ui.envDivY
}

func (ui *AppUI) ColRowH() int {
	return ui.colRowH
}

func SetProbeRegion(f func(name string, dims layout.Dimensions)) {
	probeRegion = f
}

func FallbackFontFiles() []string {
	return fallbackFontFiles
}

func LoadEmbeddedTTF(name string) ([]byte, error) {
	return loadEmbeddedTTF(name)
}

func HarSkipHeader(name string) bool {
	return harSkipHeader(name)
}

func HarWSURL(raw string) string {
	return harWSURL(raw)
}
