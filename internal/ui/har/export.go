package har

import (
	"io"
	"strconv"
	"strings"

	"tracto/internal/har"
	"tracto/pkg/folderpick"
)

func (s *Section) browse() {
	choose := s.host.ChooseHAR
	win := s.host.Window
	if choose == nil {
		return
	}
	go func() {
		rc, err := choose()
		if err != nil || rc == nil {
			if err != nil {
				s.queueLoad(nil, "", err)
				win.Invalidate()
			}
			return
		}
		defer func() { _ = rc.Close() }()
		data, rerr := io.ReadAll(rc)
		s.queueLoad(data, "", rerr)
		win.Invalidate()
	}()
}

func (s *Section) exportZip() {
	create := s.host.CreateFile
	win := s.host.Window
	if create == nil || len(s.Resources) == 0 {
		return
	}
	resources := s.Resources
	suggested := exportName(s.Source)
	go func() {
		w, err := create(suggested)
		if err != nil || w == nil {
			if err != nil {
				s.setBanner("Export failed: "+err.Error(), true)
				win.Invalidate()
			}
			return
		}
		n, werr := har.WriteZip(w, resources)
		cerr := w.Close()
		switch {
		case werr != nil:
			s.setBanner("Export failed: "+werr.Error(), true)
		case cerr != nil:
			s.setBanner("Export failed: "+cerr.Error(), true)
		default:
			s.setBanner(exportMsg(n, len(resources), suggested), false)
		}
		win.Invalidate()
	}()
}

func (s *Section) exportDir() {
	win := s.host.Window
	if len(s.Resources) == 0 {
		return
	}
	resources := s.Resources
	go func() {
		dir, ok := folderpick.PickFolderDialog("Export HAR resources to folder")
		if !ok {
			return
		}
		n, err := har.WriteDirOS(dir, resources)
		if err != nil {
			s.setBanner("Export failed: "+err.Error(), true)
		} else {
			s.setBanner(exportMsg(n, len(resources), dir), n < len(resources))
		}
		win.Invalidate()
	}()
}

func exportMsg(written, total int, dest string) string {
	if written < total {
		return "Exported " + strconv.Itoa(written) + " of " + strconv.Itoa(total) +
			" files to " + dest + " (" + strconv.Itoa(total-written) + " skipped)"
	}
	return "Exported " + strconv.Itoa(written) + " files to " + dest
}

func exportName(source string) string {
	base := source
	if base == "" {
		base = "har-export"
	}
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	return base + ".zip"
}
