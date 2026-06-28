package widgets

import (
	"image"
	"testing"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func tableGtx(r *input.Router, w int) layout.Context {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, 400)),
	}
	if r != nil {
		gtx.Source = r.Source()
	}
	return gtx
}

func sampleCols() []TableColumn {
	return []TableColumn{
		{Title: "A", Width: unit.Dp(50), Min: unit.Dp(20), Align: text.Start},
		{Title: "B", Width: 0, Align: text.Start},
		{Title: "C", Width: unit.Dp(60), Align: text.End},
	}
}

func TestNewTable_DetectsFlexColumn(t *testing.T) {
	tbl := NewTable(sampleCols())
	if tbl.flexIdx != 1 {
		t.Errorf("flexIdx = %d, want 1 (the zero-width column)", tbl.flexIdx)
	}
}

func TestTable_Resizable(t *testing.T) {
	tbl := NewTable(sampleCols())
	if !tbl.resizable(0) {
		t.Error("fixed non-last column 0 must be resizable")
	}
	if tbl.resizable(1) {
		t.Error("the flexible column must not be resizable")
	}
	if !tbl.resizable(2) {
		t.Error("the last column (right of flex) must be resizable via its inner edge")
	}
}

func TestTable_ResizeColClampsToMin(t *testing.T) {
	tbl := NewTable(sampleCols())
	gtx := tableGtx(nil, 1000)
	tbl.resizeCol(gtx, 0, -1000)
	if tbl.override[0] != 20 {
		t.Errorf("override[0] = %d, want clamped to Min 20", tbl.override[0])
	}
}

func TestTable_ResizeColKeepsFlexAboveMinimum(t *testing.T) {
	tbl := NewTable(sampleCols())
	gtx := tableGtx(nil, 1000)
	tbl.resizeCol(gtx, 0, 100000)
	if tbl.override[0] != 876 {
		t.Errorf("override[0] = %d, want 876 (flex kept at its %d-dp minimum)", tbl.override[0], tableMinFlex)
	}
}

func TestTable_HeaderAndRowRender(t *testing.T) {
	tbl := NewTable(sampleCols())
	th := material.NewTheme()
	var r input.Router

	for i := 0; i < 2; i++ {
		gtx := tableGtx(&r, 800)
		hd := tbl.Header(gtx, th)
		if hd.Size.X != 800 || hd.Size.Y <= 0 {
			t.Fatalf("header dims = %+v, want full width and positive height", hd.Size)
		}
		rd := tbl.Row(gtx, func(i int) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), tbl.cols[i].Title)
				return lbl.Layout(gtx)
			}
		})
		if rd.Size.X <= 0 {
			t.Fatalf("row dims = %+v", rd.Size)
		}
		r.Frame(gtx.Ops)
	}
}

func TestTable_HandleXSideRelativeToFlex(t *testing.T) {
	tbl := NewTable([]TableColumn{
		{Title: "A", Width: unit.Dp(50), Align: text.Start},
		{Title: "B", Width: 0, Align: text.Start},
		{Title: "C", Width: unit.Dp(60), Align: text.Start},
		{Title: "D", Width: unit.Dp(40), Align: text.End},
	})
	gtx := tableGtx(nil, 1000)
	xs := tbl.boundaries(gtx)

	if got := tbl.handleX(xs, 0); got != xs[0] {
		t.Errorf("handleX(0) = %d, want right edge xs[0]=%d", got, xs[0])
	}
	if got := tbl.handleX(xs, 2); got != xs[1] {
		t.Errorf("handleX(2) = %d, want left edge xs[1]=%d (not right edge xs[2]=%d)", got, xs[1], xs[2])
	}
}

func TestTable_NoFlexColumnDisablesResize(t *testing.T) {
	tbl := NewTable([]TableColumn{
		{Title: "A", Width: unit.Dp(40)},
		{Title: "B", Width: unit.Dp(40)},
	})
	if tbl.flexIdx != -1 {
		t.Fatalf("flexIdx = %d, want -1 when no zero-width column", tbl.flexIdx)
	}
	if tbl.resizable(0) {
		t.Error("without a flexible absorber, no column should be resizable")
	}
}
