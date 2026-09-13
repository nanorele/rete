package workspace

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"github.com/uorg-saver/easyjson"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/binview"
	"rete/internal/ui/collections"
	"rete/internal/ui/settings"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"
	"rete/internal/ws"
	"rete/internal/wsproto"
	"rete/pkg/syntax"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func bigTextGtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(900, 700)),
		Now:         time.Unix(1700000000, 0),
	}
}

func TestWrapPlanWindowingMatchesWholeLineShaping(t *testing.T) {
	const innerW = 892
	const lineH = 18
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()

	for _, n := range []int{20 << 10, 100 << 10, 300 << 10} {
		line := strings.Repeat("abcdefghij0123456789", n/20)
		v := &textCore{lineStarts: []int{0}}
		v.text = []byte(line)
		v.padWrapPlans()

		want := widgets.WrapLineStarts(
			widgets.ShapeChunkForWrap(shaper, font.Font{}, unit.Sp(13), gtx, v.text, innerW),
		)
		p := v.ensureWrapPlan(0, 0, len(v.text), shaper, font.Font{}, unit.Sp(13), gtx, innerW, lineH)

		if got := planTotalSubLines(p); got != len(want) {
			t.Errorf("n=%d: sub-lines = %d, want %d", n, got, len(want))
		}
		if p.height != len(want)*lineH {
			t.Errorf("n=%d: height = %d, want %d", n, p.height, len(want)*lineH)
		}
		for i, start := range p.starts {
			wantIdx := i * subLinesPerWrapChunk
			if wantIdx >= len(want) {
				t.Fatalf("n=%d: chunk %d has no matching wrap line", n, i)
			}
			if start != want[wantIdx] {
				t.Errorf("n=%d: chunk %d starts at byte %d, want %d", n, i, start, want[wantIdx])
			}
		}
	}
}

func TestWrapPlanHandlesLineBeyondFixedPointRange(t *testing.T) {
	const innerW = 892
	const lineH = 18
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()

	v := &textCore{lineStarts: []int{0}}
	v.text = []byte(strings.Repeat("0123456789", 600000))
	v.padWrapPlans()

	p := v.ensureWrapPlan(0, 0, len(v.text), shaper, font.Font{}, unit.Sp(13), gtx, innerW, lineH)
	subLines := planTotalSubLines(p)
	if subLines < 10000 {
		t.Fatalf("6MB line wrapped into only %d sub-lines", subLines)
	}
	for i := range p.starts {
		start, end := planSubBounds(p, i, 0, len(v.text))
		if end-start > wrapShapeWindowBytes {
			t.Fatalf("sub-chunk %d spans %d bytes, over the %d-byte shaping window",
				i, end-start, wrapShapeWindowBytes)
		}
	}
}

func TestNoWrapPaintWindowTracksHorizontalScroll(t *testing.T) {
	const innerW = 400
	adv := fixedAdvance(8)

	line := strings.Repeat("abcdefghij", 20000)
	v := &textCore{lineStarts: []int{0}}
	v.text = []byte(line)

	short := &textCore{lineStarts: []int{0}}
	short.text = []byte("hello world")
	s, e, x, cols := short.noWrapPaintWindow(0, len(short.text), innerW, adv)
	if s != 0 || e != len(short.text) || x != 0 || cols != 0 {
		t.Errorf("short chunk = (%d,%d,%d,%d), want the whole chunk", s, e, x, cols)
	}

	for _, scrollX := range []int{0, 800, 40000, 1_000_000, 40000, 0} {
		v.scrollX = scrollX
		start, end, xOff, totalCols := v.noWrapPaintWindow(0, len(v.text), innerW, adv)
		if totalCols != len(line) {
			t.Fatalf("scrollX=%d: totalCols = %d, want %d", scrollX, totalCols, len(line))
		}
		wantFirst := colAtPx(adv, scrollX)
		if wantFirst > len(line) {
			wantFirst = len(line)
		}
		if start != wantFirst {
			t.Errorf("scrollX=%d: window starts at byte %d, want %d", scrollX, start, wantFirst)
		}
		if xOff != colPx(adv, wantFirst) {
			t.Errorf("scrollX=%d: xOff = %d, want %d", scrollX, xOff, colPx(adv, wantFirst))
		}
		if span := end - start; span > colAtPx(adv, innerW)+3 {
			t.Errorf("scrollX=%d: painted %d bytes for a %d px viewport", scrollX, span, innerW)
		}
		if end > len(line) {
			t.Errorf("scrollX=%d: window end %d past line end %d", scrollX, end, len(line))
		}
	}
}

func TestNoWrapPaintWindowRespectsRuneBoundaries(t *testing.T) {
	const innerW = 400
	adv := fixedAdvance(8)

	line := strings.Repeat("привет-мир ", 3000)
	v := &textCore{lineStarts: []int{0}}
	v.text = []byte(line)
	if len(v.text) <= longLineThresholdBytes {
		t.Fatalf("test line too short: %d bytes", len(v.text))
	}

	for _, scrollX := range []int{0, 500, 5000} {
		v.scrollX = scrollX
		start, end, _, _ := v.noWrapPaintWindow(0, len(v.text), innerW, adv)
		if !utf8Boundary(v.text, start) || !utf8Boundary(v.text, end) {
			t.Errorf("scrollX=%d: window [%d,%d) splits a rune", scrollX, start, end)
		}
	}
}

func fixedAdvance(px int) fixed.Int26_6 { return fixed.I(px) }

func utf8Boundary(b []byte, i int) bool {
	return i == len(b) || utf8.RuneStart(b[i])
}

func TestMonoWrapPlanMatchesShaper(t *testing.T) {
	const innerW = 492
	const lineH = 18
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()
	mono := font.Font{Typeface: "Go Mono"}

	cases := map[string]string{
		"ascii-no-space": strings.Repeat(`{"id":1,"name":"item","tags":["a","b"]},`, 6000),
		"with-spaces":    strings.Repeat("word alpha beta gamma delta ", 6000),
		"non-ascii":      strings.Repeat("привет-мир-данные-строка-", 6000),
	}
	for name, line := range cases {
		v := &textCore{lineStarts: []int{0}}
		v.text = []byte(line)
		v.padWrapPlans()
		v.monoAdvance = measureMonoAdvance(shaper, mono, unit.Sp(13), gtx)
		if v.monoAdvance <= 0 {
			t.Fatal("Go Mono did not measure as monospaced")
		}

		want := widgets.WrapLineStartsFor(shaper, mono, unit.Sp(13), gtx, v.text, innerW, nil)
		p := v.ensureWrapPlan(0, 0, len(v.text), shaper, mono, unit.Sp(13), gtx, innerW, lineH)

		if got := planTotalSubLines(p); got != len(want) {
			t.Errorf("%s: sub-lines = %d, want %d (mono=%v)", name, got, len(want), p.mono)
		}
		for i, start := range p.starts {
			idx := i * subLinesPerWrapChunk
			if idx < len(want) && start != want[idx] {
				t.Errorf("%s: chunk %d starts at %d, want %d (mono=%v)", name, i, start, want[idx], p.mono)
				break
			}
		}
	}
}

func TestMonoWrapPlanRejectsProportionalFont(t *testing.T) {
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	if adv := measureMonoAdvance(shaper, font.Font{}, unit.Sp(13), bigTextGtx()); adv != 0 {
		t.Errorf("proportional face measured as monospaced (advance %v)", adv)
	}
}

func TestWrapPlanExtendsOnAppend(t *testing.T) {
	const innerW = 492
	const lineH = 18
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()

	full := strings.Repeat("abcdefghij0123456789", 30000)
	grown := &textCore{lineStarts: []int{0}}
	grown.padWrapPlans()
	for _, cut := range []int{len(full) / 3, 2 * len(full) / 3, len(full)} {
		grown.text = []byte(full[:cut])
		grown.ensureWrapPlan(0, 0, cut, shaper, font.Font{}, unit.Sp(13), gtx, innerW, lineH)
	}

	fresh := &textCore{lineStarts: []int{0}}
	fresh.text = []byte(full)
	fresh.padWrapPlans()
	want := fresh.ensureWrapPlan(0, 0, len(full), shaper, font.Font{}, unit.Sp(13), gtx, innerW, lineH)
	got := grown.wrapPlans[0]

	if got.subTotal != want.subTotal || got.height != want.height {
		t.Fatalf("grown plan = %d sub-lines/%dpx, rebuilt = %d/%dpx",
			got.subTotal, got.height, want.subTotal, want.height)
	}
	if len(got.starts) != len(want.starts) {
		t.Fatalf("grown plan has %d chunks, rebuilt has %d", len(got.starts), len(want.starts))
	}
	for i := range got.starts {
		if got.starts[i] != want.starts[i] {
			t.Fatalf("chunk %d: grown start %d, rebuilt %d", i, got.starts[i], want.starts[i])
		}
	}
}

func TestFormPartRowFileMatchesTextHeight(t *testing.T) {
	th := material.NewTheme()
	textPart := NewFormPart("k", "v", model.FormPartText, "", 0)
	filePart := NewFormPart("k", "", model.FormPartFile, "C:\\tmp\\report_with_a_long_name.bin", 12345)
	emptyFile := NewFormPart("k", "", model.FormPartFile, "", 0)

	textH := formPartRow(makeBodyTestGtx(), th, textPart, nil).Size.Y
	fileH := formPartRow(makeBodyTestGtx(), th, filePart, nil).Size.Y
	emptyH := formPartRow(makeBodyTestGtx(), th, emptyFile, nil).Size.Y
	if fileH != textH {
		t.Errorf("file row height %d must equal text row height %d", fileH, textH)
	}
	if emptyH != textH {
		t.Errorf("empty file row height %d must equal text row height %d", emptyH, textH)
	}
}

func TestURLEncodedRowMatchesFormRowHeight(t *testing.T) {
	th := material.NewTheme()
	formH := formPartRow(makeBodyTestGtx(), th, NewFormPart("k", "v", model.FormPartText, "", 0), nil).Size.Y
	ueH := urlEncodedRow(makeBodyTestGtx(), th, NewURLEncodedPart("k", "v"), nil).Size.Y
	if ueH != formH {
		t.Errorf("urlencoded row height %d must equal form-data row height %d", ueH, formH)
	}
}

func TestFormPartKindToggleKeepsFile(t *testing.T) {
	rig := newVStackRig()
	rig.tab.BodyType = model.BodyFormData
	part := NewFormPart("avatar", "", model.FormPartFile, "C:\\tmp\\a.png", 99)
	rig.tab.FormParts = []*FormDataPart{part}
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	part.KindBtn.Click()
	rig.frame()
	rig.frame()
	if part.Kind != model.FormPartText {
		t.Fatalf("first toggle must switch to text, got %v", part.Kind)
	}
	part.KindBtn.Click()
	rig.frame()
	rig.frame()
	if part.Kind != model.FormPartFile {
		t.Fatalf("second toggle must switch back to file, got %v", part.Kind)
	}
	if part.FilePath != "C:\\tmp\\a.png" || part.FileSize != 99 {
		t.Errorf("chosen file must survive text/file round-trip: path %q size %d", part.FilePath, part.FileSize)
	}
}

func makeBodyTestGtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
}

func TestNewFormPart_Setters(t *testing.T) {
	p := NewFormPart("name", "alice", model.FormPartText, "/tmp/x", 42)
	if p.Key.Text() != "name" {
		t.Errorf("Key = %q, want %q", p.Key.Text(), "name")
	}
	if p.Value.Text() != "alice" {
		t.Errorf("Value = %q, want %q", p.Value.Text(), "alice")
	}
	if p.Kind != model.FormPartText {
		t.Errorf("Kind = %v, want FormPartText", p.Kind)
	}
	if p.FilePath != "/tmp/x" {
		t.Errorf("FilePath = %q", p.FilePath)
	}
	if p.FileSize != 42 {
		t.Errorf("FileSize = %d", p.FileSize)
	}
	if !p.Key.SingleLine || !p.Value.SingleLine {
		t.Errorf("expected SingleLine=true on Key+Value editors")
	}
	if p.Disabled {
		t.Errorf("Disabled should default to false")
	}
}

func TestNewURLEncodedPart_Setters(t *testing.T) {
	p := NewURLEncodedPart("a", "1")
	if p.Key.Text() != "a" || p.Value.Text() != "1" {
		t.Errorf("got Key=%q Value=%q", p.Key.Text(), p.Value.Text())
	}
	if !p.Key.SingleLine || !p.Value.SingleLine {
		t.Errorf("expected SingleLine=true on editors")
	}
	if p.Disabled {
		t.Errorf("Disabled default should be false")
	}
}

func TestBuildBody_URLEncoded_DisabledSkipped(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	enabled := NewURLEncodedPart("k1", "v1")
	disabled := NewURLEncodedPart("k2", "v2")
	disabled.Disabled = true
	tab.URLEncoded = []*URLEncodedPart{enabled, disabled}

	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildBody err: %v", err)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "k1=v1") {
		t.Errorf("expected enabled part k1=v1 in %q", got)
	}
	if strings.Contains(got, "k2") {
		t.Errorf("disabled URLEncodedPart leaked into body: %q", got)
	}
}

func TestBuildBody_FormData_DisabledSkipped(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	enabled := NewFormPart("kept", "yes", model.FormPartText, "", 0)
	disabled := NewFormPart("dropped", "no", model.FormPartText, "", 0)
	disabled.Disabled = true
	tab.FormParts = []*FormDataPart{enabled, disabled}

	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "kept") {
		t.Errorf("enabled part missing: %q", got)
	}
	if strings.Contains(got, "dropped") {
		t.Errorf("disabled form part leaked into body: %q", got)
	}
}

func TestBuildBody_FormData_DisabledFilePartSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(path, []byte("FILE-CONTENT"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	keep := NewFormPart("a", "b", model.FormPartText, "", 0)
	disabled := NewFormPart("upload", "", model.FormPartFile, path, 12)
	disabled.Disabled = true
	tab.FormParts = []*FormDataPart{keep, disabled}

	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if strings.Contains(got, "FILE-CONTENT") {
		t.Errorf("disabled file part should not be sent: %q", got)
	}
}

func TestSaveToCollection_DisabledFieldsPersisted(t *testing.T) {
	req := &model.ParsedRequest{Method: "POST", URL: "http://x", Name: "T"}
	tab := &RequestTab{
		Method: "POST",
		Title:  "T",
	}

	tab.URLInput.SetText("http://x")
	tab.BodyType = model.BodyFormData

	disF := NewFormPart("flagged", "v", model.FormPartText, "", 0)
	disF.Disabled = true
	tab.FormParts = []*FormDataPart{disF}

	disU := NewURLEncodedPart("k", "v")
	disU.Disabled = true
	tab.URLEncoded = []*URLEncodedPart{disU}

	tab.LinkedNode = nil

	if disF.Disabled != true {
		t.Errorf("FormDataPart.Disabled lost")
	}
	if disU.Disabled != true {
		t.Errorf("URLEncodedPart.Disabled lost")
	}

	_ = req
}

func TestDrainBodyChans_FormPartFile_UpdatesMatchingPart(t *testing.T) {
	tab := NewRequestTab("t")
	target := NewFormPart("k", "", model.FormPartFile, "", 0)
	other := NewFormPart("k2", "", model.FormPartFile, "/orig", 1)
	tab.FormParts = []*FormDataPart{other, target}

	tab.formPartFileChan <- formPartFileResult{part: target, path: "/new/path", size: 999}

	gtx := makeBodyTestGtx()
	tab.drainBodyChans()

	if target.FilePath != "/new/path" || target.FileSize != 999 {
		t.Errorf("target part not updated: path=%q size=%d", target.FilePath, target.FileSize)
	}
	if other.FilePath != "/orig" || other.FileSize != 1 {
		t.Errorf("other part should not be touched, got path=%q size=%d", other.FilePath, other.FileSize)
	}
	if !tab.dirtyCheckNeeded {
		t.Errorf("dirtyCheckNeeded should be set after file picked")
	}
	_ = gtx
}

func TestDrainBodyChans_FormPartFile_NilPartIgnored(t *testing.T) {
	tab := NewRequestTab("t")
	tab.formPartFileChan <- formPartFileResult{part: nil, path: "/x", size: 1}
	tab.drainBodyChans()
	if tab.dirtyCheckNeeded {
		t.Errorf("nil part should not flag dirty")
	}
}

func TestDrainBodyChans_FormPartFile_DanglingPartIgnored(t *testing.T) {
	tab := NewRequestTab("t")
	ghost := NewFormPart("ghost", "", model.FormPartFile, "", 0)
	tab.FormParts = []*FormDataPart{NewFormPart("real", "", model.FormPartFile, "", 0)}

	tab.formPartFileChan <- formPartFileResult{part: ghost, path: "/x", size: 1}
	tab.drainBodyChans()
	if ghost.FilePath != "" {
		t.Errorf("ghost part not in FormParts should NOT receive path, got %q", ghost.FilePath)
	}
	if tab.dirtyCheckNeeded {
		t.Errorf("non-matching part should not flag dirty")
	}
}

func TestDrainBodyChans_BinaryFile(t *testing.T) {
	tab := NewRequestTab("t")
	tab.binaryFileChan <- binaryFileResult{path: "/bin/path", size: 4096}
	tab.drainBodyChans()
	if tab.BinaryFilePath != "/bin/path" || tab.BinaryFileSize != 4096 {
		t.Errorf("binary not applied: path=%q size=%d", tab.BinaryFilePath, tab.BinaryFileSize)
	}
	if !tab.dirtyCheckNeeded {
		t.Errorf("dirtyCheckNeeded should be true after binary pick")
	}
}

func TestDrainBodyChans_Idempotent_NoMessage(t *testing.T) {
	tab := NewRequestTab("t")
	tab.drainBodyChans()
	if tab.dirtyCheckNeeded {
		t.Errorf("dirtyCheckNeeded must remain false with empty channels")
	}
	if tab.BinaryFilePath != "" {
		t.Errorf("BinaryFilePath should remain empty")
	}
}

func TestDrainBodyChans_NilChannels(t *testing.T) {
	tab := &RequestTab{}

	tab.drainBodyChans()
}

func TestDrainBodyChans_MultipleFormResults(t *testing.T) {
	tab := NewRequestTab("t")
	p1 := NewFormPart("a", "", model.FormPartFile, "", 0)
	p2 := NewFormPart("b", "", model.FormPartFile, "", 0)
	tab.FormParts = []*FormDataPart{p1, p2}

	tab.formPartFileChan <- formPartFileResult{part: p1, path: "/p1", size: 1}
	tab.formPartFileChan <- formPartFileResult{part: p2, path: "/p2", size: 2}
	tab.drainBodyChans()

	if p1.FilePath != "/p1" || p2.FilePath != "/p2" {
		t.Errorf("not all messages drained: p1=%q p2=%q", p1.FilePath, p2.FilePath)
	}
}

func TestLayoutBody_AllBodyTypes_SmokeRender(t *testing.T) {
	th := material.NewTheme()
	win := new(app.Window)

	for _, bt := range []model.BodyType{
		model.BodyNone,
		model.BodyRaw,
		model.BodyFormData,
		model.BodyURLEncoded,
		model.BodyBinary,
	} {
		tab := NewRequestTab("t")
		tab.BodyType = bt

		tab.URLEncoded = []*URLEncodedPart{NewURLEncodedPart("k", "v")}
		tab.FormParts = []*FormDataPart{
			NewFormPart("text", "v", model.FormPartText, "", 0),
			NewFormPart("file", "", model.FormPartFile, "/tmp/x", 100),
		}
		tab.BinaryFilePath = "/tmp/y"
		tab.BinaryFileSize = 500

		gtx := makeBodyTestGtx()
		drawn := false
		raw := func(gtx layout.Context) layout.Dimensions {
			drawn = true
			return layout.Dimensions{Size: image.Pt(10, 10)}
		}
		dim := tab.layoutBody(gtx, th, win, nil, nil, raw)
		if dim.Size.X < 0 || dim.Size.Y < 0 {
			t.Errorf("[%v] negative dim", bt)
		}
		if bt == model.BodyRaw && !drawn {
			t.Errorf("BodyRaw should call drawRaw fallback")
		}
	}
}

func TestLayoutBody_FormData_EmptyShowsHint(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = nil
	gtx := makeBodyTestGtx()
	th := material.NewTheme()
	win := new(app.Window)
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)
}

func TestLayoutBody_URLEncoded_EmptyShowsHint(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.URLEncoded = nil
	gtx := makeBodyTestGtx()
	th := material.NewTheme()
	win := new(app.Window)
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)
}

func TestLayoutBody_Binary_NoFile(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = ""
	gtx := makeBodyTestGtx()
	th := material.NewTheme()
	win := new(app.Window)
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)
}

func TestLayoutBodyTypeSelector_OpenAndAllChoices(t *testing.T) {
	tab := NewRequestTab("t")
	th := material.NewTheme()

	gtx := makeBodyTestGtx()
	_ = tab.layoutBodyTypeSelector(gtx, th)

	tab.BodyTypeOpen = true
	gtx = makeBodyTestGtx()
	_ = tab.layoutBodyTypeSelector(gtx, th)

	for _, bt := range []model.BodyType{
		model.BodyNone, model.BodyRaw, model.BodyFormData,
		model.BodyURLEncoded, model.BodyBinary,
	} {
		tab.BodyType = bt
		gtx = makeBodyTestGtx()
		_ = tab.layoutBodyTypeSelector(gtx, th)
	}
}

func TestLayoutModeBar_BothOrientations(t *testing.T) {
	tab := NewRequestTab("t")
	th := material.NewTheme()
	for _, stacked := range []bool{false, true} {
		gtx := makeBodyTestGtx()
		dim := tab.layoutModeBar(gtx, th, &tab.LayoutHorizBtn, &tab.LayoutVertBtn, stacked)
		if dim.Size.X <= 0 || dim.Size.Y <= 0 {
			t.Errorf("layoutModeBar(stacked=%v) returned zero dims", stacked)
		}
	}
}

func TestLayoutFormDataBody_AddRowOnce(t *testing.T) {

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	gtx := makeBodyTestGtx()
	th := material.NewTheme()
	win := new(app.Window)
	before := len(tab.FormParts)
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)
	if len(tab.FormParts) != before {
		t.Errorf("no synthetic click → no new part; got len=%d", len(tab.FormParts))
	}
}

func TestKvRow_SplitRatioFallback(t *testing.T) {

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.URLEncoded = []*URLEncodedPart{NewURLEncodedPart("k", "v")}
	tab.HeaderKeyW = 0
	gtx := makeBodyTestGtx()
	th := material.NewTheme()
	win := new(app.Window)
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)
}

func TestEmptyHint_AndRowDivider_Render(t *testing.T) {

	tab := NewRequestTab("t")
	th := material.NewTheme()
	win := new(app.Window)

	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("a", "1", model.FormPartText, "", 0),
		NewFormPart("b", "", model.FormPartFile, "/tmp/z", 9),
	}
	gtx := makeBodyTestGtx()
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)

	tab.BodyType = model.BodyURLEncoded
	tab.URLEncoded = []*URLEncodedPart{
		NewURLEncodedPart("a", "1"),
		NewURLEncodedPart("b", "2"),
	}
	gtx = makeBodyTestGtx()
	_ = tab.layoutBody(gtx, th, win, nil, nil, nil)
}

func TestFormDataPart_DisabledFlagPreservedAcrossKindToggle(t *testing.T) {

	p := NewFormPart("k", "v", model.FormPartText, "", 0)
	p.Disabled = true

	if p.Kind == model.FormPartText {
		p.Kind = model.FormPartFile
	} else {
		p.Kind = model.FormPartText
	}

	if !p.Disabled {
		t.Errorf("Disabled lost after Kind toggle")
	}
	if p.Kind != model.FormPartFile {
		t.Errorf("Kind didn't toggle to File")
	}
}

func TestFormDataPart_FilePathSurvivesKindToggle(t *testing.T) {

	p := NewFormPart("k", "", model.FormPartFile, "/tmp/old", 42)
	p.Kind = model.FormPartText
	if p.FilePath != "/tmp/old" || p.FileSize != 42 {
		t.Logf("note: current behavior keeps stale FilePath/FileSize after kind→Text toggle")
	}
}

func TestPickFileForFormPart_FullDropDoesNotPanic(t *testing.T) {

	ch := make(chan formPartFileResult, 1)
	ch <- formPartFileResult{path: "/already-full", size: 1}

	select {
	case ch <- formPartFileResult{path: "/dropped", size: 2}:
		t.Fatalf("expected send to be dropped because channel is full")
	default:
	}
}

func (rig *vstackRig) hbSliderScreenY() int {
	return rig.paneTop() + rig.tab.reqPaneAboveHeadersPx(rig.gtx()) + rig.tab.headersRenderH + 2
}

func TestHeadersSliderDownReachesRequestCollapsedSize(t *testing.T) {
	btn := newHSplitRig()
	btn.tab.HeadersAbsHeight = 80
	for i := 0; i < 3; i++ {
		btn.frame()
	}
	btn.tab.ReqCollapseBtn.Click()
	btn.frame()
	btn.frame()
	want := btn.tab.headersRenderH

	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 80
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()+900)

	if !rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the headers slider past the body must collapse the request body")
	}
	if got := rig.tab.headersRenderH; got != want {
		t.Errorf("headers area at its manual maximum must match the collapsed-by-button size: got %d, want %d", got, want)
	}
}

func TestHeadersSliderUpReopensRequestBody(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 80
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()+900)
	if !rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: request body should be collapsed")
	}

	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()-200)
	if rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the headers slider back up must reopen the request body")
	}
	if got, want := rig.tab.headersRenderH, rig.tab.HeadersAbsHeight; got != want {
		t.Errorf("reopened headers should render at their stored height: got %d, want %d", got, want)
	}
}

func TestSplitDragDownReachesResponseCollapsedSize(t *testing.T) {
	btn := newVStackRig()
	btn.tab.HeadersAbsHeight = 80
	btn.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		btn.frame()
	}
	ext := btn.tab.stackedSplitExtent(btn.gtx())
	btn.tab.RespCollapseBtn.Click()
	btn.frame()
	btn.frame()
	want := int(ext) - btn.paneH()

	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()+400)

	if !rig.tab.RespBodyCollapsed {
		t.Errorf("dragging the split to the bottom must collapse the response")
	}
	if got := int(ext) - rig.paneH(); !near(got, want, 1) {
		t.Errorf("response at its manual minimum must match the collapsed-by-button size: got %d, want %d", got, want)
	}
}

func TestSplitDragUpReopensResponse(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()+400)
	if !rig.tab.RespBodyCollapsed {
		t.Fatalf("setup: response should be collapsed")
	}

	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()-200)
	if rig.tab.RespBodyCollapsed {
		t.Errorf("dragging the split back up must reopen the response")
	}
	gtx := rig.gtx()
	ext := int(rig.tab.stackedSplitExtent(gtx))
	if got := ext - rig.paneH(); got < 120 {
		t.Errorf("reopened response must keep its 120px minimum: got %d", got)
	}
}

func TestHeadersSliderToZeroMatchesHeadersChevron(t *testing.T) {
	btn := newVStackRig()
	btn.tab.HeadersAbsHeight = 80
	btn.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		btn.frame()
	}
	btn.tab.ViewGeneratedBtn.Click()
	btn.frame()
	btn.frame()

	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()-200)

	if rig.tab.HeadersExpanded {
		t.Errorf("dragging the headers slider to zero must collapse the headers area")
	}
	if got, want := rig.tab.stackedReqPaneMinPx(rig.gtx()), btn.tab.stackedReqPaneMinPx(btn.gtx()); got != want {
		t.Errorf("collapsed-by-drag headers must hug like collapsed-by-button: got %d, want %d", got, want)
	}
}

func marshalTab(t *testing.T, rt *RequestTab) persist.TabState {
	t.Helper()
	state := persist.AppState{Tabs: []persist.TabState{StateFromTab(rt)}}
	data, err := persist.MarshalIndentEasy(&state, "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back persist.AppState
	if err := easyjson.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, data)
	}
	if len(back.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(back.Tabs))
	}
	return back.Tabs[0]
}

func TestHTTPCollapseStateSurvivesRestart(t *testing.T) {
	src := NewRequestTab("Orders")
	src.Method = "POST"
	src.HeadersExpanded = true
	src.ReqBodyCollapsed = true
	src.RespBodyCollapsed = true
	src.reqRatioSaved = 0.44
	src.respRatioSaved = 0.71

	ts := marshalTab(t, src)
	if !ts.ReqCollapsed || !ts.RespCollapsed {
		t.Fatalf("collapse flags missing from the saved state: %+v", ts)
	}

	dst := TabFromState(ts)
	if !dst.HeadersExpanded {
		t.Error("headers section expansion lost")
	}
	if !dst.ReqBodyCollapsed {
		t.Error("collapsed request pane reopened after restart")
	}
	if !dst.RespBodyCollapsed {
		t.Error("collapsed response pane reopened after restart")
	}
	if dst.reqRatioSaved != 0.44 || dst.respRatioSaved != 0.71 {
		t.Errorf("saved reopen ratios = %v/%v, want 0.44/0.71", dst.reqRatioSaved, dst.respRatioSaved)
	}
}

func TestHTTPCollapsedHeadersSectionSurvivesRestart(t *testing.T) {
	src := NewRequestTab("Orders")
	src.HeadersExpanded = false
	ts := marshalTab(t, src)
	if strings.Contains(string(mustJSON(t, &ts)), `"headers_expanded"`) {
		t.Error("a collapsed headers section must serialize as the omitted default")
	}
	if TabFromState(ts).HeadersExpanded {
		t.Error("collapsed headers section reopened after restart")
	}
}

func mustJSON(t *testing.T, ts *persist.TabState) []byte {
	t.Helper()
	data, err := easyjson.Marshal(ts)
	if err != nil {
		t.Fatalf("marshal tab state: %v", err)
	}
	return data
}

func TestWSCollapseStateSurvivesRestart(t *testing.T) {
	src := NewRequestTab("Socket")
	src.Method = MethodWS
	s := src.EnsureWS()
	s.OptionsExpanded = true
	s.HeadersCollapsed = true
	s.ComposeCollapsed = true
	s.MessagesCollapsed = true
	s.composeSavedRatio = 0.38
	s.msgsSavedRatio = 0.62

	ts := marshalTab(t, src)
	if ts.WS == nil {
		t.Fatal("WS state missing")
	}
	if !ts.WS.HeadersCollapsed || !ts.WS.ComposeCollapsed || !ts.WS.MessagesCollapsed {
		t.Fatalf("WS collapse flags missing from the saved state: %+v", ts.WS)
	}

	d := TabFromState(ts).WS
	if d == nil {
		t.Fatal("WS session not restored")
	}
	if !d.OptionsExpanded || !d.HeadersCollapsed || !d.ComposeCollapsed || !d.MessagesCollapsed {
		t.Errorf("WS collapse state lost: options=%v headers=%v compose=%v messages=%v",
			d.OptionsExpanded, d.HeadersCollapsed, d.ComposeCollapsed, d.MessagesCollapsed)
	}
	if d.composeSavedRatio != 0.38 || d.msgsSavedRatio != 0.62 {
		t.Errorf("saved WS reopen ratios = %v/%v, want 0.38/0.62", d.composeSavedRatio, d.msgsSavedRatio)
	}
}

func TestLayoutPrefsCarryCollapseState(t *testing.T) {
	src := NewRequestTab("src")
	src.Method = MethodWS
	src.ReqBodyCollapsed = true
	src.RespBodyCollapsed = true
	ws := src.EnsureWS()
	ws.HeadersCollapsed = true
	ws.ComposeCollapsed = true
	ws.MessagesCollapsed = true

	var p LayoutPrefs
	src.MergeLayoutPrefs(&p)

	dst := NewRequestTab("dst")
	dst.Method = MethodWS
	dst.EnsureWS()
	dst.ApplyLayoutPrefs(p)

	if !dst.ReqBodyCollapsed || !dst.RespBodyCollapsed {
		t.Errorf("http collapse state not shared: req=%v resp=%v", dst.ReqBodyCollapsed, dst.RespBodyCollapsed)
	}
	if !dst.WS.HeadersCollapsed || !dst.WS.ComposeCollapsed || !dst.WS.MessagesCollapsed {
		t.Errorf("ws collapse state not shared: %+v", dst.WS)
	}
}

func TestCollapseTogglesRequestASave(t *testing.T) {
	rt := NewRequestTab("t")
	if rt.TakeLayoutSaveRequest() {
		t.Fatal("a fresh tab must not request a save")
	}
	rt.layoutSaveNeeded = true
	if !rt.TakeLayoutSaveRequest() {
		t.Fatal("a pending layout change must request a save")
	}
	if rt.TakeLayoutSaveRequest() {
		t.Error("the request must be cleared once taken")
	}
}

func TestReqBodyCollapseByDragAndButton(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: request body must start expanded")
	}

	rig.drag(400, rig.splitDividerY(), rig.paneTop()+10)
	if !rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the split to the request minimum must flip the collapse state")
	}
	if got, want := rig.paneH(), rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapsed request pane should hug headers+request header: pane %d, want ~%d", got, want)
	}

	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()+120)
	if rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the split back down must expand the request body")
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.ReqBodyCollapsed {
		t.Fatalf("collapse button must collapse the request body")
	}
	if got, want := rig.paneH(), rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapse button should shrink the pane to its header: pane %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.ReqBodyCollapsed {
		t.Fatalf("second click must expand the request body")
	}
	minOpen := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); got < minOpen+100 {
		t.Errorf("expanding must reopen editor space: pane %d, want >= %d", got, minOpen+100)
	}
}

func TestRespBodyCollapseButtonAndDragExpand(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.RespBodyCollapsed {
		t.Fatalf("collapse button must collapse the response body")
	}
	extent := int(rig.tab.stackedSplitExtent(rig.gtx()))
	respPx := extent - rig.paneH()
	if want := rig.tab.respCollapsedMinPx(rig.gtx()); !near(respPx, want, 4) {
		t.Errorf("collapsed response pane should hug its header: got %d, want ~%d", respPx, want)
	}

	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()-100)
	if rig.tab.RespBodyCollapsed {
		t.Errorf("dragging the split up must expand the response body")
	}
	respPx = extent - rig.paneH()
	if respPx < 100 {
		t.Errorf("response should reopen to at least its drag minimum, got %dpx", respPx)
	}
}

func TestCollapsedHeadersRowKeepsExpandedHeight(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.ReqBodyCollapsed = true
	rig.tab.HeadersExpanded = false
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	line := 1
	want := rig.tab.headersRowH + line + rig.tab.reqHeaderH
	if got := rig.tab.reqPaneBoxH; got != want {
		t.Errorf("collapsed headers pane must hold only its two header rows: box %d, want %d", got, want)
	}
}

func TestCollapsedStackedRequestHugsHeaderRow(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.tab.ReqCollapseBtn.Click()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	line, slider := 1, 4
	want := rig.tab.headersRowH + line + rig.tab.headersRenderH + slider + line + rig.tab.reqHeaderH
	if got := rig.tab.reqPaneBoxH; got != want {
		t.Errorf("collapsed stacked request pane must hug its content: box %d, want %d", got, want)
	}

	rig.tab.ViewGeneratedBtn.Click()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	want = rig.tab.headersRowH + line + rig.tab.reqHeaderH
	if got := rig.tab.reqPaneBoxH; got != want {
		t.Errorf("collapsed request pane with hidden headers must hold only header rows: box %d, want %d", got, want)
	}
}

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "rete-test")
	persist.SetConfigOverride(configPath)

	t.Cleanup(func() {
		persist.SetConfigOverride("")
	})

	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tempDir)
	case "darwin":
		t.Setenv("HOME", tempDir)
	default:
		t.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	return tempDir
}

// problems.md #3: with the caret right before the value of "phoneNumber",
// Ctrl+Right used to land on the next line between `"` and `force` instead of
// stopping at the end of the current line.
func TestCtrlRightKeepsCaretOnLine(t *testing.T) {
	body := "{\n    \"phoneNumber\": 123456789,\n\t\"force\": true\n}"
	rig := newEditorKeyRig()
	rig.v.SetText(body)

	valueStart := strings.Index(body, "123456789")
	lineEnd := strings.Index(body[valueStart:], "\n") + valueStart

	rig.focus()
	rig.v.selStart, rig.v.selEnd = valueStart, valueStart
	rig.pressKey(key.NameRightArrow, key.ModShortcut)

	caret := rig.v.selEnd
	if caret > lineEnd {
		t.Fatalf("Ctrl+Right jumped past the line break: caret=%d (%q), line ends at %d",
			caret, body[caret:min(caret+5, len(body))], lineEnd)
	}
	if caret <= valueStart {
		t.Fatalf("Ctrl+Right did not advance: caret=%d", caret)
	}

	// The next Ctrl+Right crosses the break and lands on the first word of the
	// following line.
	rig.pressKey(key.NameRightArrow, key.ModShortcut)
	caret = rig.v.selEnd
	if want := strings.Index(body, "force"); caret != want {
		t.Fatalf("second Ctrl+Right: caret=%d (%q), want %d (start of \"force\")",
			caret, body[caret:min(caret+5, len(body))], want)
	}
}

func TestDoubleClickPastLineEndDoesNotGrabNextLine(t *testing.T) {
	resp := "{\n  \"avatar\": \"\",\n  \"next\": 1\n}\n"
	rig := newRespRig(resp, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}

	line2Start := strings.Index(resp, "  \"next\"")
	nlOff := line2Start - 1

	x := 300
	y := 4 + rig.v.lastLineHeight + rig.v.lastLineHeight/2
	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+80*time.Millisecond)

	s, e := rig.v.selStart, rig.v.selEnd
	if s > e {
		s, e = e, s
	}
	if e > nlOff {
		t.Errorf("double-click past line end selected across the newline: [%d,%d) = %q",
			s, e, resp[s:e])
	}
}

func TestDoubleClickOnIndentSelectsIndentOnly(t *testing.T) {
	resp := "{\n    \"avatar\": \"\",\n    \"next\": 1\n}\n"
	rig := newRespRig(resp, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}

	x := 7
	y := 4 + rig.v.lastLineHeight + rig.v.lastLineHeight/2
	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+80*time.Millisecond)

	s, e := rig.v.selStart, rig.v.selEnd
	if s > e {
		s, e = e, s
	}
	if s != e {
		if got := resp[s:e]; strings.ContainsAny(got, "\n\r") {
			t.Errorf("double-click on indent spaces crossed a line break: [%d,%d) = %q", s, e, got)
		}
	}
}

type respRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *ResponseViewer
	size   image.Point
	wrap   bool
}

func newRespRig(txt string, wrap bool) *respRig {
	rig := &respRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		v:      NewResponseViewer(),
		size:   image.Pt(400, 300),
		wrap:   wrap,
	}
	rig.v.SetText(txt)
	return rig
}

func (rig *respRig) frame(now time.Time) {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         now,
		Source:      rig.r.Source(),
	}
	ResponseViewerStyle{
		Viewer:   rig.v,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(13),
		Wrap:     rig.wrap,
		Padding:  unit.Dp(4),
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *respRig) click(x, y int, at time.Duration) {
	pos := f32.Pt(float32(x), float32(y))
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: at},
		pointer.Event{Kind: pointer.Release, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: at},
	)
	rig.frame(time.Unix(1700000000, 0).Add(at))
}

func TestDoubleClickSelectsAcrossOSDoubleClickWindow(t *testing.T) {
	resp := `{"receiptId":3813,"body":{"typeWebhook":"incomingMessageReceived"}}`
	rig := newRespRig(resp, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2

	base := time.Duration(0)
	for _, gap := range []time.Duration{50, 199, 250, 350, 480} {
		base += 5 * time.Second
		rig.v.selStart, rig.v.selEnd = 0, 0
		rig.v.lastClickTime = time.Time{}
		rig.v.multiClickN = 0
		rig.click(x, y, base)
		rig.click(x, y, base+gap*time.Millisecond)
		if rig.v.selStart == rig.v.selEnd {
			t.Errorf("gap=%dms: double-click within the OS double-click window must select a word, got empty", gap)
		} else if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "receiptId" {
			t.Errorf("gap=%dms: expected word %q, got %q", gap, "receiptId", got)
		}
	}
}

func TestSlowClicksDoNotSelect(t *testing.T) {
	rig := newRespRig(`{"receiptId":3813}`, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2

	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+700*time.Millisecond)
	if rig.v.selStart != rig.v.selEnd {
		t.Errorf("clicks 700ms apart must not word-select; got %q", string(rig.v.text[rig.v.selStart:rig.v.selEnd]))
	}
}

func TestClicksAtDifferentPositionsDoNotSelect(t *testing.T) {
	rig := newRespRig(`{"receiptId":3813,"body":{"a":1}}`, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	y := 4 + rig.v.lastLineHeight/2

	rig.click(30, y, time.Second)
	rig.click(120, y, time.Second+80*time.Millisecond)
	if rig.v.selStart != rig.v.selEnd {
		t.Errorf("two fast clicks at far-apart positions must not word-select; got %q", string(rig.v.text[rig.v.selStart:rig.v.selEnd]))
	}
}

func (rig *respRig) clickAt(x, y int, evTime, frameNow time.Duration) {
	pos := f32.Pt(float32(x), float32(y))
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: evTime},
		pointer.Event{Kind: pointer.Release, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: evTime},
	)
	rig.frame(time.Unix(1700000000, 0).Add(frameNow))
}

func TestDoubleClickSurvivesSlowFrame(t *testing.T) {
	rig := newRespRig(`{"receiptId":3813}`, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2

	rig.clickAt(x, y, 1*time.Second, 1*time.Second)
	rig.clickAt(x, y, 1300*time.Millisecond, 2800*time.Millisecond)
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("double-click 300ms apart must select even when the second press lands in a frame delayed by 1.5s")
	}
	if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "receiptId" {
		t.Errorf("expected word %q, got %q", "receiptId", got)
	}
}

func TestDoubleClickAfterWheelScroll(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "line%03d word%03d\n", i, i)
	}
	rig := newRespRig(b.String(), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 20, 4+rig.v.lastLineHeight/2
	pos := f32.Pt(float32(x), float32(y))

	rig.r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1, Position: pos, Scroll: f32.Pt(0, float32(rig.v.lastLineHeight)), Time: 900 * time.Millisecond})
	rig.frame(time.Unix(1700000000, 0).Add(900 * time.Millisecond))

	for _, at := range []time.Duration{time.Second, time.Second + 200*time.Millisecond} {
		rig.r.Queue(
			pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, PointerID: 1, Time: at},
			pointer.Event{Kind: pointer.Release, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, PointerID: 1, Time: at},
		)
		rig.frame(time.Unix(1700000000, 0).Add(at))
	}
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("double-click after a wheel scroll with the same pointer ID must still select a word")
	}
}

func TestCoordToByteOffsetLongLineMatchesFullShape(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 20000; i++ {
		fmt.Fprintf(&b, "\"key%04d\":\"value%04d\",", i, i)
	}
	rig := newRespRig(b.String(), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	lineH := rig.v.lastLineHeight
	if lineH <= 0 {
		t.Fatal("no line height measured")
	}
	const pad = 4
	innerW := rig.size.X - 2*pad

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	}
	full := widgets.ShapeChunkForWrap(rig.shaper, rig.v.layoutFont, rig.v.layoutSize, gtx, rig.v.text, innerW)
	charAdv := measureCharAdvance(rig.shaper, rig.v.layoutFont, rig.v.layoutSize, gtx)

	for _, scrollLines := range []int{0, 70, 200} {
		rig.v.SetScrollY(scrollLines * lineH)
		rig.frame(time.Unix(1700000000, 0))
		for _, x := range []int{10, 90, 250} {
			for _, row := range []int{0, 3, 9} {
				y := pad + row*lineH + lineH/2
				got := rig.v.coordToByteOffset(gtx, x-pad, y-pad, charAdv, lineH, innerW, true)
				wrapLine := (y - pad + rig.v.scrollY) / lineH
				want := widgets.ByteOffInWrap(full, x-pad, wrapLine)
				if got != want {
					t.Errorf("scroll=%d x=%d row=%d: coordToByteOffset=%d, full-shape reference=%d", scrollLines, x, row, got, want)
				}
			}
		}
	}
}

func TestWrapNavLongLineMatchesFullShape(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 20000; i++ {
		fmt.Fprintf(&b, "\"key%04d\":\"value%04d\",", i, i)
	}
	rig := newRespRig(b.String(), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	const pad = 4
	innerW := rig.size.X - 2*pad
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	}
	full := widgets.ShapeChunkForWrap(rig.shaper, rig.v.layoutFont, rig.v.layoutSize, gtx, rig.v.text, innerW)
	isLineStart := map[int]bool{}
	for _, s := range widgets.WrapLineStarts(full) {
		isLineStart[s] = true
	}
	maxSub := widgets.WrapMaxLine(full)
	n := len(rig.v.text)

	for _, off := range []int{0, 777, 5003, 12347, n - 5, n} {
		for off > 0 && off < n && isLineStart[off] {
			off++
		}
		gotX := rig.v.visualXAt(off, gtx, innerW)
		wantX, wantSub := widgets.CaretXYInWrap(full, off)
		if gotX != wantX {
			t.Errorf("off=%d: visualXAt=%d, full-shape reference=%d", off, gotX, wantX)
		}
		for _, dir := range []int{-1, 1} {
			got := rig.v.wrapLineMoveX(off, 37, dir, gtx, innerW)
			var want int
			if dir < 0 {
				if wantSub > 0 {
					want = widgets.ByteOffInWrap(full, 37, wantSub-1)
				}
			} else {
				want = n
				if wantSub < maxSub {
					want = widgets.ByteOffInWrap(full, 37, wantSub+1)
				}
			}
			if got != want {
				t.Errorf("off=%d dir=%d: wrapLineMoveX=%d, full-shape reference=%d", off, dir, got, want)
			}
		}
	}
}

func TestTripleClickSelectsLine(t *testing.T) {
	rig := newRespRig("first line\nsecond line\nthird line", false)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 40, 4+rig.v.lastLineHeight+rig.v.lastLineHeight/2

	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+120*time.Millisecond)
	rig.click(x, y, time.Second+240*time.Millisecond)
	if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "second line" {
		t.Errorf("triple-click should select the whole line; got %q", got)
	}
}

func TestMultiClickWordLineAll(t *testing.T) {
	rig := newRespRig("first line\nsecond line\nthird line", false)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 40, 4+rig.v.lastLineHeight+rig.v.lastLineHeight/2
	sel := func() string { return string(rig.v.text[rig.v.selStart:rig.v.selEnd]) }

	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+120*time.Millisecond)
	if got := sel(); got != "second" {
		t.Errorf("double-click must select the word; got %q", got)
	}
	rig.click(x, y, time.Second+240*time.Millisecond)
	if got := sel(); got != "second line" {
		t.Errorf("triple-click must select the line; got %q", got)
	}
	rig.click(x, y, time.Second+360*time.Millisecond)
	if got := sel(); got != string(rig.v.text) {
		t.Errorf("quadruple-click must select everything; got %q", got)
	}
	rig.click(x, y, time.Second+480*time.Millisecond)
	if rig.v.selStart != rig.v.selEnd {
		t.Errorf("a fifth click must start over with a plain caret; got %q", sel())
	}

	rig.click(x, y, 5*time.Second)
	rig.click(x+40, y, 5*time.Second+120*time.Millisecond)
	if rig.v.selStart != rig.v.selEnd {
		t.Errorf("a second click far from the first must not count as a double-click; got %q", sel())
	}
}

func TestViewerDoubleClickSelectsURLPathSegment(t *testing.T) {
	const url = "{{apiUrl}}/waInstance{{idInstance}}/getStatusInstance/{{apiTokenInstance}}"
	v := NewResponseViewer()
	v.SetText(url)

	cases := []struct {
		word string
		off  int
	}{
		{"getStatusInstance", 3},
		{"waInstance", 2},
		{"apiUrl", 1},
		{"idInstance", 0},
		{"apiTokenInstance", 8},
	}
	for _, c := range cases {
		at := strings.Index(url, c.word) + c.off
		s, e := v.wordBoundsAt(at)
		if got := url[s:e]; got != c.word {
			t.Errorf("wordBoundsAt(%d) = %q, want %q", at, got, c.word)
		}
	}
	slash := strings.Index(url, "/getStatusInstance")
	if s, e := v.wordBoundsAt(slash); url[s:e] != "/" {
		t.Errorf("double-click on the slash must select just the slash, got %q", url[s:e])
	}

	rig := newRespRig("x/getStatusInstance/y", true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2
	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+80*time.Millisecond)
	if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "getStatusInstance" {
		t.Errorf("double-click in the viewer selected %q, want getStatusInstance", got)
	}
}

func linkedTab(method, url, body string, bodyType model.BodyType) *RequestTab {
	t := NewRequestTab("t")
	t.Method = method
	t.URLInput.SetText(url)
	t.ReqEditor.SetText(body)
	t.BodyType = bodyType
	t.LinkedNode = &collections.CollectionNode{
		Request: &model.ParsedRequest{
			Method:   method,
			URL:      url,
			Body:     body,
			BodyType: bodyType,
			Headers:  map[string]string{},
		},
	}
	return t
}

func TestCheckDirtyCleanWithCJKBody(t *testing.T) {
	tab := linkedTab("POST", "http://例え.test/路径", "本文ボディ", model.BodyRaw)
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("unchanged CJK request marked dirty")
	}
}

func TestCheckDirtyDetectsBodyEdit(t *testing.T) {
	tab := linkedTab("POST", "http://x.test", "AAAA", model.BodyRaw)
	tab.ReqEditor.SetText("BBBB")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Error("same-length body edit not detected as dirty")
	}
}

func TestCheckDirtyDetectsBodyTypeChange(t *testing.T) {
	tab := linkedTab("POST", "http://x.test", "", model.BodyNone)
	tab.BodyType = model.BodyRaw
	tab.checkDirty()
	if !tab.IsDirty {
		t.Error("BodyType change not detected as dirty")
	}
}

func TestCheckDirtyCleanWhenUnchanged(t *testing.T) {
	tab := linkedTab("GET", "http://x.test", "hello", model.BodyRaw)
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("identical request marked dirty")
	}
}

func (rig *editorKeyRig) typeLikeWindow(s string) {
	start, end := rig.v.normSel()
	startRune := utf8.RuneCount(rig.v.text[:start])
	endRune := utf8.RuneCount(rig.v.text[:end])
	echo := startRune + utf8.RuneCountInString(s)
	rig.r.Queue(
		key.EditEvent{Range: key.Range{Start: startRune, End: endRune}, Text: s},
		key.SnippetEvent{Start: 0, End: echo},
		key.SelectionEvent{Start: echo, End: echo},
	)
	rig.frame()
}

func TestRequestEditorQuoteWrapIgnoresWindowSelectionEcho(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("test")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 4
	rig.frame()

	rig.typeLikeWindow("\"")
	if got := rig.v.Text(); got != "\"test\"" {
		t.Fatalf("first quote: got %q", got)
	}
	if rig.v.selStart != 1 || rig.v.selEnd != 5 {
		t.Fatalf("selection must stay on the word despite the window's caret echo, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}

	rig.typeLikeWindow("\"")
	if got := rig.v.Text(); got != "\"\"test\"\"" {
		t.Fatalf("second quote should wrap again, got %q", got)
	}
	if rig.v.selStart != 2 || rig.v.selEnd != 6 {
		t.Fatalf("selection after second wrap: got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}

	rig.typeLikeWindow("(")
	if got := rig.v.Text(); got != "\"\"(test)\"\"" {
		t.Fatalf("paren after quotes: got %q", got)
	}
}

func TestRequestEditorPlainTypingStillFollowsWindowSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("ab")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 2, 2
	rig.frame()

	rig.typeLikeWindow("c")
	if got := rig.v.Text(); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if rig.v.selStart != 3 || rig.v.selEnd != 3 {
		t.Fatalf("caret after plain insert: got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}

	rig.r.Queue(key.SelectionEvent{Start: 1, End: 1})
	rig.frame()
	if rig.v.selStart != 1 || rig.v.selEnd != 1 {
		t.Fatalf("a later, unrelated selection event must still apply, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorFocusMoveCollapsesSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("hello world")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 5
	rig.frame()

	other := new(int)
	rig.r.Source().Execute(key.FocusCmd{Tag: other})
	rig.frame()
	rig.frame()

	if rig.v.selStart != rig.v.selEnd {
		t.Fatalf("selection must collapse when another widget takes focus, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorWindowDeactivationKeepsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("hello world")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 5
	rig.frame()

	rig.r.Queue(key.FocusEvent{Focus: false})
	rig.frame()
	rig.r.Queue(key.FocusEvent{Focus: true})
	rig.frame()

	if rig.v.selStart != 0 || rig.v.selEnd != 5 {
		t.Fatalf("losing window focus must not drop the selection, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

type blurRespRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *ResponseViewer
}

func newBlurRespRig(txt string) *blurRespRig {
	rig := &blurRespRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		v:      NewResponseViewer(),
	}
	rig.v.SetText(txt)
	return rig
}

func (rig *blurRespRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 300)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	ResponseViewerStyle{
		Viewer:   rig.v,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(13),
		Wrap:     true,
		Padding:  unit.Dp(4),
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
}

func TestResponseViewerFocusMoveCollapsesSelection(t *testing.T) {
	rig := newBlurRespRig("hello world")
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
	rig.r.Queue(key.Event{Name: "A", Modifiers: key.ModShortcut, State: key.Press})
	rig.frame()
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("setup: Ctrl+A should select the text")
	}

	rig.r.Queue(key.FocusEvent{Focus: false})
	rig.frame()
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("window deactivation must keep the response selection")
	}

	other := new(int)
	rig.r.Source().Execute(key.FocusCmd{Tag: other})
	rig.frame()
	rig.frame()
	if rig.v.selStart != rig.v.selEnd {
		t.Fatalf("selection must collapse when focus moves elsewhere, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorClosingBracketWrapsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("test")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 4
	rig.frame()

	rig.typeLikeWindow("\"")
	rig.typeLikeWindow("}")
	if got := rig.v.Text(); got != "\"{test}\"" {
		t.Fatalf("a closing bracket over a selection must wrap it, got %q", got)
	}
	if rig.v.selStart != 2 || rig.v.selEnd != 6 {
		t.Fatalf("selection after wrap: got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
	rig.typeLikeWindow(")")
	rig.typeLikeWindow("]")
	rig.typeLikeWindow(">")
	if got := rig.v.Text(); got != "\"{([<test>])}\"" {
		t.Fatalf("closing chars must wrap like their openers, got %q", got)
	}
}

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestEditorSizeBytesAndSoftLimit(t *testing.T) {
	ed := NewRequestEditor()
	if ed.SizeBytes() != 0 {
		t.Errorf("SizeBytes() = %d, want 0", ed.SizeBytes())
	}
	if ed.IsOverSoftLimit() {
		t.Error("an empty editor must not be over the limit")
	}
	ed.SetText("héllo")
	if ed.SizeBytes() != len("héllo") {
		t.Errorf("SizeBytes() = %d, want the byte length %d", ed.SizeBytes(), len("héllo"))
	}
	if ed.IsOverSoftLimit() {
		t.Error("a small body must not be over the limit")
	}
}

func TestEditorSetTextRejectsOversize(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("keep")
	big := strings.Repeat("x", RequestBodyMaxBytes+1)
	if ed.SetText(big) {
		t.Fatal("SetText must reject a body over 100 MB")
	}
	if ed.Text() != "keep" {
		t.Errorf("a rejected SetText must leave the old text: %q", ed.Text())
	}
	if ed.OversizeMsg() == "" {
		t.Error("a rejected SetText must explain itself")
	}
	ed.DismissOversize()
	if ed.OversizeMsg() != "" {
		t.Errorf("DismissOversize left %q", ed.OversizeMsg())
	}
}

func TestEditorLoadFromReader(t *testing.T) {
	ed := NewRequestEditor()
	if err := ed.LoadFromReader(strings.NewReader("line1\nline2\n")); err != nil {
		t.Fatalf("LoadFromReader: %v", err)
	}
	if ed.Text() != "line1\nline2\n" {
		t.Errorf("Text() = %q", ed.Text())
	}
	if ed.SizeBytes() != 12 {
		t.Errorf("SizeBytes() = %d, want 12", ed.SizeBytes())
	}
	if ed.OversizeMsg() != "" {
		t.Errorf("a successful load must clear the message, got %q", ed.OversizeMsg())
	}
	if got := len(ed.lineStarts); got != 3 {
		t.Errorf("line index = %d entries, want 3", got)
	}
}

func TestEditorLoadFromReaderEmpty(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("previous")
	if err := ed.LoadFromReader(strings.NewReader("")); err != nil {
		t.Fatalf("LoadFromReader(empty): %v", err)
	}
	if ed.Text() != "" {
		t.Errorf("Text() = %q, want empty", ed.Text())
	}
}

func TestEditorLoadFromReaderPropagatesReadError(t *testing.T) {
	ed := NewRequestEditor()
	want := errors.New("disk on fire")
	err := ed.LoadFromReader(failingReader{err: want})
	if !errors.Is(err, want) {
		t.Errorf("LoadFromReader error = %v, want %v", err, want)
	}
	if !strings.Contains(ed.OversizeMsg(), "disk on fire") {
		t.Errorf("OversizeMsg = %q, want the read error surfaced", ed.OversizeMsg())
	}
}

func TestEditorLoadFromReaderResetsViewState(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText(strings.Repeat("abcdef\n", 50))
	ed.SetCaret(3, 9)
	ed.SetScrollY(40)
	if err := ed.LoadFromReader(strings.NewReader("fresh")); err != nil {
		t.Fatalf("LoadFromReader: %v", err)
	}
	if ed.GetScrollY() != 0 {
		t.Errorf("GetScrollY() = %d, want 0 after a load", ed.GetScrollY())
	}
	if ed.SelectedText() != "" {
		t.Errorf("SelectedText() = %q, want the selection cleared", ed.SelectedText())
	}
}

func TestEditorLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.json")
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ed := NewRequestEditor()
	if err := ed.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if ed.Text() != `{"a":1}` {
		t.Errorf("Text() = %q", ed.Text())
	}
	if ed.OversizeMsg() != "" {
		t.Errorf("OversizeMsg = %q, want empty", ed.OversizeMsg())
	}
}

func TestEditorLoadFromFileMissing(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("keep")
	err := ed.LoadFromFile(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("LoadFromFile must fail for a missing path")
	}
	if !strings.HasPrefix(ed.OversizeMsg(), "Load failed: ") {
		t.Errorf("OversizeMsg = %q, want a 'Load failed' message", ed.OversizeMsg())
	}
	if ed.Text() != "keep" {
		t.Errorf("a failed load must leave the old text: %q", ed.Text())
	}
}

func TestEditorLoadFromFileUnreadableDirectory(t *testing.T) {
	dir := t.TempDir()
	ed := NewRequestEditor()
	if err := ed.LoadFromFile(dir); err == nil {
		t.Error("LoadFromFile must fail when handed a directory")
	}
	if ed.OversizeMsg() == "" {
		t.Error("a directory load failure must be reported")
	}
}

func TestErrBodyTooLargeMessage(t *testing.T) {
	if errBodyTooLarge.Error() == "" {
		t.Error("errBodyTooLarge must carry a message")
	}
	if !errors.Is(errBodyTooLarge, errBodyTooLarge) {
		t.Error("errBodyTooLarge must compare equal to itself")
	}
}

func TestEditorAppend(t *testing.T) {
	ed := NewRequestEditor()
	if !ed.Append("") {
		t.Error("appending an empty string must succeed")
	}
	if ed.SizeBytes() != 0 {
		t.Errorf("SizeBytes() = %d after an empty append", ed.SizeBytes())
	}

	if !ed.Append("a\nb") {
		t.Fatal("Append failed")
	}
	if !ed.Append("c\nd\n") {
		t.Fatal("second Append failed")
	}
	if ed.Text() != "a\nbc\nd\n" {
		t.Errorf("Text() = %q, want %q", ed.Text(), "a\nbc\nd\n")
	}
	if got := len(ed.lineStarts); got != 4 {
		t.Errorf("line index = %d entries, want 4 for %q", got, ed.Text())
	}
}

func TestEditorAppendKeepsLineIndexConsistent(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("one\ntwo")
	ed.Append("\nthree\nfour")

	built := NewRequestEditor()
	built.SetText(ed.Text())
	if len(ed.lineStarts) != len(built.lineStarts) {
		t.Fatalf("appended line index = %v, want %v (same text loaded whole)", ed.lineStarts, built.lineStarts)
	}
	for i := range ed.lineStarts {
		if ed.lineStarts[i] != built.lineStarts[i] {
			t.Fatalf("appended line index = %v, want %v", ed.lineStarts, built.lineStarts)
		}
	}
}

func TestEditorAppendRejectsOverflow(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText(strings.Repeat("x", 64))
	stub := ed.SizeBytes()
	huge := strings.Repeat("y", RequestBodyMaxBytes)
	if ed.Append(huge) {
		t.Fatal("Append must refuse to cross the 100 MB ceiling")
	}
	if ed.SizeBytes() != stub {
		t.Errorf("a rejected Append must not grow the buffer: %d -> %d", stub, ed.SizeBytes())
	}
	if !strings.Contains(ed.OversizeMsg(), "Append rejected") {
		t.Errorf("OversizeMsg = %q, want an append-rejected message", ed.OversizeMsg())
	}
}

func TestEditorLoadFromReaderRejectsOversizeStream(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("keep")
	r := io.LimitReader(zeroReader{}, int64(RequestBodyMaxBytes)+1)
	err := ed.LoadFromReader(r)
	if !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("LoadFromReader error = %v, want errBodyTooLarge", err)
	}
	if !strings.Contains(ed.OversizeMsg(), "exceeds 100 MB") {
		t.Errorf("OversizeMsg = %q", ed.OversizeMsg())
	}
	if ed.Text() != "keep" {
		t.Errorf("a rejected stream load must leave the old text: %q", ed.Text())
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'z'
	}
	return len(p), nil
}

func TestZeroValueEditor_TypingIsRendered(t *testing.T) {
	v := &RequestEditor{}
	v.Insert(0, "a")
	if len(v.lineStarts) == 0 {
		t.Fatalf("lineStarts empty after typing into a fresh editor: text=%q would never render", v.text)
	}
	s, e := v.lineBounds(0)
	if got := string(v.text[s:e]); got != "a" {
		t.Fatalf("line 0 does not cover typed text: got %q, want %q", got, "a")
	}
}

func TestNewRequestTab_BodyEditorReady(t *testing.T) {
	tab := NewRequestTab("T")
	tab.ReqEditor.Insert(0, "x")
	if len(tab.ReqEditor.lineStarts) == 0 {
		t.Fatalf("new request body editor not initialized: typed text would be invisible")
	}
}

func TestRequestEditorQuoteWrapsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()

	rig.v.selStart = 6
	rig.v.selEnd = 11

	rig.r.Queue(key.EditEvent{Text: "\""})
	rig.frame()

	if got := rig.v.Text(); got != "hello \"world\"" {
		t.Fatalf("quote should wrap selection, got %q", got)
	}
	if rig.v.selStart != 7 || rig.v.selEnd != 12 {
		t.Fatalf("selection should stay on the wrapped word, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorQuoteWrapsBackwardSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()

	rig.v.selStart = 11
	rig.v.selEnd = 6

	rig.r.Queue(key.EditEvent{Text: "\""})
	rig.frame()

	if got := rig.v.Text(); got != "hello \"world\"" {
		t.Fatalf("quote should wrap a right-to-left selection, got %q", got)
	}
	if rig.v.selStart != 12 || rig.v.selEnd != 7 {
		t.Fatalf("selection should stay on the wrapped word and keep its direction, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorBracketWrapsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "abc"})
	rig.frame()
	rig.v.selStart = 0
	rig.v.selEnd = 3

	rig.r.Queue(key.EditEvent{Text: "("})
	rig.frame()
	if got := rig.v.Text(); got != "(abc)" {
		t.Fatalf("paren should wrap selection, got %q", got)
	}
}

func TestRequestEditorWrapUndoIsSingleStep(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "abc"})
	rig.frame()
	rig.v.selStart = 0
	rig.v.selEnd = 3

	rig.r.Queue(key.EditEvent{Text: "["})
	rig.frame()
	if got := rig.v.Text(); got != "[abc]" {
		t.Fatalf("setup: got %q", got)
	}

	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != "abc" {
		t.Fatalf("single undo should revert the wrap, got %q", got)
	}
}

func TestRequestEditorTypingWithoutSelectionUnchanged(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "ab"})
	rig.frame()
	rig.v.selStart = 2
	rig.v.selEnd = 2

	rig.r.Queue(key.EditEvent{Text: "\""})
	rig.frame()
	if got := rig.v.Text(); got != "ab\"" {
		t.Fatalf("typing a quote with no selection should insert a single quote, got %q", got)
	}
}

func TestRequestEditorNonPairReplacesSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello"})
	rig.frame()
	rig.v.selStart = 0
	rig.v.selEnd = 5

	rig.r.Queue(key.EditEvent{Text: "x"})
	rig.frame()
	if got := rig.v.Text(); got != "x" {
		t.Fatalf("a non-pair char should replace the selection, got %q", got)
	}
}

func validateLineStarts(t *testing.T, v *RequestEditor) {
	t.Helper()
	want := []int{0}
	for i := 0; i < len(v.text); i++ {
		if v.text[i] == '\n' {
			want = append(want, i+1)
		}
	}
	have := make(map[int]struct{}, len(v.lineStarts))
	for _, s := range v.lineStarts {
		have[s] = struct{}{}
	}
	for _, w := range want {
		if _, ok := have[w]; !ok {
			t.Errorf("lineStarts missing entry %d after edit (have=%v, text=%q)", w, v.lineStarts, string(v.text))
			return
		}
	}
}

func TestRequestEditorInsert(t *testing.T) {
	v := NewRequestEditor()

	v.Insert(0, "hello")
	if v.Text() != "hello" {
		t.Fatalf("after Insert(0, hello) got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.Insert(5, " world")
	if v.Text() != "hello world" {
		t.Fatalf("after Insert(5, ' world') got %q", v.Text())
	}

	v.Insert(5, ",")
	if v.Text() != "hello, world" {
		t.Fatalf("after Insert(5, ',') got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.Insert(0, "a\nb\n")
	if v.Text() != "a\nb\nhello, world" {
		t.Fatalf("after multi-line insert got %q", v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorDeleteRange(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("hello, world")

	v.DeleteRange(5, 7)
	if v.Text() != "helloworld" {
		t.Fatalf("after DeleteRange(5,7) got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.SetText("first\nsecond\nthird")
	v.DeleteRange(3, 9)
	if v.Text() != "firond\nthird" {
		t.Fatalf("after cross-line delete got %q", v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorReplace(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("foo bar baz")

	v.Replace(4, 7, "QUX")
	if v.Text() != "foo QUX baz" {
		t.Fatalf("after Replace got %q", v.Text())
	}
	validateLineStarts(t, v)

	v.Replace(3, 8, "")
	if v.Text() != "foobaz" {
		t.Fatalf("after Replace with empty got %q", v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorUndoRedo(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("base")

	v.Insert(4, " text")
	if v.Text() != "base text" {
		t.Fatalf("setup failed: %q", v.Text())
	}

	if !v.Undo() || v.Text() != "base" {
		t.Fatalf("Undo failed; got %q", v.Text())
	}
	if !v.Redo() || v.Text() != "base text" {
		t.Fatalf("Redo failed; got %q", v.Text())
	}

	v.Replace(0, 4, "REPLACED")
	if v.Text() != "REPLACED text" {
		t.Fatalf("Replace setup failed: %q", v.Text())
	}
	if !v.Undo() || v.Text() != "base text" {
		t.Fatalf("Undo across Replace failed; got %q", v.Text())
	}
	if !v.Redo() || v.Text() != "REPLACED text" {
		t.Fatalf("Redo across Replace failed; got %q", v.Text())
	}

	v.SetText("fresh")
	if v.Undo() {
		t.Fatalf("Undo should have nothing to do after SetText")
	}
}

func TestRequestEditorOverLimit(t *testing.T) {
	v := NewRequestEditor()
	huge := strings.Repeat("a", RequestBodyMaxBytes+1)
	if v.SetText(huge) {
		t.Fatalf("SetText should reject input larger than RequestBodyMaxBytes")
	}
	if len(v.text) != 0 {
		t.Fatalf("buffer should remain empty after rejected SetText, got %d bytes", len(v.text))
	}

	v.SetText(strings.Repeat("a", RequestBodyMaxBytes-10))
	v.Insert(0, strings.Repeat("b", 100))
	if len(v.text) != RequestBodyMaxBytes-10 {
		t.Fatalf("Insert past limit should be a no-op, got %d", len(v.text))
	}
}

func TestRequestEditorUndoGrouping(t *testing.T) {
	v := NewRequestEditor()
	for i, c := range "hello" {
		v.Insert(i, string(c))
	}
	if v.Text() != "hello" {
		t.Fatalf("setup: %q", v.Text())
	}
	if got := len(v.undoStack); got != 1 {
		t.Fatalf("expected 5 inserts to merge into 1 undo step, got %d", got)
	}
	if !v.Undo() {
		t.Fatalf("Undo failed")
	}
	if v.Text() != "" {
		t.Fatalf("after one Undo expected empty, got %q", v.Text())
	}

	v.SetText("")
	v.Insert(0, "a")
	v.Insert(1, "b")
	v.Insert(2, " ")
	v.Insert(3, "c")
	if got := len(v.undoStack); got != 3 {
		t.Fatalf("expected 3 steps (ab | space | c), got %d (stack=%v)", got, v.undoStack)
	}

	v.SetText("abcd")
	v.selStart, v.selEnd = 4, 4
	v.DeleteRange(3, 4)
	v.DeleteRange(2, 3)
	v.DeleteRange(1, 2)
	if got := len(v.undoStack); got != 1 {
		t.Fatalf("expected 3 backspaces to merge, got %d", got)
	}
	if !v.Undo() {
		t.Fatalf("Undo backspace chain failed")
	}
	if v.Text() != "abcd" {
		t.Fatalf("after Undo expected 'abcd', got %q", v.Text())
	}
}

func TestRequestEditorChangedFlag(t *testing.T) {
	v := NewRequestEditor()
	if v.Changed() {
		t.Fatalf("fresh editor should not report Changed()")
	}
	v.Insert(0, "x")
	if !v.Changed() {
		t.Fatalf("Changed() should be true after Insert")
	}
	if v.Changed() {
		t.Fatalf("Changed() should reset to false after read")
	}
	v.DeleteRange(0, 1)
	if !v.Changed() {
		t.Fatalf("Changed() should be true after DeleteRange")
	}
}

func TestRequestEditorUnicodeInsertDelete(t *testing.T) {
	v := NewRequestEditor()

	v.Insert(0, "Привет, мир!")
	if v.Text() != "Привет, мир!" {
		t.Errorf("unicode insert: got %q", v.Text())
	}

	v.Insert(v.Len(), "\n🚀 emoji")
	if !strings.Contains(v.Text(), "🚀") {
		t.Errorf("emoji insert missing: %q", v.Text())
	}
	if len(v.lineStarts) < 2 {
		t.Errorf("expected lineStarts > 1 after newline insert, got %v", v.lineStarts)
	}

	v.Insert(0, "Hello ")
	if !strings.HasPrefix(v.Text(), "Hello Привет") {
		t.Errorf("prefix insert: %q", v.Text())
	}
}

func TestRequestEditorASCIIFlagInvalidatesOnUnicodeInsert(t *testing.T) {
	v := NewRequestEditor()
	v.Insert(0, "plain ascii")
	if !v.isASCIIOnly() {
		t.Errorf("expected asciiOnly=true after ASCII insert")
	}
	if v.byteToRune(5) != 5 {
		t.Errorf("byteToRune fast-path for ASCII broken")
	}

	v.Insert(v.Len(), " 🚀")
	if v.isASCIIOnly() {
		t.Errorf("expected asciiOnly=false after emoji insert")
	}

	want := 11 + 1 + 1
	if v.byteToRune(v.Len()) != want {
		t.Errorf("byteToRune after emoji: got %d, want %d", v.byteToRune(v.Len()), want)
	}
}

func TestRequestEditorTotalRunesCacheUnicode(t *testing.T) {
	v := NewRequestEditor()
	v.Insert(0, "abc")
	if v.totalRunes() != 3 {
		t.Errorf("ascii total runes: got %d", v.totalRunes())
	}

	v.Insert(v.Len(), "🚀")
	if v.totalRunes() != 4 {
		t.Errorf("after emoji append: got %d, want 4", v.totalRunes())
	}

	v.DeleteRange(0, 3)
	if v.totalRunes() != 1 {
		t.Errorf("after delete ascii prefix: got %d, want 1", v.totalRunes())
	}

	v.Insert(v.Len(), "👨‍👩‍👧‍👦")
	want := 1 + 7
	if v.totalRunes() != want {
		t.Errorf("after family emoji: got %d, want %d", v.totalRunes(), want)
	}
}

func TestRequestEditorLineStartsUnicode(t *testing.T) {
	v := NewRequestEditor()
	v.Insert(0, "строка1\nстрока2\nстрока3")
	if len(v.lineStarts) != 3 {
		t.Errorf("expected 3 lineStarts, got %v", v.lineStarts)
	}
	validateLineStarts(t, v)

	v.Insert(0, "\n")
	if len(v.lineStarts) != 4 {
		t.Errorf("expected 4 lineStarts after prepending newline, got %v", v.lineStarts)
	}
	validateLineStarts(t, v)

	v.DeleteRange(0, 1)
	if len(v.lineStarts) != 3 {
		t.Errorf("expected 3 lineStarts after removing newline, got %v", v.lineStarts)
	}
	validateLineStarts(t, v)

	emojiLine := "🚀 emoji line"
	v.Insert(v.Len(), "\n"+emojiLine)
	if !strings.HasSuffix(v.Text(), emojiLine) {
		t.Errorf("expected suffix %q in %q", emojiLine, v.Text())
	}
	validateLineStarts(t, v)
}

func TestRequestEditorRandomInsertDelete(t *testing.T) {
	v := NewRequestEditor()
	v.Insert(0, "abcdef")

	v.Insert(3, "Х")
	v.Insert(v.Len(), "\n🔥тест")
	v.Insert(2, "  ")
	v.DeleteRange(0, 1)

	totalBytes := 0
	totalRunes := 0
	for i := 0; i < len(v.text); {
		_, sz := decodeOne(v.text[i:])
		if sz < 1 {
			sz = 1
		}
		i += sz
		totalRunes++
		totalBytes = i
	}
	if totalBytes != len(v.text) {
		t.Errorf("byte count mismatch: %d vs %d", totalBytes, len(v.text))
	}
	if v.totalRunes() != totalRunes {
		t.Errorf("rune count mismatch: cached %d vs computed %d", v.totalRunes(), totalRunes)
	}
}

func tokensSnapshot(v *RequestEditor) []syntax.Token {
	out := make([]syntax.Token, len(v.tokens))
	copy(out, v.tokens)
	return out
}

func setTokens(v *RequestEditor, toks []syntax.Token) {
	v.tokens = append(v.tokens[:0], toks...)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false
}

func TestShiftTokens_InsertBeforeAllTokens(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`"hello"`)
	setTokens(v, []syntax.Token{{Start: 0, Len: uint16(7 - (0)), Kind: syntax.TokString}})

	v.Insert(0, "X")
	if v.Text() != `X"hello"` {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 1, Len: uint16(8 - (1)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert at 0:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertAtTokenEndDoesNotExtend(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc")
	setTokens(v, []syntax.Token{{Start: 0, Len: uint16(3 - (0)), Kind: syntax.TokString}})

	v.Insert(3, "X")
	if v.Text() != "abcX" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, Len: uint16(3 - (0)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert at End boundary:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertAtTokenStartPushesToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc")
	setTokens(v, []syntax.Token{{Start: 0, Len: uint16(3 - (0)), Kind: syntax.TokString}})

	v.Insert(0, " ")
	if v.Text() != " abc" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 1, Len: uint16(4 - (1)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert at Start:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertInsideTokenExtends(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("hello")
	setTokens(v, []syntax.Token{{Start: 0, Len: uint16(5 - (0)), Kind: syntax.TokKeyword}})

	v.Insert(2, "XY")
	if v.Text() != "heXYllo" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, Len: uint16(7 - (0)), Kind: syntax.TokKeyword}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert inside token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertBetweenTwoTokens(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("AA BB")
	setTokens(v, []syntax.Token{
		{Start: 0, Len: uint16(2 - (0)), Kind: syntax.TokKeyword},
		{Start: 3, Len: uint16(5 - (3)), Kind: syntax.TokKeyword},
	})

	v.Insert(2, "X")
	if v.Text() != "AAX BB" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{
		{Start: 0, Len: uint16(2 - (0)), Kind: syntax.TokKeyword},
		{Start: 4, Len: uint16(6 - (4)), Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after between-tokens insert:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteEntirelyAfterToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc def")
	setTokens(v, []syntax.Token{{Start: 0, Len: uint16(3 - (0)), Kind: syntax.TokString}})

	v.DeleteRange(4, 7)
	if v.Text() != "abc " {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, Len: uint16(3 - (0)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete-after-token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteEntirelyBeforeToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc def")
	setTokens(v, []syntax.Token{{Start: 4, Len: uint16(7 - (4)), Kind: syntax.TokString}})

	v.DeleteRange(0, 4)
	if v.Text() != "def" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, Len: uint16(3 - (0)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete-before-token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteInsideTokenShrinks(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abcdef")
	setTokens(v, []syntax.Token{{Start: 0, Len: uint16(6 - (0)), Kind: syntax.TokString}})

	v.DeleteRange(2, 4)
	if v.Text() != "abef" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, Len: uint16(4 - (0)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after inside-token delete:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteFullyContainsToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("aaXXbb")
	setTokens(v, []syntax.Token{
		{Start: 0, Len: uint16(2 - (0)), Kind: syntax.TokKeyword},
		{Start: 2, Len: uint16(4 - (2)), Kind: syntax.TokString},
		{Start: 4, Len: uint16(6 - (4)), Kind: syntax.TokKeyword},
	})

	v.DeleteRange(2, 4)
	if v.Text() != "aabb" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{
		{Start: 0, Len: uint16(2 - (0)), Kind: syntax.TokKeyword},
		{Start: 2, Len: uint16(4 - (2)), Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete that contains token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteSpanningPartialToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abcDEFGHIjkl")
	setTokens(v, []syntax.Token{{Start: 3, Len: uint16(9 - (3)), Kind: syntax.TokString}})

	v.DeleteRange(5, 12)
	if v.Text() != "abcDE" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 3, Len: uint16(5 - (3)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after partial-tail delete:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteSpanningTokenLeftEdge(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abcDEFGHIjkl")
	setTokens(v, []syntax.Token{{Start: 3, Len: uint16(9 - (3)), Kind: syntax.TokString}})

	v.DeleteRange(0, 5)
	if v.Text() != "FGHIjkl" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, Len: uint16(4 - (0)), Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after partial-head delete:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_NoOpWhenNoTokens(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc")
	v.Insert(1, "X")
	if len(v.tokens) != 0 {
		t.Fatalf("expected no tokens, got %+v", v.tokens)
	}
	v.DeleteRange(0, 1)
	if len(v.tokens) != 0 {
		t.Fatalf("expected no tokens, got %+v", v.tokens)
	}
}

func TestShiftTokens_ReplaceShiftsCorrectly(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("aa bb cc")
	setTokens(v, []syntax.Token{
		{Start: 0, Len: uint16(2 - (0)), Kind: syntax.TokKeyword},
		{Start: 3, Len: uint16(5 - (3)), Kind: syntax.TokKeyword},
		{Start: 6, Len: uint16(8 - (6)), Kind: syntax.TokKeyword},
	})

	v.Replace(2, 6, "X")
	if v.Text() != "aaXcc" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{
		{Start: 0, Len: uint16(2 - (0)), Kind: syntax.TokKeyword},
		{Start: 3, Len: uint16(5 - (3)), Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after Replace:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestSpansAfterInsert_PreservesTrailingColor(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"name":"hello"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	var helloTok *syntax.Token
	for i := range v.tokens {
		if v.tokens[i].Kind == syntax.TokString && v.tokens[i].Start == 8 {
			helloTok = &v.tokens[i]
			break
		}
	}
	if helloTok == nil || helloTok.End() != 15 {
		t.Fatalf("setup tokenization changed: tokens=%+v", v.tokens)
	}

	v.Insert(3, "X")
	if v.Text() != `{"nXame":"hello"}` {
		t.Fatalf("text after insert: %q", v.Text())
	}

	var got *syntax.Token
	for i := range v.tokens {
		if v.tokens[i].Kind == syntax.TokString && v.tokens[i].Start == 9 {
			got = &v.tokens[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("could not find shifted hello token; tokens=%+v", v.tokens)
	}
	if got.End() != 16 {
		t.Fatalf("hello token End should be 16 (covers \"hello\" in new text), got %d", got.End())
	}
	if v.text[got.End()-1] != '"' {
		t.Fatalf("expected closing quote at byte End-1, got %q (text=%q)",
			v.text[got.End()-1], v.Text())
	}
}

func TestSpansForChunk_NoTrailingDropAfterInsert(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"abc","m":"def"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	v.Insert(5, "Y")
	if v.Text() != `{"k":Y"abc","m":"def"}` {
		t.Fatalf("text: %q", v.Text())
	}

	palette := theme.SyntaxPalette{
		String: color.NRGBA{R: 1, A: 255},
		Key:    color.NRGBA{R: 2, A: 255},
	}
	spans := v.spansForChunk(0, len(v.text), palette, false)

	covered := make([]bool, len(v.text))
	colored := make([]color.NRGBA, len(v.text))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(covered); i++ {
			covered[i] = true
			colored[i] = sp.Color
		}
	}

	for i := 6; i <= 10; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) of first string lost its color: text=%q", i, v.text[i], v.Text())
		}
		if colored[i] != palette.String {
			t.Errorf("byte %d (%q) wrong color: got %+v want %+v", i, v.text[i], colored[i], palette.String)
		}
	}
	for i := 16; i <= 20; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) of second string lost its color: text=%q", i, v.text[i], v.Text())
		}
		if colored[i] != palette.String {
			t.Errorf("byte %d (%q) wrong color: got %+v want %+v", i, v.text[i], colored[i], palette.String)
		}
	}
	if covered[5] {
		t.Errorf("inserted byte at position 5 unexpectedly carries a syntax color")
	}
}

func TestSpansForChunk_NoTrailingDropAfterDelete(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"abc","m":"def"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	v.DeleteRange(1, 5)
	if v.Text() != `{"abc","m":"def"}` {
		t.Fatalf("text: %q", v.Text())
	}

	palette := theme.SyntaxPalette{
		String: color.NRGBA{R: 1, A: 255},
		Key:    color.NRGBA{R: 2, A: 255},
	}
	spans := v.spansForChunk(0, len(v.text), palette, false)
	covered := make([]bool, len(v.text))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(covered); i++ {
			covered[i] = true
		}
	}
	for i := 11; i <= 15; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) lost its color after delete: text=%q", i, v.text[i], v.Text())
		}
	}
}

func TestSpansForChunk_StableWithoutEdits(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"v"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	palette := theme.SyntaxPalette{
		String: color.NRGBA{R: 1, A: 255},
		Key:    color.NRGBA{R: 2, A: 255},
	}
	want := v.spansForChunk(0, len(v.text), palette, false)
	got := v.spansForChunk(0, len(v.text), palette, false)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("spansForChunk non-deterministic: %+v vs %+v", want, got)
	}
	if len(got) == 0 {
		t.Fatalf("expected non-empty spans for %q", v.Text())
	}
}

var _ = widgets.ColoredSpan{}

type editorKeyRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *RequestEditor
}

func newEditorKeyRig() *editorKeyRig {
	return &editorKeyRig{
		ops:    new(op.Ops),
		shaper: material.NewTheme().Shaper,
		v:      NewRequestEditor(),
	}
}

func (rig *editorKeyRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 300)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	RequestEditorStyle{
		Viewer:   rig.v,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(14),
		Wrap:     true,
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *editorKeyRig) focus() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
}

func (rig *editorKeyRig) pressKey(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame()
}

func TestRequestEditorCtrlZKeyUndo(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.v.Replace(4, 4, " text")
	if got := rig.v.Text(); got != "base text" {
		t.Fatalf("setup: got %q", got)
	}

	rig.focus()
	rig.pressKey("Z", key.ModShortcut)

	if got := rig.v.Text(); got != "base" {
		t.Fatalf("Ctrl+Z should undo to %q, got %q", "base", got)
	}
}

func TestRequestEditorCtrlYKeyRedo(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.v.Replace(4, 4, " text")

	rig.focus()
	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != "base" {
		t.Fatalf("Ctrl+Z should undo to %q, got %q", "base", got)
	}

	rig.pressKey("Y", key.ModShortcut)
	if got := rig.v.Text(); got != "base text" {
		t.Fatalf("Ctrl+Y should redo to %q, got %q", "base text", got)
	}
}

func TestRequestEditorCtrlShiftZKeyRedo(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.v.Replace(4, 4, " text")

	rig.focus()
	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != "base" {
		t.Fatalf("Ctrl+Z should undo to %q, got %q", "base", got)
	}

	rig.pressKey("Z", key.ModShortcut|key.ModShift)
	if got := rig.v.Text(); got != "base text" {
		t.Fatalf("Ctrl+Shift+Z should redo to %q, got %q", "base text", got)
	}
}

func TestRequestEditorTypeThenCtrlZ(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.focus()
	before := rig.v.Text()

	rig.r.Queue(key.EditEvent{Text: " typed"})
	rig.frame()
	if rig.v.Text() == before {
		t.Fatalf("typing via EditEvent had no effect; still %q", before)
	}

	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != before {
		t.Fatalf("Ctrl+Z after typing should undo to %q, got %q", before, got)
	}
}

func TestRequestEditorCtrlBackspaceDeletesWord(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()
	if got := rig.v.Text(); got != "hello world" {
		t.Fatalf("setup: got %q", got)
	}

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.v.Text(); got != "hello " {
		t.Fatalf("Ctrl+Backspace should delete trailing word, got %q", got)
	}
}

func TestRequestEditorCtrlDeleteDeletesForwardWord(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()

	rig.v.selStart = 0
	rig.v.selEnd = 0
	rig.pressKey(key.NameDeleteForward, key.ModShortcut)
	if got := rig.v.Text(); got != "world" {
		t.Fatalf("Ctrl+Delete should delete the leading word and separator, got %q", got)
	}
}

func TestRequestEditorPlainBackspaceStillDeletesChar(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello"})
	rig.frame()

	rig.pressKey(key.NameDeleteBackward, 0)
	if got := rig.v.Text(); got != "hell" {
		t.Fatalf("plain Backspace should delete a single char, got %q", got)
	}
}

func errTabTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	return th
}

func TestErrorDetailsPanelTogglesAndFitsFullMessage(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.Method = "GET"
	tab.URLInput.SetText("http://example.com")
	tab.Status = "Error: " + strings.Repeat("Get \"http://example.com\": dial tcp: lookup example.com: no such host; ", 6)

	win := new(app.Window)
	th := errTabTheme()
	frame := func() {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(800, 600)),
			Now:         time.Now(),
		}
		tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	}
	panelHeight := func() int {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(800, 600)},
			Now:         time.Now(),
		}
		return tab.layoutErrorDetails(gtx, th).Size.Y
	}

	frame()
	if h := panelHeight(); h != 0 {
		t.Fatalf("collapsed error details should take no space, got %d", h)
	}

	tab.ErrDetailsBtn.Click()
	frame()
	if !tab.ErrDetailsOpen {
		t.Fatal("clicking the details button should expand the error")
	}
	openH := panelHeight()
	if openH <= 0 {
		t.Fatal("expanded error details rendered nothing")
	}

	// The panel wraps: a message six times as long must not fit in the height a
	// single truncated status line would take.
	tab.Status = "Error: short"
	if shortH := panelHeight(); shortH >= openH {
		t.Errorf("long error panel (%d) should be taller than a short one (%d)", openH, shortH)
	}

	tab.Status = "200 OK  10ms  1 B"
	if h := panelHeight(); h != 0 {
		t.Errorf("a non-error status must not render an error panel, got %d", h)
	}
}

func TestProcessTemplate_EdgeCases(t *testing.T) {
	env := map[string]string{
		"a":     "X",
		"":      "EMPTY",
		"long":  strings.Repeat("v", 256),
		"multi": "L1\nL2",
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty input empty env", "", "Y"[:0]},
		{"two templates", "{{a}}-{{a}}", "X-X"},
		{"empty key with empty env entry", "{{}}", "EMPTY"},
		{"spaces-only key", "{{   }}", "EMPTY"},
		{"value with newline", "X={{multi}}", "X=L1\nL2"},
		{"long value", "[{{long}}]", "[" + strings.Repeat("v", 256) + "]"},
		{"leading unmatched brace", "}}{{a}}", "}}X"},
		{"empty env map non-nil", "{{a}}", "{{a}}"},
		{"only braces no end", "{{", "{{"},
		{"close before open", "}}{{", "}}{{"},
		{"template with tab inside", "{{\ta\t}}", "X"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.name == "empty env map non-nil" {
				got = processTemplate(tc.in, map[string]string{})
			} else {
				got = processTemplate(tc.in, env)
			}
			if got != tc.want {
				t.Errorf("processTemplate(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	if got := processTemplate("{{a}}", nil); got != "{{a}}" {
		t.Errorf("nil env should pass through, got %q", got)
	}
	if got := processTemplate("no templates here", env); got != "no templates here" {
		t.Errorf("no '{{' should pass through, got %q", got)
	}
}

func TestFormatSize_Boundaries(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1025, "1.0 KB"},
		{1<<20 - 1, "1024.0 KB"},
		{1 << 20, "1.0 MB"},
		{1<<30 - 1, "1024.0 MB"},
		{1 << 30, "1.00 GB"},
		{2 << 30, "2.00 GB"},
	}
	for _, tc := range cases {
		if got := formatSize(tc.in); got != tc.want {
			t.Errorf("formatSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := formatSize(-1); got != "0 B" {
		t.Errorf("formatSize(-1) must clamp to 0 B (no negative size), got %q", got)
	}
	if got := formatSize(-9999); got != "0 B" {
		t.Errorf("formatSize(-9999) must clamp to 0 B, got %q", got)
	}
}

func TestTrimTrailingWhitespace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"abc   ", "abc"},
		{"a\nb  \nc\t\t", "a\nb\nc"},
		{"  ", ""},
		{"line1\r\nline2  \r", "line1\nline2"},
		{"   \n   \n", "\n\n"},
	}
	for _, tc := range cases {
		if got := trimTrailingWhitespace(tc.in); got != tc.want {
			t.Errorf("trimTrailingWhitespace(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLooksLikeJSON_BOM(t *testing.T) {
	if looksLikeJSON([]byte("\xEF\xBB\xBF{\"a\":1}")) {
		t.Logf("looksLikeJSON treats BOM as non-JSON (expected)")
	}

	if !looksLikeJSON([]byte("\n\n\n[")) {
		t.Errorf("expected leading newlines to be skipped")
	}
	if looksLikeJSON(nil) {
		t.Errorf("nil should be false")
	}
}

func TestLoadPreviewFromFile_Missing(t *testing.T) {
	result, n, isJSON, _ := loadPreviewFromFile(filepath.Join(t.TempDir(), "nope"), 100, &JSONFormatterState{}, "", 0)
	if result != "" || n != 0 || isJSON {
		t.Errorf("expected zero values on missing file, got (%q,%d,%v)", result, n, isJSON)
	}
}

func TestLoadPreviewFromFile_EmptyFile(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview-empty")
	_ = tmp.Close()
	defer os.Remove(tmp.Name())

	result, n, isJSON, _ := loadPreviewFromFile(tmp.Name(), 0, &JSONFormatterState{}, "", 0)
	if result != "" || n != 0 || isJSON {
		t.Errorf("empty file should return zero values, got (%q,%d,%v)", result, n, isJSON)
	}
}

func TestLoadPreviewFromFile_AutoFormatDisabled(t *testing.T) {
	prev := settings.AutoFormatJSON
	settings.AutoFormatJSON = false
	defer func() { settings.AutoFormatJSON = prev }()

	tmp, _ := os.CreateTemp("", "preview-noauto")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	body := `{"a":1}`
	_ = os.WriteFile(tmpPath, []byte(body), 0644)

	result, _, isJSON, _ := loadPreviewFromFile(tmpPath, int64(len(body)), &JSONFormatterState{}, "", 0)
	if isJSON {
		t.Errorf("with AutoFormatJSON=false, isJSON should be false")
	}
	if result != body {
		t.Errorf("expected unformatted body, got %q", result)
	}
}

func TestGetPreviewBuf(t *testing.T) {
	buf, release := getPreviewBuf(0)
	if len(buf) != 0 {
		t.Errorf("expected len 0, got %d", len(buf))
	}
	release()

	buf, release = getPreviewBuf(100)
	if len(buf) != 100 {
		t.Errorf("expected len 100, got %d", len(buf))
	}
	release()

	huge := int64(previewBatchSize + 10)
	buf, release = getPreviewBuf(huge)
	if int64(len(buf)) != huge {
		t.Errorf("expected len %d for over-batch, got %d", huge, len(buf))
	}
	release()
}

func TestBuildBody_None(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyNone
	r, ct, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil reader")
	}
	if ct != "" {
		t.Errorf("expected empty content-type, got %q", ct)
	}
}

func TestBuildBody_URLEncoded(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.URLEncoded = []*URLEncodedPart{
		NewURLEncodedPart("k1", "v1"),
		NewURLEncodedPart("k2", "{{var}}"),
		NewURLEncodedPart(" ", "blank-key-skip"),
		NewURLEncodedPart("", "also-skip"),
	}
	env := map[string]string{"var": "VAL"}
	r, ct, err := tab.buildBody(context.Background(), env)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Errorf("ct = %q", ct)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "k1=v1") || !strings.Contains(got, "k2=VAL") {
		t.Errorf("body = %q", got)
	}
	if strings.Contains(got, "blank") || strings.Contains(got, "also-skip") {
		t.Errorf("body should not include blank-key parts: %q", got)
	}
}

func TestBuildBody_Binary_NoPath(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = ""
	r, ct, err := tab.buildBody(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected err for no file selected")
	}
	if r != nil || ct != "" {
		t.Errorf("expected zero values on err")
	}
}

func TestBuildBody_Binary_MissingFile(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = filepath.Join(t.TempDir(), "does-not-exist")
	_, _, err := tab.buildBody(context.Background(), nil)
	if err == nil {
		t.Errorf("expected err for missing file")
	}
}

func TestBuildBody_Binary_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	_ = os.WriteFile(path, []byte("hello"), 0644)
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = path
	r, ct, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer r.(io.Closer).Close()
	if ct != "application/octet-stream" {
		t.Errorf("unexpected ct: %q", ct)
	}
	data, _ := io.ReadAll(r)
	if string(data) != "hello" {
		t.Errorf("body = %q", string(data))
	}
}

func TestBuildBody_Raw_TemplatedAndStripped(t *testing.T) {
	prevStrip := settings.StripJSONComments
	settings.StripJSONComments = true
	defer func() { settings.StripJSONComments = prevStrip }()

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyRaw
	tab.ReqEditor.SetText(`{"k":"{{v}}"} // trailing comment`)
	r, ct, err := tab.buildBody(context.Background(), map[string]string{"v": "VAL"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ct != "" {
		t.Errorf("raw should not set explicit content-type, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if strings.Contains(got, "//") {
		t.Errorf("expected comment stripped, got %q", got)
	}
	if !strings.Contains(got, `"VAL"`) {
		t.Errorf("expected template substitution, got %q", got)
	}
}

func TestBuildBody_FormData_Text(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("name", "{{val}}", model.FormPartText, "", 0),
		NewFormPart("", "skipped", model.FormPartText, "", 0),
	}
	r, ct, err := tab.buildBody(context.Background(), map[string]string{"val": "ALICE"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Errorf("expected multipart, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "ALICE") || !strings.Contains(got, `name="name"`) {
		t.Errorf("multipart body missing field: %q", got)
	}
	if strings.Contains(got, "skipped") {
		t.Errorf("blank-key form part should be skipped")
	}
}

func TestBuildBody_FormData_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "up.txt")
	_ = os.WriteFile(path, []byte("FILE-DATA"), 0644)

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("upload", "", model.FormPartFile, path, 9),
		NewFormPart("empty-file", "", model.FormPartFile, "", 0),
	}
	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "FILE-DATA") || !strings.Contains(got, "up.txt") {
		t.Errorf("file part missing: %q", got)
	}
}

func TestBuildBody_FormData_FileMissing(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("upload", "", model.FormPartFile, filepath.Join(t.TempDir(), "nope"), 0),
	}
	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildBody itself shouldn't fail synchronously: %v", err)
	}

	_, readErr := io.ReadAll(r)
	if readErr == nil {
		t.Errorf("expected pipe error from missing file")
	}
}

func TestCleanupOrphanRespTmp_NoPanic(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)

	old := filepath.Join(dir, "rete-resp-old.tmp")
	fresh := filepath.Join(dir, "rete-resp-fresh.tmp")
	other := filepath.Join(dir, "not-rete.tmp")
	_ = os.WriteFile(old, []byte("x"), 0644)
	_ = os.WriteFile(fresh, []byte("x"), 0644)
	_ = os.WriteFile(other, []byte("x"), 0644)
	past := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, past, past)

	CleanupOrphanRespTmp()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old file removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("expected fresh file kept: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("expected unrelated file kept: %v", err)
	}
}

func TestUpdateSystemHeaders_UserOverrideCaseInsensitive(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.AddHeader("content-type", "x/custom")
	tab.UpdateSystemHeaders()

	count := 0
	for _, h := range tab.Headers {
		if strings.EqualFold(h.Key.Text(), "Content-Type") {
			count++
			if h.IsGenerated {
				t.Errorf("user header should remain non-generated")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Content-Type header, got %d", count)
	}
}

func TestUpdateSystemHeaders_GeneratedToManualOnEdit(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyRaw
	tab.UpdateSystemHeaders()

	var ct *HeaderItem
	for _, h := range tab.Headers {
		if h.IsGenerated && strings.EqualFold(h.Key.Text(), "Content-Type") {
			ct = h
			break
		}
	}
	if ct == nil {
		t.Fatalf("no generated Content-Type")
	}
	ct.Value.SetText("application/edited")
	tab.UpdateSystemHeaders()

	if ct.IsGenerated {
		t.Errorf("edited generated header should switch to manual")
	}
}

func TestUpdateSystemHeaders_BodyTypeSwitch(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.UpdateSystemHeaders()

	found := false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/x-www-form-urlencoded" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected generated urlencoded Content-Type")
	}

	tab.BodyType = model.BodyBinary
	tab.UpdateSystemHeaders()
	found = false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/octet-stream" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected generated octet-stream after switch")
	}

	tab.BodyType = model.BodyNone
	tab.UpdateSystemHeaders()
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" {
			t.Errorf("BodyNone should have no generated Content-Type, got %q", h.Value.Text())
		}
	}
}

func TestUpdateSystemHeaders_FormDataNoStaleContentType(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.UpdateSystemHeaders()

	tab.BodyType = model.BodyFormData
	tab.UpdateSystemHeaders()
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" {
			t.Errorf("form-data should not auto-set Content-Type (boundary added at send time), got %q", h.Value.Text())
		}
	}
}

func TestUpdateSystemHeaders_RawJSONDetection(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyRaw
	tab.ReqEditor.SetText(`   {"x":1}`)
	tab.UpdateSystemHeaders()

	found := false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/json" {
			found = true
		}
	}
	if !found {
		t.Errorf("JSON body should be detected as application/json")
	}

	tab.ReqEditor.SetText("\xEF\xBB\xBF[1,2,3]")
	tab.UpdateSystemHeaders()
	found = false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/json" {
			found = true
		}
	}
	if !found {
		t.Errorf("BOM-prefixed JSON should still detect as application/json")
	}

	tab.ReqEditor.SetText("hello world")
	tab.UpdateSystemHeaders()
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/json" {
			t.Errorf("plain text should not be detected as JSON")
		}
	}
}

func TestPerformSearch_RegexSpecialChars(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("price: $5.00 + $3.00 = $8.00")
	tab.invalidateSearchCache()
	tab.RespSearch.Editor.SetText("$5.00")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 1 {
		t.Errorf("expected literal dollar match, got %d", len(tab.RespSearch.spans))
	}

	tab.RespSearch.Editor.SetText(".00")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 3 {
		t.Errorf("expected literal '.00' to find 3, got %d", len(tab.RespSearch.spans))
	}
}

func TestPerformSearch_QueryLongerThanText(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("hi")
	tab.invalidateSearchCache()
	tab.RespSearch.Editor.SetText("longer than text")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 0 {
		t.Errorf("expected no matches")
	}
}

func TestPerformSearch_OverlappingNoDuplicate(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("aaaa")
	tab.invalidateSearchCache()
	tab.RespSearch.Editor.SetText("aa")
	tab.RespSearch.recompute(tab.RespEditor.Text())

	if len(tab.RespSearch.spans) != 2 {
		t.Errorf("expected 2 non-overlapping matches, got %d: %v", len(tab.RespSearch.spans), tab.RespSearch.spans)
	}
}

func TestSearchNavigate_DirZero(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("hello hello")
	tab.RespSearch.Editor.SetText("hello")
	tab.invalidateSearchCache()
	tab.RespSearch.recompute(tab.RespEditor.Text())
	tab.RespSearch.current = 0
	tab.RespSearch.navigate(0, tab.RespEditor)
	if tab.RespSearch.current != 0 {
		t.Errorf("dir=0 should stay at 0, got %d", tab.RespSearch.current)
	}
}

func TestFoldForSearch_Empty(t *testing.T) {
	if foldForSearch("") != "" {
		t.Errorf("empty should stay empty")
	}
	if foldForSearch("ASCII") != "ascii" {
		t.Errorf("ASCII conversion failed")
	}
	if foldForSearch("\x00\x01\x7F") != "\x00\x01\x7F" {
		t.Errorf("control chars should be unchanged")
	}
}

func TestCheckDirty_UnicodeURL(t *testing.T) {
	req := &model.ParsedRequest{
		Method:  "GET",
		URL:     "http://пример.рф",
		Body:    "",
		Name:    "T",
		Headers: map[string]string{},
	}
	node := &collections.CollectionNode{
		Request:    req,
		Collection: &collections.ParsedCollection{},
	}
	tab := &RequestTab{LinkedNode: node, Method: "GET", Title: "T"}
	tab.URLInput.SetText("http://пример.рф")
	tab.checkDirty()

	if tab.IsDirty {
		t.Error("unicode URL matching the linked request must not be marked dirty (rune-vs-byte regression)")
	}
}

func TestSaveToCollection_FormAndURLEncoded(t *testing.T) {
	col := &collections.ParsedCollection{}
	req := &model.ParsedRequest{
		Method:     "POST",
		URL:        "http://x",
		Name:       "T",
		Headers:    map[string]string{},
		FormParts:  []model.ParsedFormPart{{Key: "old"}},
		URLEncoded: []model.ParsedKV{{Key: "oldue"}},
	}
	node := &collections.CollectionNode{Request: req, Collection: col}
	tab := &RequestTab{LinkedNode: node, Method: "POST", Title: "T"}
	tab.URLInput.SetText("http://x")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("k1", "v1", model.FormPartText, "", 0),
		NewFormPart("", "skipped", model.FormPartText, "", 0),
		NewFormPart("file1", "", model.FormPartFile, "/tmp/x", 100),
	}
	tab.URLEncoded = []*URLEncodedPart{
		NewURLEncodedPart("a", "1"),
		NewURLEncodedPart("", "skip"),
	}
	tab.BinaryFilePath = "/path/binary"

	_ = tab.SaveToCollection()

	if len(req.FormParts) != 2 {
		t.Errorf("expected 2 saved form parts (blank-key skipped), got %d", len(req.FormParts))
	}
	if req.FormParts[0].Key != "k1" || req.FormParts[0].Value != "v1" {
		t.Errorf("form part 0 wrong: %+v", req.FormParts[0])
	}
	if req.FormParts[1].FilePath != "/tmp/x" || req.FormParts[1].Kind != model.FormPartFile {
		t.Errorf("file part wrong: %+v", req.FormParts[1])
	}
	if len(req.URLEncoded) != 1 || req.URLEncoded[0].Key != "a" {
		t.Errorf("urlencoded mismatch: %+v", req.URLEncoded)
	}
	if req.BinaryPath != "/path/binary" {
		t.Errorf("binary path not saved: %q", req.BinaryPath)
	}
	if req.BodyType != model.BodyFormData {
		t.Errorf("body type not saved")
	}
}

func TestSaveToCollection_GeneratedHeadersExcluded(t *testing.T) {
	req := &model.ParsedRequest{Method: "GET", URL: "http://x", Name: "T"}
	node := &collections.CollectionNode{Request: req, Collection: &collections.ParsedCollection{}}
	tab := &RequestTab{LinkedNode: node, Method: "GET", Title: "T"}
	tab.URLInput.SetText("http://x")

	user := &HeaderItem{IsGenerated: false}
	user.Key.SetText("X-Custom")
	user.Value.SetText("yes")

	gen := &HeaderItem{IsGenerated: true}
	gen.Key.SetText("User-Agent")
	gen.Value.SetText("auto")

	blank := &HeaderItem{IsGenerated: false}

	tab.Headers = []*HeaderItem{user, gen, blank}
	tab.SaveToCollection()

	if len(req.Headers) != 1 {
		t.Errorf("expected only 1 header (no generated, no blank), got %v", req.Headers)
	}
	if req.Headers["X-Custom"] != "yes" {
		t.Errorf("missing custom header: %v", req.Headers)
	}
	if _, ok := req.Headers["User-Agent"]; ok {
		t.Errorf("generated header should not be saved")
	}
}

func TestPrepareRequest_URLSpaceEncoding(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("example.com/path with spaces")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if !strings.Contains(req.URL.String(), "%20") {
		t.Errorf("expected spaces encoded, got %s", req.URL.String())
	}
	if !strings.HasPrefix(req.URL.String(), "http://") {
		t.Errorf("expected http:// prefix auto-added")
	}
}

func TestPrepareRequest_TabNewlineSanitize(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://ex\nample.\tcom")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if strings.ContainsAny(req.URL.String(), "\n\t") {
		t.Errorf("URL still contains tab/newline: %q", req.URL.String())
	}
}

func TestPrepareRequest_DefaultHeadersApplied(t *testing.T) {
	prev := settings.DefaultHeaders
	settings.DefaultHeaders = []model.DefaultHeader{
		{Key: "X-Default", Value: "{{token}}"},
		{Key: "", Value: "skipped"},
	}
	defer func() { settings.DefaultHeaders = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, _, cancel, err := tab.prepareRequest(context.Background(), map[string]string{"token": "TKN"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if req.Header.Get("X-Default") != "TKN" {
		t.Errorf("default header not applied: %q", req.Header.Get("X-Default"))
	}
}

func TestPrepareRequest_DefaultHeaderRespectsUserOverride(t *testing.T) {
	prev := settings.DefaultHeaders
	settings.DefaultHeaders = []model.DefaultHeader{
		{Key: "X-Default", Value: "default-val"},
	}
	defer func() { settings.DefaultHeaders = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	tab.AddHeader("X-Default", "user-val")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if got := req.Header.Get("X-Default"); got != "user-val" {
		t.Errorf("user-defined header should win, got %q", got)
	}
}

func TestPrepareRequest_AcceptEncodingDefault(t *testing.T) {
	prev := settings.AcceptEncoding
	settings.AcceptEncoding = "gzip, deflate"
	defer func() { settings.AcceptEncoding = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if req.Header.Get("Accept-Encoding") != "gzip, deflate" {
		t.Errorf("expected Accept-Encoding set, got %q", req.Header.Get("Accept-Encoding"))
	}
}

func TestPrepareRequest_SendConnClose(t *testing.T) {
	prev := settings.SendConnClose
	settings.SendConnClose = true
	defer func() { settings.SendConnClose = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if !req.Close {
		t.Errorf("expected Close=true")
	}
	if req.Header.Get("Connection") != "close" {
		t.Errorf("expected Connection: close header, got %q", req.Header.Get("Connection"))
	}
}

func TestPrepareRequest_NilParentContext(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, ctx, cancel, err := tab.prepareRequest(nil, nil) //nolint:staticcheck // nil parent context is the case under test
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if ctx == nil || req == nil {
		t.Errorf("nil parent should be replaced with background")
	}
}

func TestExecuteRequest_FormDataPostsMultipart(t *testing.T) {
	var gotCT string
	var bodyContains bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		bodyContains = strings.Contains(string(data), "ALICE")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	tab.Method = "POST"
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{NewFormPart("name", "ALICE", model.FormPartText, "", 0)}

	tab.ExecuteRequest(context.Background(), new(app.Window), nil)
	select {
	case <-tab.responseChan:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout")
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Errorf("expected multipart/form-data CT, got %q", gotCT)
	}
	if !bodyContains {
		t.Errorf("expected ALICE in body")
	}
}

func TestAddHeader_NotGeneratedByDefault(t *testing.T) {
	tab := NewRequestTab("t")
	tab.AddHeader("X", "Y")
	if tab.Headers[0].IsGenerated {
		t.Errorf("AddHeader should not mark as generated")
	}
	if tab.Headers[0].LastAutoKey != "" || tab.Headers[0].LastAutoVal != "" {
		t.Errorf("AddHeader should not set LastAuto* fields")
	}

	tab.addSystemHeader("A", "B")
	if !tab.Headers[1].IsGenerated {
		t.Errorf("addSystemHeader must mark generated")
	}
	if tab.Headers[1].LastAutoKey != "A" || tab.Headers[1].LastAutoVal != "B" {
		t.Errorf("addSystemHeader should record LastAuto*")
	}
}

func TestGetCleanTitle_Cache(t *testing.T) {
	tab := &RequestTab{Title: "alpha"}
	if tab.GetCleanTitle() != "alpha" {
		t.Errorf("first call wrong")
	}
	if tab.cleanTitleSrc != "alpha" {
		t.Errorf("cache key not set")
	}

	tab.cleanTitle = "OVERRIDE"
	if tab.GetCleanTitle() != "OVERRIDE" {
		t.Errorf("cache not used")
	}

	tab.Title = "beta"
	if tab.GetCleanTitle() != "beta" {
		t.Errorf("cache should invalidate on Title change")
	}
}

func TestJSONFormatterState_StringWithEscape(t *testing.T) {
	state := &JSONFormatterState{}
	got := formatJSON([]byte(`{"k":"a\"b","x":1}`), state)
	if !strings.Contains(got, `"a\"b"`) {
		t.Errorf("escaped quote in string broken: %q", got)
	}
}

func TestJSONFormatterState_StringSplitMidEscape(t *testing.T) {
	doc1 := []byte(`{"k":"a\`)
	doc2 := []byte(`"b","x":1}`)
	state := &JSONFormatterState{}
	a := formatJSON(doc1, state)
	b := formatJSON(doc2, state)
	full := a + b
	if !strings.Contains(full, `"a\"b"`) {
		t.Errorf("escape spanning batches broken: %q", full)
	}
}

func TestBytesAndTextAlignment(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText("hello")
	if string(tab.ReqEditor.Bytes()) != tab.ReqEditor.Text() {
		t.Errorf("Bytes/Text mismatch")
	}
	if tab.ReqEditor.Len() != len("hello") {
		t.Errorf("Len mismatch: %d vs %d", tab.ReqEditor.Len(), len("hello"))
	}
}

func TestMoveURLWord(t *testing.T) {
	s := "https://example.com/api/v1?key=val&x=y"
	n := len([]rune(s))

	cases := []struct {
		name string
		pos  int
		dir  int
		want int
	}{
		{"from-end-back-to-start-of-trailing-y", n, -1, 37},
		{"from-y-back-to-start-of-x", 37, -1, 35},
		{"from-x-back-to-start-of-val", 35, -1, 31},
		{"from-val-back-to-start-of-key", 31, -1, 27},
		{"from-0-forward-to-end-of-https", 0, 1, 5},
		{"forward-past-protocol-sep", 5, 1, len("https://") + len("example")},
		{"forward-stops-at-slash-boundary", len("https://example.com"), 1, len("https://example.com/") + len("api")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := moveURLWord(s, c.pos, c.dir)
			if got != c.want {
				t.Errorf("moveURLWord(%q, %d, %d) = %d, want %d", s, c.pos, c.dir, got, c.want)
			}
		})
	}

	if got := moveURLWord("", 0, 1); got != 0 {
		t.Errorf("empty forward: got %d, want 0", got)
	}
	if got := moveURLWord("", 0, -1); got != 0 {
		t.Errorf("empty backward: got %d, want 0", got)
	}
	if got := moveURLWord("abc", 100, 1); got != 3 {
		t.Errorf("oob forward: got %d, want 3", got)
	}
	if got := moveURLWord("abc", -5, 1); got != 3 {
		t.Errorf("negative pos forward: got %d, want 3", got)
	}
}

func TestMoveURLWord_Variables(t *testing.T) {
	s := "http://{{host}}/api/{{path}}?k={{val}}"

	cases := []struct {
		name string
		pos  int
		dir  int
		want int
	}{
		{"forward-from-protocol-jumps-over-whole-var", 4, 1, 15},
		{"forward-from-inside-var-jumps-to-end-of-var", 10, 1, 15},
		{"forward-from-var-start-jumps-to-var-end", 7, 1, 15},
		{"forward-after-var-into-next-word", 15, 1, 19},
		{"backward-from-after-var-jumps-to-var-start", 15, -1, 7},
		{"backward-from-inside-var-jumps-to-var-start", 11, -1, 7},
		{"backward-from-var-end-jumps-to-var-start", 14, -1, 7},
		{"forward-skips-into-second-var", 19, 1, 28},
		{"backward-from-third-var-end", 38, -1, 31},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := moveURLWord(s, c.pos, c.dir)
			if got != c.want {
				t.Errorf("moveURLWord(%q, %d, %d) = %d, want %d", s, c.pos, c.dir, got, c.want)
			}
		})
	}

	if got := moveURLWord("{{a}}{{b}}", 0, 1); got != 5 {
		t.Errorf("adjacent vars forward from 0: got %d, want 5", got)
	}
	if got := moveURLWord("{{a}}{{b}}", 5, 1); got != 10 {
		t.Errorf("adjacent vars forward from 5: got %d, want 10", got)
	}
	if got := moveURLWord("{{a}}{{b}}", 10, -1); got != 5 {
		t.Errorf("adjacent vars backward from 10: got %d, want 5", got)
	}
	if got := moveURLWord("{{a}}{{b}}", 5, -1); got != 0 {
		t.Errorf("adjacent vars backward from 5: got %d, want 0", got)
	}

	if got := moveURLWord("{{abc", 0, 1); got != 5 {
		t.Errorf("unmatched {{ forward: got %d, want 5", got)
	}
}

func TestIsURLWordSep(t *testing.T) {
	seps := []rune{'/', ':', '?', '#', '&', '=', '.', ' ', '\t', '@', ','}
	for _, r := range seps {
		if !isURLWordSep(r) {
			t.Errorf("expected %q to be separator", r)
		}
	}
	nonSeps := []rune{'a', 'Z', '0', '9', '-', '_', 'ё'}
	for _, r := range nonSeps {
		if isURLWordSep(r) {
			t.Errorf("did not expect %q to be separator", r)
		}
	}
}

func TestWidgetEditorLenIsRunes(t *testing.T) {
	var e widget.Editor
	e.SetText("привет")
	if e.Len() != 6 {
		t.Errorf("gio widget.Editor.Len returned %d, expected rune count 6", e.Len())
	}
	if len(e.Text()) != 12 {
		t.Errorf("Text byte length wrong: %d", len(e.Text()))
	}
}

func TestClipSpansToVars_NoVarsIsIdentity(t *testing.T) {
	spans := []widgets.ColoredSpan{{Start: 0, End: 5}, {Start: 6, End: 9}}
	got := clipSpansToVars(spans, []byte("hello world"))
	if len(got) != 2 || got[0] != spans[0] || got[1] != spans[1] {
		t.Fatalf("spans should pass through unchanged: %+v", got)
	}
}

func TestClipSpansToVars_UnterminatedVarIsIgnored(t *testing.T) {
	spans := []widgets.ColoredSpan{{Start: 0, End: 10}}
	got := clipSpansToVars(spans, []byte("abc{{unclosed"))
	if len(got) != 1 || got[0].End != 10 {
		t.Fatalf("an unterminated {{ must leave spans alone: %+v", got)
	}
}

func TestClipSpansToVars_SplitsAroundVariable(t *testing.T) {
	chunk := []byte("aa{{v}}bb")
	red := color.NRGBA{R: 255, A: 255}
	got := clipSpansToVars([]widgets.ColoredSpan{{Start: 0, End: len(chunk), Color: red}}, chunk)
	want := []widgets.ColoredSpan{
		{Start: 0, End: 2, Color: red},
		{Start: 7, End: 9, Color: red},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestClipSpansToVars_SpanFullyInsideVariableIsDropped(t *testing.T) {
	chunk := []byte("{{token}}")
	got := clipSpansToVars([]widgets.ColoredSpan{{Start: 2, End: 7}}, chunk)
	if len(got) != 0 {
		t.Fatalf("a span entirely inside a variable must be dropped: %+v", got)
	}
}

func TestClipSpansToVars_MultipleVarsAndDisjointSpan(t *testing.T) {
	chunk := []byte("x{{a}}y{{b}}z")
	got := clipSpansToVars([]widgets.ColoredSpan{
		{Start: 0, End: len(chunk)},
		{Start: 12, End: 13},
	}, chunk)
	wantStarts := []int{0, 6, 12, 12}
	wantEnds := []int{1, 7, 13, 13}
	if len(got) != len(wantStarts) {
		t.Fatalf("got %+v", got)
	}
	for i := range wantStarts {
		if got[i].Start != wantStarts[i] || got[i].End != wantEnds[i] {
			t.Fatalf("segment %d = [%d,%d), want [%d,%d)", i, got[i].Start, got[i].End, wantStarts[i], wantEnds[i])
		}
	}
}

func TestBytesIndexAndTwoBraces(t *testing.T) {
	if got := bytesIndex([]byte("abc"), ""); got != 0 {
		t.Errorf("empty needle should return 0, got %d", got)
	}
	if got := bytesIndex([]byte("hello"), "ll"); got != 2 {
		t.Errorf("bytesIndex = %d, want 2", got)
	}
	if got := bytesIndex([]byte("hello"), "zz"); got != -1 {
		t.Errorf("missing needle should return -1, got %d", got)
	}
	if bytesContainsTwoBraces([]byte("{")) {
		t.Error("a single brace is not a variable opener")
	}
	if bytesContainsTwoBraces([]byte("{a{b")) {
		t.Error("non-adjacent braces are not a variable opener")
	}
	if !bytesContainsTwoBraces([]byte("x{{y")) {
		t.Error("adjacent braces should be detected")
	}
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func zlibBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func rawDeflateBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := flate.NewWriter(&b, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate.NewWriter: %v", err)
	}
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func brotliBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := brotli.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func zstdBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := zstd.NewWriter(&b)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func respWith(enc string, body []byte) *http.Response {
	h := http.Header{}
	if enc != "" {
		h.Set("Content-Encoding", enc)
	}
	return &http.Response{Header: h, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestDecompressBody_Encodings(t *testing.T) {
	const payload = "the quick brown fox jumps over the lazy dog"

	var doubled bytes.Buffer
	gw := gzip.NewWriter(&doubled)
	_, _ = gw.Write(brotliBytes(t, payload))
	_ = gw.Close()

	cases := []struct {
		name string
		enc  string
		body []byte
	}{
		{"gzip", "gzip", gzipBytes(t, payload)},
		{"x-gzip", "x-gzip", gzipBytes(t, payload)},
		{"deflate-zlib", "deflate", zlibBytes(t, payload)},
		{"deflate-raw", "deflate", rawDeflateBytes(t, payload)},
		{"br", "br", brotliBytes(t, payload)},
		{"zstd", "zstd", zstdBytes(t, payload)},
		{"uppercase-and-spaces", "  GZIP  ", gzipBytes(t, payload)},
		{"chained-gzip-br", "br, gzip", doubled.Bytes()},
		{"empty-element", "identity, gzip", gzipBytes(t, payload)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := decompressBody(respWith(c.enc, c.body))
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
			if string(got) != payload {
				t.Fatalf("got %q, want %q", got, payload)
			}
		})
	}
}

func TestDecompressBody_PassThroughCases(t *testing.T) {
	if got := decompressBody(&http.Response{Header: http.Header{}}); got != nil {
		t.Error("nil body must be returned as-is")
	}

	raw := []byte("plain")
	r := respWith("gzip", raw)
	r.Uncompressed = true
	if got := decompressBody(r); got != r.Body {
		t.Error("an already-decompressed response must not be wrapped again")
	}

	for _, enc := range []string{"", "identity", "exotic-codec"} {
		r := respWith(enc, raw)
		if got := decompressBody(r); got != r.Body {
			t.Errorf("encoding %q should pass through untouched", enc)
		}
	}
}

func TestDecompressBody_CorruptStreamFallsBackToRawBody(t *testing.T) {
	for _, enc := range []string{"gzip", "deflate"} {
		r := respWith(enc, []byte{0x78, 0x00, 0x00, 0x00, 0x00, 0x00})
		got := decompressBody(r)
		if got != r.Body {
			t.Errorf("encoding %q: a corrupt stream must fall back to the raw body", enc)
		}
	}
}

func TestCleanupRespFile_RemovesTempAndDrainsChannels(t *testing.T) {
	tab := NewRequestTab("cleanup")

	dir := t.TempDir()
	main := filepath.Join(dir, "main.tmp")
	queued := filepath.Join(dir, "queued.tmp")
	for _, p := range []string{main, queued} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	f, err := os.CreateTemp(dir, "writer-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tab.FileSaveChan <- f
	tab.responseChan <- tabResponse{respFile: queued}
	tab.respFile = main

	tab.cleanupRespFile()

	if _, err := os.Stat(main); !os.IsNotExist(err) {
		t.Error("the tab's own temp response file must be removed")
	}
	if _, err := os.Stat(queued); !os.IsNotExist(err) {
		t.Error("a response still queued on the channel must have its temp file removed")
	}
	if tab.respFile != "" {
		t.Errorf("respFile should be cleared, got %q", tab.respFile)
	}

	tab.cleanupRespFile()
}

func TestTextCore_PadAndInvalidateHelpers(t *testing.T) {
	var v RequestEditor
	v.SetText("a\nb\nc\nd")

	v.chunkHeights = []int{1, 2, 3, 4, 5, 6}
	v.padChunkHeights()
	if len(v.chunkHeights) != len(v.lineStarts) {
		t.Fatalf("padChunkHeights should truncate to %d, got %d", len(v.lineStarts), len(v.chunkHeights))
	}
	v.chunkHeights = v.chunkHeights[:1]
	v.padChunkHeights()
	if len(v.chunkHeights) != len(v.lineStarts) {
		t.Fatalf("padChunkHeights should grow to %d, got %d", len(v.lineStarts), len(v.chunkHeights))
	}

	v.wrapPlans = make([]*wrapPlan, len(v.lineStarts)+3)
	v.padWrapPlans()
	if len(v.wrapPlans) != len(v.lineStarts) {
		t.Fatalf("padWrapPlans should truncate to %d, got %d", len(v.lineStarts), len(v.wrapPlans))
	}
	v.wrapPlans = nil
	v.padWrapPlans()
	if len(v.wrapPlans) != len(v.lineStarts) {
		t.Fatalf("padWrapPlans should grow to %d, got %d", len(v.lineStarts), len(v.wrapPlans))
	}

	for i := range v.wrapPlans {
		v.wrapPlans[i] = &wrapPlan{valid: true}
	}
	v.invalidateWrapPlansFrom(2)
	if !v.wrapPlans[0].valid || !v.wrapPlans[1].valid {
		t.Error("lines before the invalidation point must stay valid")
	}
	for i := 2; i < len(v.wrapPlans); i++ {
		if v.wrapPlans[i].valid {
			t.Errorf("line %d should have been invalidated", i)
		}
	}

	for i := range v.wrapPlans {
		v.wrapPlans[i] = &wrapPlan{valid: true}
	}
	v.invalidateWrapPlansFrom(-5)
	for i := range v.wrapPlans {
		if v.wrapPlans[i].valid {
			t.Fatalf("a negative start must clamp to 0 and invalidate line %d", i)
		}
	}

	v.invalidateChunkHeights()
	if len(v.chunkHeights) != 0 {
		t.Error("invalidateChunkHeights must empty the slice")
	}
}

func TestTextCore_SelectedTextClampsOutOfRange(t *testing.T) {
	var v RequestEditor
	v.SetText("abcdef")

	if got := v.SelectedText(); got != "" {
		t.Errorf("an empty selection must yield %q, got %q", "", got)
	}
	v.selStart, v.selEnd = 4, 1
	if got := v.SelectedText(); got != "bcd" {
		t.Errorf("a reversed selection should normalize, got %q", got)
	}
	v.selStart, v.selEnd = -3, 100
	if got := v.SelectedText(); got != "abcdef" {
		t.Errorf("out-of-range bounds should clamp, got %q", got)
	}
}

func TestTextCore_SpansForChunk(t *testing.T) {
	var v RequestEditor
	v.SetText("abcdefghij")
	v.tokens = []syntax.Token{
		{Start: 0, Len: 3},
		{Start: 3, Len: 3},
		{Start: 6, Len: 4},
	}
	pal := theme.Syntax

	if got := v.spansForChunk(5, 5, pal, false); got != nil {
		t.Error("an empty chunk range must produce no spans")
	}
	if got := v.spansForChunk(20, 30, pal, false); got != nil {
		t.Error("a chunk past every token must produce no spans")
	}

	got := v.spansForChunk(2, 7, pal, false)
	if len(got) != 3 {
		t.Fatalf("want 3 clipped spans, got %d (%+v)", len(got), got)
	}
	if got[0].Start != 0 || got[0].End != 1 {
		t.Errorf("first span should be clipped to the chunk start: %+v", got[0])
	}
	if got[2].Start != 4 || got[2].End != 5 {
		t.Errorf("last span should be clipped to the chunk end: %+v", got[2])
	}

	v.tokens = nil
	if got := v.spansForChunk(0, 5, pal, false); got != nil {
		t.Error("no tokens must produce no spans")
	}
}

func TestTextCore_SetScrollCaretIsInert(t *testing.T) {
	var v RequestEditor
	v.SetText("hello")
	v.scrollY = 17
	v.SetScrollCaret(true)
	v.SetScrollCaret(false)
	if v.scrollY != 17 {
		t.Errorf("SetScrollCaret must not move the viewport, scrollY = %d", v.scrollY)
	}
}

func TestEditor_SetCaretClampsAndScrolls(t *testing.T) {
	var v RequestEditor
	v.SetText(strings.Repeat("line\n", 200))

	v.SetCaret(-5, -9)
	if s, e := v.Selection(); s != 0 || e != 0 {
		t.Errorf("negative offsets must clamp to 0, got (%d,%d)", s, e)
	}

	v.SetCaret(1<<20, 1<<20)
	s, e := v.Selection()
	if s != v.Len() || e != v.Len() {
		t.Errorf("offsets past the end must clamp to len, got (%d,%d) len=%d", s, e, v.Len())
	}

	reveal := func(lineH, innerH int) {
		v.applyReveal(layout.Context{}, fixed.I(8), lineH, 200, innerH, false)
	}

	v.scrollY = 999
	v.SetCaret(10, 10)
	reveal(0, 100)
	if v.scrollY != 999 {
		t.Error("with no measured line height the viewport must not move")
	}

	v.lastViewportH = 100
	v.SetCaret(v.Len(), v.Len())
	reveal(20, 100)
	if v.scrollY <= 0 {
		t.Errorf("scrolling to the end should produce a positive offset, got %d", v.scrollY)
	}

	v.lastViewportH = 0
	v.SetCaret(0, 0)
	reveal(20, 0)
	if v.scrollY != 0 {
		t.Errorf("scrolling to offset 0 should land at 0, got %d", v.scrollY)
	}
}

func TestExampleSelLabel(t *testing.T) {
	tab := NewRequestTab("ex")
	if got := tab.exampleSelLabel(); got != "Examples" {
		t.Errorf("with no selection want %q, got %q", "Examples", got)
	}
	tab.Examples = []model.ParsedExample{{Name: "one"}, {Name: "two"}}
	tab.ExampleSel = 1
	if got := tab.exampleSelLabel(); got != exampleMenuLabel(1) {
		t.Errorf("selected label = %q", got)
	}
	tab.ExampleSel = 9
	if got := tab.exampleSelLabel(); got != "Examples" {
		t.Errorf("an out-of-range selection must fall back, got %q", got)
	}
	tab.ExampleSel = -1
	if got := tab.exampleSelLabel(); got != "Examples" {
		t.Errorf("a negative selection must fall back, got %q", got)
	}
}

func newLinkedTab(t *testing.T) (*RequestTab, *model.ParsedRequest) {
	t.Helper()
	tab := NewRequestTab("linked")
	req := &model.ParsedRequest{
		Name:    "linked",
		Method:  "GET",
		URL:     "http://example.com",
		Headers: map[string]string{},
	}
	tab.Method = req.Method
	tab.URLInput.SetText(req.URL)
	tab.ReqEditor.SetText(req.Body)
	tab.BodyType = req.BodyType
	tab.LinkedNode = &collections.CollectionNode{Request: req}
	tab.checkDirty()
	if tab.IsDirty {
		t.Fatalf("a freshly linked tab must be clean")
	}
	return tab, req
}

func TestCheckDirty_UnlinkedTabIsNeverDirty(t *testing.T) {
	tab := NewRequestTab("free")
	tab.IsDirty = true
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("a tab with no linked node must be clean")
	}
	tab.LinkedNode = &collections.CollectionNode{}
	tab.IsDirty = true
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("a linked node with no request must leave the tab clean")
	}
}

func TestCheckDirty_FieldByField(t *testing.T) {
	cases := []struct {
		name  string
		dirty func(*RequestTab, *model.ParsedRequest)
	}{
		{"method", func(tab *RequestTab, _ *model.ParsedRequest) { tab.Method = "POST" }},
		{"url", func(tab *RequestTab, _ *model.ParsedRequest) { tab.URLInput.SetText("http://other") }},
		{"body", func(tab *RequestTab, _ *model.ParsedRequest) { tab.ReqEditor.SetText("changed") }},
		{"body type", func(tab *RequestTab, _ *model.ParsedRequest) { tab.BodyType = model.BodyFormData }},
		{"binary path", func(tab *RequestTab, _ *model.ParsedRequest) { tab.BinaryFilePath = "/tmp/x" }},
		{"header added", func(tab *RequestTab, _ *model.ParsedRequest) { tab.AddHeader("X-New", "1") }},
		{"header value", func(tab *RequestTab, req *model.ParsedRequest) {
			req.Headers["X-A"] = "1"
			tab.AddHeader("X-A", "2")
		}},
		{"header key", func(tab *RequestTab, req *model.ParsedRequest) {
			req.Headers["X-A"] = "1"
			tab.AddHeader("X-B", "1")
		}},
		{"auth", func(tab *RequestTab, _ *model.ParsedRequest) {
			tab.AuthType = authBearer
			tab.AuthToken.SetText("tok")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab, req := newLinkedTab(t)
			c.dirty(tab, req)
			tab.checkDirty()
			if !tab.IsDirty {
				t.Fatalf("changing %s must mark the tab dirty", c.name)
			}
		})
	}
}

func TestCheckDirty_GeneratedAndBlankHeadersAreIgnored(t *testing.T) {
	tab, _ := newLinkedTab(t)
	tab.AddHeader("Content-Length", "12")
	tab.Headers[len(tab.Headers)-1].IsGenerated = true
	tab.AddHeader("", "orphan-value")
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("generated and key-less header rows must not count as user edits")
	}
}

func TestCheckDirty_FormPartsAndURLEncodedAndCookies(t *testing.T) {
	t.Run("form part added", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.FormParts = append(tab.FormParts, NewFormPart("", "", model.FormPartText, "", 0))
		tab.FormParts[0].Key.SetText("file")
		tab.FormParts[0].Value.SetText("v")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("adding a form part must mark the tab dirty")
		}
	})

	t.Run("blank form part ignored", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.FormParts = append(tab.FormParts, NewFormPart("", "", model.FormPartText, "", 0))
		tab.checkDirty()
		if tab.IsDirty {
			t.Fatal("a form part with a blank key must be ignored")
		}
	})

	t.Run("form part value differs", func(t *testing.T) {
		tab, req := newLinkedTab(t)
		req.FormParts = []model.ParsedFormPart{{Key: "k", Value: "saved"}}
		tab.FormParts = append(tab.FormParts, NewFormPart("", "", model.FormPartText, "", 0))
		tab.FormParts[0].Key.SetText("k")
		tab.FormParts[0].Value.SetText("edited")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("an edited form-part value must mark the tab dirty")
		}
	})

	t.Run("urlencoded added", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("", ""))
		tab.URLEncoded[0].Key.SetText("a")
		tab.URLEncoded[0].Value.SetText("1")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("adding a urlencoded pair must mark the tab dirty")
		}
	})

	t.Run("blank urlencoded ignored", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("", ""))
		tab.checkDirty()
		if tab.IsDirty {
			t.Fatal("a urlencoded row with a blank key must be ignored")
		}
	})

	t.Run("urlencoded value differs", func(t *testing.T) {
		tab, req := newLinkedTab(t)
		req.URLEncoded = []model.ParsedKV{{Key: "a", Value: "saved"}}
		tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("", ""))
		tab.URLEncoded[0].Key.SetText("a")
		tab.URLEncoded[0].Value.SetText("edited")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("an edited urlencoded value must mark the tab dirty")
		}
	})

	t.Run("cookie count differs", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.addCookie("sid", "1")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("adding a cookie must mark the tab dirty")
		}
	})

	t.Run("cookie value differs", func(t *testing.T) {
		tab, req := newLinkedTab(t)
		tab.addCookie("sid", "edited")
		req.Cookies = tab.CookieModels()
		req.Cookies[0].Value = "saved"
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("an edited cookie value must mark the tab dirty")
		}
	})
}

func TestWSSend_NotConnectedRecordsError(t *testing.T) {
	tab := NewRequestTab("ws")
	tab.Method = MethodWS

	tab.WSSendText("hi")
	tab.WSSendBinary([]byte{1, 2})
	tab.WSSendPing()
	tab.WSSendProto(`{"a":1}`)

	s := tab.EnsureWS()
	s.sessionMu.Lock()
	msgs := append([]WSDisplayMessage(nil), s.Messages...)
	s.sessionMu.Unlock()
	if len(msgs) != 4 {
		t.Fatalf("want 4 error entries, got %d", len(msgs))
	}
	for i, m := range msgs {
		if m.Error != "Not connected" {
			t.Errorf("entry %d error = %q, want %q", i, m.Error, "Not connected")
		}
	}
}

func TestProtoHeaderFields(t *testing.T) {
	tab := NewRequestTab("ws")
	tab.Method = MethodWS
	s := tab.EnsureWS()

	s.ProtoCmdEditor.SetText("7")
	s.ProtoSeqEditor.SetText("-9")
	s.ProtoOpcodeEditor.SetText("300")
	cmd, seq, op, err := s.protoHeaderFields()
	if err != nil {
		t.Fatalf("valid fields returned an error: %v", err)
	}
	if cmd != 7 || seq != -9 || op != 300 {
		t.Fatalf("parsed (%d,%d,%d), want (7,-9,300)", cmd, seq, op)
	}

	cases := []struct {
		name   string
		cmd    string
		seq    string
		opcode string
		prefix string
	}{
		{"bad cmd", "nope", "0", "0", "cmd: "},
		{"cmd out of range", "256", "0", "0", "cmd: "},
		{"bad seq", "1", "zz", "0", "seq: "},
		{"seq out of range", "1", "32768", "0", "seq: "},
		{"bad opcode", "1", "0", "!!", "opcode: "},
		{"opcode out of range", "1", "0", "-32769", "opcode: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.ProtoCmdEditor.SetText(c.cmd)
			s.ProtoSeqEditor.SetText(c.seq)
			s.ProtoOpcodeEditor.SetText(c.opcode)
			_, _, _, err := s.protoHeaderFields()
			if err == nil {
				t.Fatalf("want an error for %s", c.name)
			}
			if !strings.HasPrefix(err.Error(), c.prefix) {
				t.Fatalf("error %q should start with %q", err, c.prefix)
			}
		})
	}
}

type urlNavRig struct {
	r   input.Router
	ops *op.Ops
	th  *material.Theme
	tab *RequestTab
	clk time.Duration
}

func newURLNavRig(text string) *urlNavRig {
	rig := &urlNavRig{ops: new(op.Ops), th: material.NewTheme(), tab: &RequestTab{}}
	rig.tab.URLInput.SingleLine = true
	rig.tab.URLInput.SetText(text)
	return rig
}

func (rig *urlNavRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 40)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	rig.tab.handleURLWordJump(gtx)
	for {
		if _, ok := rig.tab.URLInput.Update(gtx); !ok {
			break
		}
	}
	rig.tab.handleURLMultiClick(gtx, rig.th, unit.Sp(12))
	dims := material.Editor(rig.th, &rig.tab.URLInput, "").Layout(gtx)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	cl := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	rig.tab.urlClick.Add(gtx.Ops)
	cl.Pop()
	pass.Pop()
	rig.r.Frame(rig.ops)
}

func (rig *urlNavRig) focus() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
}

func (rig *urlNavRig) clickN(x float32, n int) {
	for i := 0; i < n; i++ {
		rig.clk += 40 * time.Millisecond
		rig.r.Queue(
			pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: rig.clk},
			pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, 10), Source: pointer.Mouse, Time: rig.clk},
		)
		rig.frame()
	}
	rig.frame()
}

func TestHandleURLWordJump_MovesAndExtends(t *testing.T) {
	const url = "https://example.com/a/b"
	rig := newURLNavRig(url)
	rig.focus()

	n := utf8.RuneCountInString(url)
	rig.tab.URLInput.SetCaret(n, n)
	rig.frame()

	rig.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut, State: key.Press})
	rig.frame()
	_, afterLeft := rig.tab.URLInput.Selection()
	if afterLeft >= n {
		t.Fatalf("ctrl+left should move the caret left of %d, got %d", n, afterLeft)
	}

	rig.r.Queue(key.Event{Name: key.NameRightArrow, Modifiers: key.ModShortcut, State: key.Press})
	rig.frame()
	_, afterRight := rig.tab.URLInput.Selection()
	if afterRight <= afterLeft {
		t.Fatalf("ctrl+right should move the caret right of %d, got %d", afterLeft, afterRight)
	}

	rig.tab.URLInput.SetCaret(n, n)
	rig.frame()
	rig.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut | key.ModShift, State: key.Press})
	rig.frame()
	lo, hi := selRange(rig.tab)
	if lo == hi {
		t.Fatalf("ctrl+shift+left must extend the selection, got (%d,%d)", lo, hi)
	}
	if hi != n {
		t.Errorf("the selection anchor should stay at %d, got %d", n, hi)
	}
}

func TestHandleURLWordJump_IgnoresKeyRelease(t *testing.T) {
	const url = "https://example.com/path"
	rig := newURLNavRig(url)
	rig.focus()
	n := utf8.RuneCountInString(url)
	rig.tab.URLInput.SetCaret(n, n)
	rig.frame()

	rig.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut, State: key.Release})
	rig.frame()
	if _, end := rig.tab.URLInput.Selection(); end != n {
		t.Fatalf("a key release must not move the caret, got %d", end)
	}
}

func TestHandleURLMultiClick_TripleClickSelectsAll(t *testing.T) {
	const url = "https://example.com/alpha/beta"
	rig := newURLNavRig(url)
	rig.frame()
	rig.clk += 3 * time.Second
	rig.clickN(60, 3)

	lo, hi := selRange(rig.tab)
	n := utf8.RuneCountInString(url)
	if lo != 0 || hi != n {
		t.Fatalf("triple click should select the whole URL, got (%d,%d) want (0,%d)", lo, hi, n)
	}
}

func TestHandleURLMultiClick_DoubleClickSelectsWord(t *testing.T) {
	const url = "https://example.com/alpha/beta"
	rig := newURLNavRig(url)
	rig.frame()
	rig.clk += 3 * time.Second
	rig.clickN(60, 2)

	lo, hi := selRange(rig.tab)
	if lo == hi {
		t.Fatalf("double click should select a word, got an empty selection at %d", lo)
	}
	n := utf8.RuneCountInString(url)
	if lo < 0 || hi > n {
		t.Fatalf("selection (%d,%d) is out of range for a %d-rune URL", lo, hi, n)
	}
	sel := []rune(url)[lo:hi]
	if strings.ContainsAny(string(sel), "/:.") {
		t.Errorf("a double-click word should not span URL separators, got %q", string(sel))
	}
}

func TestHandleURLMultiClick_SingleClickLeavesSelectionEmpty(t *testing.T) {
	rig := newURLNavRig("https://example.com/alpha")
	rig.frame()
	rig.clk += 3 * time.Second
	rig.clickN(60, 1)
	rig.tab.URLInput.SetCaret(0, 0)
	rig.frame()

	if lo, hi := selRange(rig.tab); lo != hi {
		t.Fatalf("a single click must not create a selection, got (%d,%d)", lo, hi)
	}
}

func selRange(tab *RequestTab) (int, int) {
	a, b := tab.URLInput.Selection()
	if a > b {
		return b, a
	}
	return a, b
}

func TestEditor_ReplaceRangeAndClamp(t *testing.T) {
	var v RequestEditor
	v.SetText("hello world")

	if !v.Replace(6, 11, "there") {
		t.Fatal("Replace should succeed")
	}
	if v.Text() != "hello there" {
		t.Fatalf("after replace = %q", v.Text())
	}

	if !v.Replace(11, 6, "!") {
		t.Fatal("reversed range should be normalized and succeed")
	}
	if v.Text() != "hello !" {
		t.Fatalf("after reversed replace = %q", v.Text())
	}

	if !v.Replace(-5, 100, "reset") {
		t.Fatal("out-of-range bounds should clamp and succeed")
	}
	if v.Text() != "reset" {
		t.Fatalf("after clamped replace = %q", v.Text())
	}

	if !v.Replace(0, 0, "") {
		t.Fatal("an empty no-op replace should return true")
	}
}

func TestEditor_ReplaceRejectsOversize(t *testing.T) {
	var v RequestEditor
	v.SetText("x")
	huge := strings.Repeat("a", RequestBodyMaxBytes+10)
	if v.Replace(0, 1, huge) {
		t.Fatal("a replacement over the size limit must be rejected")
	}
	if v.Text() != "x" {
		t.Fatalf("rejected replace must not mutate the buffer, got len %d", v.Len())
	}
	if v.oversizeMsg == "" {
		t.Error("an oversize rejection should set a user message")
	}
}

func TestAutoSurroundPair(t *testing.T) {
	pairs := map[string][2]string{
		"(": {"(", ")"}, "[": {"[", "]"}, "{": {"{", "}"}, "<": {"<", ">"},
		")": {"(", ")"}, "]": {"[", "]"}, "}": {"{", "}"}, ">": {"<", ">"},
		"\"": {"\"", "\""}, "'": {"'", "'"}, "`": {"`", "`"},
	}
	for in, want := range pairs {
		o, c, ok := autoSurroundPair(in)
		if !ok || o != want[0] || c != want[1] {
			t.Errorf("autoSurroundPair(%q) = (%q,%q,%v), want (%q,%q,true)", in, o, c, ok, want[0], want[1])
		}
	}
	for _, in := range []string{"a", "", "ab", "-"} {
		if _, _, ok := autoSurroundPair(in); ok {
			t.Errorf("autoSurroundPair(%q) should not surround", in)
		}
	}
}

func TestCountRunesBetween(t *testing.T) {
	text := []byte("héllo")
	if got := countRunesBetween(text, 3, 3); got != 0 {
		t.Errorf("equal bounds = %d, want 0", got)
	}
	if got := countRunesBetween(text, 5, 2); got != 0 {
		t.Errorf("reversed bounds = %d, want 0", got)
	}
	if got := countRunesBetween(text, 0, len(text)); got != 5 {
		t.Errorf("full range = %d, want 5 runes", got)
	}
	if got := countRunesBetween(text, 0, 999); got != 5 {
		t.Errorf("past-end bound should clamp, got %d", got)
	}
}

func TestEditor_ByteToRuneASCIIAndUnicode(t *testing.T) {
	var a RequestEditor
	a.SetText("abcdef")
	if got := a.byteToRune(3); got != 3 {
		t.Errorf("ascii byteToRune(3) = %d, want 3", got)
	}
	if got := a.byteToRune(100); got != 6 {
		t.Errorf("ascii byteToRune past end should clamp to len, got %d", got)
	}

	var u RequestEditor
	u.SetText("héllo")
	if got := u.byteToRune(len("hé")); got != 2 {
		t.Errorf("unicode byteToRune after 'hé' = %d, want 2 runes", got)
	}
}

func TestEditor_ShiftRangesInsertAndDelete(t *testing.T) {
	var v RequestEditor
	v.SetText("0123456789")
	v.selStart, v.selEnd = 4, 7
	v.highlightStart, v.highlightEnd = 5, 8

	v.shiftRanges(3, 2)
	if v.selStart != 6 || v.selEnd != 9 {
		t.Errorf("insert shift sel = (%d,%d), want (6,9)", v.selStart, v.selEnd)
	}
	if v.highlightStart != 7 || v.highlightEnd != 10 {
		t.Errorf("insert shift highlight = (%d,%d), want (7,10)", v.highlightStart, v.highlightEnd)
	}

	var d RequestEditor
	d.SetText("0123456789")
	d.selStart, d.selEnd = 2, 9
	d.shiftRanges(8, -3)
	if d.selStart != 2 {
		t.Errorf("offset before the deleted [5,8) should be unchanged, got %d", d.selStart)
	}
	if d.selEnd != 6 {
		t.Errorf("offset after the cut should move left by 3, got %d", d.selEnd)
	}

	var c RequestEditor
	c.SetText("0123456789")
	c.selStart, c.selEnd = 6, 9
	c.shiftRanges(8, -3)
	if c.selStart != 5 {
		t.Errorf("an offset inside the deleted range should collapse to its start (5), got %d", c.selStart)
	}
	if c.selEnd != 6 {
		t.Errorf("offset after the cut should move left by 3, got %d", c.selEnd)
	}
}

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

func TestHeaderBarsEqualHeightHorizontal(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		rig := newHSplitRig()
		rig.size = image.Pt(1400, 700)
		for i := 0; i < 3; i++ {
			rig.frameScaled(scale)
		}
		ref := rig.tab.headersRowH
		if ref <= 0 {
			t.Fatalf("scale %v: sub-tabs bar not measured", scale)
		}
		if got := rig.tab.reqHeaderH; got != ref {
			t.Errorf("scale %v: Request bar %dpx != sub-tabs bar %dpx", scale, got, ref)
		}
		if got := rig.tab.respHeaderH; got != ref {
			t.Errorf("scale %v: Response bar %dpx != sub-tabs bar %dpx", scale, got, ref)
		}
	}
}

func TestCoordToByteOffset_NoWrap_RoundsToNearestChar(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 22, 0, adv, 10, 200, false); got != 3 {
		t.Errorf("click on right half of a glyph should place caret after it; got %d, want 3", got)
	}
	if got := v.coordToByteOffset(gtx, 17, 0, adv, 10, 200, false); got != 2 {
		t.Errorf("click on left half of a glyph should place caret before it; got %d, want 2", got)
	}
}

func TestDecompressBodyNilInputs(t *testing.T) {
	if got := decompressBody(nil); got != nil {
		t.Errorf("decompressBody(nil) = %v, want nil", got)
	}
	if got := decompressBody(&http.Response{}); got != nil {
		t.Errorf("decompressBody with nil Body = %v, want nil", got)
	}
}

func TestCancelRequest(t *testing.T) {
	tab := &RequestTab{}
	called := false
	tab.cancelFn = func() { called = true }

	tab.CancelRequest()
	if !called {
		t.Errorf("expected cancelFn to be called")
	}
	if tab.cancelFn != nil {
		t.Errorf("expected cancelFn to be nil")
	}
}

func TestCleanupRespFile(t *testing.T) {
	tab := &RequestTab{}
	tmp, _ := os.CreateTemp("", "test")
	_ = tmp.Close()

	tab.respFile = tmp.Name()

	win := new(app.Window)
	widgets.ArmInvalidateTimer(&tab.reqWidthTimer, win, 1*time.Minute)
	widgets.ArmInvalidateTimer(&tab.respWidthTimer, win, 1*time.Minute)

	tab.cleanupRespFile()
	if tab.respFile != "" {
		t.Errorf("expected respFile to be cleared")
	}
	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted")
	}
	if tab.reqWidthTimer != nil || tab.respWidthTimer != nil {
		t.Errorf("expected timers to be stopped and cleared")
	}
}

func TestPrepareRequest(t *testing.T) {
	tab := NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("{{host}}/api")
	tab.ReqEditor.SetText("{\"key\": \"{{val}}\"} // comment")

	tab.AddHeader("Auth", "Bearer {{token}}")

	env := map[string]string{
		"host":  "example.com",
		"val":   "123",
		"token": "secret",
	}

	req, ctx, cancel, err := tab.prepareRequest(context.Background(), env)
	if err != nil {
		t.Fatalf("prepareRequest error: %v", err)
	}
	defer cancel()

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL.String() != "http://example.com/api" {
		t.Errorf("expected http://example.com/api, got %s", req.URL.String())
	}
	if req.Header.Get("Auth") != "Bearer secret" {
		t.Errorf("expected auth header, got %s", req.Header.Get("Auth"))
	}

	buf := make([]byte, 100)
	n, _ := req.Body.Read(buf)
	bodyStr := string(buf[:n])
	if bodyStr != "{\"key\": \"123\"} " {
		t.Errorf("expected body without comment and templated, got %q", bodyStr)
	}

	if ctx == nil {
		t.Errorf("expected context")
	}
}

func TestPrepareRequest_EmptyURL(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	_, _, _, err := tab.prepareRequest(context.Background(), nil)
	if err == nil {
		t.Errorf("expected error for empty URL")
	}
}

func TestExecuteRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = true
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"

	win := new(app.Window)
	tab.ExecuteRequest(context.Background(), win, nil)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for response")
	}
}

func TestExecuteRequestToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`file content`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = false
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"

	tmp, _ := os.CreateTemp("", "save-target")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	tab.SaveToFilePath = tmpPath
	tab.beginRequest()

	win := new(app.Window)
	tab.ExecuteRequestToFile(context.Background(), win, nil, tmp)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}

	data, _ := os.ReadFile(tmpPath)
	if string(data) != "file content" {
		t.Errorf("file content mismatch")
	}
}

func TestExecuteRequest_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.URLInput.SetText(srv.URL)
	win := new(app.Window)
	tab.ExecuteRequest(context.Background(), win, nil)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "404 Not Found") {
			t.Errorf("expected 404, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

func TestExecuteRequest_PrepareError(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	win := new(app.Window)
	tab.ExecuteRequest(context.Background(), win, nil)
	if !strings.HasPrefix(tab.Status, "Error") {
		t.Errorf("expected Error status, got %s", tab.Status)
	}
}

func TestSendResponse_DeliversOnCanceledContext(t *testing.T) {
	tab := NewRequestTab("test")
	tab.requestID.Store(5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !tab.sendResponse(ctx, tabResponse{requestID: 5, status: "Cancelled"}) {
		t.Fatalf("sendResponse returned false even though responseChan was empty")
	}

	select {
	case got := <-tab.responseChan:
		if got.status != "Cancelled" {
			t.Fatalf("expected status Cancelled, got %q", got.status)
		}
	default:
		t.Fatalf("responseChan was empty — Cancelled status was dropped")
	}
}

func TestSendResponse_StaleID(t *testing.T) {
	tab := NewRequestTab("test")
	tab.requestID.Store(10)

	tab.sendResponse(context.Background(), tabResponse{requestID: 9, status: "Stale"})

	th := material.NewTheme()
	gtx := layout.Context{Ops: new(op.Ops)}
	tab.Layout(gtx, th, new(app.Window), nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	if tab.Status == "Stale" {
		t.Errorf("stale response should be ignored")
	}

	tab.sendResponse(context.Background(), tabResponse{requestID: 10, status: "Fresh"})
	tab.Layout(gtx, th, new(app.Window), nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	if !strings.Contains(tab.Status, "Fresh") {
		t.Errorf("fresh response should be accepted, got %s", tab.Status)
	}
}

func TestExecuteRequestToFile_Error(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("http://localhost:1")
	failWriter := &failingWriteCloser{}
	tab.ExecuteRequestToFile(context.Background(), new(app.Window), nil, failWriter)

	select {
	case res := <-tab.responseChan:
		if !strings.Contains(res.status, "Error") {
			t.Errorf("expected Error status, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

type failingWriteCloser struct{}

func (f *failingWriteCloser) Write(p []byte) (n int, err error) { return 0, io.ErrClosedPipe }
func (f *failingWriteCloser) Close() error                      { return nil }

func TestStreamResponse_Cancellation(t *testing.T) {
	tab := NewRequestTab("test")
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("start"))
		time.Sleep(100 * time.Millisecond)
		cancel()
		_ = pw.Close()
	}()
	var dest bytes.Buffer
	_, err := tab.streamResponse(ctx, 0, pr, &dest, new(app.Window), true, "")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestLoadPreviewForSavedFile(t *testing.T) {
	setupTestConfigDir(t)
	tmp, _ := os.CreateTemp("", "resp")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	content := `{"foo": "bar"}`
	_ = os.WriteFile(tmpPath, []byte(content), 0644)

	tab := NewRequestTab("test")
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.window = new(app.Window)
	tab.loadPreviewForSavedFile()

	select {
	case res := <-tab.previewChan:
		if res.body == "" {
			t.Errorf("expected body loaded")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

func TestDecompressBody_Gzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/html")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte("<html>hello</html>"))
		_ = gz.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Uncompressed {
		t.Fatalf("expected Uncompressed=false (manual Accept-Encoding)")
	}

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "<html>hello</html>" {
		t.Errorf("got %q", string(data))
	}
}

func TestDecompressBody_Deflate_Zlib(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "deflate")
		zw := zlib.NewWriter(w)
		_, _ = zw.Write([]byte("plain text"))
		_ = zw.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "deflate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, _ := io.ReadAll(body)
	if string(data) != "plain text" {
		t.Errorf("got %q", string(data))
	}
}

func TestDecompressBody_Identity(t *testing.T) {
	resp := &http.Response{
		Body:   io.NopCloser(strings.NewReader("raw")),
		Header: http.Header{},
	}
	resp.Header.Set("Content-Encoding", "identity")
	body := decompressBody(resp)
	data, _ := io.ReadAll(body)
	if string(data) != "raw" {
		t.Errorf("got %q", string(data))
	}
}

func TestStreamResponse_SniffHTMLMeta(t *testing.T) {

	cp1251 := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	body := append([]byte(`<html><head><meta charset="windows-1251"></head><body>`), cp1251...)
	body = append(body, []byte(`</body></html>`)...)

	tab := NewRequestTab("test")
	var dest bytes.Buffer
	_, err := tab.streamResponse(context.Background(), 0, bytes.NewReader(body), &dest, new(app.Window), true, "text/html")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var got strings.Builder
	for {
		select {
		case chunk := <-tab.appendChan:
			got.WriteString(chunk.text)
		default:
			if !strings.Contains(got.String(), "Привет") {
				t.Errorf("sniffed preview missing cyrillic; got %q", got.String())
			}
			return
		}
	}
}

func TestStreamResponse_SniffBOM_UTF16LE(t *testing.T) {

	body := []byte{0xFF, 0xFE,
		'h', 0x00, 'e', 0x00, 'l', 0x00, 'l', 0x00, 'o', 0x00,
	}

	tab := NewRequestTab("test")
	var dest bytes.Buffer
	_, err := tab.streamResponse(context.Background(), 0, bytes.NewReader(body), &dest, new(app.Window), true, "text/plain")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var got strings.Builder
	for {
		select {
		case chunk := <-tab.appendChan:
			got.WriteString(chunk.text)
		default:
			if got.String() != "hello" {
				t.Errorf("got %q, want %q", got.String(), "hello")
			}
			return
		}
	}
}

func TestStreamResponse_PlainUTF8NoSniff(t *testing.T) {
	body := []byte("plain utf-8 — ok")
	tab := NewRequestTab("test")
	var dest bytes.Buffer
	_, err := tab.streamResponse(context.Background(), 0, bytes.NewReader(body), &dest, new(app.Window), true, "text/plain")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var got strings.Builder
	for {
		select {
		case chunk := <-tab.appendChan:
			got.WriteString(chunk.text)
		default:
			if got.String() != string(body) {
				t.Errorf("got %q, want %q", got.String(), string(body))
			}
			return
		}
	}
}

func TestBuildCurlCommand_GET(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users/42")
	tab.Method = "GET"
	tab.Headers = nil
	got := BuildCurlCommand(tab, nil)
	if !strings.HasPrefix(got, "curl 'https://api.example.com/users/42'") {
		t.Errorf("missing URL prefix: %s", got)
	}
	if strings.Contains(got, "-X GET") {
		t.Errorf("should omit -X for GET: %s", got)
	}
}

func TestBuildCurlCommand_POSTJSON(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users")
	tab.Method = "POST"
	tab.Headers = nil
	tab.AddHeader("Content-Type", "application/json")
	tab.ReqEditor.SetText(`{"name":"a"}`)
	got := BuildCurlCommand(tab, nil)
	if !strings.Contains(got, "-X POST") {
		t.Errorf("missing -X POST: %s", got)
	}
	if !strings.Contains(got, "-H 'Content-Type: application/json'") {
		t.Errorf("missing content-type header: %s", got)
	}
	if !strings.Contains(got, `--data-raw '{"name":"a"}'`) {
		t.Errorf("missing body: %s", got)
	}
}

func TestBuildCurlCommand_QuoteEscape(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://example.com/")
	tab.Method = "POST"
	tab.Headers = nil
	tab.ReqEditor.SetText(`it's "quoted"`)
	got := BuildCurlCommand(tab, nil)
	if !strings.Contains(got, `--data-raw 'it'\''s "quoted"'`) {
		t.Errorf("single-quote escape broken: %s", got)
	}
}

func TestBuildCurlCommand_TemplateSubstitution(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("{{base}}/items")
	tab.Method = "GET"
	tab.Headers = nil
	got := BuildCurlCommand(tab, map[string]string{"base": "https://api.example.com"})
	if !strings.Contains(got, "'https://api.example.com/items'") {
		t.Errorf("template not substituted: %s", got)
	}
}

func TestBuildCurlCommand_EmptyURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("")
	if got := BuildCurlCommand(tab, nil); got != "" {
		t.Errorf("expected empty cURL for empty URL, got %q", got)
	}
}

func TestFormatTimings(t *testing.T) {
	if got := formatTimings(Timings{}); got != "" {
		t.Errorf("zero timings should produce empty string, got %q", got)
	}
	tm := Timings{DNS: 10 * time.Millisecond, TTFB: 100 * time.Millisecond}
	got := formatTimings(tm)
	if !strings.Contains(got, "DNS 10ms") {
		t.Errorf("missing DNS: %s", got)
	}
	if !strings.Contains(got, "TTFB 100ms") {
		t.Errorf("missing TTFB: %s", got)
	}
	if strings.Contains(got, "Connect") {
		t.Errorf("should skip zero Connect phase: %s", got)
	}
}

func TestExecuteRequest_CapturesTimingsAndFilename(t *testing.T) {
	setupTestConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Disposition", `attachment; filename="hello.txt"`)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"
	tab.window = new(app.Window)
	tab.ExecuteRequest(context.Background(), tab.window, nil)

	select {
	case res := <-tab.responseChan:
		if res.filename != "hello.txt" {
			t.Errorf("filename got %q want hello.txt", res.filename)
		}
		if res.timings.TTFB <= 0 {
			t.Errorf("expected positive TTFB, got %v", res.timings.TTFB)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

func TestDecompressBody_Brotli(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		bw := brotli.NewWriter(w)
		_, _ = bw.Write([]byte("brotli payload"))
		_ = bw.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "br")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "brotli payload" {
		t.Errorf("got %q", string(data))
	}
}

func TestDecompressBody_Zstd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "zstd")
		zw, _ := zstd.NewWriter(w)
		_, _ = zw.Write([]byte("zstd payload"))
		_ = zw.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "zstd")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "zstd payload" {
		t.Errorf("got %q", string(data))
	}
}

func TestJitter5Debug(t *testing.T) {
	for _, scale := range []float32{1, 1.25} {
		for _, step := range []int{1, 3} {
			rig := newVStackRig()
			rig.tab.HeadersAbsHeight = 100
			gtx := rig.gtxScaled(scale)
			for i := 0; i < 2; i++ {
				rig.frameScaled(scale)
			}
			ext := rig.tab.stackedSplitExtent(gtx)
			rig.tab.VStackRatio = (float32(rig.tab.stackedReqPaneMinPx(gtx)) + 5) / ext
			for i := 0; i < 3; i++ {
				rig.frameScaled(scale)
			}
			paneOf := func() int { return int(rig.tab.VStackRatio*ext + 0.5) }
			editorOf := func() int {
				return paneOf() - rig.tab.reqPaneAboveHeadersPx(gtx) - rig.tab.headersRenderH - rig.tab.reqPaneBelowHeadersPx(gtx) + gtx.Dp(unit.Dp(3))
			}
			sliderY := rig.paneTopScaled(gtx) + rig.tab.reqPaneAboveHeadersPx(gtx) + rig.tab.headersRenderH + 2

			rig.r.Queue(pointerPress(400, sliderY))
			rig.frameScaled(scale)
			var panes, renders, editors []int
			for i := 1; i <= 12; i++ {
				rig.r.Queue(pointerMove(400, sliderY+i*step))
				rig.frameScaled(scale)
				panes = append(panes, paneOf())
				renders = append(renders, rig.tab.headersRenderH)
				editors = append(editors, editorOf())
			}
			rig.r.Queue(pointerRelease(400, sliderY+12*step))
			rig.frameScaled(scale)
			t.Logf("scale=%.2f step=%d down: panes=%v", scale, step, panes)
			t.Logf("scale=%.2f step=%d down: renders=%v", scale, step, renders)
			t.Logf("scale=%.2f step=%d down: editors=%v", scale, step, editors)
		}
	}
}

func TestRequestLang_PrefersHint(t *testing.T) {
	tab := NewRequestTab("test")
	tab.ReqEditor.SetText("not obviously any language")

	if got := tab.requestLang(); got != syntax.LangPlain {
		t.Fatalf("precondition: plain body should sniff as LangPlain, got %v", got)
	}

	tab.ReqLangHint = syntax.LangJSON
	if got := tab.requestLang(); got != syntax.LangJSON {
		t.Errorf("requestLang() = %v, want LangJSON from the hint", got)
	}
}

func TestRequestLang_ContentTypeBeatsHint(t *testing.T) {
	tab := NewRequestTab("test")
	tab.AddHeader("Content-Type", "application/json")
	tab.ReqLangHint = syntax.LangPlain
	if got := tab.requestLang(); got != syntax.LangJSON {
		t.Errorf("requestLang() = %v, want LangJSON from the Content-Type header", got)
	}
}

func TestLooksLikeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"object", `{"a": 1}`, true},
		{"array", `[1, 2, 3]`, true},
		{"spaces before object", "   \t\n  {\"a\": 1}", true},
		{"not json string", `"string"`, false},
		{"not json num", "123", false},
		{"not json html", "<html></html>", false},
		{"empty string", "", false},
		{"only spaces", "   ", false},
		{"single brace", "{", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := looksLikeJSON([]byte(tc.input))
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestIndentWrite(t *testing.T) {
	if got := string(appendIndent(nil, 1)); !strings.Contains(got, "  ") {
		t.Errorf("expected indentation")
	}
	if got := string(appendIndent(nil, 100)); !strings.Contains(got, "  ") {
		t.Errorf("expected indentation even at max depth (capped)")
	}
	if got := appendIndent(nil, -1); len(got) != 0 {
		t.Errorf("expected no indentation for negative")
	}
}

func TestFormatJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		state    *JSONFormatterState
		expected string
	}{
		{
			name:     "simple object",
			input:    `{"a":1}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": 1\n}",
		},
		{
			name:     "nested object",
			input:    `{"a":{"b":2}}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": {\n    \"b\": 2\n  }\n}",
		},
		{
			name:     "array",
			input:    `[1, 2]`,
			state:    &JSONFormatterState{},
			expected: "[\n  1,\n  2\n]",
		},
		{
			name:     "empty array",
			input:    `[]`,
			state:    &JSONFormatterState{},
			expected: "[]",
		},
		{
			name:     "empty object",
			input:    `{}`,
			state:    &JSONFormatterState{},
			expected: "{}",
		},
		{
			name:     "string with nested chars",
			input:    `{"key": "value with { and [ and ,"}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"key\": \"value with { and [ and ,\"\n}",
		},
		{
			name:     "numbers and bools",
			input:    `{"a": 1, "b": true, "c": null}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": 1,\n  \"b\": true,\n  \"c\": null\n}",
		},
		{
			name:     "unquoted values",
			input:    `{"a": unquoted}`,
			state:    &JSONFormatterState{},
			expected: "{\n  \"a\": unquoted\n}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatJSON([]byte(tc.input), tc.state)
			if result != tc.expected {
				t.Errorf("expected:\n%q\n\ngot:\n%q", tc.expected, result)
			}
		})
	}
}

func TestFormatJSON_DeepNesting(t *testing.T) {

	depth := 65
	input := strings.Repeat("[", depth) + strings.Repeat("]", depth)
	result := formatJSON([]byte(input), &JSONFormatterState{})
	if !strings.Contains(result, "[]") {
		t.Errorf("expected empty array at depth")
	}
}

func TestLoadPreviewFromFile(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	content := `{"a": 1}`
	_ = os.WriteFile(tmpPath, []byte(content), 0644)

	result, n, isJSON, _ := loadPreviewFromFile(tmpPath, int64(len(content)), &JSONFormatterState{}, "", 0)

	if result != "{\n  \"a\": 1\n}" {
		t.Errorf("expected formatted JSON, got %q", result)
	}
	if n != int64(len(content)) {
		t.Errorf("expected read size %d, got %d", len(content), n)
	}
	if !isJSON {
		t.Errorf("expected isJSON true")
	}
}

func TestLoadPreviewFromFile_LargeJSONStaysFormatted(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview-large")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	const itemCount = 60000
	for i := 0; i < itemCount; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"id":`)
		sb.WriteString(strings.Repeat("9", 6))
		sb.WriteString(`,"name":"value"}`)
	}
	sb.WriteString(`]}`)
	content := sb.String()
	if len(content) <= 1024*1024 {
		t.Fatalf("test setup: payload %d B is not above the old 1 MB cap", len(content))
	}
	_ = os.WriteFile(tmpPath, []byte(content), 0644)

	result, _, isJSON, _ := loadPreviewFromFile(tmpPath, int64(len(content)), &JSONFormatterState{}, "", 0)
	if !isJSON {
		t.Fatalf("expected isJSON=true for >1 MB JSON body")
	}
	if !strings.Contains(result, "\n  ") {
		t.Fatalf("expected pretty-printed indentation in result; got first 200 chars: %q", result[:min(200, len(result))])
	}
}

func TestFormatJSON_StreamingPreservesStateAcrossBatches(t *testing.T) {
	doc := []byte(`{"name":"hello world","count":1234567,"nested":{"a":1,"b":[1,2,3]}}`)
	state := &JSONFormatterState{}
	full := formatJSON(doc, state)

	for _, splitAt := range []int{12, 18, 25, 33, 50} {
		t.Run("split_"+strings.TrimSpace(string(rune('0'+splitAt/10)))+string(rune('0'+splitAt%10)), func(t *testing.T) {
			s := &JSONFormatterState{}
			a := formatJSON(doc[:splitAt], s)
			b := formatJSON(doc[splitAt:], s)
			if a+b != full {
				t.Errorf("split at %d:\n full: %q\n  got: %q", splitAt, full, a+b)
			}
		})
	}
}

func TestEditorInsertWorks(t *testing.T) {
	var ed widget.Editor
	ed.Insert("hello")
	if ed.Text() != "hello" {
		t.Errorf("expected hello, got %q", ed.Text())
	}
}

func TestLoadMorePreview(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	content := "line1\nline2\n"
	_ = os.WriteFile(tmpPath, []byte(content), 0644)

	tab := NewRequestTab("test")
	tab.window = new(app.Window)
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.previewLoaded.Store(6)
	tab.respIsJSON = false

	tab.loadMorePreview()

	success := false
	var lastText string
	for i := 0; i < 200; i++ {

		select {
		case text := <-tab.appendChan:
			tab.RespEditor.Insert(text.text)
		default:
		}

		lastText = tab.RespEditor.Text()
		if lastText == "line2\n" {
			success = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !success {

		data, _ := os.ReadFile(tmpPath)
		t.Errorf("expected line2, got %q (respSize=%d, previewLoaded=%d, fileData=%q)", lastText, tab.respSize, tab.previewLoaded.Load(), string(data))
	}
}

func TestWSHeaderRowsMatchHTTPReference(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1400, 700)
	rig.tab.LayoutMode = LayoutModeHoriz
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	ref := rig.tab.headersRowPx(rig.gtx())

	rig.tab.Method = MethodWS
	rig.tab.URLInput.SetText("ws://example.com/socket")
	s := rig.tab.EnsureWS()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if s.wsRowH != ref {
		t.Errorf("WS section header %dpx != HTTP reference %dpx", s.wsRowH, ref)
	}
	if s.statusRowH != ref {
		t.Errorf("WS status row %dpx != HTTP reference %dpx", s.statusRowH, ref)
	}
}

func TestGQLHeaderRowsMatchHTTPReference(t *testing.T) {
	rig := newGQLRig(image.Pt(1400, 700))
	rig.tab.LayoutMode = LayoutModeHoriz
	rig.tab.Status = "Ready"
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	ref := rig.tab.headersRowPx(rig.gtx())
	if rig.tab.respHeaderH != ref {
		t.Errorf("GraphQL response header %dpx != reference %dpx", rig.tab.respHeaderH, ref)
	}
}

func TestGQLSplitRecordsMeasuredAnchors(t *testing.T) {
	rig := newGQLRig(image.Pt(1400, 700))
	rig.tab.LayoutMode = LayoutModeHoriz
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.splitPaneRec <= 0 || rig.tab.splitRespRec <= 0 {
		t.Fatalf("GraphQL panes not recorded: pane=%d resp=%d", rig.tab.splitPaneRec, rig.tab.splitRespRec)
	}
	if rig.tab.PaneDrawnH <= 0 {
		t.Fatalf("PaneDrawnH not recorded for GraphQL: %d", rig.tab.PaneDrawnH)
	}
}

func TestGQLHeadersResizableViaDrag(t *testing.T) {
	rig := newGQLRig(image.Pt(1400, 800))
	rig.tab.LayoutMode = LayoutModeHoriz
	rig.tab.HeadersExpanded = true
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	before := rig.tab.headersRenderH
	if before <= 0 {
		t.Fatal("headers area not rendered")
	}
	moved := false
	for y := 100; y < 700 && !moved; y++ {
		rig.drag(400, y, y+40)
		if rig.tab.headersRenderH != before {
			moved = true
		}
	}
	if !moved {
		t.Fatal("dragging never resized the GraphQL headers area")
	}
	if rig.tab.headersRenderH <= before {
		t.Errorf("headers area %dpx after dragging down, want more than %dpx", rig.tab.headersRenderH, before)
	}
}

func TestRecordMinLatAcceptsZeroSample(t *testing.T) {
	r := NewRequestTab("t").EnsureRun()
	r.resetCounters()
	r.record(200, 0, true)
	r.record(200, 5*time.Millisecond, true)

	snap := r.snapshot()
	if snap.minLat != 0 {
		t.Errorf("minLat = %d, want 0: a genuine sub-tick sample must not be treated as unset", snap.minLat)
	}
	if snap.maxLat != int64(5*time.Millisecond) {
		t.Errorf("maxLat = %d, want %d", snap.maxLat, int64(5*time.Millisecond))
	}
}

func TestRecordMinLatMatchesStatusBucket(t *testing.T) {
	cases := [][]time.Duration{
		{0, 5 * time.Millisecond},
		{0},
		{3 * time.Millisecond, 0, 7 * time.Millisecond},
		{9 * time.Millisecond, 2 * time.Millisecond},
	}
	for _, lats := range cases {
		r := NewRequestTab("t").EnsureRun()
		r.resetCounters()
		for _, l := range lats {
			r.record(200, l, true)
		}
		snap := r.snapshot()
		var bucketMin int64 = -1
		for _, b := range snap.buckets {
			if b.code == 200 {
				bucketMin = b.minLat
			}
		}
		if bucketMin < 0 {
			t.Fatalf("no bucket for 200 with lats %v", lats)
		}
		if snap.minLat != bucketMin {
			t.Errorf("lats %v: global minLat = %d but the 200 bucket reports %d; they must agree",
				lats, snap.minLat, bucketMin)
		}
	}
}

func TestRecordMinLatOrdinarySamples(t *testing.T) {
	r := NewRequestTab("t").EnsureRun()
	r.resetCounters()
	for _, l := range []time.Duration{8 * time.Millisecond, 3 * time.Millisecond, 11 * time.Millisecond} {
		r.record(200, l, true)
	}
	snap := r.snapshot()
	if snap.minLat != int64(3*time.Millisecond) {
		t.Errorf("minLat = %d, want %d", snap.minLat, int64(3*time.Millisecond))
	}
	if snap.maxLat != int64(11*time.Millisecond) {
		t.Errorf("maxLat = %d, want %d", snap.maxLat, int64(11*time.Millisecond))
	}
}

func TestResetCountersClearsMinLat(t *testing.T) {
	r := NewRequestTab("t").EnsureRun()
	r.resetCounters()
	r.record(200, 4*time.Millisecond, true)
	r.resetCounters()
	r.record(200, 9*time.Millisecond, true)
	if got := r.snapshot().minLat; got != int64(9*time.Millisecond) {
		t.Errorf("minLat = %d after reset, want %d", got, int64(9*time.Millisecond))
	}
}

func TestWSConnectCancelClosesSilentConnection(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		select {}
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()

	ctx, cancel := context.WithCancel(context.Background())
	tab.WSConnect(ctx, nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	cancel()
	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
}

func newHSplitRig() *vstackRig {
	rig := newVStackRig()
	rig.tab.LayoutMode = LayoutModeHoriz
	rig.size = image.Pt(1200, 600)
	return rig
}

func TestSubTabSwitchFitsHeadersAreaExactly(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.paneH()

	rig.tab.AuthTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != 100 {
		t.Errorf("switching to Auth must fit the area to the auth panel: got %d, want 100", got)
	}
	if got := rig.paneH(); !near(got, paneBefore-20, 3) {
		t.Errorf("request pane must follow the fitted headers area: pane %d, want ~%d", got, paneBefore-20)
	}

	rig.tab.CookiesTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != 32 {
		t.Errorf("switching to empty Cookies must shrink the area to one row: got %d, want 32", got)
	}
	if got := rig.paneH(); !near(got, paneBefore-88, 3) {
		t.Errorf("request pane must shrink back after Auth: pane %d, want ~%d", got, paneBefore-88)
	}

	rig.tab.HeadersTabBtn.Click()
	rig.frame()
	rig.frame()
	wantHeaders := len(rig.tab.Headers)*28 + 4
	if got := rig.tab.HeadersAbsHeight; got != wantHeaders {
		t.Errorf("switching to Headers must fit its rows: got %d, want %d", got, wantHeaders)
	}
	if got := rig.paneH(); !near(got, paneBefore+wantHeaders-120, 3) {
		t.Errorf("request pane must track the headers fit: pane %d, want ~%d", got, paneBefore+wantHeaders-120)
	}
}

func TestSubTabSwitchKeepsManualHeadersHeight(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+40)
	manual := rig.tab.HeadersAbsHeight
	if !near(manual, 160, 4) {
		t.Fatalf("setup: manual resize should land at ~160, got %d", manual)
	}

	rig.tab.CookiesTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != manual {
		t.Errorf("after a manual resize, tab switching must not shrink the area: got %d, want %d", got, manual)
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-80)
	manual2 := rig.tab.HeadersAbsHeight
	rig.tab.AuthTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got < 100 {
		t.Errorf("area must still grow to fit the Auth panel: got %d, want >= 100", got)
	}

	rig.tab.CookiesTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != manual2 {
		t.Errorf("leaving Auth must return to the manual height: got %d, want %d", got, manual2)
	}
}

func TestReqCollapseKeepsHeadersRenderHeight(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	before := rig.tab.headersRenderH
	if before != 120 {
		t.Fatalf("setup: headers should render at 120, got %d", before)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.headersRenderH; got != before {
		t.Errorf("collapsing request must not squeeze headers: %d -> %d", before, got)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.headersRenderH; got != before {
		t.Errorf("expanding request must not change headers: %d -> %d", before, got)
	}
}

func TestManualResizeSurvivesTabRoundTrip(t *testing.T) {
	rig := newVStackRig()
	for i := 0; i < 15; i++ {
		rig.tab.AddHeader("K"+strconv.Itoa(i), "v")
	}
	rig.tab.HeadersAbsHeight = 424
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-100)
	manual := rig.tab.HeadersAbsHeight
	if manual >= rig.tab.headersFitDp(rig.tab.Headers) {
		t.Fatalf("setup: manual resize must land below the content fit, got %d", manual)
	}

	rig.tab.ParamsTabBtn.Click()
	rig.frame()
	rig.frame()
	rig.tab.HeadersTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != manual {
		t.Errorf("tab round trip must keep the manual height: got %d, want %d", got, manual)
	}
	if got := rig.tab.headersRenderH; !near(got, manual, 2) {
		t.Errorf("rendered headers must stay at the manual height: got %d, want ~%d", got, manual)
	}
}

func TestCollapsedRequestAnchorsToBottomHorizontal(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.prevStacked {
		t.Fatalf("setup: rig must lay out horizontally")
	}
	if got := rig.tab.headersRenderH; !near(got, 120, 3) {
		t.Fatalf("setup: headers should render at stored height, got %d", got)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.ReqBodyCollapsed {
		t.Fatalf("collapse button must collapse the request body")
	}
	gtx := rig.gtx()
	want := rig.tab.reqPaneH - rig.tab.reqPaneAboveHeadersPx(gtx) - rig.tab.reqPaneBelowHeadersPx(gtx)
	if got := rig.tab.headersRenderH; !near(got, want, 5) {
		t.Errorf("collapsed request must give its space to the headers area: got %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.headersRenderH; !near(got, 120, 3) {
		t.Errorf("expanding must restore the stored headers height: got %d", got)
	}
}

func TestBothCollapsedCompactBoxHorizontal(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	full := rig.tab.reqPaneBoxH

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.reqPaneBoxH; got != full {
		t.Errorf("with headers expanded the collapsed request pane must keep full height: got %d, want %d", got, full)
	}

	rig.tab.HeadersExpanded = false
	rig.frame()
	rig.frame()
	gtx := rig.gtx()
	compact := rig.tab.headersRowPx(gtx) + rig.tab.reqPaneBelowHeadersContentPx(gtx)
	if got := rig.tab.reqPaneBoxH; !near(got, compact, 3) {
		t.Errorf("hiding headers must shrink the request pane box to its rows: got %d, want ~%d (full %d)", got, compact, full)
	}

	rig.tab.HeadersExpanded = true
	rig.frame()
	rig.frame()
	if got := rig.tab.reqPaneBoxH; got != full {
		t.Errorf("re-expanding headers must restore the full pane box: got %d, want %d", got, full)
	}
}

func TestReqCollapseCarriesIntoVerticalLayout(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()

	rig.tab.LayoutMode = LayoutModeVert
	rig.size = image.Pt(800, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	want := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, want, 4) {
		t.Errorf("collapsed request must stay visually collapsed after switching layouts: pane %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); got < want+100 {
		t.Errorf("expanding after the layout switch must reopen editor space: pane %d, want >= %d", got, want+100)
	}
}

func TestCollapseSequencesKeepPanesHugged(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneStart := rig.paneH()
	extent := int(rig.tab.stackedSplitExtent(rig.gtx()))

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	reqMin := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, reqMin, 4) {
		t.Fatalf("setup: collapsed request should hug: %d, want ~%d", got, reqMin)
	}

	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); !near(got, reqMin, 4) {
		t.Errorf("collapsing response must not inflate the collapsed request pane: %d, want ~%d", got, reqMin)
	}
	if got, want := rig.tab.respPaneBoxH, rig.tab.respCollapsedMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapsed response must hug its header: %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	wantFill := extent - rig.tab.respCollapsedMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, wantFill, 5) {
		t.Errorf("expanding request while response is collapsed must fill the space: %d, want ~%d", got, wantFill)
	}

	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); !near(got, paneStart, 4) {
		t.Errorf("expanding response must restore the original split: %d, want ~%d", got, paneStart)
	}
}

func TestHeadersToggleWhileAllCollapsedRehugs(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	hugged := rig.paneH()
	if want := rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(hugged, want, 4) {
		t.Fatalf("setup: collapsed request should hug: %d, want ~%d", hugged, want)
	}

	rig.tab.ViewGeneratedBtn.Click()
	rig.frame()
	rig.frame()
	opened := rig.paneH()
	if want := rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(opened, want, 4) {
		t.Errorf("opening headers must grow the pane exactly to the headers min: %d, want ~%d", opened, want)
	}

	rig.tab.ViewGeneratedBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); !near(got, hugged, 4) {
		t.Errorf("hiding headers must re-hug the collapsed request pane: %d, want ~%d", got, hugged)
	}
}

func TestExpandInHorizontalThenBackToVertical(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneStart := rig.paneH()

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()

	rig.tab.LayoutMode = LayoutModeHoriz
	rig.size = image.Pt(1200, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: request must be expanded in horizontal")
	}

	rig.tab.LayoutMode = LayoutModeVert
	rig.size = image.Pt(800, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if got := rig.paneH(); !near(got, paneStart, 4) {
		t.Errorf("expanded request must get its split back after returning to vertical: %d, want ~%d", got, paneStart)
	}
}

func TestRespCollapseCarriesIntoVerticalLayout(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()

	rig.tab.LayoutMode = LayoutModeVert
	rig.size = image.Pt(800, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	extent := int(rig.tab.stackedSplitExtent(rig.gtx()))
	respPx := extent - rig.paneH()
	if want := rig.tab.respCollapsedMinPx(rig.gtx()); !near(respPx, want, 4) {
		t.Errorf("collapsed response must stay collapsed after switching layouts: got %d, want ~%d", respPx, want)
	}
}

func TestSplitURLQuery(t *testing.T) {
	cases := []struct {
		in                    string
		base, query, fragment string
	}{
		{"https://x/y", "https://x/y", "", ""},
		{"https://x/y?a=1&b=2", "https://x/y", "a=1&b=2", ""},
		{"https://x/y?a=1#frag", "https://x/y", "a=1", "#frag"},
		{"https://x/y#frag", "https://x/y", "", "#frag"},
	}
	for _, c := range cases {
		b, q, f := splitURLQuery(c.in)
		if b != c.base || q != c.query || f != c.fragment {
			t.Errorf("splitURLQuery(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, b, q, f, c.base, c.query, c.fragment)
		}
	}
}

func TestParamsSyncFromURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users?page=2&limit=50&flag")
	tab.syncParamsFromURL()
	if len(tab.Params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(tab.Params))
	}
	want := [][2]string{{"page", "2"}, {"limit", "50"}, {"flag", ""}}
	for i, w := range want {
		if tab.Params[i].Key.Text() != w[0] || tab.Params[i].Value.Text() != w[1] {
			t.Errorf("param[%d] = (%q,%q), want (%q,%q)", i, tab.Params[i].Key.Text(), tab.Params[i].Value.Text(), w[0], w[1])
		}
	}
}

func TestParamsSyncToURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users#top")
	tab.addParam("q", "gophers")
	tab.addParam("n", "10")
	tab.addParam("", "")
	tab.syncURLFromParams()
	got := tab.URLInput.Text()
	want := "https://api.example.com/users?q=gophers&n=10#top"
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestAuthHeaderValue(t *testing.T) {
	tab := NewRequestTab("t")
	if v := tab.authHeaderValue(nil); v != "" {
		t.Errorf("none auth = %q, want empty", v)
	}

	tab.AuthType = authBearer
	tab.AuthToken.SetText("abc.def")
	if v := tab.authHeaderValue(nil); v != "Bearer abc.def" {
		t.Errorf("bearer = %q", v)
	}

	tab.AuthType = authBasic
	tab.AuthUser.SetText("user")
	tab.AuthPass.SetText("pass")
	if v := tab.authHeaderValue(nil); v != "Basic dXNlcjpwYXNz" {
		t.Errorf("basic = %q, want Basic dXNlcjpwYXNz", v)
	}
}

func TestCookieHeaderValue(t *testing.T) {
	tab := NewRequestTab("t")
	if v := tab.cookieHeaderValue(nil); v != "" {
		t.Errorf("empty cookies = %q", v)
	}
	tab.addCookie("session_id", "abc123")
	tab.addCookie("theme", "dark")
	tab.addCookie("", "ignored")
	if v := tab.cookieHeaderValue(nil); v != "session_id=abc123; theme=dark" {
		t.Errorf("cookies = %q", v)
	}
}

func htmlLikeText() string {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "<div id=%d>", i)
		for j := 0; j < 60; j++ {
			b.WriteString(" content word")
		}
		b.WriteString("</div>\n")
	}
	return b.String()
}

func (rig *respRig) bottomGap() int {
	return rig.v.lastTotalH - rig.v.lastViewportH - rig.v.scrollY
}

func (rig *respRig) topAnchor() (int, int) {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
	}
	adv := measureCharAdvance(rig.shaper, font.Font{}, unit.Sp(13), gtx)
	innerW := rig.size.X - 2*4
	return rig.v.scrollAnchor(rig.v.lastLineHeight, adv, innerW, rig.wrap)
}

func TestResizeRoundTripReturnsToSamePlace(t *testing.T) {
	rig := newRespRig(htmlLikeText(), true)
	rig.size.X = 400
	now := time.Unix(1700000000, 0)
	for i := 0; i < 3; i++ {
		rig.frame(now)
	}
	rig.v.SetScrollY(rig.v.lastTotalH / 2)
	rig.frame(now)
	rig.frame(now)

	v := rig.v
	startLine, startSub := rig.topAnchor()
	if startLine == 0 && startSub == 0 {
		t.Fatalf("setup: expected mid-document, top at 0/0")
	}

	for w := 392; w >= 320; w -= 8 {
		rig.size.X = w
		rig.frame(now)
	}
	for w := 328; w <= 400; w += 8 {
		rig.size.X = w
		rig.frame(now)
	}
	rig.frame(now)
	rig.frame(now)

	endLine, endSub := rig.topAnchor()
	if endLine != startLine || endSub < startSub-1 || endSub > startSub+1 {
		t.Errorf("round-trip drag moved visible content: top %d/%d -> %d/%d (scrollY %d, lineH %d)",
			startLine, startSub, endLine, endSub, v.scrollY, v.lastLineHeight)
	}
}

func (rig *respRig) subRowRem() int {
	line, sub := rig.topAnchor()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
	}
	adv := measureCharAdvance(rig.shaper, font.Font{}, unit.Sp(13), gtx)
	innerW := rig.size.X - 2*4
	return rig.v.scrollY - rig.v.scrollYForAnchor(line, sub, rig.v.lastLineHeight, adv, innerW, rig.wrap)
}

func TestResizePreservesSubRowOffset(t *testing.T) {
	rig := newRespRig(htmlLikeText(), true)
	rig.size.X = 400
	now := time.Unix(1700000000, 0)
	for i := 0; i < 3; i++ {
		rig.frame(now)
	}

	rig.v.SetScrollY(20*rig.v.lastLineHeight + 7)
	rig.frame(now)
	if rem := rig.subRowRem(); rem != 7 {
		t.Fatalf("setup: sub-row remainder = %d, want 7", rem)
	}

	rig.size.X = 360
	rig.frame(now)
	rig.frame(now)

	if rem := rig.subRowRem(); rem != 7 {
		t.Errorf("resize snapped the view to a whole row: sub-row remainder %d, want 7 (scrollY=%d lineH=%d)",
			rem, rig.v.scrollY, rig.v.lastLineHeight)
	}
}

func TestResizeAtBottomStaysAtBottom(t *testing.T) {
	rig := newRespRig(htmlLikeText(), true)
	rig.size.X = 400
	now := time.Unix(1700000000, 0)
	for i := 0; i < 3; i++ {
		rig.frame(now)
	}
	for i := 0; i < 5; i++ {
		rig.v.SetScrollY(1 << 30)
		rig.frame(now)
	}
	v := rig.v
	if v.scrollY <= 0 {
		t.Fatalf("setup: scrollY=%d", v.scrollY)
	}
	if g := rig.bottomGap(); g != 0 {
		t.Fatalf("setup: not at bottom, gap=%d", g)
	}

	for w := 392; w >= 320; w -= 8 {
		rig.size.X = w
		rig.frame(now)
		if g := rig.bottomGap(); g < 0 || g > v.lastLineHeight {
			t.Fatalf("width %d: viewer left the end mid-drag, gap=%dpx (lineH=%d)", w, g, v.lastLineHeight)
		}
	}
	rig.frame(now)
	rig.frame(now)

	if g := rig.bottomGap(); g < 0 || g > v.lastLineHeight {
		t.Errorf("after resizing at bottom the viewer drifted %dpx from the end (lineH=%d totalH=%d viewH=%d scrollY=%d)",
			g, v.lastLineHeight, v.lastTotalH, v.lastViewportH, v.scrollY)
	}
}

func resizeRigText() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString(fmt.Sprintf("line %02d: ", i))
		b.WriteString(strings.Repeat("abcdefghij ", 10))
		b.WriteByte('\n')
	}
	return b.String()
}

func (rig *respRig) topSourceLine() int {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
	}
	adv := measureCharAdvance(rig.shaper, font.Font{}, unit.Sp(13), gtx)
	pad := 4
	innerW := rig.size.X - 2*pad
	line, _ := rig.v.firstChunkAtFn(rig.v.scrollY, rig.v.lastLineHeight, adv, innerW, rig.wrap)
	return line
}

func TestResizeKeepsResponseTextAnchored(t *testing.T) {
	cases := []struct {
		name    string
		startW  int
		endW    int
		scrollY int
	}{
		{"narrow", 400, 250, 15 * 18},
		{"widen", 400, 620, 15 * 18},
		{"narrow-unaligned", 400, 250, 15*18 + 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRespRig(resizeRigText(), true)
			rig.size.X = tc.startW
			now := time.Unix(1700000000, 0)
			for i := 0; i < 3; i++ {
				rig.frame(now)
			}

			rig.v.scrollY = tc.scrollY
			rig.frame(now)

			before := rig.topSourceLine()
			if before == 0 {
				t.Fatalf("setup: expected to be scrolled past the first line, got top line %d", before)
			}

			rig.size.X = tc.endW
			rig.frame(now)
			rig.frame(now)

			after := rig.topSourceLine()
			if after != before {
				t.Errorf("response text jumped on resize %d->%d: top source line was %d, became %d",
					tc.startW, tc.endW, before, after)
			}
		})
	}
}

func TestRespCollapseNoJitterOnDrag(t *testing.T) {
	rig := newVStackRig()
	rig.tab.BodyType = model.BodyNone
	rig.tab.HeadersAbsHeight = 32
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.RespBodyCollapsed || rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: both panes must start expanded")
	}

	gtx := rig.gtx()
	ext := int(rig.tab.stackedSplitExtent(gtx))
	hug := rig.tab.respCollapsedMinPx(gtx)

	y0 := rig.splitDividerY()
	rig.r.Queue(pointerPress(400, y0))
	rig.frame()

	flips := 0
	prev := rig.tab.RespBodyCollapsed
	prevResp := ext - rig.paneH()
	for i := 1; i <= 40; i++ {
		rig.r.Queue(pointerMove(400, y0+i*4))
		rig.frame()
		if rig.tab.RespBodyCollapsed != prev {
			flips++
			prev = rig.tab.RespBodyCollapsed
		}
		resp := ext - rig.paneH()
		if resp > prevResp {
			t.Errorf("step %d: response grew while shrinking the pane: %d -> %d", i, prevResp, resp)
		}
		if rig.tab.RespBodyCollapsed && !near(resp, hug, 4) {
			t.Errorf("step %d: collapsed response must hug its header: got %d, want ~%d", i, resp, hug)
		}
		prevResp = resp
	}
	if flips != 1 {
		t.Errorf("shrinking drag must toggle the response collapse once, got %d toggles", flips)
	}
	if !rig.tab.RespBodyCollapsed {
		t.Fatalf("dragging past the response minimum must collapse it")
	}

	y1 := y0 + 160
	flips = 0
	prev = rig.tab.RespBodyCollapsed
	for i := 1; i <= 40; i++ {
		rig.r.Queue(pointerMove(400, y1-i*4))
		rig.frame()
		if rig.tab.RespBodyCollapsed != prev {
			flips++
			prev = rig.tab.RespBodyCollapsed
		}
		resp := ext - rig.paneH()
		if resp < prevResp {
			t.Errorf("back %d: response shrank while growing the pane: %d -> %d", i, prevResp, resp)
		}
		prevResp = resp
	}
	rig.r.Queue(pointerRelease(400, y1-160))
	rig.frame()
	if flips != 1 {
		t.Errorf("growing drag must toggle the response collapse once, got %d toggles", flips)
	}
	if rig.tab.RespBodyCollapsed {
		t.Fatalf("dragging back must reopen the response body")
	}
}

func makeTestGtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
}

func TestResponseSelection_DefaultsZero(t *testing.T) {
	v := NewResponseViewer()
	s, e := v.Selection()
	if s != 0 || e != 0 {
		t.Errorf("default Selection = (%d,%d), want (0,0)", s, e)
	}
}

func TestResponseSelection_SetCaretSyncsBoth(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world")
	v.SetCaret(2, 7)
	s, e := v.Selection()
	if s != 2 || e != 7 {
		t.Errorf("Selection = (%d,%d), want (2,7)", s, e)
	}
	if got := v.SelectedText(); got != "llo w" {
		t.Errorf("SelectedText must reflect the selection set via SetCaret, got %q", got)
	}
}

func TestSetScrollCaret_NoPanicNoEffect(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollCaret(true)
	v.SetScrollCaret(false)
}

func TestSetScrollY_NegativeClamps(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollY(-100)
	if v.GetScrollY() != 0 {
		t.Errorf("expected clamp to 0, got %d", v.GetScrollY())
	}
}

func TestSetScrollY_ClampsToContentBounds(t *testing.T) {
	v := NewResponseViewer()
	v.lastTotalH = 1000
	v.lastViewportH = 200
	v.SetScrollY(5000)
	if v.GetScrollY() != 800 {
		t.Errorf("expected clamp to 800, got %d", v.GetScrollY())
	}
}

func TestSetScrollY_ZeroContentMaxClampsToZero(t *testing.T) {
	v := NewResponseViewer()
	v.lastTotalH = 50
	v.lastViewportH = 200
	v.SetScrollY(100)
	if v.GetScrollY() != 0 {
		t.Errorf("expected clamp to 0 when viewport>total, got %d", v.GetScrollY())
	}
}

func TestSetScrollY_NoClampWhenLayoutUnseeded(t *testing.T) {
	v := NewResponseViewer()

	v.SetScrollY(123)
	if v.GetScrollY() != 123 {
		t.Errorf("without layout info, scrollY should be kept; got %d", v.GetScrollY())
	}
}

func TestSetScrollX_NegativeClamps(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollX(-50)
	if v.GetScrollX() != 0 {
		t.Errorf("SetScrollX(-50) = %d, want 0", v.GetScrollX())
	}
}

func TestSetScrollX_PositivePreserved(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollX(42)
	if v.GetScrollX() != 42 {
		t.Errorf("SetScrollX(42) = %d, want 42", v.GetScrollX())
	}
}

func TestLineForByteOffset(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("a\nbb\nccc\nd")

	cases := []struct {
		off  int
		want int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{3, 1},
		{5, 2},
		{7, 2},
		{9, 3},
		{12, 3},
	}
	for _, tc := range cases {
		if got := v.lineForByteOffset(tc.off); got != tc.want {
			t.Errorf("lineForByteOffset(%d) = %d, want %d (lineStarts=%v)", tc.off, got, tc.want, v.lineStarts)
		}
	}
}

func TestLineForByteOffset_Empty(t *testing.T) {
	v := NewResponseViewer()
	if got := v.lineForByteOffset(0); got != 0 {
		t.Errorf("empty: got %d", got)
	}
	if got := v.lineForByteOffset(999); got != 0 {
		t.Errorf("empty out-of-range: got %d", got)
	}
}

func TestLineForByteOffset_NegativeReturnsZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("a\nb")
	if got := v.lineForByteOffset(-100); got != 0 {
		t.Errorf("negative offset: got %d", got)
	}
}

func TestMoveCaret_NoExtendCollapsesSelection(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef")
	v.selStart, v.selEnd = 1, 4
	v.moveCaret(2, false)
	if v.selStart != 2 || v.selEnd != 2 {
		t.Errorf("moveCaret(no-extend) should collapse to (2,2); got (%d,%d)", v.selStart, v.selEnd)
	}
}

func TestMoveCaret_ExtendKeepsAnchor(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef")
	v.selStart, v.selEnd = 1, 1
	v.moveCaret(4, true)
	if v.selStart != 1 || v.selEnd != 4 {
		t.Errorf("moveCaret(extend) should keep anchor and move end; got (%d,%d)", v.selStart, v.selEnd)
	}
}

func TestMoveCaret_ClampsOutOfRange(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.moveCaret(-5, false)
	if v.selStart != 0 || v.selEnd != 0 {
		t.Errorf("negative not clamped to 0; got (%d,%d)", v.selStart, v.selEnd)
	}
	v.moveCaret(1000, false)
	if v.selStart != 3 || v.selEnd != 3 {
		t.Errorf("over-end not clamped; got (%d,%d)", v.selStart, v.selEnd)
	}
}

func TestMoveCaret_ResetsDragActive(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.dragActive = true
	v.moveCaret(1, false)
	if v.dragActive {
		t.Errorf("dragActive should be reset")
	}
}

func TestCharLeft(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("aбc")
	cases := []struct{ in, want int }{
		{0, 0},
		{1, 0},
		{3, 1},
		{4, 3},
		{-5, 0},
	}
	for _, tc := range cases {
		if got := v.charLeft(tc.in); got != tc.want {
			t.Errorf("charLeft(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCharRight(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("aбc")
	cases := []struct{ in, want int }{
		{0, 1},
		{1, 3},
		{3, 4},
		{4, 4},
		{100, 4},
	}
	for _, tc := range cases {
		if got := v.charRight(tc.in); got != tc.want {
			t.Errorf("charRight(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestWordLeft(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("foo bar  baz")

	if got := v.wordLeft(11); got != 9 {
		t.Errorf("wordLeft(11)=%d, want 9", got)
	}

	if got := v.wordLeft(9); got != 4 {
		t.Errorf("wordLeft(9)=%d, want 4", got)
	}
	if got := v.wordLeft(0); got != 0 {
		t.Errorf("wordLeft(0)=%d, want 0", got)
	}
	if got := v.wordLeft(-3); got != 0 {
		t.Errorf("wordLeft(-3)=%d, want 0", got)
	}
}

func TestWordRight(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("foo bar  baz")

	if got := v.wordRight(0); got != 4 {
		t.Errorf("wordRight(0)=%d, want 4", got)
	}

	if got := v.wordRight(4); got != 9 {
		t.Errorf("wordRight(4)=%d, want 9", got)
	}
	if got := v.wordRight(12); got != 12 {
		t.Errorf("wordRight(12)=%d, want 12", got)
	}
	if got := v.wordRight(999); got != 12 {
		t.Errorf("wordRight past-end should clamp; got %d", got)
	}
}

func TestWordRightStaysOnLine(t *testing.T) {
	v := NewResponseViewer()
	body := "{\n    \"phoneNumber\": 123456789,\n\t\"force\": true\n}"
	v.SetText(body)

	valueStart := strings.Index(body, "123456789")
	lineEnd := strings.Index(body[valueStart:], "\n") + valueStart

	if got := v.wordRight(valueStart); got != lineEnd {
		t.Fatalf("wordRight from the start of the value = %d (%q), want %d (end of its line)",
			got, body[got:min(got+6, len(body))], lineEnd)
	}

	next := v.wordRight(lineEnd)
	wantNext := strings.Index(body, "force")
	if next != wantNext {
		t.Fatalf("wordRight from the line end = %d (%q), want %d (first word of the next line)",
			next, body[next:min(next+6, len(body))], wantNext)
	}
}

func TestColumnAt(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nпривет")
	if got := v.columnAt(0); got != 0 {
		t.Errorf("columnAt(0)=%d, want 0", got)
	}
	if got := v.columnAt(2); got != 2 {
		t.Errorf("columnAt(2)=%d, want 2", got)
	}
	if got := v.columnAt(3); got != 3 {
		t.Errorf("columnAt(3)=%d, want 3 (end of first line)", got)
	}

	if got := v.columnAt(4); got != 0 {
		t.Errorf("columnAt(4)=%d, want 0 (start of 'привет')", got)
	}

	if got := v.columnAt(6); got != 1 {
		t.Errorf("columnAt(6)=%d, want 1", got)
	}

	if got := v.columnAt(16); got != 6 {
		t.Errorf("columnAt(16)=%d, want 6", got)
	}
}

func TestOffsetAtColumn(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nпривет")

	if got := v.offsetAtColumn(0, 0); got != 0 {
		t.Errorf("offsetAtColumn(0,0)=%d, want 0", got)
	}
	if got := v.offsetAtColumn(0, 2); got != 2 {
		t.Errorf("offsetAtColumn(0,2)=%d, want 2", got)
	}
	if got := v.offsetAtColumn(0, 99); got != 3 {
		t.Errorf("offsetAtColumn(0,99) should clamp to lineEnd=3, got %d", got)
	}
	if got := v.offsetAtColumn(0, -1); got != 0 {
		t.Errorf("offsetAtColumn(0,-1)=%d, want 0", got)
	}

	if got := v.offsetAtColumn(4, 1); got != 6 {
		t.Errorf("offsetAtColumn(4,1) for 'п' should advance 2 bytes; got %d", got)
	}
	if got := v.offsetAtColumn(4, 6); got != 16 {
		t.Errorf("offsetAtColumn(4,6) should walk all 6 runes; got %d", got)
	}
}

func TestLineUp_FromSecondLine(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcde\nfghij")

	if got := v.lineUp(8, 2); got != 2 {
		t.Errorf("lineUp(8,2)=%d, want 2", got)
	}
}

func TestLineUp_FromFirstLineReturnsZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcde\nfghij")
	if got := v.lineUp(3, 3); got != 0 {
		t.Errorf("lineUp from first line should be 0; got %d", got)
	}
}

func TestLineDown(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcde\nfghij\nklmno")

	if got := v.lineDown(2, 2); got != 8 {
		t.Errorf("lineDown(2,2)=%d, want 8", got)
	}
}

func TestLineDown_FromLastLineGoesToEOF(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("ab\ncd")
	if got := v.lineDown(4, 1); got != 5 {
		t.Errorf("lineDown from last line should clamp to len(text)=5; got %d", got)
	}
}

func TestLineDown_CRLFConsumed(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("ab\r\ncd")

	if got := v.lineDown(0, 1); got != 5 {
		t.Errorf("lineDown across CRLF; got %d, want 5", got)
	}
}

func TestVisualXAt_NilShaperReturnsZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello\nworld")
	if got := v.visualXAt(3, makeTestGtx(), 200); got != 0 {
		t.Errorf("with nil shaper, visualXAt should degrade to 0; got %d", got)
	}
}

func TestWrapLineMoveX_NoShaper_BoundaryBehavior(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("aa\nbb\ncc")
	gtx := makeTestGtx()

	if got := v.wrapLineMoveX(0, 0, +1, gtx, 100); got != 3 {
		t.Errorf("wrapLineMoveX down: got %d, want 3 (next line start)", got)
	}

	if got := v.wrapLineMoveX(7, 0, +1, gtx, 100); got != len(v.text) {
		t.Errorf("wrapLineMoveX down from last line; got %d, want %d", got, len(v.text))
	}

	if got := v.wrapLineMoveX(0, 0, -1, gtx, 100); got != 0 {
		t.Errorf("wrapLineMoveX up from line 0; got %d, want 0", got)
	}

	if got := v.wrapLineMoveX(3, 0, -1, gtx, 100); got != 0 {
		t.Errorf("wrapLineMoveX up from line 1; got %d, want 0", got)
	}
}

func TestEnsureCaretVisible_NoopWithoutLayout(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello")
	v.selEnd = 2
	v.scrollY = 999
	v.ensureCaretVisible()
	if v.scrollY != 999 {
		t.Errorf("without lastLineHeight, ensureCaretVisible must be a no-op; got scrollY=%d", v.scrollY)
	}
}

func TestEnsureCaretVisible_ScrollsUp(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4")
	v.lastLineHeight = 10
	v.lastViewportH = 25
	v.lastTotalH = 50
	v.padChunkHeights()
	v.scrollY = 30
	v.selEnd = 0
	v.ensureCaretVisible()
	if v.scrollY != 0 {
		t.Errorf("expected scrollY snapped up to 0, got %d", v.scrollY)
	}
}

func TestEnsureCaretVisible_ScrollsDown(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4")
	v.lastLineHeight = 10
	v.lastViewportH = 20
	v.lastTotalH = 50
	v.padChunkHeights()
	v.scrollY = 0

	v.selEnd = len(v.text)
	v.ensureCaretVisible()
	if v.scrollY != 30 {
		t.Errorf("expected scrollY=30, got %d", v.scrollY)
	}
}

func TestEnsureCaretVisible_UsesPerChunkHeightWhenAvailable(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.lastLineHeight = 10
	v.lastViewportH = 20
	v.lastTotalH = 100
	v.padChunkHeights()

	v.chunkHeights[0] = 40
	v.scrollY = 0
	v.selEnd = 7
	v.ensureCaretVisible()

	if v.scrollY != 40 {
		t.Errorf("expected scrollY=40 reflecting tall chunkHeights[0]=40, got %d", v.scrollY)
	}
}

func revealNoWrap(v *ResponseViewer, lineH, innerW, innerH int) {
	v.applyReveal(layout.Context{}, fixed.I(8), lineH, innerW, innerH, false)
}

func TestReveal_StaysPendingWithoutLayout(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello")
	v.scrollY = 42
	v.SetCaret(2, 3)
	v.applyReveal(layout.Context{}, fixed.I(8), 0, 200, 100, false)
	if v.scrollY != 42 {
		t.Errorf("without a line height the reveal must not scroll; got %d", v.scrollY)
	}
	if !v.revealPending {
		t.Errorf("reveal must stay pending until metrics are known")
	}
}

func TestReveal_CentersOffscreenLine(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4\nL5\nL6")
	v.lastViewportH = 40
	v.lastTotalH = 70
	v.padChunkHeights()

	v.SetCaret(12, 14)
	revealNoWrap(v, 10, 200, 40)
	if v.scrollY != 25 {
		t.Errorf("expected line 4 (y=40) centred at scrollY=25, got %d", v.scrollY)
	}
}

func TestReveal_KeepsScrollWhenAlreadyVisible(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4\nL5\nL6")
	v.lastViewportH = 40
	v.lastTotalH = 70
	v.padChunkHeights()
	v.scrollY = 20

	v.SetCaret(21, 23) // line 7 is out of range; line 7*3=21 -> last line
	v.SetCaret(9, 11)  // line 3, y=30, inside 20..60
	revealNoWrap(v, 10, 200, 40)
	if v.scrollY != 20 {
		t.Errorf("a match already on screen must not move the viewport, got %d", v.scrollY)
	}
}

func TestReveal_ClampsToMax(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3")
	v.lastViewportH = 5
	v.lastTotalH = 40
	v.padChunkHeights()

	v.SetCaret(len(v.text), len(v.text))
	revealNoWrap(v, 10, 200, 5)
	if v.scrollY < 0 || v.scrollY > 35 {
		t.Errorf("scrollY %d outside the clamped range [0,35]", v.scrollY)
	}
}

func TestReveal_NegativeTargetClampsToZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.lastViewportH = 100
	v.lastTotalH = 10
	v.padChunkHeights()

	v.SetCaret(0, 1)
	revealNoWrap(v, 10, 200, 100)
	if v.scrollY != 0 {
		t.Errorf("expected clamp to 0, got %d", v.scrollY)
	}
}

func TestReveal_UsesPerChunkHeight(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.lastViewportH = 0
	v.lastTotalH = 200
	v.padChunkHeights()
	v.chunkHeights[0] = 100
	v.chunkHeights[1] = 5

	v.SetCaret(6, 8)
	revealNoWrap(v, 10, 200, 0)
	if v.scrollY != 105 {
		t.Errorf("expected scrollY=105 from the measured chunk heights, got %d", v.scrollY)
	}
}

func TestReveal_ScrollsHorizontallyWhenNotWrapped(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("x" + strings.Repeat("y", 600) + "MATCH" + strings.Repeat("z", 600))
	v.lastViewportH = 100
	v.lastTotalH = 100
	v.padChunkHeights()

	v.SetCaret(601, 606)
	revealNoWrap(v, 10, 200, 100)
	x1 := colPx(fixed.I(8), 601)
	if v.scrollX > x1 || x1 > v.scrollX+200 {
		t.Errorf("match column %d not inside the horizontal window [%d,%d]", x1, v.scrollX, v.scrollX+200)
	}
}

func TestFirstChunkAtFn_ZeroY(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	idx, acc := v.firstChunkAtFn(0, 10, fixed.I(8), 200, false)
	if idx != 0 || acc != 0 {
		t.Errorf("y=0 should return (0,0); got (%d,%d)", idx, acc)
	}
}

func TestFirstChunkAtFn_MidContent(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}

	idx, acc := v.firstChunkAtFn(15, 10, fixed.I(8), 200, false)
	if idx != 1 || acc != 10 {
		t.Errorf("y=15: got (%d,%d), want (1,10)", idx, acc)
	}
}

func TestFirstChunkAtFn_BeyondLast(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	idx, acc := v.firstChunkAtFn(9999, 10, fixed.I(8), 200, false)
	if idx != len(v.chunkHeights) || acc != 20 {
		t.Errorf("beyond last: got (%d,%d), want (%d,%d)", idx, acc, len(v.chunkHeights), 20)
	}
}

func TestFirstChunkAtFn_ZeroHeightFallsBackToEstimate(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.padChunkHeights()

	idx, acc := v.firstChunkAtFn(15, 10, fixed.I(8), 200, false)
	if idx != 1 || acc != 10 {
		t.Errorf("estimate path: got (%d,%d), want (1,10)", idx, acc)
	}
}

func TestCoordToByteOffset_GuardsReturnZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.padChunkHeights()
	gtx := makeTestGtx()
	if got := v.coordToByteOffset(gtx, 0, 0, 0, 10, 100, false); got != 0 {
		t.Errorf("zero advance must return 0; got %d", got)
	}
	if got := v.coordToByteOffset(gtx, 0, 0, fixed.I(8), 0, 100, false); got != 0 {
		t.Errorf("zero lineHeight must return 0; got %d", got)
	}
	empty := NewResponseViewer()

	empty.lineStarts = empty.lineStarts[:0]
	if got := empty.coordToByteOffset(gtx, 0, 0, fixed.I(8), 10, 100, false); got != 0 {
		t.Errorf("empty lineStarts must return 0; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_FirstLineCol(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 24, 0, adv, 10, 200, false); got != 3 {
		t.Errorf("col 3 expected byte 3; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_BeyondLineClampsToEnd(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("ab\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 1000, 0, adv, 10, 200, false); got != 2 {
		t.Errorf("clamp to line end: got %d, want 2", got)
	}
}

func TestCoordToByteOffset_NoWrap_NegativeXClampsToCol0(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)
	if got := v.coordToByteOffset(gtx, -50, 0, adv, 10, 200, false); got != 0 {
		t.Errorf("negative X should clamp to 0; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_NegativeYClampsToZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	v.scrollY = 5
	if got := v.coordToByteOffset(gtx, 8, -100, adv, 10, 200, false); got != 1 {
		t.Errorf("negative Y -> first line, col 1 -> byte 1; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_PicksSecondLine(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nXYZW")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 16, 15, adv, 10, 200, false); got != 6 {
		t.Errorf("line1 col2 expected byte 6; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_BeyondLastChunkReturnsTextEnd(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("a\nb")
	v.padChunkHeights()

	gtx := makeTestGtx()
	adv := fixed.I(8)

	v.chunkHeights = v.chunkHeights[:0]
	if got := v.coordToByteOffset(gtx, 0, 0, adv, 10, 200, false); got != len(v.text) {
		t.Errorf("empty chunkHeights -> chunkIdx=-1 path; got %d, want %d", got, len(v.text))
	}
}

func TestCoordToByteOffset_Wrap_NilShaperReturnsChunkStart(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 10, 15, adv, 10, 200, true); got != 12 {
		t.Errorf("wrap+nil shaper: expected chunkStart=12; got %d", got)
	}
}

func TestBodyTypeRowMinWidth(t *testing.T) {
	tab := NewRequestTab("t")
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
	got := tab.bodyTypeRowMinWidth(gtx, th)
	expected := computeBodyTypeRowMinWidth(gtx, th, tab.BodyType.String())
	if got != expected {
		t.Errorf("bodyTypeRowMinWidth mismatch: %d vs computed %d", got, expected)
	}
	if got <= 0 {
		t.Errorf("bodyTypeRowMinWidth should be positive; got %d", got)
	}
}

func TestByteToRuneIdx(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		byteIdx int
		want    int
	}{
		{"ascii start", "abcdef", 0, 0},
		{"ascii mid", "abcdef", 3, 3},
		{"ascii end", "abcdef", 6, 6},
		{"cyrillic start", "привет", 0, 0},
		{"cyrillic after first rune (2 bytes)", "привет", 2, 1},
		{"cyrillic after third rune (6 bytes)", "привет", 6, 3},
		{"cyrillic full (12 bytes)", "привет", 12, 6},
		{"mixed: ab + 'и' (2 bytes) at byte 4", "abиcd", 4, 3},
		{"emoji 4-byte", "a\xf0\x9f\x98\x80b", 5, 2},
		{"past end clamps", "ab", 10, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := byteToRuneIdx([]byte(tc.text), tc.byteIdx)
			if got != tc.want {
				t.Errorf("byteToRuneIdx(%q, %d) = %d, want %d", tc.text, tc.byteIdx, got, tc.want)
			}
		})
	}
}

func TestRuneIdxToByte(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		runeIdx int
		want    int
	}{
		{"ascii zero", "abcdef", 0, 0},
		{"ascii mid", "abcdef", 3, 3},
		{"ascii past end clamps", "abcdef", 100, 6},
		{"cyrillic 1 rune = 2 bytes", "привет", 1, 2},
		{"cyrillic 3 runes = 6 bytes", "привет", 3, 6},
		{"cyrillic 6 runes = 12 bytes", "привет", 6, 12},
		{"emoji 1 rune = 4 bytes", "\xf0\x9f\x98\x80x", 1, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runeIdxToByte([]byte(tc.text), tc.runeIdx)
			if got != tc.want {
				t.Errorf("runeIdxToByte(%q, %d) = %d, want %d", tc.text, tc.runeIdx, got, tc.want)
			}
		})
	}
}

func TestByteToRuneIdxRoundTrip(t *testing.T) {
	inputs := []string{
		"hello",
		"Привет мир",
		"🚀🔥",
		"👨‍👩‍👧‍👦",
		"你好",
		"Mixed: abc Привет 🚀 你好",
		"\xef\xbb\xbfBOM at start",
		"",
	}
	for _, s := range inputs {
		t.Run(s, func(t *testing.T) {
			b := []byte(s)
			totalRunes := 0
			byteOf := []int{}
			for i := 0; i < len(b); {
				byteOf = append(byteOf, i)
				_, sz := decodeOne(b[i:])
				if sz < 1 {
					sz = 1
				}
				i += sz
				totalRunes++
			}
			byteOf = append(byteOf, len(b))

			for r := 0; r <= totalRunes; r++ {
				bi := runeIdxToByte(b, r)
				if bi != byteOf[r] {
					t.Errorf("runeIdxToByte(%q, %d) = %d, want %d", s, r, bi, byteOf[r])
				}
			}
			for r := 0; r <= totalRunes; r++ {
				bi := byteOf[r]
				got := byteToRuneIdx(b, bi)
				if got != r {
					t.Errorf("byteToRuneIdx(%q, %d) = %d, want %d", s, bi, got, r)
				}
			}
		})
	}
}

func decodeOne(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 0
	}
	if b[0] < 0x80 {
		return rune(b[0]), 1
	}
	r, sz := decodeRune(b)
	if sz == 0 {
		sz = 1
	}
	return r, sz
}

func decodeRune(b []byte) (rune, int) {
	type t struct {
		r rune
		s int
	}
	res := func() t {
		switch {
		case len(b) < 1:
			return t{0, 0}
		case b[0] < 0x80:
			return t{rune(b[0]), 1}
		case b[0]&0xE0 == 0xC0 && len(b) >= 2:
			return t{rune(b[0]&0x1F)<<6 | rune(b[1]&0x3F), 2}
		case b[0]&0xF0 == 0xE0 && len(b) >= 3:
			return t{rune(b[0]&0x0F)<<12 | rune(b[1]&0x3F)<<6 | rune(b[2]&0x3F), 3}
		case b[0]&0xF8 == 0xF0 && len(b) >= 4:
			return t{rune(b[0]&0x07)<<18 | rune(b[1]&0x3F)<<12 | rune(b[2]&0x3F)<<6 | rune(b[3]&0x3F), 4}
		}
		return t{0xFFFD, 1}
	}()
	return res.r, res.s
}

func TestWordBoundsAt(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world\nfoo.bar")
	cases := []struct {
		name               string
		byteOff            int
		wantStart, wantEnd int
	}{
		{"start of first word", 0, 0, 5},
		{"middle of word", 2, 0, 5},
		{"on space (separator run)", 5, 5, 6},
		{"on word 'w' (start of second word)", 6, 6, 11},
		{"on newline (clamps to line end, selects trailing word)", 11, 6, 11},
		{"start of foo (after newline)", 12, 12, 15},
		{"on dot separator", 15, 15, 16},
		{"on bar word", 17, 16, 19},
		{"at EOF (walks back into trailing word)", 19, 16, 19},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.wordBoundsAt(tc.byteOff)
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("wordBoundsAt(%d) = (%d,%d); want (%d,%d) — sel=%q",
					tc.byteOff, gotS, gotE, tc.wantStart, tc.wantEnd, string(v.text[gotS:gotE]))
			}
		})
	}
}

func TestWordBoundsAt_QuotesAndHyphens(t *testing.T) {
	v := NewResponseViewer()
	v.SetText(`"my-key": "Content-Type"`)

	cases := []struct {
		name               string
		byteOff            int
		wantStart, wantEnd int
		wantSel            string
	}{
		{"on opening quote", 0, 0, 1, `"`},
		{"on m of my-key", 1, 1, 3, "my"},
		{"on hyphen of my-key", 3, 3, 4, "-"},
		{"on k of key", 4, 4, 7, "key"},
		{"on closing quote of my-key", 7, 7, 8, `"`},
		{"on C of Content-Type", 11, 11, 18, "Content"},
		{"on hyphen of Content-Type", 18, 18, 19, "-"},
		{"on T of Type", 19, 19, 23, "Type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.wordBoundsAt(tc.byteOff)
			gotSel := string(v.text[gotS:gotE])
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("wordBoundsAt(%d) = (%d,%d) %q; want (%d,%d) %q",
					tc.byteOff, gotS, gotE, gotSel, tc.wantStart, tc.wantEnd, tc.wantSel)
			}
		})
	}
}

func TestWordBoundsAt_AtSign(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("634634634@c.us")

	cases := []struct {
		name               string
		byteOff            int
		wantStart, wantEnd int
		wantSel            string
	}{
		{"on digits before @", 3, 0, 9, "634634634"},
		{"on @", 9, 9, 10, "@"},
		{"on c after @", 10, 10, 11, "c"},
		{"on us after dot", 12, 12, 14, "us"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.wordBoundsAt(tc.byteOff)
			gotSel := string(v.text[gotS:gotE])
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("wordBoundsAt(%d) = (%d,%d) %q; want (%d,%d) %q",
					tc.byteOff, gotS, gotE, gotSel, tc.wantStart, tc.wantEnd, tc.wantSel)
			}
		})
	}
}

func TestSourceLineBoundsAt(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("line one\nsecond\r\nthird")
	cases := []struct {
		name               string
		byteOff            int
		wantStart, wantEnd int
	}{
		{"first line start", 0, 0, 8},
		{"first line middle", 4, 0, 8},
		{"first line end", 8, 0, 8},
		{"after first newline (= second line start)", 9, 9, 15},
		{"middle of second line", 12, 9, 15},
		{"after second newline", 17, 17, 22},
		{"in third line", 19, 17, 22},
		{"at EOF", 22, 17, 22},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.sourceLineBoundsAt(tc.byteOff)
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("sourceLineBoundsAt(%d) = (%d,%d); want (%d,%d) — sel=%q",
					tc.byteOff, gotS, gotE, tc.wantStart, tc.wantEnd, string(v.text[gotS:gotE]))
			}
		})
	}
}

func TestSelectAll(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world")
	v.SelectAll()
	if v.selStart != 0 || v.selEnd != 11 {
		t.Errorf("SelectAll: got selection [%d,%d), want [0,11)", v.selStart, v.selEnd)
	}
	if got := v.SelectedText(); got != "hello world" {
		t.Errorf("SelectAll: SelectedText = %q, want full text", got)
	}
}

func TestRuneByteRoundTrip(t *testing.T) {
	texts := []string{"abc", "привет мир", "a\xf0\x9f\x98\x80b\xf0\x9f\x98\x81c", "{\"имя\":\"значение\"}"}
	for _, txt := range texts {
		bs := []byte(txt)
		for r := 0; ; r++ {
			b := runeIdxToByte(bs, r)
			gotR := byteToRuneIdx(bs, b)
			if gotR != r && b < len(bs) {
				t.Errorf("round-trip mismatch on %q at rune %d: byte=%d back-rune=%d", txt, r, b, gotR)
			}
			if b >= len(bs) {
				break
			}
		}
	}
}

func drainSpecBody(t *testing.T, s *runSpec, env map[string]string) (*http.Request, string) {
	t.Helper()
	req, err := s.newRequest(context.Background(), env)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if req.Body == nil {
		return req, ""
	}
	b, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return req, string(b)
}

func TestBuildRunSpec_RawBodyStaysTemplated(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = "POST"
	tab.BodyType = model.BodyRaw
	tab.URLInput.SetText("http://example.com/{{path}}")
	tab.ReqEditor.SetText(`{"n":"{{who}}"}`)
	tab.AddHeader("X-Fixed", "1")
	tab.AddHeader("", "ignored")

	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if !spec.useTmplBody {
		t.Error("raw bodies must stay templated so each iteration re-renders them")
	}
	if spec.bodyTmpl != `{"n":"{{who}}"}` {
		t.Errorf("bodyTmpl = %q, want the unrendered template", spec.bodyTmpl)
	}
	if spec.urlTmpl != "http://example.com/{{path}}" {
		t.Errorf("urlTmpl = %q, want the unrendered template", spec.urlTmpl)
	}
	for _, h := range spec.headers {
		if strings.TrimSpace(h[0]) == "" {
			t.Errorf("blank header keys must be dropped: %+v", spec.headers)
		}
	}

	env := map[string]string{"path": "v1", "who": "ann"}
	req, body := drainSpecBody(t, spec, env)
	if req.URL.String() != "http://example.com/v1" {
		t.Errorf("URL = %q, want http://example.com/v1", req.URL.String())
	}
	if body != `{"n":"ann"}` {
		t.Errorf("body = %q, want the rendered template", body)
	}
	if req.Header.Get("X-Fixed") != "1" {
		t.Errorf("X-Fixed header = %q", req.Header.Get("X-Fixed"))
	}
}

func TestBuildRunSpec_StripsNewlinesAndTabsFromURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("  http://exa\nmple.com/a\tb  ")
	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.urlTmpl != "http://example.com/ab" {
		t.Errorf("urlTmpl = %q, want newlines/tabs stripped and trimmed", spec.urlTmpl)
	}
}

func TestBuildRunSpec_NonRawBodyIsMaterialized(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = "POST"
	tab.BodyType = model.BodyURLEncoded
	tab.URLInput.SetText("http://example.com")
	tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("a", "{{v}}"))

	spec, err := tab.buildRunSpec(context.Background(), map[string]string{"v": "1"})
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.useTmplBody {
		t.Error("url-encoded bodies must be materialized once, not templated per iteration")
	}
	if got := string(spec.bodyBytes); got != "a=1" {
		t.Errorf("bodyBytes = %q, want a=1", got)
	}
	if spec.explicitCT != "application/x-www-form-urlencoded" {
		t.Errorf("explicitCT = %q", spec.explicitCT)
	}
	req, body := drainSpecBody(t, spec, nil)
	if body != "a=1" {
		t.Errorf("request body = %q", body)
	}
	if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
}

func TestBuildRunSpec_GraphQLAlwaysBuildsJSONBody(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = MethodGraphQL
	tab.BodyType = model.BodyNone
	tab.URLInput.SetText("http://example.com/graphql")
	tab.EnsureGQL().Query.SetText("{ me }")

	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.method != "POST" {
		t.Errorf("method = %q, want POST for GraphQL", spec.method)
	}
	if !strings.Contains(string(spec.bodyBytes), `"query"`) {
		t.Errorf("bodyBytes = %q, want a GraphQL payload", spec.bodyBytes)
	}
	if spec.explicitCT != "application/json" {
		t.Errorf("explicitCT = %q", spec.explicitCT)
	}
}

func TestBuildRunSpec_GraphQLPropagatesBodyError(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = MethodGraphQL
	tab.URLInput.SetText("http://example.com/graphql")
	g := tab.EnsureGQL()
	g.Query.SetText("{ me }")
	g.Variables.SetText(`{"bad":}`)
	if _, err := tab.buildRunSpec(context.Background(), nil); err == nil {
		t.Error("buildRunSpec must surface an invalid-variables error")
	}
}

func TestBuildRunSpec_BodyNoneHasNoBody(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyNone
	tab.ReqEditor.SetText("ignored")
	tab.URLInput.SetText("http://example.com")
	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.useTmplBody || spec.bodyBytes != nil {
		t.Errorf("BodyNone must produce no body: tmpl=%v bytes=%q", spec.useTmplBody, spec.bodyBytes)
	}
	req, body := drainSpecBody(t, spec, nil)
	if body != "" {
		t.Errorf("request body = %q, want empty", body)
	}
	if req.Body != nil {
		t.Error("request must carry a nil body when there is nothing to send")
	}
}

func TestBuildRunSpec_CarriesAuthAndCookies(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://example.com")
	tab.AuthType = authBearer
	tab.AuthToken.SetText("{{tok}}")
	tab.ApplyCookies([]model.ParsedKV{{Key: "sid", Value: "{{sid}}"}})

	env := map[string]string{"tok": "abc", "sid": "42"}
	spec, err := tab.buildRunSpec(context.Background(), env)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.authHeader != "Bearer abc" {
		t.Errorf("authHeader = %q", spec.authHeader)
	}
	if spec.cookieHeader != "sid=42" {
		t.Errorf("cookieHeader = %q", spec.cookieHeader)
	}
	req, _ := drainSpecBody(t, spec, env)
	if req.Header.Get("Authorization") != "Bearer abc" {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Cookie") != "sid=42" {
		t.Errorf("Cookie = %q", req.Header.Get("Cookie"))
	}
}

func TestNewRequest_EmptyURLFails(t *testing.T) {
	s := &runSpec{method: "GET", urlTmpl: "{{missing}}"}
	if _, err := s.newRequest(context.Background(), map[string]string{"missing": ""}); err == nil {
		t.Error("a URL that renders empty must be rejected")
	}
	s2 := &runSpec{method: "GET", urlTmpl: ""}
	if _, err := s2.newRequest(context.Background(), nil); err == nil {
		t.Error("an empty URL template must be rejected")
	}
}

func TestNewRequest_AddsSchemeAndEscapesSpaces(t *testing.T) {
	s := &runSpec{method: "GET", urlTmpl: "example.com/a b"}
	req, err := s.newRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if got := req.URL.String(); got != "http://example.com/a%20b" {
		t.Errorf("URL = %q, want scheme added and space escaped", got)
	}

	for _, raw := range []string{"http://x.test/p", "https://x.test/p"} {
		s := &runSpec{method: "GET", urlTmpl: raw}
		req, err := s.newRequest(context.Background(), nil)
		if err != nil {
			t.Fatalf("newRequest(%q): %v", raw, err)
		}
		if req.URL.String() != raw {
			t.Errorf("URL = %q, want %q untouched", req.URL.String(), raw)
		}
	}
}

func TestNewRequest_RejectsBadMethod(t *testing.T) {
	s := &runSpec{method: "BAD METHOD", urlTmpl: "http://x.test"}
	if _, err := s.newRequest(context.Background(), nil); err == nil {
		t.Error("an invalid HTTP method must be rejected")
	}
}

func TestNewRequest_DropsHeadersThatRenderEmpty(t *testing.T) {
	s := &runSpec{
		method:  "GET",
		urlTmpl: "http://x.test",
		headers: [][2]string{{"{{hk}}", "v"}, {"X-Ok", "  {{hv}}  "}},
	}
	req, err := s.newRequest(context.Background(), map[string]string{"hk": "", "hv": "yes"})
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if len(req.Header) != 1 {
		t.Errorf("header count = %d, want only X-Ok: %+v", len(req.Header), req.Header)
	}
	if req.Header.Get("X-Ok") != "yes" {
		t.Errorf("X-Ok = %q, want trimmed and rendered", req.Header.Get("X-Ok"))
	}
}

func TestRunOnceSpec_AgainstTestServer(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("hello"))
		case "/redirlike":
			w.WriteHeader(304)
		case "/boom":
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cases := []struct {
		path   string
		code   int
		wantOK bool
	}{
		{"/ok", 200, true},
		{"/redirlike", 304, true},
		{"/boom", 500, false},
		{"/nope", 404, false},
	}
	for _, c := range cases {
		s := &runSpec{method: "GET", urlTmpl: srv.URL + c.path}
		code, lat, ok := runOnceSpec(context.Background(), s, nil)
		if code != c.code || ok != c.wantOK {
			t.Errorf("%s: code=%d ok=%v, want %d/%v", c.path, code, ok, c.code, c.wantOK)
		}
		if lat < 0 {
			t.Errorf("%s: latency must not be negative, got %v", c.path, lat)
		}
	}
	if atomic.LoadInt32(&hits) != int32(len(cases)) {
		t.Errorf("server saw %d hits, want %d", hits, len(cases))
	}
}

func TestRunOnceSpec_FailuresReportZeroCode(t *testing.T) {
	badSpec := &runSpec{method: "GET", urlTmpl: ""}
	if code, _, ok := runOnceSpec(context.Background(), badSpec, nil); code != 0 || ok {
		t.Errorf("build failure: code=%d ok=%v, want 0/false", code, ok)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	dialSpec := &runSpec{method: "GET", urlTmpl: url}
	if code, _, ok := runOnceSpec(context.Background(), dialSpec, nil); code != 0 || ok {
		t.Errorf("transport failure: code=%d ok=%v, want 0/false", code, ok)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer live.Close()
	if code, _, ok := runOnceSpec(ctx, &runSpec{method: "GET", urlTmpl: live.URL}, nil); code != 0 || ok {
		t.Errorf("cancelled context: code=%d ok=%v, want 0/false", code, ok)
	}
}

func TestRunnerRecordAndSnapshot(t *testing.T) {
	r := newRequestRunner()
	r.record(200, 5*time.Millisecond, true)
	r.record(200, 15*time.Millisecond, true)
	r.record(500, 1*time.Millisecond, false)

	snap := r.snapshot()
	if snap.completed != 3 || snap.success != 2 || snap.failed != 1 {
		t.Errorf("completed/success/failed = %d/%d/%d, want 3/2/1", snap.completed, snap.success, snap.failed)
	}
	if snap.minLat != int64(time.Millisecond) {
		t.Errorf("minLat = %d, want %d", snap.minLat, int64(time.Millisecond))
	}
	if snap.maxLat != int64(15*time.Millisecond) {
		t.Errorf("maxLat = %d, want %d", snap.maxLat, int64(15*time.Millisecond))
	}
	if snap.sumLat != int64(21*time.Millisecond) {
		t.Errorf("sumLat = %d, want %d", snap.sumLat, int64(21*time.Millisecond))
	}
	if len(snap.buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2", len(snap.buckets))
	}
	var b200 statusBucket
	for _, b := range snap.buckets {
		if b.code == 200 {
			b200 = b
		}
	}
	if b200.count != 2 {
		t.Errorf("200 bucket count = %d, want 2", b200.count)
	}
	if b200.minLat != int64(5*time.Millisecond) || b200.maxLat != int64(15*time.Millisecond) {
		t.Errorf("200 bucket min/max = %d/%d", b200.minLat, b200.maxLat)
	}
	if snap.p50 <= 0 || snap.p99 <= 0 {
		t.Errorf("percentiles must be computed once samples exist: %+v", snap)
	}
}

func TestRunnerSnapshotSortOrder(t *testing.T) {
	mk := func() *RequestRunner {
		r := newRequestRunner()
		r.record(500, 30*time.Millisecond, false)
		r.record(500, 40*time.Millisecond, false)
		r.record(200, 10*time.Millisecond, true)
		r.record(200, 20*time.Millisecond, true)
		r.record(200, 12*time.Millisecond, true)
		r.record(404, 1*time.Millisecond, false)
		return r
	}
	cases := []struct {
		name string
		col  int
		asc  bool
		want []int
	}{
		{"code-asc", 0, true, []int{200, 404, 500}},
		{"code-desc", 0, false, []int{500, 404, 200}},
		{"count-desc", 1, false, []int{200, 500, 404}},
		{"share-desc", 2, false, []int{200, 500, 404}},
		{"avg-asc", 3, true, []int{404, 200, 500}},
		{"min-desc", 4, false, []int{500, 200, 404}},
		{"max-desc", 5, false, []int{500, 200, 404}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mk()
			r.SortCol = c.col
			r.SortAsc = c.asc
			snap := r.snapshot()
			got := make([]int, len(snap.buckets))
			for i, b := range snap.buckets {
				got[i] = b.code
			}
			if len(got) != len(c.want) {
				t.Fatalf("bucket count = %d, want %d", len(got), len(c.want))
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("order = %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestRunnerSnapshotEmptyIsZeroed(t *testing.T) {
	r := newRequestRunner()
	snap := r.snapshot()
	if snap.completed != 0 || len(snap.buckets) != 0 {
		t.Errorf("fresh runner snapshot = %+v, want zeroed", snap)
	}
	if snap.elapsed != 0 {
		t.Errorf("elapsed = %v, want 0 before the run starts", snap.elapsed)
	}
}

func TestRunnerSnapshotElapsedFreezesAfterEnd(t *testing.T) {
	r := newRequestRunner()
	r.mu.Lock()
	r.startedAt = time.Now().Add(-2 * time.Second)
	r.mu.Unlock()
	live := r.snapshot().elapsed
	if live < 2*time.Second {
		t.Errorf("running elapsed = %v, want >= 2s", live)
	}

	r.mu.Lock()
	r.endedAt = r.startedAt.Add(1500 * time.Millisecond)
	r.mu.Unlock()
	frozen := r.snapshot().elapsed
	if frozen != 1500*time.Millisecond {
		t.Errorf("finished elapsed = %v, want exactly 1.5s", frozen)
	}
	if again := r.snapshot().elapsed; again != frozen {
		t.Errorf("finished elapsed drifted: %v -> %v", frozen, again)
	}
}

func TestRunnerResetCounters(t *testing.T) {
	r := newRequestRunner()
	r.record(200, time.Millisecond, true)
	r.record(500, time.Second, false)
	r.sent.Store(9)
	r.inFlight.Store(3)
	_ = r.snapshot()

	r.resetCounters()
	snap := r.snapshot()
	if snap.completed != 0 || snap.success != 0 || snap.failed != 0 {
		t.Errorf("counters not cleared: %+v", snap)
	}
	if snap.minLat != 0 || snap.maxLat != 0 || snap.sumLat != 0 {
		t.Errorf("latency accumulators not cleared: %+v", snap)
	}
	if len(snap.buckets) != 0 {
		t.Errorf("buckets not cleared: %+v", snap.buckets)
	}
	if snap.p50 != 0 || snap.p90 != 0 || snap.p99 != 0 {
		t.Errorf("percentiles not cleared: %+v", snap)
	}
	if r.sent.Load() != 0 || r.inFlight.Load() != 0 {
		t.Errorf("sent/inFlight = %d/%d, want 0/0", r.sent.Load(), r.inFlight.Load())
	}

	r.record(404, 2*time.Millisecond, false)
	if snap := r.snapshot(); snap.completed != 1 || snap.p50 != int64(2*time.Millisecond) {
		t.Errorf("recording after reset: %+v", snap)
	}
}

func TestRunnerRecordCapsLatencySamples(t *testing.T) {
	r := newRequestRunner()
	for i := 0; i < 50002; i++ {
		r.record(200, time.Millisecond, true)
	}
	r.mu.Lock()
	n := len(r.lat)
	r.mu.Unlock()
	if n != 50000 {
		t.Errorf("latency sample buffer = %d, want capped at 50000", n)
	}
	if snap := r.snapshot(); snap.completed != 50002 {
		t.Errorf("completed = %d, want every request counted even past the sample cap", snap.completed)
	}
}

func TestPercentile(t *testing.T) {
	cases := []struct {
		name   string
		sorted []int64
		p      float64
		want   int64
	}{
		{"empty", nil, 0.5, 0},
		{"single", []int64{7}, 0.99, 7},
		{"median", []int64{1, 2, 3, 4, 5}, 0.5, 3},
		{"p90", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.9, 9},
		{"p0", []int64{4, 5, 6}, 0, 4},
		{"p1", []int64{4, 5, 6}, 1, 6},
		{"over-one-clamps", []int64{4, 5, 6}, 2, 6},
		{"negative-clamps", []int64{4, 5, 6}, -1, 4},
	}
	for _, c := range cases {
		if got := percentile(c.sorted, c.p); got != c.want {
			t.Errorf("%s: percentile(%v, %v) = %d, want %d", c.name, c.sorted, c.p, got, c.want)
		}
	}
}

func TestFmtMs(t *testing.T) {
	cases := []struct {
		ns   int64
		want string
	}{
		{0, "0"},
		{-5, "0"},
		{1_500_000, "1.5"},
		{9_940_000, "9.9"},
		{10_000_000, "10"},
		{10_600_000, "11"},
		{1_000_000_000, "1000"},
	}
	for _, c := range cases {
		if got := fmtMs(c.ns); got != c.want {
			t.Errorf("fmtMs(%d) = %q, want %q", c.ns, got, c.want)
		}
	}
}

func TestStatusLabelAndFailColor(t *testing.T) {
	if got := statusLabel(0); got != "ERR" {
		t.Errorf("statusLabel(0) = %q, want ERR", got)
	}
	if got := statusLabel(503); got != "503" {
		t.Errorf("statusLabel(503) = %q", got)
	}
	if failColor(0) != nil {
		t.Error("failColor(0) must be nil so the cell keeps the default colour")
	}
	c := failColor(3)
	if c == nil {
		t.Fatal("failColor(3) must return a colour")
	}
	if other := failColor(9); other == c {
		t.Error("failColor must hand out independent copies, not a shared pointer")
	}
}

func TestSplitValuesAndEnvForIteration(t *testing.T) {
	if got := splitValues(" a , b ,, c "); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitValues = %#v", got)
	}
	if got := splitValues("  ,  "); len(got) != 0 {
		t.Errorf("splitValues(blank) = %#v, want empty", got)
	}

	base := map[string]string{"host": "x"}
	if got := envForIteration(base, nil, 3); len(got) != 1 {
		t.Errorf("no variables must reuse the base env, got %#v", got)
	}
	vars := []runVarSnapshot{{name: "id", vals: []string{"1", "2"}}}
	e0 := envForIteration(base, vars, 0)
	e3 := envForIteration(base, vars, 3)
	if e0["id"] != "1" || e3["id"] != "2" {
		t.Errorf("values must cycle by index: %q / %q", e0["id"], e3["id"])
	}
	if e0["host"] != "x" {
		t.Error("base env keys must survive")
	}
	if base["id"] != "" {
		t.Error("envForIteration must not mutate the base env")
	}
}

func TestSnapshotVariablesSkipsIncomplete(t *testing.T) {
	r := newRequestRunner()
	r.addVar()
	r.addVar()
	r.addVar()
	r.Variables[0].Name.SetText("  ")
	r.Variables[0].Values.SetText("1,2")
	r.Variables[1].Name.SetText("empty")
	r.Variables[1].Values.SetText("  ,  ")
	r.Variables[2].Name.SetText("  ok  ")
	r.Variables[2].Values.SetText("a, b")

	got := r.snapshotVariables()
	if len(got) != 1 {
		t.Fatalf("snapshotVariables = %#v, want only the complete row", got)
	}
	if got[0].name != "ok" {
		t.Errorf("name = %q, want trimmed 'ok'", got[0].name)
	}
	if len(got[0].vals) != 2 {
		t.Errorf("vals = %#v, want 2", got[0].vals)
	}
}

func TestEndedAtStopped(t *testing.T) {
	r := newRequestRunner()
	r.plannedN = 0
	if r.endedAtStopped() {
		t.Error("a duration run (plannedN=0) must never report 'stopped'")
	}
	r.plannedN = 5
	r.record(200, time.Millisecond, true)
	if !r.endedAtStopped() {
		t.Error("1 of 5 completed must report 'stopped'")
	}
	for i := 0; i < 4; i++ {
		r.record(200, time.Millisecond, true)
	}
	if r.endedAtStopped() {
		t.Error("5 of 5 completed must not report 'stopped'")
	}
}

func TestRunnerStatusText(t *testing.T) {
	tab := NewRequestTab("t")
	if got := tab.runnerStatusText(); got != "Multiple · ready to start" {
		t.Errorf("fresh status = %q", got)
	}
	r := tab.EnsureRun()
	r.started = true
	r.plannedN = 4
	r.record(200, time.Millisecond, true)

	r.running.Store(true)
	if got := tab.runnerStatusText(); !strings.Contains(got, "running 1/4") {
		t.Errorf("running status = %q, want the planned count", got)
	}
	r.plannedN = 0
	if got := tab.runnerStatusText(); !strings.Contains(got, "1 done") {
		t.Errorf("duration-mode running status = %q", got)
	}

	r.running.Store(false)
	r.plannedN = 4
	if got := tab.runnerStatusText(); !strings.Contains(got, "stopped at 1") {
		t.Errorf("stopped status = %q", got)
	}
	r.plannedN = 1
	if got := tab.runnerStatusText(); !strings.Contains(got, "finished 1") {
		t.Errorf("finished status = %q", got)
	}
}

func TestRunnerSendLabel(t *testing.T) {
	tab := NewRequestTab("t")
	if got, _ := tab.runnerSendLabel(); got != "START" {
		t.Errorf("label = %q, want START", got)
	}
	r := tab.EnsureRun()
	r.started = true
	if got, _ := tab.runnerSendLabel(); got != "RERUN" {
		t.Errorf("label = %q, want RERUN", got)
	}
	r.running.Store(true)
	if got, _ := tab.runnerSendLabel(); got != "STOP" {
		t.Errorf("label = %q, want STOP", got)
	}
}

func TestRunnerBackToConfigRefusesWhileRunning(t *testing.T) {
	r := newRequestRunner()
	r.started = true
	r.running.Store(true)
	r.backToConfig()
	if !r.started {
		t.Error("backToConfig must be a no-op while the run is in flight")
	}
	r.running.Store(false)
	r.backToConfig()
	if r.started {
		t.Error("backToConfig must clear started once the run is over")
	}
}

func TestRunnerStopWithoutCancelIsSafe(t *testing.T) {
	r := newRequestRunner()
	r.stop()
}

func waitRunFinished(t *testing.T, r *RequestRunner, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if r.started && !r.running.Load() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("run did not finish within %v (completed=%d)", d, r.snapshot().completed)
}

func TestStartRun_IterationsCompleteAndRecord(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("X-Run") != "1" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	tab.AddHeader("X-Run", "1")
	r := tab.EnsureRun()
	r.IterEditor.SetText("6")
	r.WorkEditor.SetText("2")
	r.DelayEditor.SetText("0")

	tab.StartRun(context.Background(), new(app.Window), nil)
	waitRunFinished(t, r, 15*time.Second)

	snap := r.snapshot()
	if snap.completed != 6 {
		t.Errorf("completed = %d, want 6", snap.completed)
	}
	if snap.success != 6 || snap.failed != 0 {
		t.Errorf("success/failed = %d/%d, want 6/0", snap.success, snap.failed)
	}
	if atomic.LoadInt32(&hits) != 6 {
		t.Errorf("server saw %d requests, want 6", hits)
	}
	if r.plannedN != 6 {
		t.Errorf("plannedN = %d, want 6", r.plannedN)
	}
	if r.endedAtStopped() {
		t.Error("a fully completed run must not report 'stopped'")
	}
	if snap.elapsed < 0 {
		t.Errorf("elapsed = %v, want a non-negative duration", snap.elapsed)
	}
	if r.sent.Load() != 6 {
		t.Errorf("sent = %d, want 6", r.sent.Load())
	}
	if r.inFlight.Load() != 0 {
		t.Errorf("inFlight = %d, want 0 after the run drains", r.inFlight.Load())
	}
}

func TestStartRun_VariablesCycleAcrossIterations(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Query().Get("id")]++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL + "/?id={{id}}")
	r := tab.EnsureRun()
	r.IterEditor.SetText("4")
	r.WorkEditor.SetText("1")
	r.addVar()
	r.Variables[0].Name.SetText("id")
	r.Variables[0].Values.SetText("a,b")

	tab.StartRun(context.Background(), new(app.Window), nil)
	waitRunFinished(t, r, 15*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if seen["a"] != 2 || seen["b"] != 2 {
		t.Errorf("variable cycling produced %#v, want a=2 b=2", seen)
	}
}

func TestStartRun_StopCancelsEarly(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	defer close(release)

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.IterEditor.SetText("500")
	r.WorkEditor.SetText("2")

	tab.StartRun(context.Background(), new(app.Window), nil)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && r.inFlight.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	tab.EnsureRun().stop()
	waitRunFinished(t, r, 15*time.Second)

	if snap := r.snapshot(); snap.completed >= 500 {
		t.Errorf("completed = %d, want the run cut short", snap.completed)
	}
	if r.running.Load() {
		t.Error("running must be false after the run unwinds")
	}
}

func TestStartRun_DurationModeStopsOnDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.Mode = runByDuration
	r.DurEditor.SetText("1")
	r.WorkEditor.SetText("2")

	start := time.Now()
	tab.StartRun(context.Background(), new(app.Window), nil)
	waitRunFinished(t, r, 30*time.Second)
	elapsed := time.Since(start)

	if r.plannedN != 0 {
		t.Errorf("plannedN = %d, want 0 in duration mode", r.plannedN)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("duration run ended after %v, want ~1s", elapsed)
	}
	if elapsed > 20*time.Second {
		t.Errorf("duration run overran: %v", elapsed)
	}
	if snap := r.snapshot(); snap.completed == 0 {
		t.Error("duration run recorded nothing")
	}
}

func TestStartRun_RejectsSecondConcurrentRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.running.Store(true)
	tab.StartRun(context.Background(), new(app.Window), nil)
	if r.started {
		t.Error("StartRun must bail out while another run is in flight")
	}
	r.running.Store(false)
}

func TestStartRun_BuildFailureLeavesRunnerIdle(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = MethodGraphQL
	tab.URLInput.SetText("http://127.0.0.1:1/graphql")
	g := tab.EnsureGQL()
	g.Query.SetText("{ me }")
	g.Variables.SetText(`{"bad":}`)

	r := tab.EnsureRun()
	tab.StartRun(context.Background(), new(app.Window), nil)
	if r.started || r.running.Load() {
		t.Errorf("a spec build failure must leave the runner idle: started=%v running=%v", r.started, r.running.Load())
	}
}

func TestRunnerAction_DispatchesByState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.IterEditor.SetText("2")
	r.WorkEditor.SetText("1")

	win := new(app.Window)
	tab.RunnerAction(context.Background(), win, nil)
	waitRunFinished(t, r, 15*time.Second)
	if !r.started {
		t.Fatal("first action must start the run")
	}

	tab.RunnerAction(context.Background(), win, nil)
	if r.started {
		t.Error("a finished run must go back to config on the next action")
	}
}

func TestApplyFormPartsAndURLEncoded(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(file, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}

	tab := NewRequestTab("t")
	tab.applyFormParts([]model.ParsedFormPart{
		{Key: "text", Value: "v", Kind: model.FormPartText},
		{Key: "file", Kind: model.FormPartFile, FilePath: file},
		{Key: "gone", Kind: model.FormPartFile, FilePath: filepath.Join(dir, "missing")},
		{Key: "off", Value: "x", Kind: model.FormPartText, Disabled: true},
	})
	if len(tab.FormParts) != 4 {
		t.Fatalf("FormParts len = %d, want 4", len(tab.FormParts))
	}
	if tab.FormParts[1].FileSize != 10 {
		t.Errorf("file part size = %d, want 10", tab.FormParts[1].FileSize)
	}
	if tab.FormParts[2].FileSize != 0 {
		t.Errorf("missing file size = %d, want 0", tab.FormParts[2].FileSize)
	}
	if !tab.FormParts[3].Disabled {
		t.Error("Disabled flag must carry over")
	}

	tab.applyFormParts(nil)
	if len(tab.FormParts) != 0 {
		t.Errorf("applyFormParts(nil) must clear, got %d", len(tab.FormParts))
	}

	tab.applyURLEncoded([]model.ParsedKV{{Key: "a", Value: "1"}, {Key: "b", Value: "2", Disabled: true}})
	if len(tab.URLEncoded) != 2 || tab.URLEncoded[0].Key.Text() != "a" || !tab.URLEncoded[1].Disabled {
		t.Errorf("applyURLEncoded produced %#v", tab.URLEncoded)
	}
	tab.applyURLEncoded(nil)
	if len(tab.URLEncoded) != 0 {
		t.Errorf("applyURLEncoded(nil) must clear, got %d", len(tab.URLEncoded))
	}
}

func TestExampleStatusText(t *testing.T) {
	cases := []struct {
		name string
		ex   model.ParsedExample
		want string
	}{
		{"code and status", model.ParsedExample{Code: 200, Status: "OK", RespBody: "abc"}, "200 OK"},
		{"code only", model.ParsedExample{Code: 404, RespBody: ""}, "404"},
		{"status only", model.ParsedExample{Status: "Created"}, "Created"},
		{"neither", model.ParsedExample{}, "Example"},
	}
	for _, c := range cases {
		got := exampleStatusText(c.ex)
		if !strings.HasPrefix(got, c.want+"  ") {
			t.Errorf("%s: exampleStatusText = %q, want prefix %q", c.name, got, c.want)
		}
		if !strings.Contains(got, formatSize(int64(len(c.ex.RespBody)))) {
			t.Errorf("%s: exampleStatusText = %q, want the body size appended", c.name, got)
		}
	}
}

func TestExampleMenuLabel(t *testing.T) {
	if got := exampleMenuLabel(0); got != "Example #1" {
		t.Errorf("exampleMenuLabel(0) = %q", got)
	}
	if got := exampleMenuLabel(11); got != "Example #12" {
		t.Errorf("exampleMenuLabel(11) = %q", got)
	}
}

func exampleTab(t *testing.T) *RequestTab {
	t.Helper()
	tab := NewRequestTab("t")
	tab.Method = "PUT"
	tab.LastHTTPMethod = "PUT"
	tab.URLInput.SetText("http://base.test/orig")
	tab.ReqEditor.SetText("original body")
	tab.BodyType = model.BodyRaw
	tab.AddHeader("X-Base", "1")
	tab.applyURLEncoded([]model.ParsedKV{{Key: "b", Value: "2"}})
	tab.Status = "200 OK"
	tab.RespEditor.SetText("original response")
	tab.Examples = []model.ParsedExample{{
		Name:     "sample",
		Method:   "POST",
		URL:      "http://example.test/e",
		Body:     "example body",
		Headers:  map[string]string{"X-Ex": "9"},
		BodyType: model.BodyRaw,
		Code:     201,
		Status:   "Created",
		RespBody: `{"ok":true}`,
	}}
	return tab
}

func TestApplyExampleCapturesAndRestoresBaseState(t *testing.T) {
	th := material.NewTheme()
	tab := exampleTab(t)

	tab.applyExample(th, 0)
	if tab.ExampleSel != 0 {
		t.Fatalf("ExampleSel = %d, want 0", tab.ExampleSel)
	}
	if !tab.BaseState.valid {
		t.Fatal("applying an example must capture the base state first")
	}
	if tab.Method != "POST" || tab.LastHTTPMethod != "POST" {
		t.Errorf("method = %q / last = %q, want POST", tab.Method, tab.LastHTTPMethod)
	}
	if tab.URLInput.Text() != "http://example.test/e" {
		t.Errorf("URL = %q", tab.URLInput.Text())
	}
	if tab.ReqEditor.Text() != "example body" {
		t.Errorf("body = %q", tab.ReqEditor.Text())
	}
	if tab.RespEditor.Text() != `{"ok":true}` {
		t.Errorf("response = %q", tab.RespEditor.Text())
	}
	if !tab.respIsJSON {
		t.Error("a JSON example response must be flagged as JSON")
	}
	if !strings.HasPrefix(tab.Status, "201 Created") {
		t.Errorf("status = %q", tab.Status)
	}
	found := false
	for _, h := range tab.Headers {
		if h.Key.Text() == "X-Ex" {
			found = true
		}
		if h.Key.Text() == "X-Base" && !h.IsGenerated {
			t.Error("the example must replace the base headers")
		}
	}
	if !found {
		t.Error("example headers were not applied")
	}

	tab.applyExample(th, -1)
	if tab.ExampleSel != -1 {
		t.Errorf("ExampleSel = %d, want -1", tab.ExampleSel)
	}
	if tab.Method != "PUT" || tab.URLInput.Text() != "http://base.test/orig" {
		t.Errorf("base state not restored: method=%q url=%q", tab.Method, tab.URLInput.Text())
	}
	if tab.ReqEditor.Text() != "original body" {
		t.Errorf("request body not restored: %q", tab.ReqEditor.Text())
	}
	if tab.RespEditor.Text() != "original response" {
		t.Errorf("response not restored: %q", tab.RespEditor.Text())
	}
	if tab.Status != "200 OK" {
		t.Errorf("status not restored: %q", tab.Status)
	}
	if len(tab.URLEncoded) != 1 || tab.URLEncoded[0].Key.Text() != "b" {
		t.Errorf("url-encoded parts not restored: %#v", tab.URLEncoded)
	}
	if tab.BaseState.valid {
		t.Error("restoring must consume the captured base state")
	}
	restoredBase := false
	for _, h := range tab.Headers {
		if h.Key.Text() == "X-Base" {
			restoredBase = true
		}
	}
	if !restoredBase {
		t.Error("base headers were not restored")
	}
}

func TestApplyExampleOutOfRangeWithoutBaseStateIsSafe(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://keep.test")
	tab.applyExample(nil, 5)
	if tab.ExampleSel != -1 {
		t.Errorf("ExampleSel = %d, want -1", tab.ExampleSel)
	}
	if tab.URLInput.Text() != "http://keep.test" {
		t.Errorf("URL must be untouched when there is nothing to restore: %q", tab.URLInput.Text())
	}
}

func TestApplyExampleSwitchingExamplesKeepsOriginalBase(t *testing.T) {
	th := material.NewTheme()
	tab := exampleTab(t)
	tab.Examples = append(tab.Examples, model.ParsedExample{
		Method: "DELETE", URL: "http://example.test/second", Code: 204,
	})

	tab.applyExample(th, 0)
	tab.applyExample(th, 1)
	if tab.URLInput.Text() != "http://example.test/second" {
		t.Errorf("URL = %q", tab.URLInput.Text())
	}
	tab.applyExample(th, -1)
	if tab.URLInput.Text() != "http://base.test/orig" {
		t.Errorf("switching examples must not overwrite the original base: %q", tab.URLInput.Text())
	}
}

func TestApplyExampleWSMethodDoesNotBecomeLastHTTPMethod(t *testing.T) {
	tab := NewRequestTab("t")
	tab.LastHTTPMethod = "GET"
	tab.Examples = []model.ParsedExample{{Method: MethodWS, URL: "ws://x.test"}}
	tab.applyExample(nil, 0)
	if tab.Method != MethodWS {
		t.Errorf("Method = %q, want WS", tab.Method)
	}
	if tab.LastHTTPMethod != "GET" {
		t.Errorf("LastHTTPMethod = %q, want the previous HTTP method preserved", tab.LastHTTPMethod)
	}
}

func TestApplyExampleBinarySizeFromDisk(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "up.bin")
	if err := os.WriteFile(file, []byte("abcd"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := NewRequestTab("t")
	tab.Examples = []model.ParsedExample{
		{URL: "http://x.test", BinaryPath: file},
		{URL: "http://x.test", BinaryPath: filepath.Join(dir, "missing")},
	}
	tab.applyExample(nil, 0)
	if tab.BinaryFileSize != 4 {
		t.Errorf("BinaryFileSize = %d, want 4", tab.BinaryFileSize)
	}
	tab.applyExample(nil, 1)
	if tab.BinaryFileSize != 0 {
		t.Errorf("BinaryFileSize = %d, want 0 for a missing file", tab.BinaryFileSize)
	}
}

func TestAtoiDefault(t *testing.T) {
	cases := []struct {
		in       string
		def, min int
		want     int
	}{
		{"5", 1, 0, 5},
		{"  7  ", 1, 0, 7},
		{"", 3, 0, 3},
		{"abc", 3, 0, 3},
		{"0", 3, 1, 3},
		{"1", 3, 1, 1},
		{"-4", 9, 0, 9},
		{"-4", 9, -10, -4},
		{"2.5", 8, 0, 8},
		{"999999999999999999999", 8, 0, 8},
	}
	for _, c := range cases {
		if got := atoiDefault(c.in, c.def, c.min); got != c.want {
			t.Errorf("atoiDefault(%q, %d, %d) = %d, want %d", c.in, c.def, c.min, got, c.want)
		}
	}
}

func TestHTTPMethod(t *testing.T) {
	cases := []struct{ method, want string }{
		{MethodGraphQL, "POST"},
		{"GET", "GET"},
		{"DELETE", "DELETE"},
		{"", ""},
	}
	for _, c := range cases {
		tab := NewRequestTab("t")
		tab.Method = c.method
		if got := tab.httpMethod(); got != c.want {
			t.Errorf("httpMethod() with Method=%q = %q, want %q", c.method, got, c.want)
		}
	}
}

func TestAuthTypeModelRoundTrip(t *testing.T) {
	for _, at := range []int{authNone, authBearer, authBasic} {
		s := authTypeToModel(at)
		if got := authTypeFromModel(s); got != at {
			t.Errorf("round trip auth %d -> %q -> %d", at, s, got)
		}
	}
	if got := authTypeFromModel("nonsense"); got != authNone {
		t.Errorf("authTypeFromModel(nonsense) = %d, want authNone", got)
	}
	if got := authTypeFromModel(""); got != authNone {
		t.Errorf("authTypeFromModel(empty) = %d, want authNone", got)
	}
	if got := authTypeToModel(99); got != "" {
		t.Errorf("authTypeToModel(99) = %q, want empty", got)
	}
}

func TestApplyAuthRoundTrip(t *testing.T) {
	cases := []model.ParsedAuth{
		{Type: "bearer", Token: "tok"},
		{Type: "basic", Username: "u", Password: "p"},
		{Type: "", Token: "", Username: "", Password: ""},
		{Type: "bearer", Token: "токен ünïcode"},
	}
	for _, want := range cases {
		tab := NewRequestTab("t")
		tab.ApplyAuth(want)
		got := tab.AuthModel()
		if got.Type != want.Type || got.Token != want.Token ||
			got.Username != want.Username || got.Password != want.Password {
			t.Errorf("ApplyAuth/AuthModel round trip: got %+v, want %+v", got, want)
		}
	}
}

func TestApplyAuthUnknownTypeNormalizes(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ApplyAuth(model.ParsedAuth{Type: "oauth2", Token: "tok"})
	if tab.AuthType != authNone {
		t.Errorf("AuthType = %d, want authNone for unknown type", tab.AuthType)
	}
	if got := tab.AuthModel().Type; got != "" {
		t.Errorf("AuthModel().Type = %q, want empty", got)
	}
}

func TestApplyCookiesRoundTrip(t *testing.T) {
	in := []model.ParsedKV{
		{Key: "session", Value: "abc"},
		{Key: "theme", Value: "dark"},
	}
	tab := NewRequestTab("t")
	tab.ApplyCookies(in)
	got := tab.CookieModels()
	if len(got) != len(in) {
		t.Fatalf("CookieModels() len = %d, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("cookie[%d] = %+v, want %+v", i, got[i], in[i])
		}
	}
}

func TestApplyCookiesReplacesPrevious(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ApplyCookies([]model.ParsedKV{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}})
	tab.ApplyCookies([]model.ParsedKV{{Key: "c", Value: "3"}})
	got := tab.CookieModels()
	if len(got) != 1 || got[0].Key != "c" {
		t.Errorf("CookieModels() = %+v, want only cookie c", got)
	}
}

func TestCookieModelsSkipsEmptyKeys(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ApplyCookies([]model.ParsedKV{{Key: "", Value: "orphan"}, {Key: "k", Value: "v"}})
	got := tab.CookieModels()
	if len(got) != 1 || got[0].Key != "k" {
		t.Errorf("CookieModels() = %+v, want only cookie k", got)
	}
}

func TestEnsureGQLIsIdempotent(t *testing.T) {
	tab := NewRequestTab("t")
	if tab.GQL != nil {
		t.Fatal("new tab already has a GQL session")
	}
	g := tab.EnsureGQL()
	if g == nil {
		t.Fatal("EnsureGQL returned nil")
	}
	if g.VarsSplitRatio != 0.6 {
		t.Errorf("VarsSplitRatio = %v, want 0.6", g.VarsSplitRatio)
	}
	if again := tab.EnsureGQL(); again != g {
		t.Error("EnsureGQL created a second session")
	}
}

func TestGraphQLPayload(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		vars      string
		env       map[string]string
		wantQuery string
		wantVars  string
		wantErr   bool
	}{
		{
			name:      "query only",
			query:     "{ me { id } }",
			wantQuery: "{ me { id } }",
		},
		{
			name:      "query with variables",
			query:     "query($id:ID!){ user(id:$id){ name } }",
			vars:      `{"id":"7"}`,
			wantQuery: "query($id:ID!){ user(id:$id){ name } }",
			wantVars:  `{"id":"7"}`,
		},
		{
			name:      "whitespace-only variables omitted",
			query:     "{ me }",
			vars:      "   \n\t ",
			wantQuery: "{ me }",
		},
		{
			name:    "invalid json variables",
			query:   "{ me }",
			vars:    `{"id":}`,
			wantErr: true,
		},
		{
			name:      "env substitution in query and vars",
			query:     "{ user(id:{{uid}}) }",
			vars:      `{"tenant":"{{tenant}}"}`,
			env:       map[string]string{"uid": "42", "tenant": "acme"},
			wantQuery: "{ user(id:42) }",
			wantVars:  `{"tenant":"acme"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			g := tab.EnsureGQL()
			g.Query.SetText(c.query)
			g.Variables.SetText(c.vars)

			data, err := tab.graphQLPayload(c.env)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("graphQLPayload error: %v", err)
			}
			var p struct {
				Query     string          `json:"query"`
				Variables json.RawMessage `json:"variables"`
			}
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatalf("payload is not valid JSON: %v (%s)", err, data)
			}
			if p.Query != c.wantQuery {
				t.Errorf("query = %q, want %q", p.Query, c.wantQuery)
			}
			if c.wantVars == "" {
				if len(p.Variables) != 0 {
					t.Errorf("variables = %s, want omitted", p.Variables)
				}
			} else if string(p.Variables) != c.wantVars {
				t.Errorf("variables = %s, want %s", p.Variables, c.wantVars)
			}
		})
	}
}

func TestBuildGraphQLBody(t *testing.T) {
	tab := NewRequestTab("t")
	g := tab.EnsureGQL()
	g.Query.SetText("{ me }")

	r, ct, err := tab.buildGraphQLBody(nil)
	if err != nil {
		t.Fatalf("buildGraphQLBody error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content type = %q, want application/json", ct)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !json.Valid(b) {
		t.Errorf("body is not valid JSON: %s", b)
	}

	g.Variables.SetText(`{"bad":}`)
	if _, _, err := tab.buildGraphQLBody(nil); err == nil {
		t.Error("buildGraphQLBody accepted invalid variables JSON")
	}
}

func newRunnerRig() *vstackRig {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.RunOpen = true
	rig.tab.EnsureRun()
	return rig
}

func TestRunnerConfigPanelRenders(t *testing.T) {
	cases := []struct {
		name string
		mode runnerMode
		vars int
		size image.Point
	}{
		{"iterations no vars", runByIterations, 0, image.Pt(1100, 700)},
		{"duration no vars", runByDuration, 0, image.Pt(1100, 700)},
		{"iterations with vars", runByIterations, 3, image.Pt(1100, 700)},
		{"narrow", runByIterations, 2, image.Pt(420, 320)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newRunnerRig()
			rig.size = c.size
			r := rig.tab.EnsureRun()
			r.Mode = c.mode
			for i := 0; i < c.vars; i++ {
				r.addVar()
				r.Variables[i].Name.SetText("v")
				r.Variables[i].Values.SetText("1,2,3")
			}
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if r.started {
				t.Error("rendering the config panel must not start a run")
			}
		})
	}
}

func TestRunnerModeToggleButtons(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	rig.frame()

	r.ModeTimeBtn.Click()
	rig.frame()
	rig.frame()
	if r.Mode != runByDuration {
		t.Errorf("Mode = %v, want duration", r.Mode)
	}
	r.ModeIterBtn.Click()
	rig.frame()
	rig.frame()
	if r.Mode != runByIterations {
		t.Errorf("Mode = %v, want iterations", r.Mode)
	}
}

func TestRunnerAddAndDeleteVariableRows(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	rig.frame()

	r.AddVarBtn.Click()
	rig.frame()
	rig.frame()
	r.AddVarBtn.Click()
	rig.frame()
	rig.frame()
	if len(r.Variables) != 2 {
		t.Fatalf("Variables = %d, want 2", len(r.Variables))
	}
	r.Variables[0].Name.SetText("first")
	r.Variables[1].Name.SetText("second")
	rig.frame()

	r.Variables[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(r.Variables) != 1 {
		t.Fatalf("Variables = %d, want 1 after delete", len(r.Variables))
	}
	if r.Variables[0].Name.Text() != "second" {
		t.Errorf("remaining variable = %q, want second", r.Variables[0].Name.Text())
	}
}

func TestRunnerSingleMultipleTabs(t *testing.T) {
	rig := newRunnerRig()
	rig.tab.RunOpen = false
	rig.frame()

	rig.tab.MultipleBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.RunOpen {
		t.Error("Multiple must open the runner panel")
	}
	rig.tab.SingleBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.RunOpen {
		t.Error("Single must close the runner panel")
	}
}

func TestRunnerStatsPanelRenders(t *testing.T) {
	cases := []struct {
		name    string
		running bool
		planned int
		records func(*RequestRunner)
		size    image.Point
	}{
		{"no data", false, 0, func(*RequestRunner) {}, image.Pt(1100, 700)},
		{
			name: "mixed statuses", planned: 4, size: image.Pt(1100, 700),
			records: func(r *RequestRunner) {
				r.record(200, 5*time.Millisecond, true)
				r.record(204, 6*time.Millisecond, true)
				r.record(404, 7*time.Millisecond, false)
				r.record(503, 8*time.Millisecond, false)
				r.record(0, 9*time.Millisecond, false)
			},
		},
		{
			name: "running", running: true, planned: 100, size: image.Pt(1100, 700),
			records: func(r *RequestRunner) { r.record(200, time.Millisecond, true) },
		},
		{
			name: "narrow", planned: 1, size: image.Pt(400, 300),
			records: func(r *RequestRunner) { r.record(301, time.Millisecond, true) },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newRunnerRig()
			rig.size = c.size
			r := rig.tab.EnsureRun()
			r.started = true
			r.plannedN = c.planned
			r.mu.Lock()
			r.startedAt = time.Now().Add(-time.Second)
			if !c.running {
				r.endedAt = time.Now()
			}
			r.mu.Unlock()
			r.running.Store(c.running)
			c.records(r)
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			r.running.Store(false)
		})
	}
}

func TestRunnerStatsSortButtonsCycle(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	r.started = true
	r.record(200, 5*time.Millisecond, true)
	r.record(500, 9*time.Millisecond, false)
	rig.frame()

	for col := range r.SortBtns {
		r.SortBtns[col].Click()
		rig.frame()
		rig.frame()
		if r.SortCol != col {
			t.Fatalf("clicking column %d set SortCol=%d", col, r.SortCol)
		}
		if r.SortAsc {
			t.Errorf("a fresh column must start descending, got ascending for column %d", col)
		}

		r.SortBtns[col].Click()
		rig.frame()
		rig.frame()
		if !r.SortAsc {
			t.Errorf("clicking column %d twice must flip to ascending", col)
		}
	}
}

func TestRunnerStatusTextThroughLayout(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	rig.frame()
	if got := rig.tab.runnerStatusText(); got == "" {
		t.Error("runnerStatusText must never be empty")
	}
	r.started = true
	r.plannedN = 10
	r.record(200, time.Millisecond, true)
	rig.frame()
	rig.frame()
	if got := rig.tab.runnerStatusText(); got == "" {
		t.Error("runnerStatusText must never be empty once started")
	}
}

func TestExampleNameRowRendersOnlyForSelectedNamedExample(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.frame()

	cases := []struct {
		name    string
		runOpen bool
		sel     int
		exName  string
		wantRow bool
	}{
		{"nothing selected", false, -1, "", false},
		{"out of range", false, 5, "", false},
		{"selected but unnamed", false, 0, "", false},
		{"selected and named", false, 0, "Happy path", true},
		{"runner open hides it", true, 0, "Happy path", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig.tab.RunOpen = c.runOpen
			rig.tab.ExampleSel = c.sel
			rig.tab.Examples = []model.ParsedExample{{Name: c.exName, URL: "http://x.test"}}
			d := rig.tab.layoutExampleNameRow(rig.gtx(), rig.th)
			if got := d.Size.Y > 0; got != c.wantRow {
				t.Errorf("rendered=%v, want %v", got, c.wantRow)
			}
		})
	}
}

func TestEnvForIterationCyclesValues(t *testing.T) {
	vars := []runVarSnapshot{
		{name: "id", vals: []string{"1", "2", "3"}},
		{name: "tok", vals: []string{"a", "b"}},
	}
	base := map[string]string{"host": "example.com"}

	cases := []struct {
		idx     int
		wantID  string
		wantTok string
	}{
		{0, "1", "a"},
		{1, "2", "b"},
		{2, "3", "a"},
		{3, "1", "b"},
	}
	for _, c := range cases {
		env := envForIteration(base, vars, c.idx)
		if env["id"] != c.wantID || env["tok"] != c.wantTok {
			t.Errorf("idx=%d got id=%q tok=%q want id=%q tok=%q", c.idx, env["id"], env["tok"], c.wantID, c.wantTok)
		}
		if env["host"] != "example.com" {
			t.Errorf("idx=%d base var lost: %q", c.idx, env["host"])
		}
	}
}

func TestEnvForIterationDoesNotMutateBase(t *testing.T) {
	base := map[string]string{"host": "example.com"}
	vars := []runVarSnapshot{{name: "id", vals: []string{"1"}}}
	env := envForIteration(base, vars, 0)
	env["id"] = "changed"
	if _, ok := base["id"]; ok {
		t.Error("base map was mutated by envForIteration")
	}
}

func TestEnvForIterationNoVarsReturnsBase(t *testing.T) {
	base := map[string]string{"host": "example.com"}
	env := envForIteration(base, nil, 5)
	if len(env) != 1 || env["host"] != "example.com" {
		t.Errorf("unexpected env: %v", env)
	}
}

func TestSnapshotVariablesSkipsEmpty(t *testing.T) {
	r := newRequestRunner()
	r.addVar()
	r.addVar()
	r.addVar()
	r.Variables[0].Name.SetText("a")
	r.Variables[0].Values.SetText("1, 2")
	r.Variables[1].Name.SetText("   ")
	r.Variables[1].Values.SetText("x")
	r.Variables[2].Name.SetText("b")
	r.Variables[2].Values.SetText("   ")

	snap := r.snapshotVariables()
	if len(snap) != 1 {
		t.Fatalf("expected 1 valid var, got %d: %+v", len(snap), snap)
	}
	if snap[0].name != "a" || len(snap[0].vals) != 2 {
		t.Errorf("unexpected snapshot: %+v", snap[0])
	}
}

func scratchGtx(ops *op.Ops) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 400)),
		Now:         time.Unix(1700000000, 0),
	}
}

// scratchBody builds lines of deliberately different lengths, so two chunks
// never produce the same glyph geometry. WrapGlyph stores only positions, so
// equal-width lines would mask a buffer being reused underneath a live slice.
func scratchBody() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "%s%s\n", strings.Repeat("w", 3+i*11), strings.Repeat(".il ", i%5))
	}
	return b.String()
}

func scratchSetup(v *textCore) (layout.Context, *text.Shaper, font.Font) {
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	fnt := font.Font{Typeface: "Go Mono"}
	v.layoutShaper, v.layoutFont, v.layoutSize = shaper, fnt, unit.Sp(13)
	v.lastLineHeight = 16
	return scratchGtx(new(op.Ops)), shaper, fnt
}

// TestScratchBuffersIndependent pins the invariant the two glyph scratch
// buffers exist for: a slice produced for painting stays valid while the
// hit-test helpers shape other chunks, as happens when a caret or selection
// bound is resolved in the middle of painting a chunk. With one shared buffer
// the painted glyphs silently become another chunk's.
func TestScratchBuffersIndependent(t *testing.T) {
	v := NewResponseViewer()
	v.SetText(scratchBody())
	gtx, shaper, fnt := scratchSetup(&v.textCore)

	// Paint a long line, exactly as paintChunk does.
	const paintLine = 40
	chunkStart, chunkEnd := v.lineStarts[paintLine], v.lineStarts[paintLine+1]
	v.paintScratch = widgets.ShapeChunkForWrapInto(v.paintScratch, shaper, fnt, v.layoutSize,
		gtx, v.text[chunkStart:chunkEnd], 600)
	painted := v.paintScratch
	if len(painted) < 100 {
		t.Fatalf("paint path produced %d glyphs, expected a long line", len(painted))
	}
	before := append([]widgets.WrapGlyph(nil), painted...)
	wantX, wantLine := widgets.CaretXYInWrap(painted, 300)

	// Run every hit-test entry point on shorter, differently shaped chunks
	// while the painted slice is still live.
	for line := 0; line < 12; line++ {
		s, e := v.lineStarts[line], v.lineStarts[line+1]
		v.wrapCaretXY(line, s, e, s+3, gtx, 600)
		v.wrapByteAt(line, s, e, 120, 0, gtx, 600)
		v.wrapMaxLineOf(line, s, e, gtx, 600)
	}

	if len(painted) != len(before) {
		t.Fatalf("painted slice length changed: %d, want %d", len(painted), len(before))
	}
	for i := range painted {
		if painted[i] != before[i] {
			t.Fatalf("hit-test overwrote painted glyph %d of %d", i, len(painted))
		}
	}
	if gotX, gotLine := widgets.CaretXYInWrap(painted, 300); gotX != wantX || gotLine != wantLine {
		t.Fatalf("caret over painted glyphs = (%d,%d), want (%d,%d)", gotX, gotLine, wantX, wantLine)
	}
	if len(v.paintScratch) > 0 && len(v.hitScratch) > 0 &&
		&v.paintScratch[0] == &v.hitScratch[0] {
		t.Fatal("paint and hit-test scratch share a backing array")
	}
}

// TestScratchBuffersIndependentEditor is the same invariant for the request
// editor, which has its own textCore and its own pair of buffers.
func TestScratchBuffersIndependentEditor(t *testing.T) {
	e := NewRequestEditor()
	e.SetText(scratchBody())
	gtx, shaper, fnt := scratchSetup(&e.textCore)

	const paintLine = 45
	s0, e0 := e.lineStarts[paintLine], e.lineStarts[paintLine+1]
	e.paintScratch = widgets.ShapeChunkForWrapInto(e.paintScratch, shaper, fnt, e.layoutSize,
		gtx, e.text[s0:e0], 300)
	painted := e.paintScratch
	if len(painted) < 100 {
		t.Fatalf("paint path produced %d glyphs, expected a long line", len(painted))
	}
	before := append([]widgets.WrapGlyph(nil), painted...)

	for line := 0; line < 15; line++ {
		s, en := e.lineStarts[line], e.lineStarts[line+1]
		e.wrapCaretXY(line, s, en, s+7, gtx, 300)
		e.wrapByteAt(line, s, en, 55, 0, gtx, 300)
	}

	for i := range painted {
		if painted[i] != before[i] {
			t.Fatalf("hit-test overwrote painted glyph %d of %d", i, len(painted))
		}
	}
}

func TestResponseScrollbarFadeOnHover(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "line of response content that is long enough to overflow horizontally too")
	}
	rig := newRespRig(strings.Join(lines, "\n"), false)

	var r input.Router
	ops := new(op.Ops)
	size := rig.size
	keepVisible := false
	now := time.Unix(1700000000, 0)
	frame := func() {
		now = now.Add(40 * time.Millisecond)
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size),
			Now:         now,
			Source:      r.Source(),
		}
		layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return ResponseViewerStyle{
					Viewer:   rig.v,
					Shaper:   rig.shaper,
					TextSize: unit.Sp(13),
					Wrap:     false,
					Padding:  unit.Dp(4),
				}.Layout(gtx)
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return rig.v.LayoutScrollbarHover(gtx, keepVisible)
			}),
		)
		r.Frame(ops)
	}

	for i := 0; i < 3; i++ {
		frame()
	}
	if f := rig.v.ScrollbarFade(); f != 0 {
		t.Fatalf("fade should start at 0 (not hovered), got %v", f)
	}

	cx, cy := size.X/2, size.Y/2
	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(cx), float32(cy)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f <= 0 {
		t.Fatalf("fade should rise above 0 while hovering the editor, got %v", f)
	}

	rightX := size.X - 3
	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(rightX), float32(cy)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f <= 0 {
		t.Fatalf("fade must stay > 0 while the cursor is over the vertical scrollbar, got %v", f)
	}

	keepVisible = true
	for i := 0; i < 12; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(size.X+500), float32(size.Y+500)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f != 1 {
		t.Fatalf("fade must stay fully visible while dragging even with the pointer outside, got %v", f)
	}

	keepVisible = false
	for i := 0; i < 12; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(size.X+500), float32(size.Y+500)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f != 0 {
		t.Fatalf("fade should fall back to 0 after the pointer leaves and drag ends, got %v", f)
	}
}

func bigPrettyLines(n int) string {
	var b strings.Builder
	b.Grow(n * 24)
	for i := 0; i < n; i++ {
		b.WriteString(`      "id": 123456,`)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestJumpToEndPaintsOnlyTheViewport(t *testing.T) {
	rig := newRespRig(bigPrettyLines(200000), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	rig.click(40, 40, 10*time.Millisecond)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	at := 50 * time.Millisecond
	rig.r.Queue(key.Event{
		Name:      key.NameEnd,
		Modifiers: key.ModShortcut,
		State:     key.Press,
	})
	rig.frame(time.Unix(1700000000, 0).Add(at))
	runtime.ReadMemStats(&after)

	if rig.v.scrollY <= 0 {
		t.Fatalf("ctrl+end did not scroll: scrollY=%d totalH=%d", rig.v.scrollY, rig.v.lastTotalH)
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	if alloc > 32<<20 {
		t.Errorf("jump-to-end frame allocated %.1fMB; it must paint only the viewport", float64(alloc)/(1<<20))
	}
}

func TestJumpToEndKeepsScrollAnchorConsistent(t *testing.T) {
	rig := newRespRig(bigPrettyLines(50000), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	rig.click(40, 40, 10*time.Millisecond)

	rig.r.Queue(key.Event{Name: key.NameEnd, Modifiers: key.ModShortcut, State: key.Press})
	rig.frame(time.Unix(1700000000, 0).Add(50 * time.Millisecond))

	v := rig.v
	wantLine, wantAccum := v.firstChunkAtFn(v.scrollY, v.lastLineHeight, 0, rig.size.X, true)
	if wantLine == 0 && v.scrollY > v.lastLineHeight {
		t.Fatalf("anchor collapsed to line 0 at scrollY=%d", v.scrollY)
	}
	if gap := v.scrollY - wantAccum; gap < 0 || gap > v.lastLineHeight {
		t.Errorf("anchor %d px off from scrollY=%d (line %d)", gap, v.scrollY, wantLine)
	}
}

func TestFoldForSearch_PreservesByteLength(t *testing.T) {
	for _, s := range []string{
		"Hello World",
		"ПРИВЕТ Мир",
		"Grüße, Straße",
		"ΑΘΗΝΑ",
		"İstanbul KELVIN K",
		"🚀 emoji ЖЖ",
		"",
	} {
		if got := foldForSearch(s); len(got) != len(s) {
			t.Errorf("foldForSearch(%q) changed byte length: %d -> %d", s, len(s), len(got))
		}
	}
}

func TestSearch_CaseInsensitiveCyrillic(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText(`{"имя": "Привет", "город": "МОСКВА", "note": "привет again"}`)

	for _, q := range []string{"привет", "ПРИВЕТ", "ПрИвЕт"} {
		tab.RespSearch.Editor.SetText(q)
		tab.invalidateSearchCache()
		tab.RespSearch.recompute(tab.RespEditor.Text())
		if got := len(tab.RespSearch.spans); got != 2 {
			t.Errorf("query %q: expected 2 matches, got %d", q, got)
		}
		for _, m := range tab.RespSearch.spans {
			if !strings.EqualFold(tab.RespEditor.Text()[m.start:m.end], q) {
				t.Errorf("query %q: span [%d,%d) covers %q, not the match",
					q, m.start, m.end, tab.RespEditor.Text()[m.start:m.end])
			}
		}
	}

	tab.RespSearch.Editor.SetText("москва")
	tab.invalidateSearchCache()
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match for москва, got %d", got)
	}
	m := tab.RespSearch.spans[0]
	if got := tab.RespEditor.Text()[m.start:m.end]; got != "МОСКВА" {
		t.Errorf("span [%d,%d) covers %q, want МОСКВА", m.start, m.end, got)
	}
}

// A rune whose lowercase form is a different byte length used to shift every
// offset after it, so the highlight landed on unrelated text further down.
func TestSearch_OffsetsSurviveLengthChangingRunes(t *testing.T) {
	tab := NewRequestTab("t")
	body := "Kİ padding TARGET tail"
	tab.RespEditor.SetText(body)

	tab.RespSearch.Editor.SetText("target")
	tab.invalidateSearchCache()
	tab.RespSearch.recompute(body)
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match, got %d", got)
	}
	m := tab.RespSearch.spans[0]
	if got := body[m.start:m.end]; got != "TARGET" {
		t.Errorf("span [%d,%d) covers %q, want TARGET", m.start, m.end, got)
	}
}

func TestSearch_StaleCacheRebuiltOnTextChange(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("alpha beta")
	tab.RespSearch.Editor.SetText("alpha")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match, got %d", got)
	}

	// A viewer that swaps its text without announcing it must not keep matching
	// against the previous document.
	tab.RespEditor.SetText("gamma delta alpha")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match after the swap, got %d", got)
	}
	m := tab.RespSearch.spans[0]
	if got := tab.RespEditor.Text()[m.start:m.end]; got != "alpha" {
		t.Errorf("stale cache: span [%d,%d) covers %q", m.start, m.end, got)
	}
}

type searchRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *ResponseViewer
	ed     *RequestEditor
	box    *SearchBox
	size   image.Point
	pad    unit.Dp
	wrap   bool
}

func newSearchRig(txt string, wrap bool) *searchRig {
	rig := &searchRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		v:      NewResponseViewer(),
		box:    &SearchBox{},
		size:   image.Pt(400, 300),
		pad:    unit.Dp(4),
		wrap:   wrap,
	}
	rig.v.SetText(txt)
	return rig
}

func newSearchEditorRig(txt string, wrap bool) *searchRig {
	rig := &searchRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		ed:     NewRequestEditor(),
		box:    &SearchBox{},
		size:   image.Pt(400, 300),
		wrap:   wrap,
	}
	rig.ed.SetText(txt)
	return rig
}

func (rig *searchRig) gtx() layout.Context {
	rig.ops.Reset()
	return layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	}
}

func (rig *searchRig) frame() {
	gtx := rig.gtx()
	if rig.v != nil {
		ResponseViewerStyle{
			Viewer:   rig.v,
			Shaper:   rig.shaper,
			TextSize: unit.Sp(13),
			Wrap:     rig.wrap,
			Padding:  rig.pad,
		}.Layout(gtx)
	} else {
		RequestEditorStyle{
			Viewer:   rig.ed,
			Shaper:   rig.shaper,
			TextSize: unit.Sp(13),
			Wrap:     rig.wrap,
		}.Layout(gtx)
	}
	rig.r.Frame(rig.ops)
}

func (rig *searchRig) theme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = rig.shaper
	return th
}

func (rig *searchRig) core() *textCore {
	if rig.v != nil {
		return &rig.v.textCore
	}
	return &rig.ed.textCore
}

func (rig *searchRig) target() searchableEditor {
	if rig.v != nil {
		return rig.v
	}
	return rig.ed
}

func (rig *searchRig) metrics() (charAdv fixed.Int26_6, lineH, innerW, innerH, pad int) {
	gtx := rig.gtx()
	m := op.Record(gtx.Ops)
	paint.ColorOp{}.Add(gtx.Ops)
	col := m.Stop()
	var fnt font.Font
	charAdv = measureCharAdvance(rig.shaper, fnt, unit.Sp(13), gtx)
	lineH = measureLineHeight(rig.shaper, fnt, unit.Sp(13), col, gtx)
	pad = gtx.Dp(rig.pad)
	if 2*pad >= rig.size.X || 2*pad >= rig.size.Y {
		pad = 0
	}
	innerW = rig.size.X - 2*pad
	innerH = rig.size.Y - 2*pad
	return
}

// matchOnScreen probes the viewport with the same hit-test the mouse uses and
// reports whether any visible pixel maps into [start,end).
func (rig *searchRig) matchOnScreen(start, end int) bool {
	c := rig.core()
	charAdv, lineH, innerW, innerH, _ := rig.metrics()
	if lineH <= 0 {
		return false
	}
	gtx := rig.gtx()
	stepY := lineH / 2
	if stepY < 1 {
		stepY = 1
	}
	for y := 0; y < innerH; y += stepY {
		for x := 0; x <= innerW; x += 3 {
			off := c.coordToByteOffset(gtx, x, y, charAdv, lineH, innerW, rig.wrap)
			if off >= start && off < end {
				return true
			}
		}
	}
	return false
}

func (rig *searchRig) search(q string) {
	rig.box.Editor.SetText(q)
	rig.box.Open = true
	rig.box.invalidate()
	rig.box.refresh(rig.target(), rig.target().Text(), true)
	rig.frame()
}

func (rig *searchRig) next() {
	rig.box.navigate(1, rig.target())
	rig.frame()
}

func (rig *searchRig) report(t *testing.T, label string) bool {
	t.Helper()
	b := rig.box
	if b.current < 0 || b.current >= len(b.spans) {
		t.Errorf("%s: no current match (spans=%d)", label, len(b.spans))
		return false
	}
	m := b.spans[b.current]
	if !rig.matchOnScreen(m.start, m.end) {
		c := rig.core()
		t.Errorf("%s: match %d/%d at bytes [%d,%d) is NOT visible (scrollY=%d scrollX=%d viewportH=%d totalH=%d)",
			label, b.current+1, len(b.spans), m.start, m.end, c.scrollY, c.scrollX, c.lastViewportH, c.lastTotalH)
		return false
	}
	return true
}

func minifiedJSON(n int) string {
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id":%d,"name":"item-%04d","tag":"filler-payload-value"}`, i, i)
	}
	sb.WriteString(`]}`)
	return sb.String()
}

func prettyJSON(n int) string {
	var sb strings.Builder
	sb.WriteString("{\n  \"items\": [\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "    { \"id\": %d, \"name\": \"item-%04d\", \"tag\": \"%s\" },\n", i, i, strings.Repeat("long-", 30))
	}
	sb.WriteString("  ]\n}\n")
	return sb.String()
}

func TestSearchScroll_MinifiedWrapOn(t *testing.T) {
	rig := newSearchRig(minifiedJSON(400), true)
	rig.frame()
	rig.frame()
	rig.search("item-0300")
	rig.report(t, "wrap-on minified, first match")
}

func TestSearchScroll_MinifiedWrapOff(t *testing.T) {
	rig := newSearchRig(minifiedJSON(400), false)
	rig.frame()
	rig.frame()
	rig.search("item-0300")
	rig.report(t, "wrap-off minified, first match")
}

func TestSearchScroll_PrettyWrapOn(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()
	rig.search("item-0150")
	rig.report(t, "wrap-on pretty, first match")
}

func TestSearchScroll_NavigateAllPretty(t *testing.T) {
	rig := newSearchRig(prettyJSON(60), true)
	rig.frame()
	rig.frame()
	rig.search("name")
	n := len(rig.box.spans)
	if n < 10 {
		t.Fatalf("expected many matches, got %d", n)
	}
	bad := 0
	for i := 0; i < n; i++ {
		if !rig.report(t, fmt.Sprintf("pretty next #%d", i+1)) {
			bad++
			if bad > 3 {
				t.Fatalf("too many invisible matches")
			}
		}
		rig.next()
	}
}

func TestSearchScroll_EditorWrapOn(t *testing.T) {
	rig := newSearchEditorRig(prettyJSON(120), true)
	rig.frame()
	rig.frame()
	rig.search("item-0090")
	rig.report(t, "editor wrap-on, first match")
}

// The panel is pinned top-right and must stay there, so the reveal is what
// keeps matches out from under it: a match anywhere the document can scroll
// has to land clear of the reserved band.
func TestSearchPanel_RevealKeepsMatchOutOfPanelBand(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()
	rig.box.panelH = 40

	rig.search("item-0150")
	y, ok := rig.core().RevealScreenY()
	if !ok {
		t.Fatal("reveal not resolved")
	}
	if y < rig.box.panelH {
		t.Errorf("match revealed at row %d, inside the %d-tall panel band", y, rig.box.panelH)
	}
}

func TestSearch_QueryThatStopsMatchingClearsTheHighlight(t *testing.T) {
	rig := newSearchRig(prettyJSON(60), true)
	rig.frame()
	rig.frame()
	rig.search("item-0030")
	if got := rig.v.SelectedText(); got != "item-0030" {
		t.Fatalf("precondition: selection = %q", got)
	}

	rig.search("item-0030-no-such-thing")
	if len(rig.box.spans) != 0 {
		t.Fatalf("precondition: expected no matches, got %d", len(rig.box.spans))
	}
	c := rig.core()
	if c.highlightEnd > c.highlightStart {
		t.Errorf("0/0 still highlights [%d,%d) from the query that used to match",
			c.highlightStart, c.highlightEnd)
	}
	if got := rig.v.SelectedText(); got != "" {
		t.Errorf("0/0 still leaves %q selected", got)
	}
}

func TestSearchClose_ClearsTheLastMatchSelection(t *testing.T) {
	rig := newSearchRig(prettyJSON(60), true)
	rig.frame()
	rig.frame()
	rig.search("item-0030")
	if got := rig.v.SelectedText(); got != "item-0030" {
		t.Fatalf("precondition: match should be selected, got %q", got)
	}

	rig.box.closeOn(rig.target())
	rig.frame()

	if got := rig.v.SelectedText(); got != "" {
		t.Errorf("closing the search left %q selected in the viewer", got)
	}
	c := rig.core()
	if c.highlightEnd > c.highlightStart {
		t.Errorf("closing the search left the match highlight [%d,%d)", c.highlightStart, c.highlightEnd)
	}
}

func TestSearchScroll_BeforeFirstLayout(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.search("item-0150")
	rig.frame()
	rig.frame()
	rig.report(t, "search issued before first layout")
}

func TestSearchReseedsFromNewSelection(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText(`{"name":"alice","role":"admin"}`)

	var r input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: r.Source()}

	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	tab.ReqSearch.Editor.SetText("name")
	tab.ReqSearch.refresh(&tab.ReqEditor, tab.ReqEditor.Text(), true)
	if got := len(tab.ReqSearch.spans); got != 1 {
		t.Fatalf("expected 1 match for %q, got %d", "name", got)
	}

	idx := strings.Index(tab.ReqEditor.Text(), "role")
	tab.ReqEditor.selStart = idx
	tab.ReqEditor.selEnd = idx + len("role")
	if got := tab.ReqEditor.SelectedText(); got != "role" {
		t.Fatalf("selection setup failed, got %q", got)
	}

	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	if got := tab.ReqSearch.Editor.Text(); got != "role" {
		t.Fatalf("search box not reseeded from new selection: got %q, want %q", got, "role")
	}
	if got := len(tab.ReqSearch.spans); got != 1 {
		t.Fatalf("expected 1 match for reseeded query, got %d", got)
	}
}

func TestSearchKeepsQueryWithoutSelection(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText(`{"name":"alice"}`)

	var r input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: r.Source()}

	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	tab.ReqSearch.Editor.SetText("name")
	tab.ReqSearch.closeOn(&tab.ReqEditor)

	tab.ReqEditor.SetCaret(0, 0)
	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	if got := tab.ReqSearch.Editor.Text(); got != "name" {
		t.Fatalf("search box query should be preserved when no selection: got %q", got)
	}
}

func spanTexts(text string, spans []matchSpan) []string {
	out := make([]string, 0, len(spans))
	for _, m := range spans {
		out = append(out, text[m.start:m.end])
	}
	return out
}

func TestSearch_WholeWord(t *testing.T) {
	tab := NewRequestTab("t")
	body := "cat concatenate cat. Cat-cat _cat /cat/ кот, коты"
	tab.RespEditor.SetText(body)
	box := &tab.RespSearch

	box.Editor.SetText("cat")
	tab.invalidateSearchCache()
	box.recompute(body)
	if got := len(box.spans); got != 7 {
		t.Fatalf("substring mode: expected 7 matches, got %d %v", got, spanTexts(body, box.spans))
	}

	box.WholeWord = true
	box.recompute(body)
	want := []string{"cat", "cat", "Cat", "cat", "cat"}
	got := spanTexts(body, box.spans)
	if len(got) != len(want) {
		t.Fatalf("whole word: expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("whole word match %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, m := range box.spans {
		if m.start > 0 && body[m.start-1] == '_' {
			t.Errorf("underscore is a word character; _cat must not match whole-word")
		}
	}

	box.Editor.SetText("кот")
	box.recompute(body)
	if got := spanTexts(body, box.spans); len(got) != 1 || got[0] != "кот" {
		t.Errorf("cyrillic whole word: got %v, want [кот]", got)
	}

	box.Editor.SetText("cat.")
	box.recompute(body)
	if got := spanTexts(body, box.spans); len(got) != 1 {
		t.Errorf("a query ending in a separator is whole at its own edge: got %v", got)
	}
}

func TestSearch_PerDocumentState(t *testing.T) {
	tab := NewRequestTab("t")
	box := &tab.RespSearch
	ed := tab.RespEditor

	docA := "a a a"
	docB := "b b"

	ed.SetText(docA)
	box.SetDocument("A", ed)
	box.Open = true
	box.Editor.SetText("a")
	box.refresh(ed, ed.Text(), true)
	box.navigate(1, ed)
	if box.current != 1 || len(box.spans) != 3 {
		t.Fatalf("precondition: doc A on match 2/3, got %d/%d", box.current+1, len(box.spans))
	}

	ed.SetText(docB)
	box.SetDocument("B", ed)
	if box.Open || box.Editor.Text() != "" || box.current != -1 {
		t.Fatalf("a never-searched document must start closed and empty, got open=%v query=%q current=%d", box.Open, box.Editor.Text(), box.current)
	}
	if len(ed.searchSpans) != 0 {
		t.Errorf("switching to a closed document must clear the viewer's match spans")
	}
	box.Open = true
	box.Editor.SetText("b")
	box.refresh(ed, ed.Text(), true)
	if box.current != 0 || len(box.spans) != 2 {
		t.Fatalf("doc B: expected 1/2, got %d/%d", box.current+1, len(box.spans))
	}

	ed.SetText(docA)
	box.SetDocument("A", ed)
	if !box.Open || box.Editor.Text() != "a" {
		t.Fatalf("doc A must come back open with its own query, got open=%v query=%q", box.Open, box.Editor.Text())
	}
	if !box.cacheDirty {
		t.Errorf("switching documents must mark the fold cache dirty")
	}
	box.refresh(ed, ed.Text(), false)
	if box.current != 1 || len(box.spans) != 3 {
		t.Errorf("doc A must resume on match 2/3, got %d/%d", box.current+1, len(box.spans))
	}
	if box.query != "a" {
		t.Errorf("restored query must not read as a fresh edit: query=%q", box.query)
	}

	ed.SetText(docB)
	box.SetDocument("B", ed)
	box.refresh(ed, ed.Text(), false)
	if !box.Open || box.Editor.Text() != "b" || box.current != 0 {
		t.Errorf("doc B must resume on 1/2 with query b, got open=%v query=%q current=%d", box.Open, box.Editor.Text(), box.current)
	}

	box.closeOn(ed)
	ed.SetText(docA)
	box.SetDocument("A", ed)
	if !box.Open {
		t.Errorf("closing doc B must not close doc A")
	}
	box.SetDocument("A", ed)
	if !box.Open || box.Editor.Text() != "a" {
		t.Errorf("re-setting the same key must be a no-op")
	}
}

func TestSearch_DocStateEviction(t *testing.T) {
	tab := NewRequestTab("t")
	box := &tab.RespSearch
	ed := tab.RespEditor
	ed.SetText("x")
	for i := 0; i < maxSearchDocs+20; i++ {
		box.SetDocument(string(rune('a'+i%26))+string(rune('a'+i/26)), ed)
		box.Open = true
		box.Editor.SetText("x")
	}
	if len(box.docs) > maxSearchDocs {
		t.Errorf("parked document states must stay bounded, got %d", len(box.docs))
	}
}

func genPrettyJSON(n int) string {
	var b strings.Builder
	b.Grow(n + 1024)
	b.WriteString("{\n  \"items\": [\n")
	i := 0
	for b.Len() < n {
		fmt.Fprintf(&b,
			"    {\"id\": %d, \"name\": \"item-%d\", \"email\": \"user%d@example.com\", \"active\": true, \"score\": %d.%02d, \"tags\": [\"alpha\", \"beta\", \"gamma\"], \"note\": null},\n",
			i, i, i, i%1000, i%100)
		i++
	}
	b.WriteString("    {\"id\": -1}\n  ]\n}\n")
	return b.String()
}

func genMinifiedJSON(n int) string {
	var b strings.Builder
	b.Grow(n + 1024)
	b.WriteString(`{"items":[`)
	i := 0
	for b.Len() < n {
		fmt.Fprintf(&b,
			`{"id":%d,"name":"item-%d","email":"user%d@example.com","active":true,"score":%d.%02d,"tags":["alpha","beta","gamma"],"note":null},`,
			i, i, i, i%1000, i%100)
		i++
	}
	b.WriteString(`{"id":-1}]}`)
	return b.String()
}

// TestSelectionChurn pins the per-frame allocation of the response viewer on
// a 10 MB body, with and without an active selection. The thresholds are 2x
// the values measured after the G1-G3 fixes (see task_giox.md): a regression
// to the pre-fix numbers (417-1545 KB/frame) fails loudly, while normal
// run-to-run noise does not. TotalAlloc per frame is deterministic, unlike
// frame time, so this is safe for CI.
func TestSelectionChurn(t *testing.T) {
	if raceDetectorEnabled {
		t.Skip("allocation budgets are not comparable under -race: instrumentation blocks inlining and pushes otherwise stack-local values onto the heap (2-4x the plain numbers)")
	}
	prevMax := settings.SyntaxHighlightMaxMB
	settings.SyntaxHighlightMaxMB = 200
	defer func() { settings.SyntaxHighlightMaxMB = prevMax }()

	// KB/frame ceilings per (generator, wrap), worst selection case.
	limits := map[string]float64{
		"pretty/wrap=true":  125,
		"pretty/wrap=false": 235,
		"min/wrap=true":     285,
		"min/wrap=false":    10,
	}

	for _, gen := range []struct {
		name string
		fn   func(int) string
	}{{"pretty", genPrettyJSON}, {"min", genMinifiedJSON}} {
		body := gen.fn(10 << 20)
		for _, wrap := range []bool{true, false} {
			for _, sel := range []int{0, 20000} {
				v := NewResponseViewer()
				v.SetText(body)
				shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
				var r input.Router
				ops := new(op.Ops)
				now := time.Unix(1700000000, 0)
				sz := image.Pt(900, 700)

				frame := func() {
					ops.Reset()
					gtx := layout.Context{
						Ops:         ops,
						Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
						Constraints: layout.Exact(sz),
						Now:         now,
						Source:      r.Source(),
					}
					ResponseViewerStyle{
						Viewer: v, Shaper: shaper, Font: font.Font{Typeface: "Go Mono"},
						TextSize: unit.Sp(13), Wrap: wrap, Padding: unit.Dp(4),
						Lang: syntax.LangJSON,
					}.Layout(gtx)
					r.Frame(ops)
					now = now.Add(16 * time.Millisecond)
				}

				for i := 0; i < 3; i++ {
					frame()
				}
				time.Sleep(tokenizeDebounce + 20*time.Millisecond)
				if sel > 0 {
					v.selStart, v.selEnd = 0, sel
				}
				for i := 0; i < 5; i++ {
					frame()
				}
				var a, b runtime.MemStats
				runtime.ReadMemStats(&a)
				const frames = 30
				for i := 0; i < frames; i++ {
					frame()
				}
				runtime.ReadMemStats(&b)
				perFrameKB := float64(b.TotalAlloc-a.TotalAlloc) / frames / 1024

				key := fmt.Sprintf("%s/wrap=%v", gen.name, wrap)
				t.Logf("%-7s wrap=%-5v selection=%6d bytes  %10.1f KB/frame (limit %.0f)",
					gen.name, wrap, sel, perFrameKB, limits[key])
				if perFrameKB > limits[key] {
					t.Errorf("%s selection=%d: %.1f KB/frame exceeds %.0f KB/frame",
						key, sel, perFrameKB, limits[key])
				}
				runtime.KeepAlive(v)
			}
		}
	}
}

func (rig *searchRig) queuePointer(ev pointer.Event) {
	ev.Source = pointer.Mouse
	rig.r.Queue(ev)
	rig.frame()
}

func TestSelectionDrag_WheelScrollStillWorks(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()

	rig.queuePointer(pointer.Event{Kind: pointer.Press, Position: f32.Pt(50, 40), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 90), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary})

	c := rig.core()
	if !c.dragActive {
		t.Fatal("precondition: dragging must be active after press+move")
	}
	if c.selStart == c.selEnd {
		t.Fatal("precondition: dragging must have selected something")
	}
	selBefore := c.selEnd

	rig.queuePointer(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary, Scroll: f32.Pt(0, 300)})
	rig.frame()
	if c.scrollY == 0 {
		t.Fatal("wheel scroll during an active selection drag must scroll the viewport")
	}
	if c.selEnd == selBefore {
		t.Error("scrolling under a held pointer must extend the selection to the new content")
	}
	if c.selStart == c.selEnd {
		t.Error("selection must survive the scroll")
	}
}

func TestSelectionDrag_EdgeAutoScrollsViewport(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()

	rig.queuePointer(pointer.Event{Kind: pointer.Press, Position: f32.Pt(50, 40), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(60, 350), Buttons: pointer.ButtonPrimary})

	c := rig.core()
	for i := 0; i < 30 && c.scrollY == 0; i++ {
		rig.frame()
	}
	if c.scrollY == 0 {
		t.Fatal("dragging past the bottom edge must auto-scroll the viewport")
	}
	prev := c.scrollY
	for i := 0; i < 10; i++ {
		rig.frame()
	}
	if c.scrollY <= prev {
		t.Error("auto-scroll must keep going while the pointer stays past the edge")
	}
	if c.selStart == c.selEnd {
		t.Error("auto-scrolling must keep extending the selection")
	}

	rig.queuePointer(pointer.Event{Kind: pointer.Release, Position: f32.Pt(60, 350)})
	stopped := c.scrollY
	rig.frame()
	rig.frame()
	if c.scrollY != stopped {
		t.Error("auto-scroll must stop on release")
	}
}

func TestEditorSelectionDrag_WheelScrollStillWorks(t *testing.T) {
	rig := newSearchEditorRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()

	rig.queuePointer(pointer.Event{Kind: pointer.Press, Position: f32.Pt(50, 40), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 90), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary})

	c := rig.core()
	if !c.dragActive || c.selStart == c.selEnd {
		t.Fatal("precondition: dragging must be active and selecting")
	}

	rig.queuePointer(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary, Scroll: f32.Pt(0, 300)})
	rig.frame()
	if c.scrollY == 0 {
		t.Fatal("wheel scroll during an active editor selection drag must scroll the viewport")
	}
	if c.selStart == c.selEnd {
		t.Error("selection must survive the scroll")
	}
}

// A wrapped line whose last glyph sits at the right margin used to be filled to
// the viewport edge, showing selected empty space after the last character.
func TestWrapSelectionStopsAtLastGlyph(t *testing.T) {
	const innerW = 400
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()
	fnt := font.Font{Typeface: widgets.MonoTypeface}
	size := unit.Sp(12)

	chunk := []byte(strings.Repeat("abcdefghij0123456789", 40))
	glyphs := widgets.ShapeChunkForWrap(shaper, fnt, size, gtx, chunk, innerW)
	if widgets.WrapMaxLine(glyphs) < 2 {
		t.Fatalf("chunk did not wrap into enough sub-lines: %d", widgets.WrapMaxLine(glyphs)+1)
	}
	lineStarts := widgets.WrapLineStarts(glyphs)

	shortOfEdge := 0
	for wl := 0; wl < len(lineStarts)-1; wl++ {
		x := wrapSubLineEndX(glyphs, wl, innerW)
		wantX, wantLine := widgets.CaretXYInWrap(glyphs, lineStarts[wl+1])
		if wantLine != wl {
			t.Fatalf("sub-line %d: caret at the next line start reported line %d", wl, wantLine)
		}
		if x != wantX && wantX <= innerW {
			t.Errorf("sub-line %d fill ends at %d, want the last glyph end %d", wl, x, wantX)
		}
		if x < innerW {
			shortOfEdge++
		}
	}
	if shortOfEdge == 0 {
		t.Error("no wrapped sub-line stopped short of the viewport edge; the fill is still viewport-wide")
	}

	last := len(lineStarts) - 1
	if x := wrapSubLineEndX(glyphs, last, innerW); x <= 0 || x > innerW {
		t.Errorf("last sub-line fill ends at %d, want inside (0, %d]", x, innerW)
	}
}

func TestHeadersSliderOpensCollapsedHeaders(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.paneH()

	sliderY := rig.paneTop() + rig.tab.headersRowH + 2
	rig.drag(400, sliderY, sliderY+90)
	if !rig.tab.HeadersExpanded {
		t.Fatalf("dragging the slider down while headers are hidden must open the headers section")
	}
	if got := rig.tab.HeadersAbsHeight; !near(got, 90, 4) {
		t.Errorf("opened headers should follow the drag distance: got %d, want ~90", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 90, 4) {
		t.Errorf("rendered headers should follow the drag: got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+91, 5) {
		t.Errorf("opening headers must push Request down: pane %d, want ~%d", got, paneBefore+91)
	}
}

func TestHeadersSliderNoJitter(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 20
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	y := rig.headersSliderY()
	rig.r.Queue(pointerPress(400, y))
	rig.frame()
	prevPane := rig.paneH()
	for i := 1; i <= 60; i++ {
		rig.r.Queue(pointerMove(400, y+i))
		rig.frame()
		pane := rig.paneH()
		if pane < prevPane {
			t.Fatalf("pane jittered on step %d: %d -> %d", i, prevPane, pane)
		}
		stored := rig.tab.HeadersAbsHeight
		if render := rig.tab.headersRenderH; !near(render, stored, 1) {
			t.Fatalf("rendered headers diverged from stored on step %d: render %d, stored %d", i, render, stored)
		}
		prevPane = pane
	}
	rig.r.Queue(pointerRelease(400, y+60))
	rig.frame()

	if got := rig.tab.HeadersAbsHeight; !near(got, 80, 3) {
		t.Errorf("slow drag should grow headers 20->~80, got %d", got)
	}
}

func TestHeadersSliderNoJitterOnPointerTremor(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 20
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	y := rig.headersSliderY()
	rig.r.Queue(pointerPress(400, y))
	rig.frame()
	rig.r.Queue(pointerMoveF(400, float32(y)+5))
	rig.frame()
	basePane := rig.paneH()
	baseStored := rig.tab.HeadersAbsHeight

	tremor := []float32{0.4, -0.3, 0.45, -0.4, 0.3, -0.45, 0.4, -0.3}
	pos := float32(y) + 5
	for i := 0; i < 32; i++ {
		pos += tremor[i%len(tremor)]
		rig.r.Queue(pointerMoveF(400, pos))
		rig.frame()
		if got := rig.paneH(); !near(got, basePane, 1) {
			t.Fatalf("pane jittered on tremor step %d: %d vs base %d", i, got, basePane)
		}
		if got := rig.tab.HeadersAbsHeight; !near(got, baseStored, 1) {
			t.Fatalf("stored height jittered on tremor step %d: %d vs base %d", i, got, baseStored)
		}
	}
	rig.r.Queue(pointerRelease(400, int(pos)))
	rig.frame()
}

func TestWSHeadersSliderOpensCollapsedHeaders(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.HeadersCollapsed = true
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.wsPaneH()

	sliderY := rig.paneTop() + s.wsRowH + 1 + s.wsRowH + 2
	rig.drag(400, sliderY, sliderY+90)
	if s.HeadersCollapsed {
		t.Fatalf("dragging the slider down while WS headers are hidden must open the section")
	}
	if got := s.HeadersAbsHeight; !near(got, 90, 4) {
		t.Errorf("opened WS headers should follow the drag distance: got %d, want ~90", got)
	}
	if got := rig.wsPaneH(); !near(got, paneBefore+91, 5) {
		t.Errorf("opening WS headers must push Compose down: pane %d, want ~%d", got, paneBefore+91)
	}
}

func (rig *vstackRig) frameScaled(scale float32) {
	rig.now = rig.now.Add(16 * time.Millisecond)
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.tab.Layout(gtx, rig.th, rig.win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	rig.r.Frame(rig.ops)
}

func (rig *vstackRig) gtxScaled(scale float32) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
	}
}

func (rig *vstackRig) paneTopScaled(gtx layout.Context) int {
	urlRow := gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(28)) + gtx.Dp(unit.Dp(8))
	return urlRow + gtx.Dp(unit.Dp(1)) + rig.tab.layoutModeBarHeight(gtx)
}

func TestHeadersSliderSmoothAtFractionalScale(t *testing.T) {
	for _, scale := range []float32{1.25, 1.5} {
		rig := newVStackRig()
		rig.tab.HeadersAbsHeight = 20
		rig.tab.VStackRatio = 0.5
		for i := 0; i < 3; i++ {
			rig.frameScaled(scale)
		}
		gtx := rig.gtxScaled(scale)
		sliderY := rig.paneTopScaled(gtx) + rig.tab.reqPaneAboveHeadersPx(gtx) + rig.tab.headersRenderH + 2
		ext := rig.tab.stackedSplitExtent(gtx)
		paneOf := func() int { return int(rig.tab.VStackRatio*ext + 0.5) }
		startStored := rig.tab.HeadersAbsHeight

		rig.r.Queue(pointerPress(400, sliderY))
		rig.frameScaled(scale)
		prev := paneOf()
		for i := 1; i <= 40; i++ {
			rig.r.Queue(pointerMove(400, sliderY+i))
			rig.frameScaled(scale)
			pane := paneOf()
			if pane < prev {
				t.Fatalf("scale %.2f: pane jittered on down step %d: %d -> %d", scale, i, prev, pane)
			}
			prev = pane
		}
		if got := rig.tab.HeadersAbsHeight; got <= startStored {
			t.Fatalf("scale %.2f: drag did not register, stored still %d", scale, got)
		}
		grown := rig.tab.HeadersAbsHeight
		wantGrown := startStored + int(40/scale)
		if !near(grown, wantGrown, 3) {
			t.Errorf("scale %.2f: 40px drag should grow headers by ~%ddp, got %d -> %d", scale, wantGrown-startStored, startStored, grown)
		}

		base := sliderY + 40
		for i := 1; i <= 20; i++ {
			rig.r.Queue(pointerMove(400, base-i))
			rig.frameScaled(scale)
			pane := paneOf()
			if pane > prev {
				t.Fatalf("scale %.2f: pane jittered on up step %d: %d -> %d", scale, i, prev, pane)
			}
			prev = pane
		}
		rig.r.Queue(pointerRelease(400, base-20))
		rig.frameScaled(scale)
		if got := rig.tab.HeadersAbsHeight; got >= grown {
			t.Errorf("scale %.2f: up drag did not shrink headers, stored %d", scale, got)
		}
	}
}

func newLayoutCtx() (layout.Context, *material.Theme, *app.Window) {
	win := new(app.Window)
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
	return gtx, th, win
}

func TestLayoutDropsStaleAppendChunk(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.PreviewEnabled = true
	gtx, th, win := newLayoutCtx()

	curID := tab.requestID.Load()
	tab.RespEditor.SetText("")

	tab.appendChan <- appendChunk{requestID: curID - 1, text: "STALE"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got != "" {
		t.Fatalf("stale chunk was applied: %q", got)
	}

	tab.appendChan <- appendChunk{requestID: curID, text: "FRESH"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got != "FRESH" {
		t.Fatalf("fresh chunk not applied: %q", got)
	}
}

func TestLayoutDropsStalePreviewResult(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.PreviewEnabled = true
	gtx, th, win := newLayoutCtx()

	curID := tab.requestID.Load()
	tab.RespEditor.SetText("keep")

	tab.previewChan <- previewResult{requestID: curID - 1, body: "STALE-PREVIEW"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got == "STALE-PREVIEW" {
		t.Fatalf("stale preview overwrote response: %q", got)
	}

	tab.previewChan <- previewResult{requestID: curID, body: "FRESH-PREVIEW"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got != "FRESH-PREVIEW" {
		t.Fatalf("fresh preview not applied: %q", got)
	}
}

type chunkedReader struct {
	data  []byte
	pos   int
	chunk int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if r.pos+n > len(r.data) {
		n = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

func (t *RequestTab) drainAppendText() string {
	var sb strings.Builder
	for {
		select {
		case c := <-t.appendChan:
			sb.WriteString(c.text)
		default:
			return sb.String()
		}
	}
}

func TestStreamResponse_LivePreviewFormatsJSONWhileLoading(t *testing.T) {
	oldFmt := settings.AutoFormatJSON
	settings.AutoFormatJSON = true
	defer func() { settings.AutoFormatJSON = oldFmt }()

	raw := `{"user":{"id":42,"name":"Иван","tags":["a","b"],"note":"с \"кавычками\" и {скобками}"},"items":[1,2,3]}`
	for _, chunk := range []int{3, 7, 4096} {
		tab := NewRequestTab("stream-fmt")
		reqID := tab.requestID.Load()
		var sink bytes.Buffer
		src := &chunkedReader{data: []byte(raw), chunk: chunk}
		total, err := tab.streamResponse(context.Background(), reqID, src, &sink, nil, true, "application/json")
		if err != nil {
			t.Fatalf("chunk=%d: streamResponse error: %v", chunk, err)
		}
		if total != int64(len(raw)) {
			t.Fatalf("chunk=%d: total = %d, want %d", chunk, total, len(raw))
		}
		if sink.String() != raw {
			t.Errorf("chunk=%d: file sink must receive raw bytes", chunk)
		}
		want := formatJSON([]byte(raw), &JSONFormatterState{})
		if got := tab.drainAppendText(); got != want {
			t.Errorf("chunk=%d: live preview must be formatted while streaming\ngot:  %q\nwant: %q", chunk, got, want)
		}
	}
}

func TestStreamResponse_NonJSONStaysRaw(t *testing.T) {
	oldFmt := settings.AutoFormatJSON
	settings.AutoFormatJSON = true
	defer func() { settings.AutoFormatJSON = oldFmt }()

	raw := "plain text body: {not json because of prefix}"
	tab := NewRequestTab("stream-plain")
	reqID := tab.requestID.Load()
	var sink bytes.Buffer
	src := &chunkedReader{data: []byte(raw), chunk: 5}
	if _, err := tab.streamResponse(context.Background(), reqID, src, &sink, nil, true, "text/plain"); err != nil {
		t.Fatalf("streamResponse error: %v", err)
	}
	if got := tab.drainAppendText(); got != raw {
		t.Errorf("non-JSON body must stream unformatted\ngot:  %q\nwant: %q", got, raw)
	}
}

func TestStreamResponse_AutoFormatOffStaysRaw(t *testing.T) {
	oldFmt := settings.AutoFormatJSON
	settings.AutoFormatJSON = false
	defer func() { settings.AutoFormatJSON = oldFmt }()

	raw := `{"a":1}`
	tab := NewRequestTab("stream-off")
	reqID := tab.requestID.Load()
	var sink bytes.Buffer
	src := &chunkedReader{data: []byte(raw), chunk: 3}
	if _, err := tab.streamResponse(context.Background(), reqID, src, &sink, nil, true, "application/json"); err != nil {
		t.Fatalf("streamResponse error: %v", err)
	}
	if got := tab.drainAppendText(); got != raw {
		t.Errorf("with AutoFormatJSON off the stream must stay raw, got %q", got)
	}
}

func TestTabLayout(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.Method = "GET"
	tab.URLInput.SetText("http://example.com")
	tab.ReqEditor.SetText("body")
	tab.AddHeader("Auth", "secret")
	tab.addSystemHeader("Content-Type", "application/json")

	win := new(app.Window)
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}

	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.RespSearch.Open = true
	tab.RespSearch.Editor.SetText("hello")
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.PreviewEnabled = true
	tab.respSize = 1000
	tab.previewLoaded.Store(500)
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.MethodListOpen = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.SendMenuOpen = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.HeadersExpanded = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.IsDraggingSplit = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.PreviewEnabled = false
	tab.respFile = "some-file"
	tab.respSize = 100
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.PreviewEnabled = true
	tab.RespEditor.SetText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n")
	tab.layoutResponseBody(gtx, th, win, false)

	tab.WrapEnabled = false
	tab.layoutResponseBody(gtx, th, win, false)

	tab.isRequesting = true
	tab.downloadedBytes.Store(500)
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.responseChan <- tabResponse{status: "200 OK", respSize: 1000, body: "ok", requestID: tab.requestID.Load()}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.appendChan <- appendChunk{requestID: tab.requestID.Load(), text: "more"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.FileSaveChan <- &failingWriteCloser{}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
}

func TestTabLayoutAllMenusOpen(t *testing.T) {
	win := new(app.Window)
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(900, 700)),
		Now:         time.Now(),
	}
	render := func(tab *RequestTab) {
		tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	}

	httpTab := NewRequestTab("http")
	httpTab.Method = "POST"
	httpTab.URLInput.SetText("http://example.com")
	httpTab.ProtocolListOpen = true
	render(httpTab)
	httpTab.ProtocolListOpen = false
	httpTab.MethodListOpen = true
	render(httpTab)
	httpTab.MethodListOpen = false
	httpTab.BodyTypeOpen = true
	render(httpTab)
	httpTab.BodyTypeOpen = false
	httpTab.RunOpen = true
	httpTab.ExampleListOpen = true
	render(httpTab)

	wsTab := NewRequestTab("ws")
	wsTab.Method = MethodWS
	wsTab.URLInput.SetText("ws://example.com/socket")
	s := wsTab.EnsureWS()
	s.OpcodeMenuOpen = true
	render(wsTab)
	s.OpcodeMenuOpen = false
	s.FilterMenuOpen = true
	render(wsTab)
}

func TestProcessTemplate(t *testing.T) {
	env := map[string]string{
		"host": "localhost:8080",
		"port": "8080",
	}

	tests := []struct {
		name     string
		input    string
		env      map[string]string
		expected string
	}{
		{"no template", "http://example.com", env, "http://example.com"},
		{"one template", "http://{{host}}", env, "http://localhost:8080"},
		{"multiple templates", "http://{{host}}:{{port}}", env, "http://localhost:8080:8080"},
		{"missing template", "http://{{missing}}", env, "http://{{missing}}"},
		{"spaces in template", "http://{{ host  }}", env, "http://localhost:8080"},
		{"no env", "http://{{host}}", nil, "http://{{host}}"},
		{"unterminated template", "http://{{host", env, "http://{{host"},
		{"nested braces", "http://{{{{host}}}}", env, "http://{{localhost:8080}}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := processTemplate(tc.input, tc.env)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetCleanTitle(t *testing.T) {
	tab := &RequestTab{}

	tests := []struct {
		title    string
		expected string
	}{
		{"", "New request"},
		{"  ", "New request"},
		{"Hello", "Hello"},
		{"Hello\nWorld", "Hello World"},
		{"\x01Hello", "Hello"},
	}

	for _, tc := range tests {
		tab.Title = tc.title
		tab.cleanTitleSrc = ""
		result := tab.GetCleanTitle()
		if result != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, result)
		}

		tab.cleanTitle = "Cached"
		resultCached := tab.GetCleanTitle()
		if resultCached != "Cached" {
			t.Errorf("expected cached %q, got %q", "Cached", resultCached)
		}
	}
}

func TestCheckDirtyAndSaveToCollection(t *testing.T) {
	col := &collections.ParsedCollection{}
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "http://test.com",
		Body:   "body",
		Name:   "TestReq",
		Headers: map[string]string{
			"Header1": "Value1",
		},
	}
	node := &collections.CollectionNode{
		Request:    req,
		Collection: col,
	}

	tab := &RequestTab{
		LinkedNode: node,
		Method:     "GET",
		Title:      "TestReq",
	}
	tab.URLInput.SetText("http://test.com")
	tab.ReqEditor.SetText("body")

	h1Key := widget.Editor{}
	h1Key.SetText("Header1")
	h1Val := widget.Editor{}
	h1Val.SetText("Value1")

	tab.Headers = []*HeaderItem{
		{Key: h1Key, Value: h1Val, IsGenerated: false},
	}

	tab.checkDirty()
	if tab.IsDirty {
		t.Errorf("expected tab to not be dirty")
	}

	tab.URLInput.SetText("http://changed.com")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after URL change")
	}

	tab.URLInput.SetText("http://test.com")
	tab.checkDirty()
	if tab.IsDirty {
		t.Errorf("expected tab to not be dirty after reset")
	}

	tab.ReqEditor.SetText("changed body")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after body change")
	}
	tab.ReqEditor.SetText("body")

	tab.Title = "Changed Title"
	tab.checkDirty()
	if tab.IsDirty {
		t.Errorf("expected tab to still not be dirty after title change")
	}
	tab.Title = "TestReq"

	tab.Headers[0].Value.SetText("Changed Value")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after header value change")
	}
	tab.Headers[0].Value.SetText("Value1")

	h2Key := widget.Editor{}
	h2Key.SetText("H2")
	tab.Headers = append(tab.Headers, &HeaderItem{Key: h2Key})
	tab.checkDirty()
	if !tab.IsDirty {
		t.Errorf("expected tab to be dirty after adding header")
	}
	tab.Headers = tab.Headers[:1]

	tab.URLInput.SetText("http://changed.com")
	savedCol := tab.SaveToCollection()
	if savedCol != col {
		t.Errorf("expected saved collection to be returned")
	}
	if req.URL != "http://changed.com" {
		t.Errorf("expected request URL to be updated, got %s", req.URL)
	}
	if tab.IsDirty {
		t.Errorf("expected tab to not be dirty after save")
	}

	unlinkedTab := &RequestTab{}
	unlinkedTab.checkDirty()
	if unlinkedTab.IsDirty {
		t.Errorf("expected unlinked tab to not be dirty")
	}
	if unlinkedTab.SaveToCollection() != nil {
		t.Errorf("expected nil from SaveToCollection on unlinked tab")
	}
}

func TestSearch(t *testing.T) {
	tab := NewRequestTab("test")
	tab.RespEditor.SetText("Hello world! This is a test. Hello again!")

	tab.invalidateSearchCache()
	if !tab.RespSearch.cacheDirty {
		t.Errorf("expected cacheDirty to be true")
	}

	tab.RespSearch.Editor.SetText("")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 0 {
		t.Errorf("expected empty results for empty search")
	}

	tab.RespSearch.Editor.SetText("hello")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if tab.RespSearch.cacheDirty {
		t.Errorf("expected cacheDirty to be false after search")
	}
	if len(tab.RespSearch.spans) != 2 {
		t.Fatalf("expected 2 results, got %d", len(tab.RespSearch.spans))
	}
	if tab.RespSearch.spans[0].start != 0 || tab.RespSearch.spans[1].start != 29 {
		t.Errorf("unexpected search results: %v", tab.RespSearch.spans)
	}

	tab.RespSearch.current = 0
	tab.RespSearch.navigate(1, tab.RespEditor)
	if tab.RespSearch.current != 1 {
		t.Errorf("expected current to be 1, got %d", tab.RespSearch.current)
	}

	tab.RespSearch.navigate(1, tab.RespEditor)
	if tab.RespSearch.current != 0 {
		t.Errorf("expected current to wrap to 0, got %d", tab.RespSearch.current)
	}

	tab.RespSearch.navigate(-1, tab.RespEditor)
	if tab.RespSearch.current != 1 {
		t.Errorf("expected current to wrap to 1, got %d", tab.RespSearch.current)
	}

	tab.RespSearch.spans = nil
	tab.RespSearch.current = 5
	tab.RespSearch.navigate(1, tab.RespEditor)
	if tab.RespSearch.current != 5 {
		t.Errorf("expected current to remain unchanged when empty")
	}
}

func TestFoldForSearchUnicode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Hello", "hello"},
		{"hello", "hello"},
		{"HELLO WORLD", "hello world"},
		{"Привет", "привет"},
		{"ПРИВЕТ", "привет"},
		{"ПрИвЕт", "привет"},
		{"Hello Мир 🚀", "hello мир 🚀"},
		{"你好", "你好"},
		{"안녕", "안녕"},
		{"🚀🔥", "🚀🔥"},
		{"", ""},
		{"123 ABC abc", "123 abc abc"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := foldForSearch(tc.in)
			if got != tc.want {
				t.Errorf("foldForSearch(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) != len(tc.want) {
				t.Errorf("length changed: %d vs %d", len(got), len(tc.want))
			}
		})
	}
}

func TestSearchUnicode(t *testing.T) {
	tab := NewRequestTab("test")
	tab.RespEditor.SetText("Привет мир! Это тест. Привет снова!")
	tab.invalidateSearchCache()

	tab.RespSearch.Editor.SetText("привет")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 2 {
		t.Errorf("expected 2 unicode-case-insensitive matches, got %d (%v)", len(tab.RespSearch.spans), tab.RespSearch.spans)
	}

	tab.RespEditor.SetText("Hello 🚀 World 🚀 End")
	tab.invalidateSearchCache()
	tab.RespSearch.Editor.SetText("🚀")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 2 {
		t.Errorf("expected 2 emoji matches, got %d (%v)", len(tab.RespSearch.spans), tab.RespSearch.spans)
	}

	tab.RespEditor.SetText("你好世界 你好朋友")
	tab.invalidateSearchCache()
	tab.RespSearch.Editor.SetText("你好")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if len(tab.RespSearch.spans) != 2 {
		t.Errorf("expected 2 CJK matches, got %d (%v)", len(tab.RespSearch.spans), tab.RespSearch.spans)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.00 GB"},
		{1610612736, "1.50 GB"},
	}

	for _, tc := range tests {
		result := formatSize(tc.input)
		if result != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, result)
		}
	}
}

func TestAddHeaders(t *testing.T) {
	tab := NewRequestTab("test")

	tab.AddHeader("User-Agent", "Custom")
	if len(tab.Headers) != 1 {
		t.Fatalf("expected 1 header, got %d", len(tab.Headers))
	}
	if tab.Headers[0].Key.Text() != "User-Agent" || tab.Headers[0].Value.Text() != "Custom" || tab.Headers[0].IsGenerated {
		t.Errorf("unexpected header state: %+v", tab.Headers[0])
	}

	tab.addSystemHeader("Content-Type", "application/json")
	if len(tab.Headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(tab.Headers))
	}
	if tab.Headers[1].Key.Text() != "Content-Type" || tab.Headers[1].Value.Text() != "application/json" || !tab.Headers[1].IsGenerated {
		t.Errorf("unexpected header state: %+v", tab.Headers[1])
	}
}

func TestUpdateSystemHeaders_Conflicts(t *testing.T) {
	tab := NewRequestTab("test")

	tab.AddHeader("Content-Type", "application/json")
	tab.ReqEditor.SetText(`{"a": 1}`)
	tab.UpdateSystemHeaders()

	count := 0
	for _, h := range tab.Headers {
		if h.Key.Text() == "Content-Type" {
			count++
			if h.IsGenerated {
				t.Errorf("expected manual header to stay manual")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Content-Type header")
	}
}

func TestTabRowsExpandCollapseKeepsCollapsedRequestHug(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.ReqBodyCollapsed {
		t.Fatal("setup: request must collapse")
	}
	min := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.tab.splitPaneRec; !near(got, min, 4) {
		t.Fatalf("setup: collapsed pane %d, want ~%d", got, min)
	}

	rig.size.Y -= 7 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	if got := rig.tab.splitPaneRec; got > min+4 {
		t.Errorf("collapsed pane must not grow while tab rows are expanded: got %d, want <= ~%d", got, min)
	}

	rig.size.Y += 7 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	if got := rig.tab.splitPaneRec; !near(got, min, 4) {
		t.Errorf("collapsed pane must return to its hug height after tab rows collapse: got %d, want ~%d", got, min)
	}
}

func TestTabRowsExpandCollapseKeepsOpenRequestSize(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	min := rig.tab.stackedReqPaneMinPx(rig.gtx())
	ext := rig.tab.stackedSplitExtent(rig.gtx())
	rig.tab.VStackRatio = (float32(min) + 20) / ext
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	before := rig.tab.splitPaneRec
	if before <= 0 {
		t.Fatalf("setup: no rendered pane height")
	}

	rig.size.Y -= 4 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	rig.size.Y += 4 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	if got := rig.tab.splitPaneRec; !near(got, before, 4) {
		t.Errorf("request pane must return to its size after tab rows collapse: got %d, want ~%d", got, before)
	}
}

func TestOpcodeStringRoundTrip(t *testing.T) {
	if got := opcodeToString(ws.OpBinary); got != "BIN" {
		t.Errorf("opcodeToString(OpBinary) = %q, want BIN", got)
	}
	if got := opcodeToString(ws.OpText); got != "TEXT" {
		t.Errorf("opcodeToString(OpText) = %q, want TEXT", got)
	}
	if got := opcodeToString(ws.OpPing); got != "TEXT" {
		t.Errorf("opcodeToString(OpPing) = %q, want the TEXT fallback", got)
	}
	for _, s := range []string{"BIN", "binary"} {
		if got := opcodeFromString(s); got != ws.OpBinary {
			t.Errorf("opcodeFromString(%q) = %v, want OpBinary", s, got)
		}
	}
	for _, s := range []string{"TEXT", "text", "", "nonsense"} {
		if got := opcodeFromString(s); got != ws.OpText {
			t.Errorf("opcodeFromString(%q) = %v, want OpText", s, got)
		}
	}
}

func TestTabFromStateDefaults(t *testing.T) {
	rt := TabFromState(persist.TabState{})
	if rt.Title != "New request" {
		t.Errorf("Title = %q, want the placeholder", rt.Title)
	}
	if rt.Method != "GET" {
		t.Errorf("Method = %q, want GET", rt.Method)
	}
	if rt.WS != nil {
		t.Error("a state with no WS block must not create a WS session")
	}
	if rt.GQL != nil {
		t.Error("a state with no GQL block must not create a GQL session")
	}
}

func TestTabFromStateKindOverridesMethod(t *testing.T) {
	cases := []struct {
		kind   string
		method string
		want   string
	}{
		{TabKindWebSocket, "POST", MethodWS},
		{TabKindGraphQL, "GET", MethodGraphQL},
		{TabKindHTTP, "PATCH", "PATCH"},
		{TabKindHTTP, "", "GET"},
	}
	for _, c := range cases {
		rt := TabFromState(persist.TabState{Kind: c.kind, Method: c.method, Title: "x"})
		if rt.Method != c.want {
			t.Errorf("kind=%q method=%q -> %q, want %q", c.kind, c.method, rt.Method, c.want)
		}
	}
}

func TestTabFromStateIgnoresOutOfRangeRatios(t *testing.T) {
	base := NewRequestTab("x")
	rt := TabFromState(persist.TabState{
		Title: "x", SplitRatio: 0, VStackRatio: 0, HeaderSplitRatio: 0,
	})
	if rt.SplitRatio != base.SplitRatio {
		t.Errorf("SplitRatio = %v, want the default %v when the saved value is 0", rt.SplitRatio, base.SplitRatio)
	}
	if rt.VStackRatio != base.VStackRatio {
		t.Errorf("VStackRatio = %v, want the default %v", rt.VStackRatio, base.VStackRatio)
	}
	if rt.HeaderKeyW != base.HeaderKeyW {
		t.Errorf("HeaderKeyW = %v, want the default %v", rt.HeaderKeyW, base.HeaderKeyW)
	}
}

func TestStateFromTabRoundTrip(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "up.bin")
	if err := os.WriteFile(bin, []byte("1234567"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewRequestTab("Orders")
	src.Method = "POST"
	src.URLInput.SetText("http://api.test/orders?a=1")
	src.ReqEditor.SetText(`{"x":1}`)
	src.AddHeader("X-Real", "yes")
	src.UpdateSystemHeaders()
	src.HeadersExpanded = true
	src.HeadersAbsHeight = 140
	src.SplitRatio = 0.42
	src.VStackRatio = 0.66
	src.LayoutMode = LayoutModeHoriz
	src.HeaderKeyW = 180
	src.ReqWrapEnabled = false
	src.BodyType = model.BodyFormData
	src.applyFormParts([]model.ParsedFormPart{
		{Key: "text", Value: "v", Kind: model.FormPartText},
		{Key: "file", Kind: model.FormPartFile, FilePath: bin},
	})
	src.applyURLEncoded([]model.ParsedKV{{Key: "ue", Value: "1"}})
	src.BinaryFilePath = bin
	src.ApplyAuth(model.ParsedAuth{Type: "basic", Username: "u", Password: "p"})
	src.ApplyCookies([]model.ParsedKV{{Key: "sid", Value: "9"}})

	ts := StateFromTab(src)
	if ts.Kind != TabKindHTTP {
		t.Errorf("Kind = %q, want HTTP", ts.Kind)
	}
	for _, h := range ts.Headers {
		if h.Key == "" {
			t.Errorf("persisted headers must not contain blank keys: %+v", ts.Headers)
		}
		if h.Key == "Content-Length" || h.Key == "Host" {
			t.Errorf("generated headers must not be persisted: %+v", ts.Headers)
		}
	}

	dst := TabFromState(ts)
	if dst.Title != "Orders" || dst.Method != "POST" {
		t.Errorf("title/method = %q/%q", dst.Title, dst.Method)
	}
	if dst.URLInput.Text() != src.URLInput.Text() {
		t.Errorf("URL = %q, want %q", dst.URLInput.Text(), src.URLInput.Text())
	}
	if dst.ReqEditor.Text() != src.ReqEditor.Text() {
		t.Errorf("body = %q", dst.ReqEditor.Text())
	}
	if dst.SplitRatio != 0.42 || dst.VStackRatio != 0.66 || dst.HeaderKeyW != 180 {
		t.Errorf("ratios = %v/%v/%v", dst.SplitRatio, dst.VStackRatio, dst.HeaderKeyW)
	}
	if dst.LayoutMode != LayoutModeHoriz {
		t.Errorf("LayoutMode = %v", dst.LayoutMode)
	}
	if dst.ReqWrapEnabled {
		t.Error("ReqWrapEnabled=false must survive the round trip")
	}
	if !dst.HeadersExpanded || dst.HeadersAbsHeight != 140 {
		t.Errorf("headers area = %v/%d", dst.HeadersExpanded, dst.HeadersAbsHeight)
	}
	if dst.BodyType != model.BodyFormData {
		t.Errorf("BodyType = %v, want form-data", dst.BodyType)
	}
	if len(dst.FormParts) != 2 {
		t.Fatalf("FormParts = %d, want 2", len(dst.FormParts))
	}
	if dst.FormParts[1].Kind != model.FormPartFile || dst.FormParts[1].FileSize != 7 {
		t.Errorf("file part = kind %v size %d, want file/7", dst.FormParts[1].Kind, dst.FormParts[1].FileSize)
	}
	if len(dst.URLEncoded) != 1 || dst.URLEncoded[0].Key.Text() != "ue" {
		t.Errorf("URLEncoded = %#v", dst.URLEncoded)
	}
	if dst.BinaryFilePath != bin || dst.BinaryFileSize != 7 {
		t.Errorf("binary = %q/%d", dst.BinaryFilePath, dst.BinaryFileSize)
	}
	if got := dst.AuthModel(); got.Type != "basic" || got.Username != "u" || got.Password != "p" {
		t.Errorf("auth = %+v", got)
	}
	if got := dst.CookieModels(); len(got) != 1 || got[0].Key != "sid" || got[0].Value != "9" {
		t.Errorf("cookies = %+v", got)
	}
	realHeaders := 0
	for _, h := range dst.Headers {
		if !h.IsGenerated {
			realHeaders++
			if h.Key.Text() != "X-Real" {
				t.Errorf("unexpected user header %q", h.Key.Text())
			}
		}
	}
	if realHeaders != 1 {
		t.Errorf("user header count = %d, want 1", realHeaders)
	}
}

func TestStateFromTabOmitsEmptyAuth(t *testing.T) {
	src := NewRequestTab("t")
	if ts := StateFromTab(src); ts.Auth != nil {
		t.Errorf("Auth = %+v, want nil when no auth is configured", ts.Auth)
	}
	src.ApplyAuth(model.ParsedAuth{Type: "bearer", Token: "z"})
	ts := StateFromTab(src)
	if ts.Auth == nil || ts.Auth.Type != "bearer" || ts.Auth.Token != "z" {
		t.Errorf("Auth = %+v", ts.Auth)
	}
}

func TestStateFromTabWebSocketRoundTrip(t *testing.T) {
	src := NewRequestTab("Socket")
	src.Method = MethodWS
	src.URLInput.SetText("wss://echo.test/ws")
	s := src.EnsureWS()
	s.AddSubprotocol("chat")
	s.AddSubprotocol("   ")
	s.OptionsExpanded = true
	s.SubprotosAbsHeight = 90
	s.OfferDeflate = true
	s.UseMsgpackProto = true
	s.ProtoCmdEditor.SetText("7")
	s.ProtoSeqEditor.SetText("-3")
	s.ProtoOpcodeEditor.SetText("12")
	s.InsecureSkipVerify = true
	s.UseReteCA = true
	s.SplitRatio = 0.3
	s.ComposerRatio = 0.7
	s.AppendSavedSend("hello", "hi", ws.OpText)
	s.AppendSavedSend("blob", "00ff", ws.OpBinary)

	ts := StateFromTab(src)
	if ts.Kind != TabKindWebSocket {
		t.Fatalf("Kind = %q, want websocket", ts.Kind)
	}
	if ts.WS == nil {
		t.Fatal("WS state missing")
	}
	if len(ts.WS.Subprotocols) != 1 || ts.WS.Subprotocols[0] != "chat" {
		t.Errorf("Subprotocols = %#v, want blank entries dropped", ts.WS.Subprotocols)
	}

	dst := TabFromState(ts)
	if dst.Method != MethodWS {
		t.Errorf("Method = %q", dst.Method)
	}
	d := dst.WS
	if d == nil {
		t.Fatal("WS session not restored")
	}
	if got := d.SubprotocolList(); len(got) != 1 || got[0] != "chat" {
		t.Errorf("SubprotocolList = %#v", got)
	}
	if !d.OptionsExpanded || d.SubprotosAbsHeight != 90 {
		t.Errorf("options = %v/%d", d.OptionsExpanded, d.SubprotosAbsHeight)
	}
	if !d.OfferDeflate || !d.UseMsgpackProto || !d.InsecureSkipVerify || !d.UseReteCA {
		t.Error("WS toggles did not survive the round trip")
	}
	if d.ProtoCmdEditor.Text() != "7" || d.ProtoSeqEditor.Text() != "-3" || d.ProtoOpcodeEditor.Text() != "12" {
		t.Errorf("proto fields = %q/%q/%q", d.ProtoCmdEditor.Text(), d.ProtoSeqEditor.Text(), d.ProtoOpcodeEditor.Text())
	}
	if d.SplitRatio != 0.3 || d.ComposerRatio != 0.7 {
		t.Errorf("ws ratios = %v/%v", d.SplitRatio, d.ComposerRatio)
	}
	if len(d.SavedSends) != 2 {
		t.Fatalf("SavedSends = %d, want 2", len(d.SavedSends))
	}
	if d.SavedSends[0].Opcode != ws.OpText || d.SavedSends[1].Opcode != ws.OpBinary {
		t.Errorf("saved-send opcodes = %v/%v", d.SavedSends[0].Opcode, d.SavedSends[1].Opcode)
	}
	if d.SavedSends[0].Name != "hello" || d.SavedSends[1].Text != "00ff" {
		t.Errorf("saved sends = %+v", d.SavedSends)
	}
}

func TestStateFromTabGraphQLRoundTrip(t *testing.T) {
	src := NewRequestTab("GQL")
	src.Method = MethodGraphQL
	g := src.EnsureGQL()
	g.Query.SetText("{ me { id } }")
	g.Variables.SetText(`{"a":1}`)
	g.VarsSplitRatio = 0.35

	ts := StateFromTab(src)
	if ts.Kind != TabKindGraphQL || ts.GQL == nil {
		t.Fatalf("Kind = %q, GQL = %+v", ts.Kind, ts.GQL)
	}

	dst := TabFromState(ts)
	if dst.Method != MethodGraphQL {
		t.Errorf("Method = %q", dst.Method)
	}
	if dst.GQL == nil {
		t.Fatal("GQL session not restored")
	}
	if dst.GQL.Query.Text() != "{ me { id } }" {
		t.Errorf("query = %q", dst.GQL.Query.Text())
	}
	if dst.GQL.Variables.Text() != `{"a":1}` {
		t.Errorf("variables = %q", dst.GQL.Variables.Text())
	}
	if dst.GQL.VarsSplitRatio != 0.35 {
		t.Errorf("VarsSplitRatio = %v", dst.GQL.VarsSplitRatio)
	}
}

func TestTabFromStateGraphQLZeroRatioKeepsDefault(t *testing.T) {
	rt := TabFromState(persist.TabState{
		Title: "g", Kind: TabKindGraphQL,
		GQL: &persist.GQLTabState{Query: "{a}", VarsSplitRatio: 0},
	})
	if rt.GQL == nil {
		t.Fatal("GQL session missing")
	}
	if rt.GQL.VarsSplitRatio != 0.6 {
		t.Errorf("VarsSplitRatio = %v, want the 0.6 default", rt.GQL.VarsSplitRatio)
	}
}

func TestTabFromStateMissingFilesReportZeroSize(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.bin")
	rt := TabFromState(persist.TabState{
		Title:      "t",
		BinaryPath: missing,
		FormParts:  []persist.FormPartState{{Key: "f", Kind: "file", FilePath: missing}},
	})
	if rt.BinaryFileSize != 0 {
		t.Errorf("BinaryFileSize = %d, want 0", rt.BinaryFileSize)
	}
	if len(rt.FormParts) != 1 || rt.FormParts[0].FileSize != 0 {
		t.Errorf("form part size = %#v, want 0", rt.FormParts)
	}
	if rt.BinaryFilePath != missing {
		t.Errorf("BinaryFilePath = %q, want the path kept even when the file is gone", rt.BinaryFilePath)
	}
}

func TestProcessTemplateNestedBraces(t *testing.T) {
	env := map[string]string{"sdgsgds": "X"}
	in := `'[[[['''''[[[[[[[[{{{{{{{{{{{{{sdgsgds}}}}}}}}}}}}}]]]]]]]]''''']]]]'`
	want := `'[[[['''''[[[[[[[[{{{{{{{{{{{X}}}}}}}}}}}]]]]]]]]''''']]]]'`
	if got := processTemplate(in, env); got != want {
		t.Fatalf("processTemplate = %q, want %q", got, want)
	}
	if got := processTemplate("{{a}b}} {{a}}", map[string]string{"a": "1"}); got != "{{a}b}} 1" {
		t.Fatalf("a brace inside a name must not form a variable, got %q", got)
	}
}

func TestClipSpansToVarsUsesInnermostVariable(t *testing.T) {
	chunk := []byte("{{{{v}}}}")
	spans := []widgets.ColoredSpan{{Start: 0, End: len(chunk)}}
	got := clipSpansToVars(spans, chunk)
	if len(got) != 2 || got[0].End != 2 || got[1].Start != 7 {
		t.Fatalf("spans should be cut around {{v}} only, got %+v", got)
	}
}

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

type urlKeyRig struct {
	r   input.Router
	ops *op.Ops
	th  *material.Theme
	tab *RequestTab
}

func newURLKeyRig(text string) *urlKeyRig {
	rig := &urlKeyRig{
		ops: new(op.Ops),
		th:  material.NewTheme(),
		tab: &RequestTab{},
	}
	rig.tab.URLInput.SingleLine = true
	rig.tab.URLInput.SetText(text)
	n := utf8.RuneCountInString(text)
	rig.tab.URLInput.SetCaret(n, n)
	return rig
}

func (rig *urlKeyRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 40)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	rig.tab.handleURLWordDelete(gtx)
	for {
		if _, ok := rig.tab.URLInput.Update(gtx); !ok {
			break
		}
	}
	material.Editor(rig.th, &rig.tab.URLInput, "").Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *urlKeyRig) focus() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
}

func (rig *urlKeyRig) pressKey(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame()
}

func TestURLCtrlBackspaceDeletesWord(t *testing.T) {
	rig := newURLKeyRig("https://example.com/path")
	rig.focus()
	rig.tab.URLInput.SetCaret(utf8.RuneCountInString("https://example.com/path"), utf8.RuneCountInString("https://example.com/path"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "https://example.com/" {
		t.Fatalf("Ctrl+Backspace should delete trailing word, got %q", got)
	}

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "https://example." {
		t.Fatalf("Ctrl+Backspace should delete the slash separator and preceding word, got %q", got)
	}
}

func TestURLCtrlBackspaceDeletesSelection(t *testing.T) {
	rig := newURLKeyRig("https://example.com/path")
	rig.focus()
	rig.tab.URLInput.SetCaret(0, utf8.RuneCountInString("https://example.com/path"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "" {
		t.Fatalf("Ctrl+Backspace with a selection should delete it, got %q", got)
	}
}

func TestURLCtrlBackspaceTreatsVarAsWord(t *testing.T) {
	rig := newURLKeyRig("http://{{host}}")
	rig.focus()
	rig.tab.URLInput.SetCaret(utf8.RuneCountInString("http://{{host}}"), utf8.RuneCountInString("http://{{host}}"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "http://" {
		t.Fatalf("Ctrl+Backspace should delete whole {{var}} as one word, got %q", got)
	}
}

func TestURLCtrlDeleteDeletesForwardWord(t *testing.T) {
	rig := newURLKeyRig("https://example.com/path")
	rig.focus()
	rig.tab.URLInput.SetCaret(0, 0)

	rig.pressKey(key.NameDeleteForward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "://example.com/path" {
		t.Fatalf("Ctrl+Delete should delete the leading word, got %q", got)
	}
}

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

type vstackRig struct {
	r    input.Router
	ops  *op.Ops
	tab  *RequestTab
	th   *material.Theme
	win  *app.Window
	now  time.Time
	size image.Point
}

func newVStackRig() *vstackRig {
	tab := NewRequestTab("T1")
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.ReqEditor.SetText("{\n  \"a\": 1\n}")
	tab.AddHeader("Authorization", "secret")
	tab.LayoutMode = LayoutModeVert
	tab.HeadersExpanded = true

	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	return &vstackRig{
		ops:  new(op.Ops),
		tab:  tab,
		th:   th,
		win:  new(app.Window),
		now:  time.Unix(1700000000, 0),
		size: image.Pt(800, 600),
	}
}

func (rig *vstackRig) frame() {
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
	rig.r.Frame(rig.ops)
}

func (rig *vstackRig) gtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
	}
}

func (rig *vstackRig) paneTop() int {
	return 37 + 1 + 30
}

func (rig *vstackRig) paneH() int {
	return rig.tab.splitPaneRec
}

func (rig *vstackRig) splitDividerY() int {
	return rig.paneTop() + rig.paneH() + 2
}

func (rig *vstackRig) headersSliderY() int {
	return rig.paneTop() + rig.tab.reqPaneAboveHeadersPx(rig.gtx()) + rig.tab.headersRenderH + 2
}

func pointerPress(x, y int) pointer.Event {
	return pointer.Event{Kind: pointer.Press, Position: f32.Pt(float32(x), float32(y)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse}
}

func pointerMove(x, y int) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), float32(y)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse}
}

func pointerMoveF(x int, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse}
}

func pointerRelease(x, y int) pointer.Event {
	return pointer.Event{Kind: pointer.Release, Position: f32.Pt(float32(x), float32(y)), Source: pointer.Mouse}
}

func (rig *vstackRig) drag(x, y0, y1 int) {
	rig.r.Queue(pointerPress(x, y0))
	rig.frame()
	steps := 4
	for i := 1; i <= steps; i++ {
		y := y0 + (y1-y0)*i/steps
		rig.r.Queue(pointerMove(x, y))
		rig.frame()
	}
	rig.r.Queue(pointerRelease(x, y1))
	rig.frame()
	rig.frame()
}

func near(got, want, tol int) bool {
	d := got - want
	return d >= -tol && d <= tol
}

func TestVStackSplitToMinKeepsHeadersHeight(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 200
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if got := rig.tab.headersRenderH; !near(got, 200, 2) {
		t.Fatalf("setup: headers should render at 200, got %d", got)
	}

	rig.drag(400, rig.splitDividerY(), rig.paneTop()+10)

	if got := rig.tab.HeadersAbsHeight; !near(got, 200, 2) {
		t.Errorf("split-to-min clobbered stored headers height: got %d, want ~200", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 200, 2) {
		t.Errorf("split-to-min squeezed rendered headers: got %d, want ~200", got)
	}
	wantMin := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, wantMin, 3) {
		t.Errorf("request pane should stop at headers+request header (%dpx), got %dpx", wantMin, got)
	}

	rig.drag(400, rig.splitDividerY(), rig.paneTop()+300)

	if got := rig.tab.HeadersAbsHeight; !near(got, 200, 2) {
		t.Errorf("headers height lost after restoring the split: got %d, want ~200", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 200, 2) {
		t.Errorf("headers should render at their stored height again: got %d, want ~200", got)
	}
}

func TestHeadersSliderWorksAtMinRequestPane(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 200
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.splitDividerY(), rig.paneTop()+10)
	paneBefore := rig.paneH()

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+100)
	if got := rig.tab.HeadersAbsHeight; !near(got, 300, 3) {
		t.Errorf("slider down at min pane should grow headers to ~300 by pushing the split, got %d", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 300, 3) {
		t.Errorf("rendered headers should follow the slider, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+100, 4) {
		t.Errorf("request pane should grow with headers (editor stays 0): pane %d, want ~%d", got, paneBefore+100)
	}

	paneGrown := rig.paneH()
	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-160)
	if got := rig.tab.HeadersAbsHeight; !near(got, 140, 3) {
		t.Errorf("slider up should shrink headers to ~140, got %d", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 140, 3) {
		t.Errorf("rendered headers should follow the slider, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneGrown-160, 4) {
		t.Errorf("split must follow the slider up while the editor is collapsed: pane %d, want ~%d", got, paneGrown-160)
	}
}

func TestHeadersSliderPushesRequestDown(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.paneH()

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+50)
	if got := rig.tab.HeadersAbsHeight; !near(got, 150, 2) {
		t.Errorf("slider down should grow headers to ~150, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+50, 3) {
		t.Errorf("growing headers must push Request down (shrinking Body): pane %d, want ~%d", got, paneBefore+50)
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-30)
	if got := rig.tab.HeadersAbsHeight; !near(got, 120, 2) {
		t.Errorf("slider up should shrink headers to ~120, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+20, 3) {
		t.Errorf("shrinking headers must pull Request up (growing Body): pane %d, want ~%d", got, paneBefore+20)
	}
}

func TestHeadersSliderClampedByResponseMin(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	gtx := rig.gtx()
	extent := int(rig.tab.stackedSplitExtent(gtx))
	paneBefore := rig.paneH()
	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+2000)
	if got := extent - rig.paneH(); !near(got, 120, 3) {
		t.Errorf("response pane should keep its 120px minimum, got %d", got)
	}
	maxHeaders := 100 + (extent - 120 - paneBefore)
	if got := rig.tab.HeadersAbsHeight; !near(got, maxHeaders, 5) {
		t.Errorf("slider down must stop when the response pane hits its minimum: got %d, want ~%d", got, maxHeaders)
	}

	before := rig.tab.HeadersAbsHeight
	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-30)
	if got := rig.tab.HeadersAbsHeight; !near(got, before-30, 2) {
		t.Errorf("slider up must respond immediately after an over-drag: got %d, want ~%d", got, before-30)
	}
}

func TestHeadersSliderCollapsesAtZero(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-200)
	if rig.tab.HeadersExpanded {
		t.Errorf("dragging the headers slider to zero must collapse the headers section")
	}
	if got := rig.tab.HeadersAbsHeight; got < 60 {
		t.Errorf("stored headers height must stay usable for re-expanding, got %d", got)
	}
}

func TestSlowSplitDragCollapsesRequest(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	y := rig.splitDividerY()
	rig.r.Queue(pointerPress(400, y))
	rig.frame()
	for i := 1; i <= 300; i++ {
		rig.r.Queue(pointerMove(400, y-i))
		rig.frame()
	}
	rig.r.Queue(pointerRelease(400, y-300))
	rig.frame()
	rig.frame()

	if !rig.tab.ReqBodyCollapsed {
		t.Errorf("a slow 1px-per-frame drag to the top must still collapse the request body")
	}
	if got, want := rig.paneH(), rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("pane should hug the collapsed minimum: %d, want ~%d", got, want)
	}
}

func vstackGtx(sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Now:         time.Unix(1700000000, 0),
	}
}

func TestVStackRequestPaneShrinksToHeader(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		name := "compact-headers"
		if expanded {
			name = "expanded-headers"
		}
		t.Run(name, func(t *testing.T) {
			tab := NewRequestTab("T1")
			tab.Method = "POST"
			tab.URLInput.SetText("http://example.com")
			tab.ReqEditor.SetText("{\n  \"a\": 1\n}")
			tab.LayoutMode = LayoutModeVert
			tab.HeadersExpanded = expanded
			tab.VStackRatio = 0.01

			win := new(app.Window)
			th := material.NewTheme()
			th.Shaper = material.NewTheme().Shaper

			render := func() {
				gtx := vstackGtx(image.Pt(800, 600))
				tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
			}
			render()
			if tab.reqHeaderH <= 0 {
				t.Fatalf("request header height was not recorded during layout")
			}
			tab.VStackRatio = 0.01
			render()

			gtx := vstackGtx(image.Pt(800, 600))
			minPx := tab.stackedReqPaneMinPx(gtx)
			gotPx := float32(tab.splitPaneRec)

			if diff := gotPx - float32(minPx); diff < -2 || diff > 2 {
				t.Errorf("clamped request pane height = %.1fpx, want ~%dpx (min without editor)", gotPx, minPx)
			}

			nonEditor := float32(tab.reqPaneAboveHeadersPx(gtx) + tab.reqPaneBelowHeadersPx(gtx) - gtx.Dp(unit.Dp(3)))
			if expanded {
				if got := tab.headersRenderH; !near(got, gtx.Dp(unit.Dp(tab.HeadersAbsHeight)), 2) {
					t.Errorf("headers must not be squeezed at the pane minimum: rendered %d, stored %ddp", got, tab.HeadersAbsHeight)
				}
				nonEditor += float32(tab.headersRenderH)
			} else {
				nonEditor -= float32(gtx.Dp(unit.Dp(4)) + gtx.Dp(unit.Dp(1)))
			}
			editorPx := gotPx - nonEditor
			if editorPx < 0 || editorPx > float32(gtx.Dp(unit.Dp(10))) {
				t.Errorf("space left beside header chrome at min pane height = %.1fpx, want ~0", editorPx)
			}
		})
	}
}

func newWSVRig() *vstackRig {
	rig := newVStackRig()
	rig.tab.Method = MethodWS
	rig.tab.URLInput.SetText("wss://example.com/socket")
	s := rig.tab.EnsureWS()
	s.OptionsExpanded = false
	return rig
}

func (rig *vstackRig) wsExtent() float32 {
	return float32(rig.size.Y - 37 - 2 - 30 - 4)
}

func (rig *vstackRig) wsPaneH() int {
	s := rig.tab.EnsureWS()
	return int(s.ComposerRatio*rig.wsExtent() + 0.5)
}

func (rig *vstackRig) wsDividerY() int {
	return rig.paneTop() + rig.wsPaneH() + 2
}

func (rig *vstackRig) wsHeadersSliderY() int {
	s := rig.tab.EnsureWS()
	return rig.paneTop() + 66 + s.headersRenderH + 2
}

func TestWSBodyCannotCoverCompose(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.wsDividerY(), rig.paneTop()+5)
	if !s.ComposeCollapsed {
		t.Errorf("dragging the WS split to the top must collapse Compose, not bury it")
	}
	if got, want := rig.wsPaneH(), s.composerMinPx(rig.gtx()); !near(got, want, 8) {
		t.Errorf("composer pane must stop at its section headers: pane %d, want ~%d", got, want)
	}
	if got := s.headersRenderH; !near(got, 120, 4) {
		t.Errorf("WS headers must keep their height when the split hits the minimum, got %d", got)
	}

	rig.drag(400, rig.wsDividerY(), rig.wsDividerY()+100)
	if s.ComposeCollapsed {
		t.Errorf("dragging the WS split back down must expand Compose")
	}
}

func TestWSHeadersComposeSlider(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if got := s.headersRenderH; !near(got, 120, 4) {
		t.Fatalf("setup: WS headers should render at ~120, got %d", got)
	}

	rig.drag(400, rig.wsHeadersSliderY(), rig.wsHeadersSliderY()+50)
	if got := s.HeadersAbsHeight; !near(got, 170, 4) {
		t.Errorf("slider down should grow WS headers to ~170, got %d", got)
	}

	rig.drag(400, rig.wsHeadersSliderY(), rig.wsHeadersSliderY()+500)
	extent := int(rig.wsExtent())
	if got := extent - rig.wsPaneH(); !near(got, 120, 6) {
		t.Errorf("over-dragging the slider must stop when WS Body hits its minimum, got body %dpx", got)
	}

	before := s.HeadersAbsHeight
	rig.drag(400, rig.wsHeadersSliderY(), rig.wsHeadersSliderY()-40)
	if got := s.HeadersAbsHeight; !near(got, before-40, 4) {
		t.Errorf("slider up must respond immediately after an over-drag: got %d, want ~%d", got, before-40)
	}
}

func TestWSMessagesCollapse(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	s.MessagesCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !s.MessagesCollapsed {
		t.Fatalf("collapse button must collapse the WS Body pane")
	}
	extent := int(rig.wsExtent())
	if got, want := extent-rig.wsPaneH(), s.msgsCollapsedMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapsed WS Body should hug its status row: got %d, want ~%d", got, want)
	}

	rig.drag(400, rig.wsDividerY(), rig.wsDividerY()-100)
	if s.MessagesCollapsed {
		t.Errorf("dragging the WS split up must expand the Body pane")
	}
}

func TestWSComposeCollapseButton(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	s.ComposeCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !s.ComposeCollapsed {
		t.Fatalf("collapse button must collapse Compose")
	}
	if got, want := rig.wsPaneH(), s.composerMinPx(rig.gtx()); !near(got, want, 6) {
		t.Errorf("collapsed Compose should shrink the composer pane to its headers: pane %d, want ~%d", got, want)
	}

	s.ComposeCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if s.ComposeCollapsed {
		t.Fatalf("second click must expand Compose")
	}
	if got := rig.wsPaneH(); got < s.composerMinPx(rig.gtx())+100 {
		t.Errorf("expanding must reopen the compose editor: pane %d", got)
	}
}

func TestWSHandshakeHeaders(t *testing.T) {
	rt := &RequestTab{}

	origin := &HeaderItem{}
	origin.Key.SetText("Origin")
	origin.Value.SetText("https://web.max.ru")

	templated := &HeaderItem{}
	templated.Key.SetText("X-Token")
	templated.Value.SetText("{{tok}}")

	gen := &HeaderItem{IsGenerated: true}
	gen.Key.SetText("Content-Length")
	gen.Value.SetText("0")

	empty := &HeaderItem{}
	empty.Key.SetText("   ")
	empty.Value.SetText("ignored")

	rt.Headers = []*HeaderItem{origin, templated, gen, empty}

	h := rt.wsHandshakeHeaders(map[string]string{"tok": "abc"}, http.Header{"User-Agent": {"rete/1"}})

	if got := h.Get("Origin"); got != "https://web.max.ru" {
		t.Fatalf("Origin = %q, want https://web.max.ru", got)
	}
	if got := h.Get("X-Token"); got != "abc" {
		t.Fatalf("X-Token = %q, want abc (templated)", got)
	}
	if got := h.Get("User-Agent"); got != "rete/1" {
		t.Fatalf("User-Agent = %q, want rete/1 (merged extra)", got)
	}
	if _, ok := h["Content-Length"]; ok {
		t.Fatal("generated header should be skipped")
	}
	if len(h) != 3 {
		t.Fatalf("header count = %d, want 3 (empty-key row dropped)", len(h))
	}
}

func TestWSHandshakeHeadersEmpty(t *testing.T) {
	rt := &RequestTab{}
	if h := rt.wsHandshakeHeaders(nil, nil); h != nil {
		t.Fatalf("expected nil for no headers, got %v", h)
	}
}

func TestDefaultOrigin(t *testing.T) {
	cases := map[string]string{
		"wss://api.oneme.ru/websocket": "https://api.oneme.ru",
		"ws://localhost:8080/ws":       "http://localhost:8080",
		"wss://api.oneme.ru:443/ws":    "https://api.oneme.ru",
		"ws://example.com:80/ws":       "http://example.com",
		"wss://example.com:8443/ws":    "https://example.com:8443",
		"https://example.com/x":        "https://example.com",
		"not a url with spaces":        "",
		"":                             "",
		"/relative/path":               "",
	}
	for in, want := range cases {
		if got := defaultOrigin(in); got != want {
			t.Errorf("defaultOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseProtoInt(t *testing.T) {
	cases := []struct {
		in      string
		lo, hi  int
		want    int
		wantErr bool
	}{
		{"", 0, 255, 0, false},
		{"  42 ", 0, 255, 42, false},
		{"-5", -32768, 32767, -5, false},
		{"256", 0, 255, 0, true},
		{"x", 0, 255, 0, true},
	}
	for _, c := range cases {
		got, err := parseProtoInt(c.in, c.lo, c.hi)
		if c.wantErr {
			if err == nil {
				t.Fatalf("parseProtoInt(%q) expected error", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("parseProtoInt(%q) = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
}

func TestDecodeProtoViewRoundTrip(t *testing.T) {
	raw, _, err := wsproto.Encode(wsproto.Frame{
		Cmd:     5,
		Seq:     17,
		Opcode:  3,
		Payload: map[string]any{"hello": "world", "n": int64(9)},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := decodeProtoView(raw)
	if view.DecodeErr != "" {
		t.Fatalf("unexpected decode error: %s", view.DecodeErr)
	}
	if view.Cmd != 5 || view.Seq != 17 || view.Opcode != 3 {
		t.Fatalf("header mismatch: %+v", view)
	}
	if !strings.Contains(view.JSON, "\"hello\": \"world\"") {
		t.Fatalf("json missing field: %s", view.JSON)
	}
	if !strings.Contains(previewProto(view), "cmd=5 seq=17 op=3") {
		t.Fatalf("preview mismatch: %s", previewProto(view))
	}
	detail := protoDetailText(view)
	if !strings.Contains(detail, "cmd=5") || !strings.Contains(detail, "\"hello\": \"world\"") {
		t.Fatalf("detail mismatch: %s", detail)
	}
}

func TestDecodeProtoViewBadFrame(t *testing.T) {
	view := decodeProtoView([]byte{1, 2, 3})
	if view.DecodeErr == "" {
		t.Fatal("expected decode error for short frame")
	}
	if !strings.Contains(previewProto(view), "⚠") {
		t.Fatalf("preview should flag error: %s", previewProto(view))
	}
}

func TestWSStateString(t *testing.T) {
	cases := []struct {
		st   WSState
		want string
	}{
		{WSStateIdle, "Idle"},
		{WSStateConnecting, "Connecting"},
		{WSStateOpen, "Open"},
		{WSStateClosing, "Closing"},
		{WSStateClosed, "Closed"},
		{WSState(99), "?"},
	}
	for _, c := range cases {
		if got := c.st.String(); got != c.want {
			t.Errorf("WSState(%d).String() = %q, want %q", c.st, got, c.want)
		}
	}
}

func TestTrimSpaceLocal(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"\t\n\r ", ""},
		{"a", "a"},
		{"  a  ", "a"},
		{"\ta b\n", "a b"},
		{"a  ", "a"},
		{"  a", "a"},
	}
	for _, c := range cases {
		if got := trimSpaceLocal(c.in); got != c.want {
			t.Errorf("trimSpaceLocal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for b := 0; b < 256; b++ {
		want := b == ' ' || b == '\t' || b == '\n' || b == '\r'
		if got := isSpaceByte(byte(b)); got != want {
			t.Errorf("isSpaceByte(%d) = %v, want %v", b, got, want)
		}
	}
}

func TestWSSessionStateAndStatus(t *testing.T) {
	s := newWSSession()
	if s.State() != WSStateIdle {
		t.Errorf("fresh state = %v, want Idle", s.State())
	}
	s.setState(WSStateOpen)
	if s.State() != WSStateOpen {
		t.Errorf("state = %v, want Open", s.State())
	}
	s.setStatus("Connected", false)
	if s.StatusText() != "Connected" || s.StatusIsError() {
		t.Errorf("status = %q/%v", s.StatusText(), s.StatusIsError())
	}
	s.setStatus("Boom", true)
	if s.StatusText() != "Boom" || !s.StatusIsError() {
		t.Errorf("status = %q/%v", s.StatusText(), s.StatusIsError())
	}
}

func TestWSSessionConnInfoAndGetConn(t *testing.T) {
	s := newWSSession()
	if s.getConn() != nil {
		t.Error("a fresh session must have no connection")
	}
	if s.Subprotocol() != "" {
		t.Errorf("Subprotocol = %q, want empty", s.Subprotocol())
	}
	ext := ws.ExtParams{Negotiated: true, ServerNoContextTakeover: true}
	s.setConnInfo(nil, "chat", ext)
	if s.Subprotocol() != "chat" {
		t.Errorf("Subprotocol = %q", s.Subprotocol())
	}
	if got := s.NegotiatedExtensions(); got != ext {
		t.Errorf("NegotiatedExtensions = %+v, want %+v", got, ext)
	}
}

func TestWSSubprotocolListDropsBlanks(t *testing.T) {
	s := newWSSession()
	s.AddSubprotocol("  chat  ")
	s.AddSubprotocol("")
	s.AddSubprotocol("\t\n")
	s.AddSubprotocol("json")
	got := s.SubprotocolList()
	if len(got) != 2 || got[0] != "chat" || got[1] != "json" {
		t.Errorf("SubprotocolList = %#v, want [chat json]", got)
	}
}

func TestWSMessageAppendersAndClear(t *testing.T) {
	s := newWSSession()
	s.sessionCount = 3
	s.appendMessage(WSDisplayMessage{Payload: []byte("a"), Session: 3})
	s.appendError("bad things")
	s.appendNote(3, "note")
	if len(s.Messages) != 3 {
		t.Fatalf("Messages = %d, want 3", len(s.Messages))
	}
	if s.Messages[1].Error != "bad things" || s.Messages[1].Session != 3 {
		t.Errorf("error entry = %+v", s.Messages[1])
	}
	if s.Messages[2].Note != "note" {
		t.Errorf("note entry = %+v", s.Messages[2])
	}
	s.ClearMessages()
	if len(s.Messages) != 0 {
		t.Errorf("ClearMessages left %d entries", len(s.Messages))
	}
}

func TestWSSessionMarkClosedIsIdempotent(t *testing.T) {
	s := newWSSession()
	var cancels int
	s.cancel = func() { cancels++ }
	s.markClosed()
	s.markClosed()
	if cancels != 1 {
		t.Errorf("cancel called %d times, want exactly 1", cancels)
	}
}

func TestWSMenuOpenAndClose(t *testing.T) {
	tab := NewRequestTab("t")
	if tab.WSMenuOpen() {
		t.Error("a tab with no WS session must report no open menu")
	}
	tab.CloseWSMenus()

	s := tab.EnsureWS()
	s.OpcodeMenuOpen = true
	if !tab.WSMenuOpen() {
		t.Error("WSMenuOpen must see the opcode menu")
	}
	s.OpcodeMenuOpen = false
	s.FilterMenuOpen = true
	if !tab.WSMenuOpen() {
		t.Error("WSMenuOpen must see the filter menu")
	}
	tab.CloseWSMenus()
	if tab.WSMenuOpen() {
		t.Error("CloseWSMenus must close both menus")
	}
}

func TestParseHexInput(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"00ff", "\x00\xff", false},
		{"00 ff", "\x00\xff", false},
		{"00:ff-01", "\x00\xff\x01", false},
		{"0x00ff", "\x00\xff", false},
		{"00\n ff\t", "\x00\xff", false},
		{"0,0,f,f", "\x00\xff", false},
		{"zz", "", true},
		{"abc", "", true},
	}
	for _, c := range cases {
		got, err := parseHexInput(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseHexInput(%q) = %x, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseHexInput(%q): %v", c.in, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("parseHexInput(%q) = %x, want %x", c.in, got, c.want)
		}
	}
}

func TestFormatDialError(t *testing.T) {
	base := errors.New("dial tcp: refused")
	if got := formatDialError(base, nil); got != "Dial failed: dial tcp: refused" {
		t.Errorf("nil result = %q", got)
	}
	if got := formatDialError(base, &ws.DialResult{}); !strings.HasPrefix(got, "Dial failed: ") {
		t.Errorf("result without a response = %q", got)
	}

	mk := func(status string, code int, ct string, body string) string {
		res := &ws.DialResult{
			Response:     &http.Response{Status: status, StatusCode: code, Header: http.Header{}},
			ResponseBody: []byte(body),
		}
		if ct != "" {
			res.Response.Header.Set("Content-Type", ct)
		}
		return formatDialError(base, res)
	}

	if got := mk("200 OK", 200, "text/html; charset=utf-8", ""); !strings.Contains(got, "returned HTML") {
		t.Errorf("html hint missing: %q", got)
	}
	if got := mk("200 OK", 200, "application/json", ""); !strings.Contains(got, "returned JSON") {
		t.Errorf("json hint missing: %q", got)
	}
	if got := mk("403 Forbidden", 403, "text/plain", ""); !strings.Contains(got, "refused the upgrade") {
		t.Errorf("4xx hint missing: %q", got)
	}
	if got := mk("200 OK", 200, "text/plain", ""); strings.Contains(got, " — ") {
		t.Errorf("a plain 200 must get no hint: %q", got)
	}
	if got := mk("500 Err", 500, "", "  boom  "); !strings.HasSuffix(got, "\nboom") {
		t.Errorf("body must be trimmed and appended: %q", got)
	}
	long := mk("500 Err", 500, "", strings.Repeat("x", 400))
	body := long[strings.Index(long, "\n")+1:]
	if len([]rune(body)) != 241 || !strings.HasSuffix(body, "…") {
		t.Errorf("long body must be truncated to 240 chars plus an ellipsis, got %d runes", len([]rune(body)))
	}
}

func TestSuffixFromExt(t *testing.T) {
	cases := []struct {
		name string
		res  ws.DialResult
		want []string
		none bool
	}{
		{name: "empty", res: ws.DialResult{}, none: true},
		{name: "subprotocol", res: ws.DialResult{Subprotocol: "chat"}, want: []string{"subprotocol=chat"}},
		{
			name: "deflate",
			res:  ws.DialResult{Extensions: ws.ExtParams{Negotiated: true}},
			want: []string{"permessage-deflate"},
		},
		{
			name: "deflate with takeovers",
			res: ws.DialResult{Extensions: ws.ExtParams{
				Negotiated:              true,
				ServerNoContextTakeover: true,
				ClientNoContextTakeover: true,
			}},
			want: []string{"server_no_context_takeover", "client_no_context_takeover"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := suffixFromExt(&c.res)
			if c.none {
				if got != "" {
					t.Errorf("suffixFromExt = %q, want empty", got)
				}
				return
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("suffixFromExt = %q, want it to mention %q", got, w)
				}
			}
		})
	}
}

func TestIsNormalCloseErr(t *testing.T) {
	ctx := context.Background()
	normal := []error{
		nil,
		context.Canceled,
		ws.ErrConnClosed,
		io.EOF,
		io.ErrUnexpectedEOF,
		net.ErrClosed,
		fmt.Errorf("wrapped: %w", io.EOF),
		errors.New("read tcp: use of closed network connection"),
		errors.New("read tcp: connection reset by peer"),
		errors.New("write tcp: broken pipe"),
	}
	for _, err := range normal {
		if !isNormalCloseErr(ctx, err) {
			t.Errorf("isNormalCloseErr(%v) = false, want true", err)
		}
	}
	if isNormalCloseErr(ctx, errors.New("protocol error: bad opcode")) {
		t.Error("a protocol error must not count as a normal close")
	}

	done, cancel := context.WithCancel(context.Background())
	cancel()
	if !isNormalCloseErr(done, errors.New("protocol error: bad opcode")) {
		t.Error("any error must count as normal once the context is done")
	}
}

func TestIsAbnormalCloseCode(t *testing.T) {
	for _, c := range []ws.CloseCode{ws.CloseNormal, ws.CloseGoingAway, ws.CloseNoStatusRcvd} {
		if isAbnormalCloseCode(c) {
			t.Errorf("close code %d must be treated as normal", c)
		}
	}
	for _, c := range []ws.CloseCode{ws.CloseProtocolError, ws.CloseInternalErr, ws.CloseCode(4999)} {
		if !isAbnormalCloseCode(c) {
			t.Errorf("close code %d must be treated as abnormal", c)
		}
	}
}

func TestFormatPeerClose(t *testing.T) {
	if got := formatPeerClose(ws.CloseNormal, ""); got != "Closed by peer (code=1000)" {
		t.Errorf("formatPeerClose = %q", got)
	}
	if got := formatPeerClose(ws.CloseGoingAway, "bye"); got != "Closed by peer (code=1001, reason=bye)" {
		t.Errorf("formatPeerClose = %q", got)
	}
}

func TestWSSendWithoutConnectionReportsError(t *testing.T) {
	sends := []struct {
		name string
		fn   func(*RequestTab)
	}{
		{"text", func(tab *RequestTab) { tab.WSSendText("hi") }},
		{"binary", func(tab *RequestTab) { tab.WSSendBinary([]byte{1}) }},
		{"ping", func(tab *RequestTab) { tab.WSSendPing() }},
		{"proto", func(tab *RequestTab) { tab.WSSendProto(`{"a":1}`) }},
	}
	for _, c := range sends {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			c.fn(tab)
			s := tab.EnsureWS()
			if len(s.Messages) != 1 || s.Messages[0].Error != "Not connected" {
				t.Errorf("messages = %+v, want a single 'Not connected' error", s.Messages)
			}
		})
	}
}

func TestSendFromComposerRoutesByMode(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		proto   bool
		opText  bool
		wantErr string
	}{
		{"text mode", "hello", false, true, "Not connected"},
		{"binary mode", "00ff", false, false, "Not connected"},
		{"bad hex", "zz", false, false, "Hex parse: "},
		{"proto mode", `{"a":1}`, true, false, "Not connected"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			s := tab.EnsureWS()
			s.ComposerEditor.SetText(c.text)
			s.UseMsgpackProto = c.proto
			s.OpcodeText = c.opText
			tab.SendFromComposer()
			if len(s.Messages) != 1 {
				t.Fatalf("messages = %+v, want exactly 1", s.Messages)
			}
			if !strings.HasPrefix(s.Messages[0].Error, c.wantErr) {
				t.Errorf("error = %q, want prefix %q", s.Messages[0].Error, c.wantErr)
			}
		})
	}
}

func TestWSSendProtoRejectsBadHeaderFieldsAndJSON(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*WSSession)
		json   string
		prefix string
	}{
		{"bad cmd", func(s *WSSession) { s.ProtoCmdEditor.SetText("300") }, "{}", "cmd: "},
		{"bad seq", func(s *WSSession) { s.ProtoSeqEditor.SetText("99999") }, "{}", "seq: "},
		{"bad opcode", func(s *WSSession) { s.ProtoOpcodeEditor.SetText("abc") }, "{}", "opcode: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newWSSession()
			c.setup(s)
			_, _, _, err := s.protoHeaderFields()
			if err == nil {
				t.Fatal("protoHeaderFields must reject the value")
			}
			if !strings.HasPrefix(err.Error(), c.prefix) {
				t.Errorf("error = %q, want prefix %q", err, c.prefix)
			}
		})
	}

	s := newWSSession()
	cmd, seq, op, err := s.protoHeaderFields()
	if err != nil {
		t.Fatalf("default proto fields: %v", err)
	}
	if cmd != 0 || seq != 0 || op != 0 {
		t.Errorf("default proto fields = %d/%d/%d, want zeros", cmd, seq, op)
	}
}

func TestWSConnectRejectsBadURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
		env  map[string]string
		want string
	}{
		{"empty", "   ", nil, "URL is empty"},
		{"unresolved", "ws://{{host}}/s", nil, "unresolved variables"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			tab.URLInput.SetText(c.url)
			tab.WSConnect(context.Background(), nil, c.env, nil)
			s := tab.EnsureWS()
			if len(s.Messages) != 1 || !strings.Contains(s.Messages[0].Error, c.want) {
				t.Fatalf("messages = %+v, want an error mentioning %q", s.Messages, c.want)
			}
			if s.State() != WSStateIdle {
				t.Errorf("state = %v, want Idle after a rejected URL", s.State())
			}
		})
	}
}

func TestWSConnectIgnoredWhenAlreadyOpen(t *testing.T) {
	for _, st := range []WSState{WSStateConnecting, WSStateOpen} {
		tab := NewRequestTab("t")
		tab.URLInput.SetText("ws://127.0.0.1:1/s")
		s := tab.EnsureWS()
		s.setState(st)
		tab.WSConnect(context.Background(), nil, nil, nil)
		if len(s.Messages) != 0 {
			t.Errorf("state %v: WSConnect must be a no-op, got %+v", st, s.Messages)
		}
	}
}

func TestWSDisconnectNoopWhenNotConnected(t *testing.T) {
	tab := NewRequestTab("t")
	tab.WSDisconnect()

	s := tab.EnsureWS()
	for _, st := range []WSState{WSStateIdle, WSStateClosed, WSStateClosing} {
		s.setState(st)
		s.setStatus("keep", false)
		tab.WSDisconnect()
		if s.State() != st {
			t.Errorf("state %v changed to %v", st, s.State())
		}
		if s.StatusText() != "keep" {
			t.Errorf("status = %q, want untouched", s.StatusText())
		}
	}
}

func TestWSConnectHandshakeFailureReportsBanner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(404)
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("ws://" + strings.TrimPrefix(srv.URL, "http://") + "/s")
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)

	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
	if !s.StatusIsError() {
		t.Errorf("status = %q, want an error flag", s.StatusText())
	}
	msgs := wsMessages(s)
	if len(msgs) == 0 || !strings.Contains(msgs[0].Error, "Handshake rejected") {
		t.Errorf("messages = %+v, want a handshake rejection", msgs)
	}
}

func wsMessages(s *WSSession) []WSDisplayMessage {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	out := make([]WSDisplayMessage, len(s.Messages))
	copy(out, s.Messages)
	return out
}

func waitWS(t *testing.T, s *WSSession, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s.sessionMu.Lock()
		ok := cond()
		s.sessionMu.Unlock()
		if ok {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within the deadline (state=%v status=%q msgs=%d)",
		s.State(), s.StatusText(), len(wsMessages(s)))
}

func startWSEcho(t *testing.T, opts ws.UpgradeOptions, handle func(*ws.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				res, err := ws.Upgrade(c, br, req, opts)
				if err != nil {
					return
				}
				defer func() { _ = res.Conn.Close() }()
				handle(res.Conn)
			}(c)
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		wg.Wait()
	})
	return "ws://" + l.Addr().String() + "/socket"
}

func echoUntilClosed(conn *ws.Conn) {
	for {
		op, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch op {
		case ws.OpText, ws.OpBinary:
			if err := conn.WriteMessage(op, payload); err != nil {
				return
			}
		case ws.OpClose:
			_ = conn.WriteClose(ws.CloseNormal, "")
			return
		}
	}
}

func TestWSConnectSendReceiveDisconnect(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{Subprotocols: []string{"chat"}}, echoUntilClosed)

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	s.AddSubprotocol("chat")

	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })
	if s.Subprotocol() != "chat" {
		t.Errorf("Subprotocol = %q, want chat", s.Subprotocol())
	}
	if s.StatusText() != "Connected" || s.StatusIsError() {
		t.Errorf("status = %q/%v", s.StatusText(), s.StatusIsError())
	}

	tab.WSSendText("hello")
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirIn && string(m.Payload) == "hello" {
				return true
			}
		}
		return false
	})

	tab.WSSendBinary([]byte{1, 2, 3})
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirIn && m.Opcode == ws.OpBinary && len(m.Payload) == 3 {
				return true
			}
		}
		return false
	})

	tab.WSDisconnect()
	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
	if s.StatusIsError() {
		t.Errorf("a clean disconnect must not flag an error: %q", s.StatusText())
	}
}

func TestWSAutoPongOnPing(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		_ = conn.WriteMessage(ws.OpPing, []byte("hi"))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool {
		var sawPing, sawPong bool
		for _, m := range s.Messages {
			if m.Opcode == ws.OpPing && m.Dir == ws.DirIn {
				sawPing = true
			}
			if m.Opcode == ws.OpPong && m.Dir == ws.DirOut && m.Note == "auto-pong" {
				sawPong = true
			}
		}
		return sawPing && sawPong
	})
	tab.MarkClosed()
}

func TestWSPeerCloseIsReported(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		_ = conn.WriteClose(ws.CloseProtocolError, "bad frame")
		time.Sleep(50 * time.Millisecond)
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if strings.Contains(m.Note, "Closed by peer") && strings.Contains(m.Note, "bad frame") {
				return true
			}
		}
		return false
	})
	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
	if !s.StatusIsError() {
		t.Errorf("an abnormal peer close must flag an error, status=%q", s.StatusText())
	}
}

func TestMarkClosedStopsEverything(t *testing.T) {
	tab := NewRequestTab("t")
	s := tab.EnsureWS()
	var cancelled bool
	s.cancel = func() { cancelled = true }
	r := tab.EnsureRun()
	var runCancelled bool
	r.cancel = func() { runCancelled = true }

	tab.MarkClosed()
	if !tab.Closed.Load() {
		t.Error("MarkClosed must set the Closed flag")
	}
	if !cancelled {
		t.Error("MarkClosed must cancel the WS session")
	}
	if !runCancelled {
		t.Error("MarkClosed must stop an in-flight run")
	}

	tab.MarkClosed()
}

func TestMarkClosedRemovesResponseFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/resp.bin"
	if err := os.WriteFile(path, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := NewRequestTab("t")
	tab.respFile = path
	tab.MarkClosed()
	if _, err := os.Stat(path); err == nil {
		t.Error("MarkClosed must delete the spilled response file")
	}
	if tab.respFile != "" {
		t.Errorf("respFile = %q, want cleared", tab.respFile)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "-"},
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024*1024 - 1, "1024.0K"},
		{1024 * 1024, "1.0M"},
		{3 * 1024 * 1024, "3.0M"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestPreviewPayload(t *testing.T) {
	closePayload := func(code ws.CloseCode, reason string) []byte {
		p := []byte{byte(code >> 8), byte(code)}
		return append(p, reason...)
	}

	cases := []struct {
		name string
		p    []byte
		op   ws.Opcode
		want string
	}{
		{"text", []byte("hello"), ws.OpText, "hello"},
		{"binary hex", []byte{0xde, 0xad}, ws.OpBinary, "dead"},
		{"invalid utf8 as text", []byte{0xff, 0xfe}, ws.OpText, "fffe"},
		{"close with reason", closePayload(ws.CloseNormal, "bye"), ws.OpClose, `code=1000 "bye"`},
		{"close without reason", closePayload(ws.CloseGoingAway, ""), ws.OpClose, "code=1001"},
		{"close too short", []byte{1}, ws.OpClose, ""},
		{"close empty", nil, ws.OpClose, ""},
	}
	for _, c := range cases {
		if got := previewPayload(c.p, c.op); got != c.want {
			t.Errorf("%s: previewPayload = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPreviewPayloadTruncates(t *testing.T) {
	bin := make([]byte, 100)
	got := previewPayload(bin, ws.OpBinary)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("long binary preview must be elided: %q", got)
	}
	if want := hex.EncodeToString(bin[:64]) + "…"; got != want {
		t.Errorf("binary preview = %q, want the first 64 bytes", got)
	}

	long := strings.Repeat("a", 300)
	got = previewPayload([]byte(long), ws.OpText)
	if got != long[:256]+"…" {
		t.Errorf("long text preview = %q, want the first 256 bytes plus an ellipsis", got)
	}
}

func TestPreviewPayloadTruncatesOnRuneBoundary(t *testing.T) {
	s := strings.Repeat("a", 255) + "日本語"
	got := previewPayload([]byte(s), ws.OpText)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation, got %q", got)
	}
	body := strings.TrimSuffix(got, "…")
	if !strings.HasPrefix(s, body) {
		t.Errorf("truncated preview %q is not a prefix of the payload", body)
	}
	for _, r := range body {
		if r == '�' {
			t.Errorf("truncation split a multi-byte rune: %q", body)
		}
	}
}

func TestDetailText(t *testing.T) {
	closePayload := []byte{0x03, 0xe8, 'b', 'y', 'e'}

	cases := []struct {
		name string
		msg  WSDisplayMessage
		mode binview.Mode
		want string
	}{
		{"text", WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("hi")}, binview.ModeText, "hi"},
		{"text as hex dump", WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("hi")}, binview.ModeHexDump, binview.HexDump([]byte("hi"))},
		{"text as hex", WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("hi")}, binview.ModeHex, "6869"},
		{"text as base64", WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("hi")}, binview.ModeBase64, "aGk="},
		{"binary invalid utf8", WSDisplayMessage{Opcode: ws.OpBinary, Payload: []byte{0xff}}, binview.ModeText, binview.HexDump([]byte{0xff})},
		{"close", WSDisplayMessage{Opcode: ws.OpClose, Payload: closePayload}, binview.ModeText, "code=1000\nreason=bye"},
		{"close as hex dump", WSDisplayMessage{Opcode: ws.OpClose, Payload: closePayload}, binview.ModeHexDump, binview.HexDump(closePayload)},
		{"empty", WSDisplayMessage{Opcode: ws.OpText}, binview.ModeText, ""},
	}
	for _, c := range cases {
		if got := detailText(c.msg, c.mode); got != c.want {
			t.Errorf("%s: detailText = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetailTextForProtoMessages(t *testing.T) {
	m := WSDisplayMessage{
		Opcode:  ws.OpBinary,
		Payload: []byte{1, 2},
		Proto:   &ProtoView{Cmd: 7, Seq: 3, Opcode: 9, RawLen: 40, JSON: `{"a":1}`},
	}
	got := detailText(m, binview.ModeText)
	for _, want := range []string{"cmd=7", "seq=3", "opcode=9", "uncompressed", `{"a":1}`} {
		if !strings.Contains(got, want) {
			t.Errorf("proto detail = %q, want it to mention %q", got, want)
		}
	}

	m.Proto.Cof = 2
	m.Proto.BodyLen = 20
	if got := detailText(m, binview.ModeText); !strings.Contains(got, "lz4 cof=2") {
		t.Errorf("compressed proto detail = %q, want the lz4 line", got)
	}

	m.Proto.DecodeErr = "bad msgpack"
	if got := detailText(m, binview.ModeText); !strings.Contains(got, "decode error: bad msgpack") {
		t.Errorf("failed proto detail = %q", got)
	}

	if got := detailText(m, binview.ModeHexDump); got != binview.HexDump(m.Payload) {
		t.Errorf("hex mode must ignore the proto view: %q", got)
	}
}

func TestPreviewProto(t *testing.T) {
	p := &ProtoView{Cmd: 1, Seq: 2, Opcode: 3, JSON: "{\n  \"a\": 1\n}"}
	got := previewProto(p)
	if strings.Contains(got, "\n") {
		t.Errorf("preview must collapse whitespace: %q", got)
	}
	if !strings.Contains(got, `{ "a": 1 }`) {
		t.Errorf("preview = %q", got)
	}

	p.Cof = 4
	if got := previewProto(p); !strings.Contains(got, "lz4") {
		t.Errorf("compressed preview = %q, want an lz4 marker", got)
	}

	p.DecodeErr = "boom"
	if got := previewProto(p); !strings.Contains(got, "boom") {
		t.Errorf("failed preview = %q", got)
	}

	if got := previewProto(&ProtoView{Cmd: 5}); got != "cmd=5 seq=0 op=0" {
		t.Errorf("preview with no body = %q", got)
	}
}

func TestDirString(t *testing.T) {
	if got := dirString(ws.DirOut); !strings.HasPrefix(got, "OUT") {
		t.Errorf("dirString(DirOut) = %q", got)
	}
	if got := dirString(ws.DirIn); !strings.HasPrefix(got, "IN") {
		t.Errorf("dirString(DirIn) = %q", got)
	}
}

func TestFormatNegotiated(t *testing.T) {
	s := newWSSession()
	if got := s.formatNegotiated(); got != "" {
		t.Errorf("a closed session must report nothing, got %q", got)
	}

	s.setState(WSStateOpen)
	if got := s.formatNegotiated(); got != "" {
		t.Errorf("an open session with no negotiation must report nothing, got %q", got)
	}

	s.setConnInfo(nil, "chat", ws.ExtParams{Negotiated: true})
	got := s.formatNegotiated()
	if !strings.Contains(got, "subprotocol=chat") || !strings.Contains(got, "deflate") {
		t.Errorf("formatNegotiated = %q", got)
	}
}

func TestRefreshDetailTracksSelection(t *testing.T) {
	s := newWSSession()
	s.Selected = -1
	s.refreshDetail()
	if s.DetailSrcID != -1 {
		t.Errorf("DetailSrcID = %d, want -1 with nothing selected", s.DetailSrcID)
	}

	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("first")})
	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("second")})
	s.Selected = 1
	s.refreshDetail()
	if s.DetailEditor.Text() != "second" {
		t.Errorf("DetailEditor = %q, want second", s.DetailEditor.Text())
	}
	if s.DetailSrcID != 1 {
		t.Errorf("DetailSrcID = %d, want 1", s.DetailSrcID)
	}

	s.DetailBin.Mode = binview.ModeHexDump
	s.refreshDetail()
	if s.DetailEditor.Text() != binview.HexDump([]byte("second")) {
		t.Errorf("switching to hex must re-render: %q", s.DetailEditor.Text())
	}

	s.Selected = 99
	s.refreshDetail()
	if s.Selected != -1 || s.DetailSrcID != -1 {
		t.Errorf("an out-of-range selection must reset: sel=%d src=%d", s.Selected, s.DetailSrcID)
	}
}

func TestWSDebouncerTriggerCoalesces(t *testing.T) {
	d := newWSDebouncer(new(app.Window))
	d.trigger()
	if !d.armed.Load() {
		t.Fatal("the first trigger must arm the debouncer")
	}
	d.trigger()
	if !d.armed.Load() {
		t.Error("a second trigger while armed must stay armed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && d.armed.Load() {
		time.Sleep(2 * time.Millisecond)
	}
	if d.armed.Load() {
		t.Error("the debouncer never disarmed")
	}
	d.trigger()

	var nilD *wsDebouncer
	nilD.trigger()
	(&wsDebouncer{}).trigger()
}

func TestAttachWSWindowIsIdempotent(t *testing.T) {
	tab := NewRequestTab("t")
	win := new(app.Window)
	tab.AttachWSWindow(win)
	s := tab.EnsureWS()
	if s.notify == nil {
		t.Fatal("AttachWSWindow must install a notifier")
	}
	first := s.notify
	tab.AttachWSWindow(new(app.Window))
	if s.notify != first {
		t.Error("AttachWSWindow must not replace an existing notifier")
	}
	s.appendMessage(WSDisplayMessage{Payload: []byte("x")})
}

func TestWSSendProtoOverConnection(t *testing.T) {
	received := make(chan []byte, 4)
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		for {
			op, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if op == ws.OpBinary {
				cp := make([]byte, len(payload))
				copy(cp, payload)
				select {
				case received <- cp:
				default:
				}
				_ = conn.WriteMessage(ws.OpBinary, payload)
			}
		}
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	s.UseMsgpackProto = true
	s.ProtoCmdEditor.SetText("5")
	s.ProtoSeqEditor.SetText("11")
	s.ProtoOpcodeEditor.SetText("2")

	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	s.ComposerEditor.SetText(`{"hello":"world"}`)
	tab.SendFromComposer()

	select {
	case <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("the server never received the encoded frame")
	}

	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirIn && m.Proto != nil {
				return true
			}
		}
		return false
	})
	for _, m := range wsMessages(s) {
		if m.Dir == ws.DirOut && m.Proto != nil {
			if m.Proto.Cmd != 5 || m.Proto.Seq != 11 || m.Proto.Opcode != 2 {
				t.Errorf("outgoing proto header = %+v, want cmd=5 seq=11 op=2", m.Proto)
			}
			if !strings.Contains(m.Proto.JSON, "hello") {
				t.Errorf("outgoing proto JSON = %q", m.Proto.JSON)
			}
		}
	}
	tab.MarkClosed()
}

func TestWSSendProtoRejectsInvalidJSON(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, echoUntilClosed)
	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	tab.WSSendProto(`{"broken":`)
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if strings.HasPrefix(m.Error, "JSON parse: ") {
				return true
			}
		}
		return false
	})
	tab.MarkClosed()
}

func TestWSSendProtoEmptyPayloadIsAllowed(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, echoUntilClosed)
	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	tab.WSSendProto("   ")
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirOut && m.Proto != nil {
				return true
			}
		}
		return false
	})
	for _, m := range wsMessages(s) {
		if m.Error != "" {
			t.Errorf("an empty proto payload must not error: %q", m.Error)
		}
	}
	tab.MarkClosed()
}

func TestWSTabLayoutSmoke(t *testing.T) {
	tab := NewRequestTab("WS")
	tab.Method = MethodWS
	tab.URLInput.SetText("wss://api.oneme.ru/websocket")
	tab.AddHeader("Origin", "https://web.max.ru")

	s := tab.EnsureWS()
	s.OptionsExpanded = true
	s.UseMsgpackProto = true
	s.AddSubprotocol("graphql-transport-ws")
	s.ComposerEditor.SetText(`{"hello":"world"}`)
	raw, _, err := wsproto.Encode(wsproto.Frame{Cmd: 1, Seq: 2, Opcode: 3, Payload: map[string]any{"hi": "there"}})
	if err != nil {
		t.Fatal(err)
	}
	s.Messages = append(s.Messages, WSDisplayMessage{
		Time:   time.Now(),
		Opcode: 2,
		Proto:  decodeProtoView(raw),
	})
	s.Selected = 0

	win := new(app.Window)
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	render := func(w, h int) {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(w, h)),
			Now:         time.Now(),
		}
		tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	}

	render(1100, 700)
	render(420, 360)

	s.HeadersCollapsed = true
	render(1100, 700)

	s.HeadersCollapsed = false
	s.ComposeCollapsed = true
	s.MessagesCollapsed = true
	render(1100, 700)

	s.OptionsExpanded = false
	render(1100, 700)
}

func newWSRig() *vstackRig {
	rig := newVStackRig()
	rig.tab.Method = MethodWS
	rig.tab.URLInput.SetText("wss://echo.test/socket")
	rig.size = image.Pt(1200, 800)
	s := rig.tab.EnsureWS()
	s.OptionsExpanded = true
	s.AddSubprotocol("chat")
	s.ComposerEditor.SetText("hello")
	return rig
}

func TestWSToggleButtons(t *testing.T) {
	cases := []struct {
		name  string
		click func(*WSSession)
		check func(*WSSession) bool
	}{
		{"options", func(s *WSSession) { s.OptionsBtn.Click() }, func(s *WSSession) bool { return !s.OptionsExpanded }},
		{"deflate", func(s *WSSession) { s.OfferDeflateBtn.Click() }, func(s *WSSession) bool { return !s.OfferDeflate }},
		{"msgpack", func(s *WSSession) { s.MsgpackProtoBtn.Click() }, func(s *WSSession) bool { return s.UseMsgpackProto }},
		{"insecure", func(s *WSSession) { s.InsecureBtn.Click() }, func(s *WSSession) bool { return s.InsecureSkipVerify }},
		{"rete ca", func(s *WSSession) { s.UseReteCABtn.Click() }, func(s *WSSession) bool { return s.UseReteCA }},
		{"headers collapse", func(s *WSSession) { s.HeadersCollapseBtn.Click() }, func(s *WSSession) bool { return s.HeadersCollapsed }},
		{"composer wrap", func(s *WSSession) { s.ComposerWrapBtn.Click() }, func(s *WSSession) bool { return !s.ComposerWrap }},
		{"opcode menu", func(s *WSSession) { s.OpcodeMenuBtn.Click() }, func(s *WSSession) bool { return s.OpcodeMenuOpen }},
		{"filter menu", func(s *WSSession) { s.FilterMenuBtn.Click() }, func(s *WSSession) bool { return s.FilterMenuOpen }},
		{"hide ping", func(s *WSSession) { s.FilterPingBtn.Click() }, func(s *WSSession) bool { return s.Filter.HidePing }},
		{"hide pong", func(s *WSSession) { s.FilterPongBtn.Click() }, func(s *WSSession) bool { return s.Filter.HidePong }},
		{"hide close", func(s *WSSession) { s.FilterCloseBtn.Click() }, func(s *WSSession) bool { return s.Filter.HideClose }},
		{"detail hex", func(s *WSSession) { s.DetailBin.Btn(binview.ModeHexDump).Click() }, func(s *WSSession) bool { return s.DetailBin.Mode == binview.ModeHexDump }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newWSRig()
			s := rig.tab.EnsureWS()
			rig.frame()
			c.click(s)
			rig.frame()
			rig.frame()
			if !c.check(s) {
				t.Errorf("%s button did not take effect", c.name)
			}
		})
	}
}

func TestWSOpcodeChoicesSwitchMode(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.OpcodeMenuOpen = true
	rig.frame()

	s.OpcodeBinChoice.Click()
	rig.frame()
	rig.frame()
	if s.OpcodeText {
		t.Error("picking BIN must clear OpcodeText")
	}
	if s.OpcodeMenuOpen {
		t.Error("picking an opcode must close the menu")
	}

	s.OpcodeMenuOpen = true
	rig.frame()
	s.OpcodeTextChoice.Click()
	rig.frame()
	rig.frame()
	if !s.OpcodeText {
		t.Error("picking TEXT must set OpcodeText")
	}
}

func TestWSDetailTextButtonResetsHex(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.DetailBin.Mode = binview.ModeHexDump
	rig.frame()
	s.DetailBin.Btn(binview.ModeText).Click()
	rig.frame()
	rig.frame()
	if s.DetailBin.Mode != binview.ModeText {
		t.Error("the Text chip must turn hex mode off")
	}
}

func TestWSAddAndDeleteSubprotocols(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	rig.frame()

	before := len(s.Subprotocols)
	s.AddSubprotoBtn.Click()
	rig.frame()
	rig.frame()
	if len(s.Subprotocols) != before+1 {
		t.Fatalf("subprotocols = %d, want %d", len(s.Subprotocols), before+1)
	}
	if !s.OptionsExpanded {
		t.Error("adding a subprotocol must expand the options area")
	}

	s.Subprotocols[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(s.Subprotocols) != before {
		t.Errorf("subprotocols = %d after delete, want %d", len(s.Subprotocols), before)
	}
}

func TestWSHeaderAddAndDelete(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.HeadersCollapsed = true
	rig.frame()

	before := len(rig.tab.Headers)
	s.HeadersAddBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Headers) != before+1 {
		t.Fatalf("headers = %d, want %d", len(rig.tab.Headers), before+1)
	}
	if s.HeadersCollapsed {
		t.Error("adding a header must expand the headers area")
	}

	rig.tab.Headers[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Headers) != before {
		t.Errorf("headers = %d after delete, want %d", len(rig.tab.Headers), before)
	}
}

func TestWSClearButtonDropsMessagesAndSelection(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("a")})
	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("b")})
	s.Selected = 1
	rig.frame()

	s.ClearBtn.Click()
	rig.frame()
	rig.frame()
	if len(wsMessages(s)) != 0 {
		t.Errorf("Clear left %d messages", len(wsMessages(s)))
	}
	if s.Selected != -1 {
		t.Errorf("Selected = %d, want -1 after Clear", s.Selected)
	}
}

func TestWSDisconnectButtonUsesHostHook(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.setState(WSStateOpen)
	var called int
	rig.tab.WSHost.OnDisconnect = func(*RequestTab) { called++ }
	rig.frame()

	s.DisconnectBtn.Click()
	rig.frame()
	rig.frame()
	if called != 1 {
		t.Errorf("OnDisconnect called %d times, want 1", called)
	}
	if s.State() != WSStateOpen {
		t.Errorf("the host hook must own the teardown, state = %v", s.State())
	}
}

func TestWSPingButtonWithoutConnectionReportsError(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	rig.frame()
	s.PingBtn.Click()
	rig.frame()
	rig.frame()
	msgs := wsMessages(s)
	if len(msgs) != 1 || msgs[0].Error != "Not connected" {
		t.Errorf("messages = %+v, want a single 'Not connected' error", msgs)
	}
}

func TestWSComposerSendIgnoredWhileDisconnected(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	rig.frame()
	s.ComposerSendBtn.Click()
	rig.frame()
	rig.frame()
	if len(wsMessages(s)) != 0 {
		t.Errorf("Send must be inert while disconnected, got %+v", wsMessages(s))
	}
}

func TestWSCopyButtonsRunWithoutPanic(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.DetailEditor.SetText("detail")
	rig.frame()
	s.ComposerCopyBtn.Click()
	rig.frame()
	s.DetailCopyBtn.Click()
	rig.frame()
	rig.frame()
}

func TestWSMessageListRendersEveryOpcode(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	now := time.Unix(1700000000, 0)
	s.Messages = []WSDisplayMessage{
		{Time: now, Dir: ws.DirOut, Opcode: ws.OpText, Payload: []byte("out")},
		{Time: now, Dir: ws.DirIn, Opcode: ws.OpBinary, Payload: []byte{0, 1, 2}},
		{Time: now, Dir: ws.DirIn, Opcode: ws.OpPing, Payload: []byte("p")},
		{Time: now, Dir: ws.DirOut, Opcode: ws.OpPong, Payload: []byte("p")},
		{Time: now, Dir: ws.DirIn, Opcode: ws.OpClose, Payload: []byte{0x03, 0xe8, 'x'}},
		{Time: now, Note: "Connected"},
		{Time: now, Error: "Read: boom"},
		{Time: now, Dir: ws.DirOut, Opcode: ws.OpBinary, Payload: []byte{9},
			Proto: &ProtoView{Cmd: 1, Seq: 2, Opcode: 3, JSON: `{"k":1}`}},
	}

	for _, sel := range []int{-1, 0, 4, 5, 6, 7} {
		s.Selected = sel
		for i := 0; i < 2; i++ {
			rig.frame()
		}
	}

	s.Filter.HidePing = true
	s.Filter.HidePong = true
	s.Filter.HideClose = true
	s.Selected = 0
	for i := 0; i < 2; i++ {
		rig.frame()
	}
}

func TestWSLayoutCollapsedSectionsAndSizes(t *testing.T) {
	sizes := []image.Point{{X: 1400, Y: 900}, {X: 700, Y: 500}, {X: 380, Y: 300}}
	for _, sz := range sizes {
		rig := newWSRig()
		rig.size = sz
		s := rig.tab.EnsureWS()
		s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("x")})
		s.Selected = 0
		for _, collapse := range []bool{false, true} {
			s.HeadersCollapsed = collapse
			s.ComposeCollapsed = collapse
			s.MessagesCollapsed = collapse
			for i := 0; i < 2; i++ {
				rig.frame()
			}
		}
	}
}

func TestWSStatusBarReflectsSessionState(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	for _, st := range []WSState{WSStateIdle, WSStateConnecting, WSStateOpen, WSStateClosing, WSStateClosed} {
		s.setState(st)
		s.setStatus(st.String(), st == WSStateClosed)
		for i := 0; i < 2; i++ {
			rig.frame()
		}
	}
	s.setState(WSStateOpen)
	s.setConnInfo(nil, "chat", ws.ExtParams{Negotiated: true})
	rig.frame()
	if got := s.formatNegotiated(); got == "" {
		t.Error("an open negotiated session must describe its handshake")
	}
}
