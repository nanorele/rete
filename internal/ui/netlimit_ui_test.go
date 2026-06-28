package ui

import (
	"image"
	"testing"

	"tracto/internal/netlimit"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
)

func TestNetlimitLayoutPaths(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1024, 768)),
	}

	ui.SidebarSection = "netlimit"
	ui.layoutApp(gtx)

	ui.Net.scope = netlimit.ScopeApp
	ui.Net.pickerOpen = true
	ui.Net.procs = []netlimit.ProcInfo{{PID: 1, Name: "test.exe", Exe: "C:/test.exe"}}
	ui.layoutApp(gtx)

	ui.Net.selApp = ui.Net.procs[0]
	ui.Net.hasApp = true
	ui.Net.inEd.SetText("5")
	ui.Net.outEd.SetText("2")
	ui.layoutNetlimitBody(gtx)
	ui.layoutNetlimitSection(gtx)

	if spec := ui.netBuildSpec(); spec.Scope != netlimit.ScopeApp || spec.AppPID != 1 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestNetConfigRoundTrip(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()

	ui.Net.scope = netlimit.ScopeApp
	ui.Net.inUnit.idx = 0
	ui.Net.outUnit.idx = 2
	ui.Net.totalUnit.idx = 1
	ui.Net.inEd.SetText("10")
	ui.Net.outEd.SetText("3")
	ui.Net.totalEd.SetText("")
	ui.Net.selApp = netlimit.ProcInfo{Name: "chrome.exe", Exe: "C:/chrome.exe"}
	ui.Net.hasApp = true
	ui.saveNetConfig()

	ui2 := NewAppUI()
	if ui2.Net.scope != netlimit.ScopeApp {
		t.Errorf("scope not restored: %v", ui2.Net.scope)
	}
	if ui2.Net.inUnit.idx != 0 || ui2.Net.outUnit.idx != 2 || ui2.Net.totalUnit.idx != 1 {
		t.Errorf("units not restored: in=%d out=%d total=%d",
			ui2.Net.inUnit.idx, ui2.Net.outUnit.idx, ui2.Net.totalUnit.idx)
	}
	if got := ui2.Net.inEd.Text(); got != "10" {
		t.Errorf("in not restored: %q", got)
	}
	if !ui2.Net.hasApp || ui2.Net.selApp.Name != "chrome.exe" {
		t.Errorf("app not restored: %+v", ui2.Net.selApp)
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
