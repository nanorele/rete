package har

import (
	"image"
	"strconv"

	"tracto/internal/har"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (s *Section) filesView(gtx layout.Context) layout.Dimensions {
	leftW, handleW, rightW := s.split(gtx)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftW
			gtx.Constraints.Max.X = leftW
			return s.fileList(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.splitHandle(gtx, handleW) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightW
			gtx.Constraints.Max.X = rightW
			return s.filePreview(gtx)
		}),
	)
}

func (s *Section) fileList(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	if len(s.Resources) == 0 {
		return centered(th, gtx, "No files with bodies in this archive")
	}
	for len(s.FileRows) < len(s.Resources) {
		s.FileRows = append(s.FileRows, &widget.Clickable{})
	}
	return material.List(th, &s.FileList).Layout(gtx, len(s.Resources), func(gtx layout.Context, i int) layout.Dimensions {
		clk := s.FileRows[i]
		for clk.Clicked(gtx) {
			s.SelFile = i
		}
		return fileRow(gtx, th, s.Resources[i], clk, s.SelFile == i)
	})
}

func fileRow(gtx layout.Context, th *material.Theme, r har.Resource, clk *widget.Clickable, selected bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rowH := gtx.Dp(unit.Dp(22))
		gtx.Constraints.Min.Y = rowH
		bg := theme.Bg
		if selected {
			bg = theme.AccentDim
		} else if clk.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, rowH)}.Op())
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), r.ZipPath)
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), humanSize(int64(r.Size)))
					lbl.Color = theme.FgMuted
					lbl.Alignment = text.End
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func (s *Section) filePreview(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	if s.SelFile < 0 || s.SelFile >= len(s.Resources) {
		return centered(th, gtx, "Select a file to preview")
	}
	r := s.Resources[s.SelFile]
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), r.ZipPath)
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.bodyHeader(gtx, paneRowPx(gtx), "Content", r.MimeType, r.Size, &s.PrettyBtn, s.Pretty, &s.CopyBodyBtn, true)
		}),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return paneSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				identity := "file/" + strconv.Itoa(s.SelFile)
				body := s.inspectorBody(identity, r.Bytes)
				return s.bodyViewer(gtx, s.FileViewer, &s.FileViewerKey, &s.FileSearch, &s.FileScrollDrag, &s.FileScrollDragY, identity, body, r.MimeType, s.Pretty)
			})
		}),
	)
}
