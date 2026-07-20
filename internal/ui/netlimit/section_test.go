package netlimit

import (
	"image"
	"testing"

	netlim "tracto/internal/netlimit"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/widget/material"
)

func TestNetlimitLayoutPaths(t *testing.T) {
	setupTestConfigDir(t)
	s := new(Section)
	s.Init()
	defer s.Close()
	host := &Host{
		Theme:  material.NewTheme(),
		Window: new(app.Window),
	}

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1024, 768)),
	}

	s.LayoutBody(gtx, host)
	s.LayoutSection(gtx, host)

	s.scope = netlim.ScopeApp
	s.pickerOpen = true
	s.procs = []netlim.ProcInfo{{PID: 1, Name: "test.exe", Exe: "C:/test.exe"}}
	s.LayoutBody(gtx, host)

	s.selApp = s.procs[0]
	s.hasApp = true
	s.inEd.SetText("5")
	s.outEd.SetText("2")
	s.LayoutBody(gtx, host)
	s.LayoutSection(gtx, host)

	if spec := s.buildSpec(); spec.Scope != netlim.ScopeApp || spec.AppPID != 1 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestNetConfigRoundTrip(t *testing.T) {
	setupTestConfigDir(t)
	s := new(Section)

	s.scope = netlim.ScopeApp
	s.inUnit.idx = 0
	s.outUnit.idx = 2
	s.totalUnit.idx = 1
	s.inEd.SetText("10")
	s.outEd.SetText("3")
	s.totalEd.SetText("")
	s.selApp = netlim.ProcInfo{Name: "chrome.exe", Exe: "C:/chrome.exe"}
	s.hasApp = true
	s.saveConfig()

	s2 := new(Section)
	s2.Init()
	defer s2.Close()
	if s2.scope != netlim.ScopeApp {
		t.Errorf("scope not restored: %v", s2.scope)
	}
	if s2.inUnit.idx != 0 || s2.outUnit.idx != 2 || s2.totalUnit.idx != 1 {
		t.Errorf("units not restored: in=%d out=%d total=%d",
			s2.inUnit.idx, s2.outUnit.idx, s2.totalUnit.idx)
	}
	if got := s2.inEd.Text(); got != "10" {
		t.Errorf("in not restored: %q", got)
	}
	if !s2.hasApp || s2.selApp.Name != "chrome.exe" {
		t.Errorf("app not restored: %+v", s2.selApp)
	}
}

func TestFormatRate(t *testing.T) {
	cases := map[int64]string{
		0:               "0 B/s",
		512:             "512 B/s",
		2048:            "2.0 KB/s",
		3 * 1024 * 1024: "3.0 MB/s",
	}
	for in, want := range cases {
		if got := formatRate(in); got != want {
			t.Errorf("formatRate(%d) = %q, want %q", in, got, want)
		}
	}
}
