package har

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"testing"
)

// bigBodyHAR builds a capture whose single entry carries a response body long
// enough that a match near its end is far outside the first screenful.
func bigBodyHAR(lines int) string {
	var sb strings.Builder
	sb.WriteString("{\n  \"items\": [\n")
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&sb, "    { \"id\": %d, \"marker\": \"MARKER-%04d\", \"pad\": \"%s\" },\n",
			i, i, strings.Repeat("long-", 20))
	}
	sb.WriteString("  ]\n}\n")
	body, _ := json.Marshal(sb.String())

	return `{"log":{"version":"1.2","entries":[
	  {"startedDateTime":"2024-01-01T10:00:00Z",
	   "request":{"method":"GET","url":"https://example.com/big.json","headers":[]},
	   "response":{"status":200,"headers":[{"name":"Content-Type","value":"application/json"}],
	     "content":{"mimeType":"application/json","text":` + string(body) + `}}}
	]}}`
}

func openBodySearch(t *testing.T, rig *harRig, query string) {
	t.Helper()
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1 // response body; the request body of a GET is empty
	rig.frames(3)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.BodySearch.Open {
		t.Fatal("Ctrl+F must open the body search on the requests tab")
	}
	rig.s.BodySearch.Editor.SetText(query)
	rig.frames(3)
}

func TestHARSearch_RevealsDeepMatch(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0350")

	v := rig.s.ReqViewer
	if got := v.SelectedText(); got != "MARKER-0350" {
		t.Fatalf("search did not land on the match, selection = %q", got)
	}
	if v.GetScrollY() <= 0 {
		t.Errorf("a match 350 entries down must scroll the viewer, scrollY = %d", v.GetScrollY())
	}
	y, ok := v.RevealScreenY()
	if !ok {
		t.Fatal("the viewer never resolved the pending reveal")
	}
	if y < 0 || y >= rig.sz.Y {
		t.Errorf("match revealed at row %d, outside the %d-tall section", y, rig.sz.Y)
	}
}

func TestHARSearch_ClosingClearsTheMatchSelection(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0350")
	if got := rig.s.ReqViewer.SelectedText(); got != "MARKER-0350" {
		t.Fatalf("precondition: match should be selected, got %q", got)
	}

	rig.s.BodySearch.Close(rig.s.ReqViewer)
	rig.frames(2)

	if got := rig.s.ReqViewer.SelectedText(); got != "" {
		t.Errorf("closing the search left %q selected in the inspector body", got)
	}
}

func TestHARSearch_FollowsEntryBodySwap(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0200")
	if got := rig.s.ReqViewer.SelectedText(); got != "MARKER-0200" {
		t.Fatalf("selection = %q before the swap", got)
	}

	// Pretty-printing swaps the viewer text under the open search box.
	rig.s.Pretty = !rig.s.Pretty
	rig.frames(3)

	v := rig.s.ReqViewer
	if got := v.SelectedText(); got != "MARKER-0200" {
		t.Errorf("after the body was reformatted the search still points at %q", got)
	}
	if y, ok := v.RevealScreenY(); !ok || y < 0 || y >= rig.sz.Y {
		t.Errorf("match not revealed after the swap: y=%d ok=%v", y, ok)
	}
}

// bodyViewer bails out before it processes or draws the search panel when the
// pane has nothing to show, so a box opened there is invisible, unclosable, and
// pops open again on the next pane that does have a body.
func TestHARSearch_NotLeftOpenOnBodylessPane(t *testing.T) {
	rig := newRig(t, bigBodyHAR(20), image.Pt(1100, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 0 // request body of a GET: empty
	rig.frames(3)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Error("search must not stay open over a pane that shows no body")
	}

	rig.s.InspTab = 1
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Error("the response pane inherited a search box the user never sees opening")
	}
}

func TestHARSearch_ClosesWhenBodyGoesAway(t *testing.T) {
	rig := newRig(t, bigBodyHAR(20), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0010")
	if !rig.s.BodySearch.Open {
		t.Fatal("precondition: search open on the response body")
	}

	rig.s.InspTab = 0 // switch to the empty request body
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Error("search stayed open on a pane where it cannot be drawn or dismissed")
	}
}

func TestHARSearch_FileViewerSharesTheFix(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	rig.s.TopTab = TabFiles
	rig.frames(3)
	if len(rig.s.Resources) == 0 {
		t.Skip("capture produced no extractable resources")
	}
	rig.s.SelFile = 0
	rig.frames(3)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.FileSearch.Open {
		t.Fatal("Ctrl+F must open the file search on the files tab")
	}
	rig.s.FileSearch.Editor.SetText("MARKER-0300")
	rig.frames(3)

	v := rig.s.FileViewer
	if got := v.SelectedText(); got != "MARKER-0300" {
		t.Fatalf("file viewer search landed on %q", got)
	}
	if y, ok := v.RevealScreenY(); !ok || y < 0 || y >= rig.sz.Y {
		t.Errorf("file match not revealed: y=%d ok=%v", y, ok)
	}
}
