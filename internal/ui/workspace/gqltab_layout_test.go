package workspace

import (
	"image"
	"strings"
	"testing"

	"tracto/internal/model"
)

func newGQLRig(size image.Point) *vstackRig {
	rig := newVStackRig()
	rig.tab.Method = MethodGraphQL
	rig.tab.URLInput.SetText("http://api.test/graphql")
	rig.size = size
	g := rig.tab.EnsureGQL()
	g.Query.SetText("query { me { id name } }")
	g.Variables.SetText(`{"id":1}`)
	return rig
}

func TestGraphQLLayoutRendersInBothOrientations(t *testing.T) {
	cases := []struct {
		name     string
		mode     int
		expanded bool
		headers  bool
		size     image.Point
	}{
		{"horizontal expanded", LayoutModeHoriz, true, true, image.Pt(1200, 700)},
		{"horizontal collapsed", LayoutModeHoriz, false, true, image.Pt(1200, 700)},
		{"vertical expanded", LayoutModeVert, true, true, image.Pt(900, 800)},
		{"vertical no headers", LayoutModeVert, true, false, image.Pt(900, 800)},
		{"narrow", LayoutModeHoriz, true, true, image.Pt(360, 300)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newGQLRig(c.size)
			rig.tab.LayoutMode = c.mode
			rig.tab.HeadersExpanded = c.expanded
			if !c.headers {
				rig.tab.Headers = rig.tab.Headers[:0]
			}
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if rig.tab.GQL == nil {
				t.Fatal("laying out a GraphQL tab must ensure the GQL session")
			}
		})
	}
}

func TestGraphQLHeaderToggleAndAddButtons(t *testing.T) {
	rig := newGQLRig(image.Pt(1200, 700))
	rig.tab.HeadersExpanded = true
	rig.frame()

	before := len(rig.tab.Headers)
	rig.tab.AddHeaderBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Headers) <= before {
		t.Errorf("Add header = %d rows, want more than %d", len(rig.tab.Headers), before)
	}

	rig.tab.ViewGeneratedBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.HeadersExpanded {
		t.Error("the toggle must collapse the headers area")
	}
	rig.tab.ViewGeneratedBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.HeadersExpanded {
		t.Error("the toggle must expand the headers area again")
	}
}

func TestGraphQLCopyButtonsRunWithoutPanic(t *testing.T) {
	rig := newGQLRig(image.Pt(1200, 700))
	rig.frame()
	g := rig.tab.GQL
	g.QueryCopyBtn.Click()
	rig.frame()
	g.VarsCopyBtn.Click()
	rig.frame()
	rig.frame()
	if g.Query.Text() != "query { me { id name } }" {
		t.Errorf("copying must not disturb the query: %q", g.Query.Text())
	}
}

func TestGraphQLVarsSplitDragMovesRatio(t *testing.T) {
	rig := newGQLRig(image.Pt(1200, 800))
	rig.tab.LayoutMode = LayoutModeHoriz
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	g := rig.tab.GQL
	before := g.VarsSplitRatio

	x := 200
	moved := false
	for y := 100; y < 780 && !moved; y++ {
		rig.drag(x, y, y+60)
		if g.VarsSplitRatio != before {
			moved = true
		}
	}
	if !moved {
		t.Fatal("dragging the query/variables splitter never changed VarsSplitRatio")
	}
	if g.VarsSplitRatio < 0.15 || g.VarsSplitRatio > 0.85 {
		t.Errorf("VarsSplitRatio = %v, want it clamped to [0.15, 0.85]", g.VarsSplitRatio)
	}
	rig.frame()
}

func TestGraphQLVarsSplitRatioIsClampedOnLayout(t *testing.T) {
	for _, start := range []float32{0.01, 0.99} {
		rig := newGQLRig(image.Pt(1200, 800))
		rig.tab.GQL.VarsSplitRatio = start
		rig.frame()
		got := rig.tab.GQL.VarsSplitRatio
		if got < 0.15 || got > 0.85 {
			t.Errorf("VarsSplitRatio started at %v and stayed %v, want it clamped", start, got)
		}
	}
}

func TestGraphQLResponsePaneShowsDownloadProgress(t *testing.T) {
	rig := newGQLRig(image.Pt(1200, 700))
	rig.tab.Status = "Ready"
	rig.frame()

	rig.tab.isRequesting = true
	rig.tab.downloadedBytes.Store(2048)
	rig.frame()
	rig.frame()

	rig.tab.isRequesting = false
	rig.tab.Status = "200 OK"
	rig.frame()
	if rig.tab.Status != "200 OK" {
		t.Errorf("Status = %q", rig.tab.Status)
	}
}

func TestGraphQLResponsePaneRendersLargeBody(t *testing.T) {
	rig := newGQLRig(image.Pt(1200, 700))
	rig.tab.RespEditor.SetText(strings.Repeat(`{"data":{"me":{"id":1}}}`+"\n", 300))
	rig.tab.respIsJSON = true
	rig.tab.Status = "200 OK"
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.RespEditor.Text() == "" {
		t.Fatal("response editor is empty")
	}
}

func TestActiveKVItemsFollowsSubTab(t *testing.T) {
	tab := NewRequestTab("t")
	tab.AddHeader("H", "1")
	tab.addParam("P", "2")
	tab.addCookie("C", "3")

	cases := []struct {
		sub  int
		want string
	}{
		{reqSubHeaders, "H"},
		{reqSubParams, "P"},
		{reqSubCookies, "C"},
		{reqSubAuth, "H"},
		{99, "H"},
	}
	for _, c := range cases {
		tab.ReqSubTab = c.sub
		items := tab.activeKVItems()
		if len(items) == 0 {
			t.Fatalf("sub-tab %d returned no items", c.sub)
		}
		if got := items[0].Key.Text(); got != c.want {
			t.Errorf("sub-tab %d -> first key %q, want %q", c.sub, got, c.want)
		}
		list := tab.activeKVList()
		if list == nil {
			t.Errorf("sub-tab %d returned a nil list", c.sub)
		}
	}
}

func TestRequestSubTabPanelsRender(t *testing.T) {
	cases := []struct {
		name     string
		sub      int
		authType int
		cookies  bool
		params   bool
	}{
		{"headers", reqSubHeaders, authNone, false, false},
		{"params", reqSubParams, authNone, false, true},
		{"params empty", reqSubParams, authNone, false, false},
		{"cookies", reqSubCookies, authNone, true, false},
		{"cookies empty", reqSubCookies, authNone, false, false},
		{"auth none", reqSubAuth, authNone, false, false},
		{"auth bearer", reqSubAuth, authBearer, false, false},
		{"auth basic", reqSubAuth, authBasic, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newVStackRig()
			rig.size = image.Pt(1100, 700)
			rig.tab.HeadersExpanded = true
			rig.tab.ReqSubTab = c.sub
			rig.tab.AuthType = c.authType
			rig.tab.AuthToken.SetText("{{tok}}")
			rig.tab.AuthUser.SetText("user")
			rig.tab.AuthPass.SetText("pass")
			if c.cookies {
				rig.tab.ApplyCookies([]model.ParsedKV{{Key: "sid", Value: "1"}})
			}
			if c.params {
				rig.tab.URLInput.SetText("http://example.com?a=1&b=2")
			}
			for i := 0; i < 3; i++ {
				rig.frame()
			}
		})
	}
}

func TestAuthTypeSelectorMenuOpensAndPicks(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.HeadersExpanded = true
	rig.tab.ReqSubTab = reqSubAuth
	rig.frame()

	rig.tab.AuthTypeBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.AuthTypeOpen {
		t.Fatal("clicking the type button must open the menu")
	}

	rig.tab.AuthTypeChoices[authBasic].Click()
	rig.frame()
	rig.frame()
	if rig.tab.AuthType != authBasic {
		t.Errorf("AuthType = %d, want basic", rig.tab.AuthType)
	}
	if rig.tab.AuthTypeOpen {
		t.Error("picking a type must close the menu")
	}
	if !rig.tab.dirtyCheckNeeded {
		t.Error("changing the auth type must mark the tab dirty")
	}
}

func TestRequestSubTabButtonsSwitchAndExpand(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.HeadersExpanded = false
	rig.frame()

	cases := []struct {
		click func(*RequestTab)
		want  int
	}{
		{func(tb *RequestTab) { tb.ParamsTabBtn.Click() }, reqSubParams},
		{func(tb *RequestTab) { tb.AuthTabBtn.Click() }, reqSubAuth},
		{func(tb *RequestTab) { tb.CookiesTabBtn.Click() }, reqSubCookies},
		{func(tb *RequestTab) { tb.HeadersTabBtn.Click() }, reqSubHeaders},
	}
	for _, c := range cases {
		c.click(rig.tab)
		rig.frame()
		rig.frame()
		if rig.tab.ReqSubTab != c.want {
			t.Fatalf("ReqSubTab = %d, want %d", rig.tab.ReqSubTab, c.want)
		}
		if !rig.tab.HeadersExpanded {
			t.Fatalf("selecting sub-tab %d must expand the area", c.want)
		}
	}
}

func TestCookieDeleteButtonRemovesRow(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.HeadersExpanded = true
	rig.tab.ReqSubTab = reqSubCookies
	rig.tab.ApplyCookies([]model.ParsedKV{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}})
	rig.frame()

	rig.tab.Cookies[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Cookies) != 1 {
		t.Fatalf("cookies = %d, want 1 after deleting a row", len(rig.tab.Cookies))
	}
	if rig.tab.Cookies[0].Key.Text() != "b" {
		t.Errorf("remaining cookie = %q, want b", rig.tab.Cookies[0].Key.Text())
	}
}

func TestParamDeleteButtonRewritesURL(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.HeadersExpanded = true
	rig.tab.ReqSubTab = reqSubParams
	rig.tab.URLInput.SetText("http://example.com/p?a=1&b=2#frag")
	rig.frame()
	rig.frame()
	if len(rig.tab.Params) != 2 {
		t.Fatalf("params = %d, want 2 synced from the URL", len(rig.tab.Params))
	}

	rig.tab.Params[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Params) != 1 {
		t.Fatalf("params = %d, want 1 after deleting", len(rig.tab.Params))
	}
	if got := rig.tab.URLInput.Text(); got != "http://example.com/p?b=2#frag" {
		t.Errorf("URL = %q, want the deleted param removed and the fragment kept", got)
	}
}
