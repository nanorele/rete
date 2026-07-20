package workspace

import (
	"image"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestURLTypingQueryKeepsCaretAndText(t *testing.T) {
	tab := NewRequestTab("t")
	var r input.Router
	ops := new(op.Ops)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(400, 40)),
			Now:         time.Now(),
			Source:      r.Source(),
		}
		tab.updateReqSubTabs(gtx)
		r.Frame(ops)
	}

	const url = "https://exammple.com??m&x&&y=1&"
	typed := ""
	for _, ch := range url {
		tab.URLInput.Insert(string(ch))
		typed += string(ch)
		frame()
		frame()
		frame()
		if got := tab.URLInput.Text(); got != typed {
			t.Fatalf("after typing %q: URL text became %q", typed, got)
		}
		start, end := tab.URLInput.Selection()
		n := utf8.RuneCountInString(typed)
		if start != n || end != n {
			t.Fatalf("after typing %q: caret = (%d,%d), want %d", typed, start, end, n)
		}
	}
}

func TestParamsEditStillRewritesURL(t *testing.T) {
	tab := NewRequestTab("t")
	var r input.Router
	ops := new(op.Ops)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(400, 40)),
			Now:         time.Now(),
			Source:      r.Source(),
		}
		tab.updateReqSubTabs(gtx)
		r.Frame(ops)
	}

	tab.URLInput.SetText("https://x/y?a=1")
	frame()
	frame()
	if len(tab.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(tab.Params))
	}

	tab.Params[0].Value.SetCaret(1, 1)
	tab.Params[0].Value.Insert("2")
	frame()
	frame()
	if got := tab.URLInput.Text(); got != "https://x/y?a=12" {
		t.Fatalf("URL after param edit = %q, want %q", got, "https://x/y?a=12")
	}
}
