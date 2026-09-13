package mitm

import (
	"strings"
	"testing"
	"time"
)

func addFlow(rig *uiRig, path string, ct string, body []byte, offset time.Duration) uint64 {
	base := time.Unix(1700000000, 0).Add(offset)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "api.example.com", Path: path,
		URL: "https://api.example.com" + path, Version: "HTTP/1.1",
		Status: "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", ct}},
		RespBody:    body,
		Started:     base, Ended: base.Add(time.Millisecond),
	})
	flows := rig.s.Store.Snapshot()
	return flows[len(flows)-1].ID
}

func TestInspectorSearch_StateIsPerFlow(t *testing.T) {
	rig := newSearchRig(t)
	first := rig.s.Selected
	second := addFlow(rig, "/second.json", "application/json", bigJSONBody(50), time.Second)

	openSearch(t, rig, "MARKER-00")
	rig.s.BodySearch.NextBtn.Click()
	rig.frames(3)
	if cur, n := rig.s.BodySearch.Position(); cur != 2 || n != 100 {
		t.Fatalf("precondition: expected 2/100 on the first flow, got %d/%d", cur, n)
	}

	rig.s.Selected = second
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Errorf("a flow that was never searched must not inherit the open panel")
	}
	if got := rig.s.BodyViewer.SelectedText(); got != "" {
		t.Errorf("first flow's match still selected on the second flow: %q", got)
	}
	openSearch(t, rig, "MARKER-0049")
	if cur, n := rig.s.BodySearch.Position(); cur != 1 || n != 1 {
		t.Fatalf("second flow: expected 1/1, got %d/%d", cur, n)
	}

	rig.s.Selected = first
	rig.frames(3)
	if !rig.s.BodySearch.Open || rig.s.BodySearch.Editor.Text() != "MARKER-00" {
		t.Fatalf("returning to the first flow must restore its search, got open=%v query=%q", rig.s.BodySearch.Open, rig.s.BodySearch.Editor.Text())
	}
	if cur, n := rig.s.BodySearch.Position(); cur != 2 || n != 100 {
		t.Errorf("first flow must resume on 2/100, got %d/%d", cur, n)
	}
	if got := rig.s.BodyViewer.SelectedText(); got != "MARKER-00" {
		t.Errorf("restored match must be selected, got %q", got)
	}

	rig.s.Selected = second
	rig.frames(3)
	if cur, n := rig.s.BodySearch.Position(); !rig.s.BodySearch.Open || cur != 1 || n != 1 {
		t.Errorf("second flow must resume on its own 1/1, got open=%v %d/%d", rig.s.BodySearch.Open, cur, n)
	}
}

func TestInspector_BinaryBodyShowsHexWithModes(t *testing.T) {
	rig := newSearchRig(t)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	id := addFlow(rig, "/logo.png", "image/png", png, time.Second)
	rig.s.Selected = id
	rig.frames(3)

	if got := rig.s.BodyViewer.Text(); !strings.HasPrefix(got, "00000000  89 50 4e 47 0d 0a 1a 0a  00 00 00 0d 49 48 44 52  |.PNG........IHDR|") {
		t.Fatalf("binary body must render as a hex dump in the Body pane, got %q", got)
	}
	rig.s.BodyBin.Btn(2).Click()
	rig.frames(3)
	if got := rig.s.BodyViewer.Text(); got != "iVBORw0KGgoAAAANSUhEUg==" {
		t.Errorf("Base64 chip must re-render the body, got %q", got)
	}

	rig.s.RenderMode = 0
	rig.frames(3)
	if got := rig.s.BodyViewer.Text(); !strings.Contains(got, "00000000  89 50 4e 47") || strings.Contains(got, "\x89PNG") {
		t.Errorf("Raw pane must hex-dump a binary body instead of pasting its bytes, got %q", got)
	}
}
