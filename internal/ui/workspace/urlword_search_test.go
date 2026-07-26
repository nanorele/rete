package workspace

import (
	"strings"
	"testing"

	"tracto/internal/model"

	"image"
	"time"

	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestURLWordBounds(t *testing.T) {
	cases := []struct {
		name string
		url  string
		pos  int
		want string
	}{
		{name: "inside a word", url: "http://a.test/users", pos: 16, want: "users"},
		{name: "at word start", url: "http://a.test/users", pos: 14, want: "users"},
		{name: "at end of text", url: "http://a.test/users", pos: 19, want: "users"},
		{name: "on the separator run", url: "http://a.test/users", pos: 5, want: "://"},
		{name: "scheme word", url: "http://a.test/users", pos: 2, want: "http"},
		{name: "caret after a word", url: "http://a.test/users", pos: 4, want: "http"},
		{name: "host label", url: "http://a.test/users", pos: 10, want: "test"},
		{name: "query key", url: "http://a.test?key=val", pos: 15, want: "key"},
		{name: "query value", url: "http://a.test?key=val", pos: 19, want: "val"},
		{name: "empty string", url: "", pos: 0, want: ""},
		{name: "position past end", url: "abc", pos: 99, want: "abc"},
		{name: "negative position", url: "abc", pos: -5, want: "abc"},
		{name: "all separators", url: "///", pos: 1, want: "///"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := urlWordBounds(c.url, c.pos)
			runes := []rune(c.url)
			if start < 0 || end > len(runes) || start > end {
				t.Fatalf("bounds (%d,%d) are out of range for %q", start, end, c.url)
			}
			if got := string(runes[start:end]); got != c.want {
				t.Errorf("urlWordBounds(%q, %d) = %q, want %q", c.url, c.pos, got, c.want)
			}
		})
	}
}

func TestURLWordBoundsSelectsWholeVariable(t *testing.T) {
	url := "http://{{host}}/api/{{ver}}/x"
	for _, pos := range []int{7, 8, 11, 14} {
		start, end := urlWordBounds(url, pos)
		if got := string([]rune(url)[start:end]); got != "{{host}}" {
			t.Errorf("pos %d -> %q, want the whole {{host}} variable", pos, got)
		}
	}
	start, end := urlWordBounds(url, 21)
	if got := string([]rune(url)[start:end]); got != "{{ver}}" {
		t.Errorf("pos 21 -> %q, want {{ver}}", got)
	}
}

func TestURLWordBoundsStopsAtVariableEdges(t *testing.T) {
	url := "abc{{v}}def"
	start, end := urlWordBounds(url, 1)
	if got := string([]rune(url)[start:end]); got != "abc" {
		t.Errorf("word before a variable = %q, want abc", got)
	}
	start, end = urlWordBounds(url, 9)
	if got := string([]rune(url)[start:end]); got != "def" {
		t.Errorf("word after a variable = %q, want def", got)
	}
}

func TestURLWordBoundsUnterminatedVariable(t *testing.T) {
	url := "http://{{host/api"
	start, end := urlWordBounds(url, 10)
	runes := []rune(url)
	if start < 0 || end > len(runes) || start > end {
		t.Fatalf("bounds (%d,%d) out of range", start, end)
	}
	if got := string(runes[start:end]); got != "host" {
		t.Errorf("an unterminated {{ must fall back to plain word bounds, got %q", got)
	}
}

func searchGtx(r *input.Router) layout.Context {
	return layout.Context{Ops: new(op.Ops), Source: r.Source()}
}

func TestHandleSearchShortcutTargetsResponseByDefault(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText("request text")
	tab.RespEditor.SetText("response text")

	var r input.Router
	tab.HandleSearchShortcut(searchGtx(&r))
	if !tab.RespSearch.Open {
		t.Error("with nothing focused the shortcut must open the response search")
	}
	if tab.ReqSearch.Open {
		t.Error("the request search must stay closed")
	}
}

func (rig *vstackRig) frameWithSearchShortcut() {
	rig.now = rig.now.Add(16 * time.Millisecond)
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.tab.Layout(gtx, rig.th, rig.win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	rig.tab.HandleSearchShortcut(gtx)
	rig.r.Frame(rig.ops)
}

func TestHandleSearchShortcutFollowsTheFocusedEditor(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.ReqEditor.SetText("request text here")
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	opened := false
	for y := 120; y < 460 && !opened; y += 4 {
		rig.tab.ReqSearch.Open = false
		rig.tab.RespSearch.Open = false
		rig.r.Queue(pointerPress(300, y))
		rig.frame()
		rig.r.Queue(pointerRelease(300, y))
		rig.frame()
		rig.frameWithSearchShortcut()
		opened = rig.tab.ReqSearch.Open && !rig.tab.RespSearch.Open
	}
	if !opened {
		t.Fatal("focusing the request body never routed the search shortcut to the request pane")
	}

	rig.frame()
	rig.frameWithSearchShortcut()
	if rig.tab.ReqSearch.Open {
		t.Error("a second shortcut with the search box focused must close it")
	}
}

func TestSearchBoxInvalidateForcesRecompute(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText("alpha beta alpha")

	var r input.Router
	gtx := searchGtx(&r)
	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	tab.ReqSearch.Editor.SetText("alpha")
	tab.ReqSearch.refresh(&tab.ReqEditor, tab.ReqEditor.Text(), false)
	if got := len(tab.ReqSearch.spans); got != 2 {
		t.Fatalf("matches = %d, want 2", got)
	}

	tab.ReqEditor.SetText("alpha alpha alpha")
	tab.ReqSearch.Invalidate()
	tab.ReqSearch.refresh(&tab.ReqEditor, tab.ReqEditor.Text(), false)
	if got := len(tab.ReqSearch.spans); got != 3 {
		t.Errorf("matches after Invalidate = %d, want 3 against the new text", got)
	}
}

func TestSearchBoxCaseSensitivity(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText("Alpha alpha ALPHA")

	var r input.Router
	gtx := searchGtx(&r)
	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	tab.ReqSearch.Editor.SetText("alpha")
	tab.ReqSearch.refresh(&tab.ReqEditor, tab.ReqEditor.Text(), false)
	if got := len(tab.ReqSearch.spans); got != 3 {
		t.Fatalf("case-insensitive matches = %d, want 3", got)
	}

	tab.ReqSearch.CaseSensitive = true
	tab.ReqSearch.Invalidate()
	tab.ReqSearch.refresh(&tab.ReqEditor, tab.ReqEditor.Text(), false)
	if got := len(tab.ReqSearch.spans); got != 1 {
		t.Errorf("case-sensitive matches = %d, want 1", got)
	}
}

func TestBuildCurlCommandBodyVariants(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*RequestTab)
		want   []string
		absent []string
	}{
		{
			name: "no body",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyNone
				tb.ReqEditor.SetText("ignored")
			},
			absent: []string{"--data-raw", "--data-urlencode", "-F ", "--data-binary"},
		},
		{
			name: "raw body",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyRaw
				tb.ReqEditor.SetText(`{"a":"{{v}}"}`)
			},
			want: []string{`--data-raw '{"a":"1"}'`},
		},
		{
			name: "url encoded",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyURLEncoded
				tb.applyURLEncoded([]model.ParsedKV{
					{Key: "a", Value: "{{v}}"},
					{Key: "off", Value: "x", Disabled: true},
					{Key: "  ", Value: "blank"},
				})
			},
			want:   []string{"--data-urlencode 'a=1'"},
			absent: []string{"off=", "blank"},
		},
		{
			name: "form data",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyFormData
				tb.applyFormParts([]model.ParsedFormPart{
					{Key: "text", Value: "{{v}}", Kind: model.FormPartText},
					{Key: "up", Kind: model.FormPartFile, FilePath: "C:/tmp/x.bin"},
					{Key: "nofile", Kind: model.FormPartFile},
					{Key: "skip", Value: "s", Kind: model.FormPartText, Disabled: true},
				})
			},
			want:   []string{"-F 'text=1'", "-F 'up=@C:/tmp/x.bin'", "-F 'nofile=@'"},
			absent: []string{"skip="},
		},
		{
			name: "binary",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyBinary
				tb.BinaryFilePath = "/tmp/blob.bin"
			},
			want: []string{"--data-binary '@/tmp/blob.bin'"},
		},
		{
			name: "binary without a path",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyBinary
				tb.BinaryFilePath = ""
			},
			absent: []string{"--data-binary"},
		},
		{
			name: "raw body that renders empty",
			setup: func(tb *RequestTab) {
				tb.BodyType = model.BodyRaw
				tb.ReqEditor.SetText("{{missing}}")
			},
			absent: []string{"--data-raw"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			tab.Method = "POST"
			tab.URLInput.SetText("http://api.test/x")
			c.setup(tab)
			got := BuildCurlCommand(tab, map[string]string{"v": "1", "missing": ""})
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("curl command missing %q:\n%s", w, got)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Errorf("curl command must not contain %q:\n%s", a, got)
				}
			}
		})
	}
}

func TestBuildCurlCommandQuotingAndAuth(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = "get"
	tab.URLInput.SetText("api.test/it's")
	tab.BodyType = model.BodyNone
	tab.AuthType = authBearer
	tab.AuthToken.SetText("tok")
	tab.ApplyCookies([]model.ParsedKV{{Key: "sid", Value: "9"}})

	got := BuildCurlCommand(tab, nil)
	if strings.Contains(got, "-X GET") {
		t.Errorf("GET must not be spelled out with -X:\n%s", got)
	}
	if !strings.Contains(got, `'http://api.test/it'\''s'`) {
		t.Errorf("single quotes must be escaped shell-style:\n%s", got)
	}
	if !strings.Contains(got, "-H 'Authorization: Bearer tok'") {
		t.Errorf("auth header missing:\n%s", got)
	}
	if !strings.Contains(got, "-H 'Cookie: sid=9'") {
		t.Errorf("cookie header missing:\n%s", got)
	}
}

func TestBuildCurlCommandEmptyURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("  \n\t ")
	if got := BuildCurlCommand(tab, nil); got != "" {
		t.Errorf("BuildCurlCommand with no URL = %q, want empty", got)
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "''"},
		{"plain", "'plain'"},
		{"it's", `'it'\''s'`},
		{"a'b'c", `'a'\''b'\''c'`},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
