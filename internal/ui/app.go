package ui

import (
	"tracto/internal/ui/workspace"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/mitm"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/tabbar"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/titlebar"
	"tracto/internal/ui/varpopup"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio-x/explorer"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/font/opentype"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/transfer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)


type AppUI struct {
	Theme           *material.Theme
	Window          *app.Window
	TitleBar        titlebar.Bar
	Explorer        *explorer.Explorer
	pendingEnvClose *environments.EnvironmentUI
	EnvColorPicker  colorpicker.State
	EnvColorEnvID   string
	windowSize      image.Point
	DraggedEnv      *environments.EnvironmentUI
	DragEnvOriginY  float32
	DragEnvCurrentY float32
	DragEnvActive   bool

	DraggedNode      *collections.CollectionNode
	DragNodeOriginY  float32
	DragNodeCurrentY float32
	DragNodeOriginX  float32
	DragNodeCurrentX float32
	DragNodeActive   bool
	Tabs             []*workspace.RequestTab
	ActiveIdx        int
	TabsList         widget.List
	TabBar           *tabbar.Strip
	ImportBtn        widget.Clickable
	AddColBtn        widget.Clickable
	Collections      []*collections.CollectionUI
	VisibleCols      []*collections.CollectionNode
	SidebarWidth     int
	SidebarDrag      gesture.Drag
	SidebarDragX     float32
	BtnSidebarToggle widget.Clickable
	ColList          widget.List
	ColLoadedChan    chan *collections.CollectionUI
	ImportEnvBtn     widget.Clickable
	AddEnvBtn        widget.Clickable
	Environments     []*environments.EnvironmentUI
	ActiveEnvID      string
	EnvList          widget.List
	EnvLoadedChan    chan *environments.EnvironmentUI
	SidebarEnvHeight int

	envRowH         int
	colRowH         int
	colRowYs        map[int]int
	colAfterLastY   int
	SidebarEnvDrag  gesture.Drag
	SidebarEnvDragY float32
	EditingEnv      *environments.EnvironmentUI

	RenamingNode *collections.CollectionNode

	TabCtxClose       widget.Clickable
	TabCtxCloseOthers widget.Clickable
	TabCtxCloseAll    widget.Clickable

	ColsExpanded    bool
	ColsHeaderClick widget.Clickable
	EnvsExpanded    bool
	EnvsHeaderClick widget.Clickable

	SidebarSection  string
	BtnSecRequests  widget.Clickable
	BtnSecMITM      widget.Clickable

	MITM mitm.UIState

	Settings      model.AppSettings
	SettingsOpen  bool
	SettingsBtn   widget.Clickable
	SettingsState *settings.Editor
	BugReportBtn  widget.Clickable
	BugReportURL  string

	SidebarDropTag bool
	LastPointerPos f32.Point

	VarPopup varpopup.State

	PopupCloseTag struct{}

	activeEnvVars           map[string]string
	activeEnvDirty          bool
	saveNeeded              bool
	stateSaveMu             sync.Mutex
	stateSaveWG             sync.WaitGroup
	dirtyCollections        map[string]*dirtyCollection
	deletedCollections      map[string]struct{}
	collectionFlushTimerSet bool
	rootCtx                 context.Context
	rootCancel              context.CancelFunc

	Title string
}

//go:embed assets/fonts/ttf
var ttfFS embed.FS

func loadEmbeddedTTF(name string) ([]byte, error) {
	f, err := ttfFS.Open("assets/fonts/ttf/" + name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func NewAppUI() *AppUI {
	th := material.NewTheme()

	var fonts []font.FontFace
	addUIFace := func(name string) bool {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			return false
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return false
		}
		fonts = append(fonts, font.FontFace{
			Font: face.Font(),
			Face: face,
		})
		return true
	}
	interLoaded := addUIFace("Inter-Regular.ttf")
	addUIFace("Inter-Bold.ttf")
	if !interLoaded {
		fonts = gofont.Collection()
	}

	if b, err := loadEmbeddedTTF("NotoColorEmoji.ttf"); err == nil {
		if emojiFace, err := opentype.Parse(b); err == nil {
			fonts = append(fonts, font.FontFace{
				Font: font.Font{},
				Face: emojiFace,
			})
		}
	}

	addJBM := func(name string, style font.Style, weight font.Weight) {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			return
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return
		}
		fonts = append(fonts, font.FontFace{
			Font: font.Font{
				Typeface: widgets.MonoTypeface,
				Style:    style,
				Weight:   weight,
			},
			Face: face,
		})
	}
	addJBM("JetBrainsMono-Regular.ttf", font.Regular, font.Normal)
	addJBM("JetBrainsMono-Bold.ttf", font.Regular, font.Bold)
	addJBM("JetBrainsMono-Italic.ttf", font.Italic, font.Normal)
	addJBM("JetBrainsMono-BoldItalic.ttf", font.Italic, font.Bold)

	th.Shaper = text.NewShaper(text.WithCollection(fonts))

	th.Bg = theme.Bg
	th.Fg = theme.Fg
	th.ContrastBg = theme.Accent
	th.ContrastFg = theme.AccentFg
	th.TextSize = unit.Sp(14)
	settings.Apply(th, model.DefaultSettings())

	win := new(app.Window)
	win.Option(
		app.Decorated(false),
		app.MinSize(unit.Dp(480), unit.Dp(360)),
		app.Size(unit.Dp(1280), unit.Dp(720)),
	)

	defs := model.DefaultSettings()
	defaultSidebar := defs.DefaultSidebarWidthPx
	if defaultSidebar <= 0 {
		defaultSidebar = 250
	}
	ui := &AppUI{
		Theme:            th,
		Window:           win,
		SidebarWidth:     defaultSidebar,
		SidebarEnvHeight: 300,
		ColLoadedChan:    make(chan *collections.CollectionUI, 64),
		EnvLoadedChan:    make(chan *environments.EnvironmentUI, 64),
		TabBar:           tabbar.NewStrip(),
		activeEnvDirty:   true,
		ColsExpanded:     true,
		EnvsExpanded:     true,
		SidebarSection:   "requests",
		dirtyCollections: make(map[string]*dirtyCollection),
		Settings:         model.DefaultSettings(),
	}
	ui.rootCtx, ui.rootCancel = context.WithCancel(context.Background())
	go workspace.CleanupOrphanRespTmp()
	ui.Explorer = explorer.NewExplorer(ui.Window)
	ui.TabsList.Axis = layout.Vertical
	ui.ColList.Axis = layout.Vertical
	ui.EnvList.Axis = layout.Vertical
	ui.loadState()
	return ui
}

func (ui *AppUI) pushColLoaded(col *collections.CollectionUI) {
	for {
		select {
		case ui.ColLoadedChan <- col:
			ui.Window.Invalidate()
			return
		default:
			ui.Window.Invalidate()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (ui *AppUI) pushEnvLoaded(env *environments.EnvironmentUI) {
	for {
		select {
		case ui.EnvLoadedChan <- env:
			ui.Window.Invalidate()
			return
		default:
			ui.Window.Invalidate()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (ui *AppUI) revealLinkedNode(tab *workspace.RequestTab) {
	if tab == nil || tab.LinkedNode == nil || tab.LinkedNode.Collection == nil {
		return
	}
	changed := false
	var walk func(node *collections.CollectionNode) bool
	walk = func(node *collections.CollectionNode) bool {
		if node == tab.LinkedNode {
			return true
		}
		for _, child := range node.Children {
			if walk(child) {
				if !node.Expanded {
					node.Expanded = true
					changed = true
				}
				return true
			}
		}
		return false
	}
	walk(tab.LinkedNode.Collection.Root)
	if changed {
		ui.updateVisibleCols()
	}
}

func (ui *AppUI) relinkTabs() {
	for _, tab := range ui.Tabs {
		if tab.LinkedNode != nil || tab.PendingColID == "" {
			continue
		}
		for _, col := range ui.Collections {
			if col.Data.ID == tab.PendingColID {
				node := collections.NodeAtPath(col.Data.Root, tab.PendingNodePath)
				if node != nil && node.Request != nil {
					tab.LinkedNode = node
					tab.PendingColID = ""
					tab.PendingNodePath = nil
				}
				break
			}
		}
	}
}

func (ui *AppUI) updateVisibleCols() {
	visible := ui.VisibleCols[:0]
	var build func(node *collections.CollectionNode)
	build = func(node *collections.CollectionNode) {
		visible = append(visible, node)
		if node.Expanded && (node.IsFolder || node.Depth == 0) {
			for _, child := range node.Children {
				build(child)
			}
		}
	}
	for _, col := range ui.Collections {
		build(col.Data.Root)
	}
	ui.VisibleCols = visible
}

func (ui *AppUI) refreshActiveEnv() {
	if !ui.activeEnvDirty {
		return
	}
	ui.activeEnvDirty = false
	ui.activeEnvVars = nil
	for _, e := range ui.Environments {
		if e.Data.ID == ui.ActiveEnvID {
			ui.activeEnvVars = make(map[string]string)
			for _, v := range e.Data.Vars {
				if !v.Enabled {
					continue
				}
				if v.Value != "" {
					ui.activeEnvVars[v.Key] = v.Value
				}
			}
			break
		}
	}
}

func (ui *AppUI) activeEnvSnapshot() map[string]string {
	if ui.activeEnvVars == nil {
		return nil
	}
	snap := make(map[string]string, len(ui.activeEnvVars))
	for k, v := range ui.activeEnvVars {
		snap[k] = v
	}
	return snap
}


func (ui *AppUI) importDroppedData(data []byte) {
	go func() {
		id := persist.NewRandomID()
		if col, err := collections.ParseCollection(bytes.NewReader(data), id); err == nil && col != nil && col.Name != "" {
			if werr := persist.AtomicWriteFile(filepath.Join(persist.CollectionsDir(), id+".json"), data); werr == nil {
				ui.pushColLoaded(&collections.CollectionUI{Data: col})
			}
			return
		}

		envID := persist.NewRandomID()
		if env, err := environments.ParseEnvironment(bytes.NewReader(data), envID); err == nil && env != nil && env.Name != "" {
			if werr := persist.AtomicWriteFile(filepath.Join(persist.EnvironmentsDir(), envID+".json"), data); werr == nil {
				ui.pushEnvLoaded(&environments.EnvironmentUI{Data: env})
			}
			return
		}
	}()
}

func (ui *AppUI) Run() error {
	var ops op.Ops
	for {
		e := ui.Window.Event()
		ui.Explorer.ListenEvents(e)
		switch e := e.(type) {
		case transfer.DataEvent:
			if data, err := io.ReadAll(e.Open()); err == nil {
				ui.importDroppedData(data)
			}
		case app.DestroyEvent:
			if ui.rootCancel != nil {
				ui.rootCancel()
			}
			for _, tab := range ui.Tabs {
				tab.CancelRequest()
				tab.MarkClosed()
			}
			ui.stateSaveWG.Wait()
			ui.flushCollectionSavesSync()
			ui.saveStateSync()
			return e.Err
		case app.ConfigEvent:
			ui.TitleBar.Maximized = e.Config.Mode == app.Maximized || e.Config.Mode == app.Fullscreen
			ui.Window.Invalidate()
		case app.FrameEvent:

			for {
				select {
				case col := <-ui.ColLoadedChan:
					ui.Collections = append(ui.Collections, col)
					ui.relinkTabs()
					ui.updateVisibleCols()
					ui.saveState()
					ui.Window.Invalidate()
				case env := <-ui.EnvLoadedChan:
					ui.Environments = append(ui.Environments, env)
					if ui.ActiveEnvID == "" {
						ui.ActiveEnvID = env.Data.ID
						ui.activeEnvDirty = true
					}
					ui.saveState()
					ui.Window.Invalidate()
				default:
					goto Render
				}
			}
		Render:
			gtx := app.NewContext(&ops, e)
			if ui.Settings.UITextSize > 0 {
				gtx.Metric.PxPerSp *= float32(ui.Settings.UITextSize) / 14
			}
			if ui.Settings.UIScale > 0 {
				gtx.Metric.PxPerDp *= ui.Settings.UIScale
				gtx.Metric.PxPerSp *= ui.Settings.UIScale
			}
			layout.Inset{
				Top:    e.Insets.Top,
				Bottom: e.Insets.Bottom,
				Left:   e.Insets.Left,
				Right:  e.Insets.Right,
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutApp(gtx)
			})
			e.Frame(gtx.Ops)
			ui.flushSaveState()
			ui.flushCollectionSaves()
		}
	}
}

func (ui *AppUI) loadState() {
	state, raw := persist.LoadWithRaw()

	if bytes.Contains(raw, []byte(`"mono_font"`)) {
		ui.saveNeeded = true
	}

	if state.Settings != nil {
		ui.Settings = settings.Sanitize(*state.Settings)
	} else {
		ui.Settings = model.DefaultSettings()
	}
	settings.Apply(ui.Theme, ui.Settings)
	if !ui.Settings.RestoreTabsOnStartup {
		state.Tabs = nil
	}
	for _, ts := range state.Tabs {
		tab := workspace.NewRequestTab(ts.Title)
		if tab.Title == "" {
			tab.Title = "New request"
		}
		tab.Method = ts.Method
		if tab.Method == "" {
			tab.Method = "GET"
		}
		tab.URLInput.SetText(ts.URL)
		tab.ReqEditor.SetText(ts.Body)
		for _, hs := range ts.Headers {
			tab.AddHeader(hs.Key, hs.Value)
		}
		if ts.SplitRatio > 0 {
			tab.SplitRatio = ts.SplitRatio
		}
		if ts.VStackRatio > 0 {
			tab.VStackRatio = ts.VStackRatio
		}
		tab.LayoutMode = ts.LayoutMode
		if ts.HeaderSplitRatio > 0 {
			tab.HeaderSplitRatio = ts.HeaderSplitRatio
		}
		if ts.ReqWrapEnabled != nil {
			tab.ReqWrapEnabled = *ts.ReqWrapEnabled
		}
		tab.PendingColID = ts.CollectionID
		tab.PendingNodePath = ts.NodePath
		tab.BodyType = model.BodyTypeFromMode(ts.BodyType)
		for _, fp := range ts.FormParts {
			kind := model.FormPartText
			if fp.Kind == "file" {
				kind = model.FormPartFile
			}
			var size int64
			if kind == model.FormPartFile && fp.FilePath != "" {
				if fi, err := os.Stat(fp.FilePath); err == nil {
					size = fi.Size()
				}
			}
			tab.FormParts = append(tab.FormParts, workspace.NewFormPart(fp.Key, fp.Value, kind, fp.FilePath, size))
		}
		for _, ue := range ts.URLEncoded {
			tab.URLEncoded = append(tab.URLEncoded, workspace.NewURLEncodedPart(ue.Key, ue.Value))
		}
		tab.BinaryFilePath = ts.BinaryPath
		if ts.BinaryPath != "" {
			if fi, err := os.Stat(ts.BinaryPath); err == nil {
				tab.BinaryFileSize = fi.Size()
			}
		}

		tab.UpdateSystemHeaders()
		ui.Tabs = append(ui.Tabs, tab)
	}
	if len(ui.Tabs) == 0 {
		ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("New request"))
	}
	ui.ActiveIdx = state.ActiveIdx
	if ui.ActiveIdx >= len(ui.Tabs) || ui.ActiveIdx < 0 {
		ui.ActiveIdx = 0
	}

	if state.SidebarWidthPx > 0 {
		ui.SidebarWidth = state.SidebarWidthPx
	} else if ui.Settings.DefaultSidebarWidthPx > 0 {
		ui.SidebarWidth = ui.Settings.DefaultSidebarWidthPx
	}
	if state.SidebarEnvHeightPx > 0 {
		ui.SidebarEnvHeight = state.SidebarEnvHeightPx
	}

	loadedCols := collections.LoadAll()
	colByID := make(map[string]*collections.ParsedCollection, len(loadedCols))
	for _, c := range loadedCols {
		colByID[c.ID] = c
	}
	addedCols := make(map[string]bool, len(loadedCols))
	for _, id := range state.CollectionIDsOrder {
		if c, ok := colByID[id]; ok && !addedCols[id] {
			ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: c})
			addedCols[id] = true
		}
	}
	for _, c := range loadedCols {
		if !addedCols[c.ID] {
			ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: c})
			addedCols[c.ID] = true
		}
	}
	ui.relinkTabs()

	loadedEnvs := environments.LoadAll()
	envByID := make(map[string]*model.ParsedEnvironment, len(loadedEnvs))
	for _, e := range loadedEnvs {
		envByID[e.ID] = e
	}
	added := make(map[string]bool, len(loadedEnvs))
	for _, id := range state.EnvIDsOrder {
		if e, ok := envByID[id]; ok && !added[id] {
			ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: e})
			added[id] = true
		}
	}
	for _, e := range loadedEnvs {
		if !added[e.ID] {
			ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: e})
			added[e.ID] = true
		}
	}
	ui.ActiveEnvID = state.ActiveEnvID
	ui.activeEnvDirty = true
	ui.updateVisibleCols()
}

func (ui *AppUI) buildStateSnapshot() persist.AppState {
	settings := ui.Settings
	state := persist.AppState{
		Tabs:               make([]persist.TabState, 0, len(ui.Tabs)),
		ActiveIdx:          ui.ActiveIdx,
		ActiveEnvID:        ui.ActiveEnvID,
		SidebarWidthPx:     ui.SidebarWidth,
		SidebarEnvHeightPx: ui.SidebarEnvHeight,
		Settings:           &settings,
	}
	for _, e := range ui.Environments {
		if e.Data != nil {
			state.EnvIDsOrder = append(state.EnvIDsOrder, e.Data.ID)
		}
	}
	for _, c := range ui.Collections {
		if c.Data != nil && c.Data.ID != "" {
			state.CollectionIDsOrder = append(state.CollectionIDsOrder, c.Data.ID)
		}
	}
	for _, tab := range ui.Tabs {
		reqWrap := tab.ReqWrapEnabled
		ts := persist.TabState{
			Title:            tab.Title,
			Method:           tab.Method,
			URL:              tab.URLInput.Text(),
			Body:             tab.ReqEditor.Text(),
			SplitRatio:       tab.SplitRatio,
			VStackRatio:      tab.VStackRatio,
			LayoutMode:       tab.LayoutMode,
			HeaderSplitRatio: tab.HeaderSplitRatio,
			ReqWrapEnabled:   &reqWrap,
			BodyType:         tab.BodyType.PostmanMode(),
			BinaryPath:       tab.BinaryFilePath,
		}
		for _, p := range tab.FormParts {
			kind := "text"
			if p.Kind == model.FormPartFile {
				kind = "file"
			}
			ts.FormParts = append(ts.FormParts, persist.FormPartState{
				Key:      p.Key.Text(),
				Kind:     kind,
				Value:    p.Value.Text(),
				FilePath: p.FilePath,
			})
		}
		for _, ue := range tab.URLEncoded {
			ts.URLEncoded = append(ts.URLEncoded, persist.HeaderState{
				Key:   ue.Key.Text(),
				Value: ue.Value.Text(),
			})
		}
		if tab.LinkedNode != nil && tab.LinkedNode.Collection != nil {
			ts.CollectionID = tab.LinkedNode.Collection.ID
			ts.NodePath = collections.NodePathFrom(tab.LinkedNode.Collection.Root, tab.LinkedNode)
		}
		ts.Headers = make([]persist.HeaderState, 0, len(tab.Headers))
		for _, h := range tab.Headers {
			if !h.IsGenerated {
				k := h.Key.Text()
				if k != "" {
					ts.Headers = append(ts.Headers, persist.HeaderState{Key: k, Value: h.Value.Text()})
				}
			}
		}
		state.Tabs = append(state.Tabs, ts)
	}
	return state
}

func (ui *AppUI) saveStateSync() {
	state := ui.buildStateSnapshot()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	ui.stateSaveMu.Lock()
	defer ui.stateSaveMu.Unlock()
	_ = persist.AtomicWriteFile(persist.StateFilePath(), data)
}

func (ui *AppUI) saveState() {
	ui.saveNeeded = true
}

func (ui *AppUI) flushSaveState() {
	if !ui.saveNeeded {
		return
	}
	ui.saveNeeded = false
	state := ui.buildStateSnapshot()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	ui.stateSaveWG.Add(1)
	go func() {
		defer ui.stateSaveWG.Done()
		ui.stateSaveMu.Lock()
		defer ui.stateSaveMu.Unlock()
		_ = persist.AtomicWriteFile(persist.StateFilePath(), data)
	}()
}

const collectionSaveDebounce = 500 * time.Millisecond

type dirtyCollection struct {
	col  *collections.ParsedCollection
	last time.Time
}

func (ui *AppUI) markCollectionDirty(col *collections.ParsedCollection) {
	if col == nil || col.ID == "" {
		return
	}
	if ui.dirtyCollections == nil {
		ui.dirtyCollections = make(map[string]*dirtyCollection)
	}
	if e, ok := ui.dirtyCollections[col.ID]; ok {
		e.col = col
		e.last = time.Now()
	} else {
		ui.dirtyCollections[col.ID] = &dirtyCollection{col: col, last: time.Now()}
	}
	ui.scheduleCollectionFlush()
}

func (ui *AppUI) scheduleCollectionFlush() {
	if ui.collectionFlushTimerSet || ui.Window == nil {
		return
	}
	ui.collectionFlushTimerSet = true
	win := ui.Window
	time.AfterFunc(collectionSaveDebounce+20*time.Millisecond, func() {
		win.Invalidate()
	})
}

func (ui *AppUI) flushCollectionSaves() {
	ui.collectionFlushTimerSet = false
	if len(ui.dirtyCollections) == 0 {
		return
	}
	type snap struct {
		id   string
		data []byte
	}
	var snaps []snap
	now := time.Now()
	pending := false
	for id, e := range ui.dirtyCollections {
		if now.Sub(e.last) < collectionSaveDebounce {
			pending = true
			continue
		}
		if _, ok := ui.deletedCollections[id]; ok {
			delete(ui.dirtyCollections, id)
			continue
		}
		if _, data := collections.Snapshot(e.col); len(data) > 0 {
			snaps = append(snaps, snap{id, data})
		}
		delete(ui.dirtyCollections, id)
	}
	if pending {
		ui.scheduleCollectionFlush()
	}
	if len(snaps) == 0 {
		return
	}
	go func() {
		for _, s := range snaps {
			_ = persist.WriteCollectionFile(s.id, s.data)
		}
	}()
}

func (ui *AppUI) flushCollectionSavesSync() {
	for _, e := range ui.dirtyCollections {
		_ = collections.SaveToFile(e.col)
	}
	for k := range ui.dirtyCollections {
		delete(ui.dirtyCollections, k)
	}
}

func (ui *AppUI) openRequestInTab(node *collections.CollectionNode) {
	// Switching tabs invalidates any pending var hover/click — those
	// reference editors from the previously active tab, and their Leave
	// events will never fire if the originating editor is no longer in
	// the tree. Without this reset, a stale GlobalVarClick from the old
	// tab can re-open VarPopup on the very next frame and paint its
	// full-screen press-catcher backdrop over the new tab (or Settings).
	widgets.GlobalVarHover = nil
	widgets.GlobalVarClick = nil
	ui.VarPopup.Close()

	for i, t := range ui.Tabs {
		if t.LinkedNode == node {
			ui.ActiveIdx = i
			ui.Window.Invalidate()
			return
		}
	}

	tab := workspace.NewRequestTab(node.Name)
	tab.LinkedNode = node
	req := node.Request
	tab.Method = req.Method
	tab.URLInput.SetText(req.URL)
	tab.ReqEditor.SetText(req.Body)
	for k, v := range req.Headers {
		tab.AddHeader(k, v)
	}
	tab.BodyType = req.BodyType
	for _, fp := range req.FormParts {
		var size int64
		if fp.Kind == model.FormPartFile && fp.FilePath != "" {
			if fi, err := os.Stat(fp.FilePath); err == nil {
				size = fi.Size()
			}
		}
		tab.FormParts = append(tab.FormParts, workspace.NewFormPart(fp.Key, fp.Value, fp.Kind, fp.FilePath, size))
	}
	for _, kv := range req.URLEncoded {
		tab.URLEncoded = append(tab.URLEncoded, workspace.NewURLEncodedPart(kv.Key, kv.Value))
	}
	tab.BinaryFilePath = req.BinaryPath
	if req.BinaryPath != "" {
		if fi, err := os.Stat(req.BinaryPath); err == nil {
			tab.BinaryFileSize = fi.Size()
		}
	}

	tab.UpdateSystemHeaders()

	ui.inheritActiveTabLayout(tab)

	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = len(ui.Tabs) - 1
	ui.saveState()
	ui.Window.Invalidate()
}

func (ui *AppUI) inheritActiveTabLayout(tab *workspace.RequestTab) {
	if len(ui.Tabs) == 0 || ui.ActiveIdx < 0 || ui.ActiveIdx >= len(ui.Tabs) {
		return
	}
	src := ui.Tabs[ui.ActiveIdx]
	if src == nil || src == tab {
		return
	}
	tab.SplitRatio = src.SplitRatio
	tab.VStackRatio = src.VStackRatio
	tab.LayoutMode = src.LayoutMode
	tab.HeaderSplitRatio = src.HeaderSplitRatio
}

func (ui *AppUI) layoutApp(gtx layout.Context) layout.Dimensions {
	ui.windowSize = gtx.Constraints.Max

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: ui,
			Kinds:  pointer.Move | pointer.Press | pointer.Release | pointer.Drag,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.LastPointerPos = pe.Position
		if pe.Kind == pointer.Press {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
		if pe.Kind == pointer.Release {
			if ui.EditingEnv != nil && !ui.SettingsOpen {
				sidebarRight := 0
				if !ui.Settings.HideSidebar {
					sidebarRight = ui.SidebarWidth + gtx.Dp(unit.Dp(6))
				}
				titleBarH := gtx.Dp(unit.Dp(30))
				if int(pe.Position.X) < sidebarRight && int(pe.Position.Y) >= titleBarH {
					ui.pendingEnvClose = ui.EditingEnv
				}
			}
		}
	}
	event.Op(gtx.Ops, ui)

	widgets.GlobalPointerPos = ui.LastPointerPos

	dim := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutTitleBar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if ui.SettingsOpen {
				paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
				if ui.SettingsState == nil {
					ui.SettingsState = settings.NewEditor(ui.Settings)
				}
				return ui.SettingsState.Layout(gtx, ui.settingsHost())
			}
			return ui.layoutContent(gtx)
		}),
	)

	anySidebarMenuOpen := false
	for _, n := range ui.VisibleCols {
		if n.MenuOpen {
			anySidebarMenuOpen = true
			break
		}
	}
	var activeTab *workspace.RequestTab
	if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
		activeTab = ui.Tabs[ui.ActiveIdx]
	}
	tabMenuOpen := activeTab != nil && (activeTab.SendMenuOpen || activeTab.MethodListOpen || activeTab.BodyTypeOpen)

	closeAllPopups := func() {
		ui.TabBar.TabCtxMenuOpen = false
		ui.closeAllSidebarMenus()
		if activeTab != nil {
			activeTab.SendMenuOpen = false
			activeTab.MethodListOpen = false
			activeTab.BodyTypeOpen = false
		}
	}

	if ui.TabBar.TabCtxMenuOpen || anySidebarMenuOpen || tabMenuOpen {
		layout.Stack{}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
				// Pass-through: the popup-close backdrop covers the whole
				// window, but its event.Op handler must NOT block Gio's
				// cursor hit-test walk. Without PassOp, the backdrop's
				// non-pass hit-node short-circuits the reverse hitTree
				// traversal (idx = n.next jump) — leaving cursor at unset
				// → fallback CursorDefault, masking CursorText/CursorPointer
				// of every widget below.
				passStack := pointer.PassOp{}.Push(gtx.Ops)
				for {
					ev, ok := gtx.Event(
						pointer.Filter{Target: &ui.PopupCloseTag, Kinds: pointer.Press},
						key.Filter{Name: key.NameEscape},
					)
					if !ok {
						break
					}
					if pe, ok := ev.(pointer.Event); ok && pe.Kind == pointer.Press {
						closeAllPopups()
						ui.Window.Invalidate()
					}
					if ke, ok := ev.(key.Event); ok && ke.State == key.Press && ke.Name == key.NameEscape {
						closeAllPopups()
						ui.Window.Invalidate()
					}
				}
				event.Op(gtx.Ops, &ui.PopupCloseTag)
				passStack.Pop()
				// Intentionally do NOT call pointer.CursorDefault.Add here:
				// this is a full-screen press-catcher backdrop drawn AFTER
				// the rest of the UI, so in Gio's reverse hit-test it would
				// otherwise win cursor resolution for every pixel in the
				// window, masking CursorText over editors, CursorPointer
				// over buttons and {{env}} chips, and CursorRowResize over
				// the splitter while any of the popup-menu flags are set.
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
		)
	}

	if widgets.GlobalVarHover != nil && !ui.VarPopup.Open && !ui.SettingsOpen {
		var val string
		found := false
		if ui.activeEnvVars != nil {
			val, found = ui.activeEnvVars[widgets.GlobalVarHover.Name]
		}

		popupGtx := gtx
		popupGtx.Constraints.Min = image.Point{}
		popupGtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(360)))

		contentMacro := op.Record(gtx.Ops)
		contentDims := layout.Stack{}.Layout(popupGtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
				paint.FillShape(gtx.Ops, theme.BgPopup, rr.Op(gtx.Ops))
				bw := gtx.Dp(unit.Dp(2))
				paint.FillShape(gtx.Ops, theme.Border, clip.Stroke{
					Path:  clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Path(gtx.Ops),
					Width: float32(bw),
				}.Op())
				// Anchor a default cursor for the hover popup; it floats
				// over chips that set CursorPointer, and without this the
				// pointer cursor leaks through under the popup rect.
				defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
				pointer.CursorDefault.Add(gtx.Ops)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(10), widgets.GlobalVarHover.Name)
							lbl.Color = theme.FgMuted
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							txt := val
							col := theme.White
							if !found {
								txt = "Not found in active environment"
								col = theme.Danger
							}
							lbl := material.Label(ui.Theme, unit.Sp(12), txt)
							lbl.Color = col
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(9), "Click to edit/select")
							lbl.Color = theme.Accent
							return lbl.Layout(gtx)
						}),
					)
				})
			}),
		)
		contentCall := contentMacro.Stop()

		px := int(widgets.GlobalVarHover.Pos.X)
		py := int(widgets.GlobalVarHover.Pos.Y)
		if px+contentDims.Size.X > gtx.Constraints.Max.X {
			px = gtx.Constraints.Max.X - contentDims.Size.X
		}
		if px < 0 {
			px = 0
		}

		deferMacro := op.Record(gtx.Ops)
		op.Offset(image.Pt(px, py)).Add(gtx.Ops)
		contentCall.Add(gtx.Ops)
		op.Defer(gtx.Ops, deferMacro.Stop())
	}

	if widgets.GlobalVarClick != nil && !ui.SettingsOpen {
		var val string
		if ui.activeEnvVars != nil {
			val = ui.activeEnvVars[widgets.GlobalVarClick.Name]
		}
		ui.VarPopup.OpenAt(
			widgets.GlobalVarClick.Name,
			val,
			widgets.GlobalVarClick.Editor,
			widgets.GlobalVarClick.Range,
			widgets.GlobalVarClick.Pos,
			ui.ActiveEnvID,
		)
		ui.Window.Invalidate()
		widgets.GlobalVarClick = nil
	}

	if ui.VarPopup.Open && !ui.SettingsOpen {
		ui.VarPopup.Layout(gtx, ui.varPopupHost())
	}

	if ui.SettingsOpen && ui.SettingsState != nil && ui.SettingsState.ColorPicker.IsOpen() {
		ui.layoutColorPickerOverlay(gtx)
	}

	if ui.EnvColorPicker.IsOpen() && !ui.SettingsOpen {
		for ui.EnvColorPicker.CloseBtn.Clicked(gtx) {
			ui.EnvColorPicker.Close()
		}
	}
	if ui.EnvColorPicker.IsOpen() && !ui.SettingsOpen {
		cur := [3]float32{ui.EnvColorPicker.H, ui.EnvColorPicker.S, ui.EnvColorPicker.V}
		if cur != ui.EnvColorPicker.LastHSV {
			for _, e := range ui.Environments {
				if e.Data != nil && e.Data.ID == ui.EnvColorEnvID {
					e.Data.HighlightColor = theme.HexFromColor(ui.EnvColorPicker.Color())
					_ = persist.SaveEnvironment(e.Data)
					break
				}
			}
			ui.EnvColorPicker.LastHSV = cur
		}
		ui.renderColorPickerOverlay(gtx, &ui.EnvColorPicker)
	}

	return dim
}

func (ui *AppUI) layoutColorPickerOverlay(gtx layout.Context) {
	ui.renderColorPickerOverlay(gtx, &ui.SettingsState.ColorPicker)
}

func (ui *AppUI) renderColorPickerOverlay(gtx layout.Context, p *colorpicker.State) {
	pickerW := gtx.Dp(unit.Dp(240))
	pickerH := gtx.Dp(unit.Dp(216))
	gap := gtx.Dp(unit.Dp(6))

	px := int(p.Anchor.X) + gap
	py := int(p.Anchor.Y) + gap
	if px+pickerW > gtx.Constraints.Max.X {
		px = gtx.Constraints.Max.X - pickerW - gap
	}
	if py+pickerH > gtx.Constraints.Max.Y {
		py = int(p.Anchor.Y) - pickerH - gap
	}
	if px < 0 {
		px = 0
	}
	if py < 0 {
		py = 0
	}
	pickerRect := image.Rect(px, py, px+pickerW, py+pickerH)

	macro := op.Record(gtx.Ops)

	backdropStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	// Pass-through (see popup-close backdrop in layoutApp for rationale):
	// without PassOp, this full-screen press-catcher's non-pass hit-node
	// short-circuits Gio's cursor hit-test walk, masking every widget's
	// cursor underneath.
	backdropPass := pointer.PassOp{}.Push(gtx.Ops)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &p.Backdrop,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		pos := image.Pt(int(pe.Position.X), int(pe.Position.Y))
		if pos.In(pickerRect) {
			continue
		}
		p.Close()
	}
	event.Op(gtx.Ops, &p.Backdrop)
	backdropPass.Pop()
	backdropStack.Pop()

	pickerOff := op.Offset(image.Pt(px, py)).Push(gtx.Ops)
	pickerGtx := gtx
	pickerGtx.Constraints.Min = image.Pt(pickerW, pickerH)
	pickerGtx.Constraints.Max = pickerGtx.Constraints.Min
	colorpicker.Render(pickerGtx, ui.Theme, p)
	pickerOff.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}


func (ui *AppUI) closeAllSidebarMenus() {
	for _, n := range ui.VisibleCols {
		n.MenuOpen = false
	}
}

func (ui *AppUI) layoutContent(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "W", Required: key.ModShortcut},
			key.Filter{Name: "F", Required: key.ModShortcut},
			key.Filter{Name: key.NameReturn, Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case "S":

				switch {
				case ui.SettingsOpen:
					if ui.SettingsState != nil {
						ui.SettingsState.Apply(ui.settingsHost())
					}
					ui.saveState()
				case ui.EditingEnv != nil:
					ui.commitEditingEnv()
				default:
					if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
						if col := ui.Tabs[ui.ActiveIdx].SaveToCollection(); col != nil {
							ui.markCollectionDirty(col)
						}
					}
				}
			case "W":
				if len(ui.Tabs) > 0 {
					ui.closeTab(ui.ActiveIdx)
				}
			case "F":
				if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
					ui.Tabs[ui.ActiveIdx].SearchOpen = !ui.Tabs[ui.ActiveIdx].SearchOpen
				}
			case key.NameReturn:
				if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
					tab := ui.Tabs[ui.ActiveIdx]
					tab.SendMenuOpen = false
					tab.ExecuteRequest(ui.rootCtx, ui.Window, ui.activeEnvSnapshot())
					ui.saveState()
				}
			}
		}
	}

	for i := range ui.Tabs {
		tab := ui.Tabs[i]
		for tab.LoadFromFileBtn.Clicked(gtx) {
			go func(tab *workspace.RequestTab) {
				rc, err := ui.Explorer.ChooseFile()
				if err != nil || rc == nil {
					return
				}
				defer func() { _ = rc.Close() }()
				_ = tab.ReqEditor.LoadFromReader(rc)
				ui.Window.Invalidate()
			}(tab)
		}
	}

	for ui.TabBar.AddTabBtn.Clicked(gtx) {
		ui.TabBar.TabCtxMenuOpen = false
		newTab := workspace.NewRequestTab("New request")
		ui.inheritActiveTabLayout(newTab)
		ui.Tabs = append(ui.Tabs, newTab)
		ui.ActiveIdx = len(ui.Tabs) - 1
	}

	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		for ui.Tabs[i].CloseBtn.Clicked(gtx) {
			ui.TabBar.TabCtxMenuOpen = false
			ui.closeTab(i)
			break
		}
	}

	for ui.TabCtxClose.Clicked(gtx) {
		ui.closeTab(ui.TabBar.TabCtxMenuIdx)
		ui.TabBar.TabCtxMenuOpen = false
	}
	for ui.TabCtxCloseOthers.Clicked(gtx) {
		keep := ui.TabBar.TabCtxMenuIdx
		for i := len(ui.Tabs) - 1; i >= 0; i-- {
			if i != keep {
				ui.closeTab(i)
				if i < keep {
					keep--
				}
			}
		}
		ui.ActiveIdx = 0
		ui.TabBar.TabCtxMenuOpen = false
	}
	for ui.TabCtxCloseAll.Clicked(gtx) {
		for i := len(ui.Tabs) - 1; i >= 0; i-- {
			ui.closeTab(i)
		}
		ui.TabBar.TabCtxMenuOpen = false
	}

	if len(ui.Tabs) == 0 {
		newTab := workspace.NewRequestTab("New request")
		ui.Tabs = append(ui.Tabs, newTab)
		ui.ActiveIdx = 0
	}

	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	ui.refreshActiveEnv()

	var moved bool
	var finalX float32
	var released bool

	for {
		e, ok := ui.SidebarDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			ui.SidebarDragX = e.Position.X
		case pointer.Drag:
			finalX = e.Position.X
			moved = true
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	minSidebarWidth := gtx.Dp(unit.Dp(160))
	maxSidebarWidth := gtx.Constraints.Max.X / 2
	if ui.SidebarWidth < minSidebarWidth {
		ui.SidebarWidth = minSidebarWidth
	}
	if ui.SidebarWidth > maxSidebarWidth && maxSidebarWidth > minSidebarWidth {
		ui.SidebarWidth = maxSidebarWidth
	}

	if moved {
		delta := finalX - ui.SidebarDragX
		oldWidth := ui.SidebarWidth
		ui.SidebarWidth += int(delta)
		if ui.SidebarWidth < minSidebarWidth {
			ui.SidebarWidth = minSidebarWidth
		}
		if ui.SidebarWidth > maxSidebarWidth && maxSidebarWidth > minSidebarWidth {
			ui.SidebarWidth = maxSidebarWidth
		}
		actualDelta := ui.SidebarWidth - oldWidth
		ui.SidebarDragX = finalX - float32(actualDelta)
		ui.Window.Invalidate()
	}
	if released {
		ui.saveState()
	}

	for ui.BtnSidebarToggle.Clicked(gtx) {
		ui.Settings.HideSidebar = !ui.Settings.HideSidebar
		ui.saveState()
	}

	hideSidebar := ui.Settings.HideSidebar
	hideTabBar := ui.Settings.HideTabBar

	dim := layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			horizChildren := []layout.FlexChild{}
			gutterW := gtx.Dp(unit.Dp(36))
			sidebarW := ui.SidebarWidth
			if hideSidebar {
				sidebarW = gutterW
			}
			horizChildren = append(horizChildren,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = sidebarW
					gtx.Constraints.Max.X = sidebarW
					return ui.layoutSidebar(gtx)
				}),
			)
			if !hideSidebar {
				horizChildren = append(horizChildren,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						hit := gtx.Dp(unit.Dp(6))
						vis := 1
						h := gtx.Constraints.Max.Y
						if h == 0 {
							h = gtx.Constraints.Min.Y
						}
						size := image.Point{X: hit, Y: h}

						lineCol := theme.Border
						if ui.SidebarDrag.Dragging() {
							lineCol = theme.Accent
						}
						lineX := (hit - vis) / 2
						paint.FillShape(gtx.Ops, lineCol, clip.Rect{Min: image.Pt(lineX, 0), Max: image.Pt(lineX+vis, h)}.Op())

						defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
						pointer.CursorColResize.Add(gtx.Ops)
						ui.SidebarDrag.Add(gtx.Ops)

						event.Op(gtx.Ops, &ui.SidebarDrag)
						for {
							_, ok := gtx.Event(pointer.Filter{Target: &ui.SidebarDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
							if !ok {
								break
							}
						}
						return layout.Dimensions{Size: size}
					}),
				)
			}
			horizChildren = append(horizChildren,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if ui.SidebarSection != "requests" {
						return ui.layoutMITMSection(gtx)
					}
					if ui.EditingEnv != nil {
						return ui.layoutEnvEditor(gtx)
					}

					tabBarChildren := []layout.FlexChild{}
					if !hideTabBar {
						tabBarChildren = append(tabBarChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutTabBar(gtx)
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, append(tabBarChildren,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
								tab := ui.Tabs[ui.ActiveIdx]

								for tab.SendBtn.Clicked(gtx) {
									tab.SendMenuOpen = false
									tab.ExecuteRequest(ui.rootCtx, ui.Window, ui.activeEnvSnapshot())
									ui.saveState()
								}
								if tab.URLSubmitted {
									tab.URLSubmitted = false
									tab.SendMenuOpen = false
									tab.ExecuteRequest(ui.rootCtx, ui.Window, ui.activeEnvSnapshot())
									ui.saveState()
								}
								for tab.CancelBtn.Clicked(gtx) {
									tab.CancelRequest()
								}
								for tab.SaveToFileBtn.Clicked(gtx) {
									tab.SendMenuOpen = false
									go func() {
										w, err := ui.Explorer.CreateFile("response.json")
										if err != nil || w == nil {
											return
										}
										tab.FileSaveMu.Lock()
										if tab.Closed.Load() {
											tab.FileSaveMu.Unlock()
											_ = w.Close()
											return
										}
										select {
										case tab.FileSaveChan <- w:
											tab.FileSaveMu.Unlock()
											ui.Window.Invalidate()
										default:
											tab.FileSaveMu.Unlock()
											_ = w.Close()
										}
									}()
								}
								select {
								case w := <-tab.FileSaveChan:
									if f, ok := w.(*os.File); ok {
										tab.SaveToFilePath = f.Name()
									}
									tab.ExecuteRequestToFile(ui.rootCtx, ui.Window, ui.activeEnvSnapshot(), w)
								default:
								}

								isDragging := ui.SidebarDrag.Dragging() || ui.SidebarEnvDrag.Dragging()
								return tab.Layout(gtx, ui.Theme, ui.Window, ui.Explorer, ui.activeEnvVars, isDragging, func() {
									ui.saveState()
								}, ui.markCollectionDirty)
							}

							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Point{X: gtx.Dp(unit.Dp(64)), Y: gtx.Dp(unit.Dp(64))}
										return widgets.IconSearch.Layout(gtx, theme.FgMuted)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.Theme, unit.Sp(16), "No request selected")
										lbl.Color = theme.FgMuted
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.Theme, unit.Sp(14), "Select one from the sidebar or click '+' to create a new one")
										lbl.Color = theme.FgDim
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if ui.TabBar.AddTabBtn.Clicked(gtx) {
											ui.TabBar.TabCtxMenuOpen = false
											newTab := workspace.NewRequestTab("New request")
											ui.Tabs = append(ui.Tabs, newTab)
											ui.ActiveIdx = len(ui.Tabs) - 1
										}
										btn := material.Button(ui.Theme, &ui.TabBar.AddTabBtn, "Create Request")
										btn.Background = theme.Accent
										btn.Color = ui.Theme.ContrastFg
										btn.TextSize = unit.Sp(14)
										btn.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(16), Right: unit.Dp(16)}
										return btn.Layout(gtx)
									}),
								)
							})
						}),
					)...)
				}),
			)
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, horizChildren...)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !ui.TabBar.TabCtxMenuOpen {
				return layout.Dimensions{}
			}

			macro := op.Record(gtx.Ops)

			offX := int(ui.TabBar.TabCtxMenuPos.X) + ui.SidebarWidth + gtx.Dp(unit.Dp(8))
			offY := int(ui.TabBar.TabCtxMenuPos.Y) + gtx.Dp(unit.Dp(8))
			op.Offset(image.Pt(offX, offY)).Add(gtx.Ops)

			menuItem := func(gtx layout.Context, clk *widget.Clickable, title string) layout.Dimensions {
				return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
					if clk.Hovered() {
						paint.FillShape(gtx.Ops, theme.BgHover, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
					}
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(140))
						lbl := material.Label(ui.Theme, unit.Sp(12), title)
						return lbl.Layout(gtx)
					})
				})
			}

			rec := op.Record(gtx.Ops)
			menuGtx := gtx
			menuGtx.Constraints.Min = image.Point{}
			menuGtx.Constraints.Max = image.Pt(gtx.Dp(unit.Dp(200)), gtx.Dp(unit.Dp(300)))
			menuDims := layout.UniformInset(unit.Dp(4)).Layout(menuGtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return menuItem(gtx, &ui.TabCtxClose, "Close")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return menuItem(gtx, &ui.TabCtxCloseOthers, "Close others")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return menuItem(gtx, &ui.TabCtxCloseAll, "Close all")
					}),
				)
			})
			menuCall := rec.Stop()

			sz := menuDims.Size
			b := 1
			if gtx.Dp(unit.Dp(1)) > 1 {
				b = gtx.Dp(unit.Dp(1))
			}
			paint.FillShape(gtx.Ops, theme.BorderLight,
				clip.UniformRRect(image.Rectangle{Max: image.Pt(sz.X+b*2, sz.Y+b*2)}, 4).Op(gtx.Ops))
			op.Offset(image.Pt(b, b)).Add(gtx.Ops)
			paint.FillShape(gtx.Ops, theme.BgPopup,
				clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			op.Offset(image.Pt(-b, -b)).Add(gtx.Ops)

			menuCall.Add(gtx.Ops)
			call := macro.Stop()
			op.Defer(gtx.Ops, call)

			return layout.Dimensions{}
		}),
	)

	if ui.pendingEnvClose != nil {
		if ui.EditingEnv == ui.pendingEnvClose {
			ui.commitEditingEnv()
			ui.EditingEnv = nil
			ui.Window.Invalidate()
		}
		ui.pendingEnvClose = nil
	}

	return dim
}

func (ui *AppUI) layoutSidebarToggleBtn(gtx layout.Context) layout.Dimensions {
	return ui.BtnSidebarToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		bg := theme.BgDark
		if ui.BtnSidebarToggle.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
		ic := widgets.IconChevronL
		if ui.Settings.HideSidebar {
			ic = widgets.IconChevronR
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(36))
			gtx.Constraints.Min = image.Pt(s, s)
			gtx.Constraints.Max = gtx.Constraints.Min
			return ic.Layout(gtx, theme.FgMuted)
		})
	})
}

func (ui *AppUI) layoutSidebarSectionBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, id string) layout.Dimensions {
	for clk.Clicked(gtx) {
		ui.SidebarSection = id
		ui.saveState()
	}
	active := ui.SidebarSection == id
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		bg := theme.BgDark
		switch {
		case active:
			bg = theme.BgHover
		case clk.Hovered():
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
		if active {
			indW := gtx.Dp(unit.Dp(2))
			paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Max: image.Pt(indW, size.Y)}.Op())
		}
		col := theme.FgMuted
		if active {
			col = theme.Fg
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(22))
			gtx.Constraints.Min = image.Pt(s, s)
			gtx.Constraints.Max = gtx.Constraints.Min
			return ic.Layout(gtx, col)
		})
	})
}

func (ui *AppUI) layoutSidebarSectionRequestsBtn(gtx layout.Context) layout.Dimensions {
	return ui.layoutSidebarSectionBtn(gtx, &ui.BtnSecRequests, widgets.IconRequests, "requests")
}

func (ui *AppUI) layoutSidebarSectionMITMBtn(gtx layout.Context) layout.Dimensions {
	return ui.layoutSidebarSectionBtn(gtx, &ui.BtnSecMITM, widgets.IconMITM, "mitm")
}
