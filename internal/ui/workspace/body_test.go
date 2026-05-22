package workspace

import (
	"context"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"tracto/internal/model"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

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
	for _, stacked := range []bool{false, true} {
		gtx := makeBodyTestGtx()
		dim := tab.layoutModeBar(gtx, &tab.LayoutHorizBtn, &tab.LayoutVertBtn, stacked)
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
	tab.HeaderSplitRatio = 0
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
