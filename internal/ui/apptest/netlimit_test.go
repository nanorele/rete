package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
)

func TestNetlimitSectionWiring(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1024, 768)),
	}

	ui.SidebarSection = "netlimit"
	ui.LayoutApp(gtx)

	ui.LayoutNetlimitBody(gtx)
	ui.LayoutNetlimitSection(gtx)
	ui.WireNetTitlebar()

	if !ui.Net.Started() {
		t.Fatal("netlimit manager not started")
	}
}
