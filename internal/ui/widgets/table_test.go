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

func TestNewTable_ColumnsRoundTrip(t *testing.T) {
	cols := sampleCols()
	tbl := NewTable(cols)
	got := tbl.Columns()
	if len(got) != len(cols) {
		t.Fatalf("Columns() len = %d, want %d", len(got), len(cols))
	}
	for i := range cols {
		if got[i] != cols[i] {
			t.Errorf("Columns()[%d] = %+v, want %+v", i, got[i], cols[i])
		}
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
				lbl := material.Label(th, unit.Sp(11), tbl.Columns()[i].Title)
				return lbl.Layout(gtx)
			}
		})
		if rd.Size.X <= 0 {
			t.Fatalf("row dims = %+v", rd.Size)
		}
		r.Frame(gtx.Ops)
	}
}
