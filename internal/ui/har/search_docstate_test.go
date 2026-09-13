package har

import (
	"image"
	"strings"
	"testing"
)

func fileIndexByMime(t *testing.T, rig *harRig, mime string) int {
	t.Helper()
	for i, r := range rig.s.Resources {
		if r.MimeType == mime {
			return i
		}
	}
	t.Fatalf("no %s resource among %d files", mime, len(rig.s.Resources))
	return -1
}

func TestHARFileSearch_StateIsPerFile(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1100, 700))
	rig.s.TopTab = TabFiles
	jsonFile := fileIndexByMime(t, rig, "application/json")
	cssFile := fileIndexByMime(t, rig, "text/css")

	rig.s.SelFile = jsonFile
	rig.frames(3)
	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.FileSearch.Open {
		t.Fatal("Ctrl+F must open the file search")
	}
	rig.s.FileSearch.Editor.SetText(`"`)
	rig.frames(3)
	_, total := rig.s.FileSearch.Position()
	if total < 2 {
		t.Fatalf("precondition: the JSON body should hold several quotes, got %d", total)
	}
	rig.s.FileSearch.NextBtn.Click()
	rig.frames(3)
	if cur, _ := rig.s.FileSearch.Position(); cur != 2 {
		t.Fatalf("precondition: expected to be on match 2, got %d", cur)
	}

	rig.s.SelFile = cssFile
	rig.frames(3)
	if rig.s.FileSearch.Open {
		t.Errorf("a file that was never searched must not inherit the open panel")
	}
	if got := rig.s.FileViewer.SelectedText(); got != "" {
		t.Errorf("the other file's match must not stay selected on this file, got %q", got)
	}

	rig.s.SelFile = jsonFile
	rig.frames(3)
	if !rig.s.FileSearch.Open || rig.s.FileSearch.Editor.Text() != `"` {
		t.Fatalf("returning to the file must restore its search, got open=%v query=%q", rig.s.FileSearch.Open, rig.s.FileSearch.Editor.Text())
	}
	if cur, n := rig.s.FileSearch.Position(); cur != 2 || n != total {
		t.Errorf("returning to the file must land on its match 2/%d, got %d/%d", total, cur, n)
	}
	if got := rig.s.FileViewer.SelectedText(); got != `"` {
		t.Errorf("the restored match must be selected again, got %q", got)
	}
}

func TestHARInspector_BinaryBodyShowsHexWithModes(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1100, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 2
	rig.s.InspTab = 1
	rig.frames(3)

	text := rig.s.ReqViewer.Text()
	if !strings.HasPrefix(text, "00000000  00 01 02 03 04 05 06 07  08 09 00 00 00 00") {
		t.Fatalf("binary response must render as a hex dump, got %q", text)
	}

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.BodySearch.Open {
		t.Error("Ctrl+F must work over the hex dump")
	}
	rig.s.BodySearch.Editor.SetText("08 09")
	rig.frames(3)
	if got := rig.s.ReqViewer.SelectedText(); got != "08 09" {
		t.Errorf("search over the hex dump landed on %q", got)
	}

	rig.s.BodyBin.Btn(2).Click()
	rig.frames(3)
	if got := rig.s.ReqViewer.Text(); got != "AAECAwQFBgcICQAAAAA=" {
		t.Errorf("Base64 chip must re-render the body, got %q", got)
	}
	rig.s.BodyBin.Btn(1).Click()
	rig.frames(3)
	if got := rig.s.ReqViewer.Text(); got != "0001020304050607080900000000" {
		t.Errorf("Hex chip must re-render the body, got %q", got)
	}
}
