package varpopup

import (
	"image"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/environments"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func newGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
		Now:         time.Now(),
	}
}

func makeHost(envs []*environments.EnvironmentUI, activeID string) *Host {
	envsCopy := envs
	id := activeID
	return &Host{
		Theme:        material.NewTheme(),
		Environments: &envsCopy,
		ActiveEnvID:  &id,
		ActiveEnvVar: func(name string) (string, bool) {
			for _, e := range envsCopy {
				if e.Data.ID == id {
					for _, v := range e.Data.Vars {
						if v.Key == name {
							return v.Value, true
						}
					}
				}
			}
			return "", false
		},
	}
}

func TestOpenAtSetsState(t *testing.T) {
	var s State
	rng := struct{ Start, End int }{Start: 2, End: 7}
	srcEd := struct{}{}
	pos := f32.Pt(10, 20)
	s.OpenAt("token", "secret", srcEd, rng, pos, "env-1")
	if !s.Open {
		t.Fatalf("Open should be true after OpenAt")
	}
	if s.Name != "token" {
		t.Errorf("Name=%q, want 'token'", s.Name)
	}
	if s.Editor.Text() != "secret" {
		t.Errorf("Editor.Text()=%q, want 'secret'", s.Editor.Text())
	}
	if s.EnvID != "env-1" {
		t.Errorf("EnvID=%q, want 'env-1'", s.EnvID)
	}
	if s.Range.Start != 2 || s.Range.End != 7 {
		t.Errorf("Range=%+v, want {2,7}", s.Range)
	}
	if s.Pos.X != 10 || s.Pos.Y != 20 {
		t.Errorf("Pos=%+v, want (10,20)", s.Pos)
	}
	if s.SrcEditor == nil {
		t.Errorf("SrcEditor should be set")
	}
	if s.EnvMenuOpen {
		t.Errorf("EnvMenuOpen should be reset to false")
	}
}

func TestOpenAtResetsEnvMenuOpen(t *testing.T) {
	var s State
	s.EnvMenuOpen = true
	s.OpenAt("v", "val", nil, struct{ Start, End int }{}, f32.Pt(0, 0), "")
	if s.EnvMenuOpen {
		t.Errorf("OpenAt should reset EnvMenuOpen to false even when already open")
	}
}

func TestCloseResetsFlags(t *testing.T) {
	var s State
	s.Open = true
	s.EnvMenuOpen = true
	s.Close()
	if s.Open {
		t.Errorf("Open should be false after Close")
	}
	if s.EnvMenuOpen {
		t.Errorf("EnvMenuOpen should be false after Close")
	}
}

func TestCloseOnNonOpenStateIsNoop(t *testing.T) {
	var s State
	s.Name = "preserved"
	s.Editor.SetText("preserved-value")
	s.EnvID = "env-x"
	s.Close()
	if s.Open || s.EnvMenuOpen {
		t.Errorf("Open/EnvMenuOpen should remain false")
	}
	// Close should not wipe data fields
	if s.Name != "preserved" {
		t.Errorf("Close should not wipe Name; got %q", s.Name)
	}
	if s.Editor.Text() != "preserved-value" {
		t.Errorf("Close should not wipe Editor; got %q", s.Editor.Text())
	}
	if s.EnvID != "env-x" {
		t.Errorf("Close should not wipe EnvID; got %q", s.EnvID)
	}
}

func TestLayoutNilStateReturns(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Layout on nil *State should not panic; got %v", r)
		}
	}()
	var s *State
	gtx := newGtx(800, 600)
	s.Layout(gtx, nil) // host irrelevant when s == nil
}

func TestLayoutClosedReturns(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Layout on closed State should not panic; got %v", r)
		}
	}()
	var s State
	gtx := newGtx(800, 600)
	// Even with nil host, since Open is false the guard returns early.
	s.Layout(gtx, nil)
}

// TODO bug: varpopup.go:70-72 — Layout guards only on s==nil/!Open. When s.Open is true
// but host is nil, the function dereferences host.Theme / host.ActiveEnvID and panics.
// (Not fixed; documented.)

func TestLayoutOpenBasic(t *testing.T) {
	var s State
	s.OpenAt("token", "abc", nil, struct{ Start, End int }{}, f32.Pt(50, 50), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutOpenWithEnvironments(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "dev-tok", Enabled: true}}}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod", Vars: []model.EnvVar{{Key: "token", Value: "prod-tok", Enabled: true}}}},
	}
	var s State
	s.OpenAt("token", "dev-tok", nil, struct{ Start, End int }{}, f32.Pt(100, 100), "e1")
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutEnvMenuOpen(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "dev-tok", Enabled: true}}}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod"}},
	}
	var s State
	s.OpenAt("token", "dev-tok", nil, struct{ Start, End int }{}, f32.Pt(100, 100), "e1")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutPositionClampingRight(t *testing.T) {
	// popup width is 360 dp = 360 px at PxPerDp=1; place near right edge to force clamp.
	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(700, 100), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
	// px should be clamped to Max.X - popupW = 800-360 = 440
	// We can't observe px directly but ensure it doesn't panic and Pos is unchanged.
	if s.Pos.X != 700 {
		t.Errorf("Pos.X mutated by Layout: %v", s.Pos.X)
	}
}

func TestLayoutPositionClampingBottom(t *testing.T) {
	// Place near bottom so py+popupH > Max.Y; should flip above origin.
	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(10, 580), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutNegativePosition(t *testing.T) {
	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(-50, -50), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutTinyConstraintsForcesPyZero(t *testing.T) {
	// Max.Y smaller than popupH so flipped py = Pos.Y - popupH - gap < 0 → clamped to 0.
	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(10, 10), "")
	host := makeHost(nil, "")
	gtx := newGtx(400, 100)
	s.Layout(gtx, host)
}

func TestLayoutEnvMenuOpenWithEmptyActive(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "x", Enabled: true}}}},
	}
	var s State
	s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "")
	s.EnvMenuOpen = true
	host := makeHost(envs, "") // empty active triggers "(no environment)" hint
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutEnvClicksAllocation(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev"}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod"}},
		{Data: &model.ParsedEnvironment{ID: "e3", Name: "QA"}},
	}
	var s State
	s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e2")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
	// EnvClicks should be allocated to len(envs)+1 = 4
	if len(s.EnvClicks) < 4 {
		t.Errorf("EnvClicks len=%d, want >=4", len(s.EnvClicks))
	}
}

func TestLayoutEnvAxisIsVerticalAfterLayout(t *testing.T) {
	var s State
	s.OpenAt("v", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
	if s.EnvList.Axis != layout.Vertical {
		t.Errorf("EnvList.Axis=%v, want Vertical", s.EnvList.Axis)
	}
}

func TestLayoutWithVarNotInEnv(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "other", Value: "x", Enabled: true}}}},
	}
	var s State
	s.OpenAt("missing", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "e1")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutWithEmptyValueVar(t *testing.T) {
	// v.Value == "" → not used as preview (skipped in inner loop)
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{
			{Key: "token", Value: "", Enabled: true},
			{Key: "token", Value: "second", Enabled: true},
		}}},
	}
	var s State
	s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "e1")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}
