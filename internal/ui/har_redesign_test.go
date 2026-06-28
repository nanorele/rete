package ui

import (
	"image"
	"testing"

	"github.com/nanorele/gio/io/input"
)

const harEmptyEntriesDoc = `{"log":{"version":"1.2","entries":[]}}`

func TestHARTable_HasResizableColumns(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	if ui.HARView.Table == nil {
		t.Fatal("HAR view must build a shared table model in ensure()")
	}
	if got := len(ui.HARView.Table.Columns()); got != 7 {
		t.Errorf("HAR table columns = %d, want 7", got)
	}
}

func TestHARSection_EmptyEntriesAndLoadedRender(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harEmptyEntriesDoc), "empty.har", nil)
	ui.HARView.TopTab = harTabRequests

	var r input.Router
	sz := image.Pt(1100, 620)
	if d := layoutHARTwice(&r, sz, ui.layoutHARSection); d.Size.Y <= 0 {
		t.Fatal("empty-entries requests view failed to render")
	}

	ui.HARView.applyLoad([]byte(harTestDoc), "x.har", nil)
	if d := layoutHARTwice(&r, sz, ui.layoutHARSection); d.Size.Y <= 0 {
		t.Fatal("loaded requests view failed to render")
	}
}

func TestHARSection_NarrowSplitRendersClipped(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harTestDoc), "x.har", nil)
	ui.HARView.TopTab = harTabRequests
	ui.HARView.SplitRatio = 0.8

	var r input.Router
	if d := layoutHARTwice(&r, image.Pt(700, 500), ui.layoutHARSection); d.Size.Y <= 0 {
		t.Fatal("narrow-split requests view failed to render")
	}
}
