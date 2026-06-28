package ui

import (
	"os"
	"strings"

	"github.com/nanorele/gio/f32"
)

type droppedPayload struct {
	paths []string
	pos   f32.Point
}

func (ui *AppUI) onOSFilesDropped(paths []string, pos f32.Point) {
	if len(paths) == 0 || ui.droppedFiles == nil {
		return
	}
	cp := append([]string(nil), paths...)
	select {
	case ui.droppedFiles <- droppedPayload{paths: cp, pos: pos}:
	default:
	}
	ui.dnd.mu.Lock()
	ui.dnd.active = false
	ui.dnd.mu.Unlock()
	if ui.Window != nil {
		ui.Window.Invalidate()
	}
}

func (ui *AppUI) drainDroppedFiles() {
	if ui.droppedFiles == nil {
		return
	}
	for {
		select {
		case p := <-ui.droppedFiles:
			ui.routeDroppedFiles(p)
		default:
			return
		}
	}
}

func (ui *AppUI) routeDroppedFiles(p droppedPayload) {
	switch zone := ui.zoneAt(p.pos); zone {
	case "har":
		if pth := firstHARPath(p.paths); pth != "" {
			ui.HARView.loadPathAsync(pth, ui.Window.Invalidate)
		}
		return
	case "collections", "variables", "scripts":
		ui.importDroppedFilesAs(p.paths, zoneImportKind(zone))
		return
	}

	if ui.SidebarSection == "har" {
		if pth := firstHARPath(p.paths); pth != "" {
			ui.HARView.loadPathAsync(pth, ui.Window.Invalidate)
		}
		return
	}
	ui.importDroppedFilesAs(p.paths, importKindAuto)
}

func (ui *AppUI) importDroppedFilesAs(paths []string, kind importKind) {
	for _, p := range paths {
		p := p
		go func() {
			data, err := os.ReadFile(p)
			if err == nil {
				ui.importDataAs(data, kind)
			}
		}()
	}
}

func firstHARPath(paths []string) string {
	for _, p := range paths {
		if strings.EqualFold(filepathExt(p), ".har") {
			return p
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func filepathExt(p string) string {
	if i := strings.LastIndexByte(p, '.'); i >= 0 && i > strings.LastIndexAny(p, `/\`) {
		return p[i:]
	}
	return ""
}
