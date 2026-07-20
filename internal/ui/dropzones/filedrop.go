package dropzones

import (
	"os"
	"strings"

	"github.com/nanorele/gio/f32"
)

type droppedPayload struct {
	paths []string
	pos   f32.Point
}

func (s *State) Dropped(host *Host, paths []string, pos f32.Point) {
	s.host = host
	if len(paths) == 0 || s.dropped == nil {
		return
	}
	cp := append([]string(nil), paths...)
	select {
	case s.dropped <- droppedPayload{paths: cp, pos: pos}:
	default:
	}
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
	if host.Window != nil {
		host.Window.Invalidate()
	}
}

func (s *State) Drain(host *Host) {
	s.host = host
	if s.dropped == nil {
		return
	}
	for {
		select {
		case p := <-s.dropped:
			s.routeDroppedFiles(p)
		default:
			return
		}
	}
}

func (s *State) routeDroppedFiles(p droppedPayload) {
	switch zone := s.zoneAt(p.pos); zone {
	case "har":
		if pth := firstHARPath(p.paths); pth != "" {
			s.host.LoadHAR(pth)
		}
		return
	case "collections", "variables", "scripts":
		s.importDroppedFilesAs(p.paths, zoneImportKind(zone))
		return
	}

	if s.host.SidebarSection == "har" {
		if pth := firstHARPath(p.paths); pth != "" {
			s.host.LoadHAR(pth)
		}
		return
	}
	s.importDroppedFilesAs(p.paths, importKindAuto)
}

func (s *State) importDroppedFilesAs(paths []string, kind importKind) {
	host := s.host
	for _, p := range paths {
		p := p
		go func() {
			data, err := os.ReadFile(p)
			if err == nil {
				importDataAs(host, data, kind)
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
