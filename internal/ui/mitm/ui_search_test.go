package mitm

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/text"
)

func bigJSONBody(n int) []byte {
	var sb strings.Builder
	sb.WriteString("{\n  \"items\": [\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "    { \"id\": %d, \"marker\": \"MARKER-%04d\" },\n", i, i)
	}
	sb.WriteString("  ]\n}\n")
	return []byte(sb.String())
}

// searchRig seeds one flow with a body long enough that a match near its end is
// far outside the first screenful, and pins a real font so the viewer's metrics
// are the same everywhere.
func newSearchRig(t *testing.T) *uiRig {
	t.Helper()
	rig := newUIRig(t, image.Pt(1200, 800))
	rig.host.Theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))

	base := time.Unix(1700000000, 0)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "api.example.com", Path: "/big.json",
		URL: "https://api.example.com/big.json", Version: "HTTP/1.1",
		ReqHeaders: [][2]string{{"Accept", "*/*"}},
		Status:     "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", "application/json"}},
		RespBody:    bigJSONBody(400),
		Started:     base, Ended: base.Add(time.Millisecond),
	})
	rig.s.Selected = rig.s.Store.Snapshot()[0].ID
	rig.s.ActTab = 1 // response
	rig.s.RenderMode = 1
	rig.s.SecTab = 1 // body
	rig.frames(3)
	return rig
}

func openSearch(t *testing.T, rig *uiRig, query string) {
	t.Helper()
	rig.s.HandleSearchShortcut(rig.gtx())
	if !rig.s.BodySearch.Open {
		t.Fatal("Ctrl+F must open the search over the inspector body")
	}
	rig.s.BodySearch.Editor.SetText(query)
	rig.frames(3)
}

func TestInspectorSearch_RevealsDeepMatch(t *testing.T) {
	rig := newSearchRig(t)
	openSearch(t, rig, "MARKER-0350")

	v := rig.s.BodyViewer
	if got := v.SelectedText(); got != "MARKER-0350" {
		t.Fatalf("search landed on %q", got)
	}
	if v.GetScrollY() <= 0 {
		t.Errorf("a match 350 entries down must scroll the viewer, scrollY = %d", v.GetScrollY())
	}
	if y, ok := v.RevealScreenY(); !ok || y < 0 || y >= rig.sz.Y {
		t.Errorf("match revealed at row %d, outside the pane", y)
	}
}

func TestInspectorSearch_FollowsRawAndHexPanes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  int
		query string
	}{
		{"raw", 0, "MARKER-0200"},
		{"hex", 2, "4d 41 52"}, // "MAR" in the hex dump
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newSearchRig(t)
			rig.s.RenderMode = tc.mode
			rig.frames(3)

			openSearch(t, rig, tc.query)
			if got := rig.s.BodyViewer.SelectedText(); !strings.EqualFold(got, tc.query) {
				t.Errorf("%s pane search landed on %q, want %q", tc.name, got, tc.query)
			}
		})
	}
}

// Headers / Params / Cookies are key-value rows, not text the viewer holds.
func TestInspectorSearch_InertOnRowPanes(t *testing.T) {
	rig := newSearchRig(t)
	for _, sec := range []int{0, 2, 3} {
		rig.s.SecTab = sec
		rig.frames(2)
		rig.s.HandleSearchShortcut(rig.gtx())
		if rig.s.BodySearch.Open {
			t.Errorf("SecTab %d has no searchable text, but Ctrl+F opened the panel", sec)
			rig.s.BodySearch.Close(rig.s.BodyViewer)
		}
	}
}

func TestInspectorSearch_ReRunsWhenTheFlowChanges(t *testing.T) {
	rig := newSearchRig(t)
	openSearch(t, rig, "MARKER-0100")
	if got := rig.s.BodyViewer.SelectedText(); got != "MARKER-0100" {
		t.Fatalf("precondition: selection = %q", got)
	}

	base := time.Unix(1700000000, 0)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "other.example.com", Path: "/small",
		Status: "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", "text/plain"}},
		RespBody:    []byte("nothing to find here"),
		Started:     base.Add(time.Second),
	})
	flows := rig.s.Store.Snapshot()
	rig.s.Selected = flows[len(flows)-1].ID
	rig.frames(3)

	if got := rig.s.BodyViewer.Text(); got != "nothing to find here" {
		t.Fatalf("viewer still holds the previous flow: %q", got)
	}
	if got := rig.s.BodyViewer.SelectedText(); got != "" {
		t.Errorf("stale match %q still selected after the flow changed", got)
	}
}

func TestInspectorSearch_ClosesOnABodylessFlow(t *testing.T) {
	rig := newSearchRig(t)
	openSearch(t, rig, "MARKER-0100")

	base := time.Unix(1700000000, 0)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "empty.example.com", Path: "/none",
		Status: "204 No Content", StatusCode: 204,
		Started: base.Add(2 * time.Second),
	})
	flows := rig.s.Store.Snapshot()
	rig.s.Selected = flows[len(flows)-1].ID
	rig.frames(3)

	if rig.s.BodySearch.Open {
		t.Error("the panel must close on a flow whose pane shows no body")
	}
}
