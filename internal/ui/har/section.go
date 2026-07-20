package har

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"strconv"

	"tracto/internal/har"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type Host struct {
	Theme  *material.Theme
	Window *app.Window

	ChooseHAR  func() (io.ReadCloser, error)
	CreateFile func(name string) (io.WriteCloser, error)
	RunEntry   func(*har.Entry)
}

func (s *Section) Layout(gtx layout.Context, host *Host) layout.Dimensions {
	s.host = host
	s.Ensure()
	s.DrainLoads()

	for s.TabReq.Clicked(gtx) {
		s.TopTab = TabRequests
	}
	for s.TabFiles.Clicked(gtx) {
		s.TopTab = TabFiles
	}
	for s.TabPages.Clicked(gtx) {
		s.TopTab = TabPages
	}
	for s.TabInfo.Clicked(gtx) {
		s.TopTab = TabInfo
	}
	for s.ExportDirBtn.Clicked(gtx) {
		s.exportDir()
	}
	for s.ExportZipBtn.Clicked(gtx) {
		s.exportZip()
	}
	for s.BrowseBtn.Clicked(gtx) {
		s.browse()
	}
	for s.ClearBtn.Clicked(gtx) {
		s.clear()
	}
	for s.InspTabReq.Clicked(gtx) {
		s.InspTab = 0
	}
	for s.InspTabResp.Clicked(gtx) {
		s.InspTab = 1
	}
	for s.PrettyBtn.Clicked(gtx) {
		s.Pretty = !s.Pretty
	}
	for s.CopyBodyBtn.Clicked(gtx) {
		s.copySelectedFile(gtx)
	}
	for s.ReqCopyBtn.Clicked(gtx) {
		s.copySelectedReqBody(gtx)
	}
	for s.RunBtn.Clicked(gtx) {
		s.runSelected()
	}

	paint.FillShape(gtx.Ops, host.Theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.toolbar(gtx) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s.Doc == nil {
				return s.emptyState(gtx)
			}
			switch s.TopTab {
			case TabFiles:
				return s.filesView(gtx)
			case TabPages:
				return s.pagesView(gtx)
			case TabInfo:
				return s.infoView(gtx)
			default:
				return s.requestsView(gtx)
			}
		}),
	)
}

func (s *Section) toolbar(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Dp(unit.Dp(18))
					gtx.Constraints.Min = image.Pt(sz, sz)
					gtx.Constraints.Max = gtx.Constraints.Min
					return widgets.IconHAR.Layout(gtx, theme.Accent)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(13), "HAR Viewer")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.tab(gtx, &s.TabReq, "Requests", reqLabel(s), s.TopTab == TabRequests)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.tab(gtx, &s.TabFiles, "Files", strconv.Itoa(len(s.Resources)), s.TopTab == TabFiles)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.tab(gtx, &s.TabPages, "Pages", pagesLabel(s), s.TopTab == TabPages)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.tab(gtx, &s.TabInfo, "Info", "", s.TopTab == TabInfo)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.toolbarStatus(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, th, &s.BrowseBtn, "Import", widgets.IconFolderOpen, theme.BtnPrimary, theme.BtnPrimaryFg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, th, &s.ExportDirBtn, "Export → Folder", widgets.IconFolderOpen, theme.Border, th.Fg, len(s.Resources) > 0)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, th, &s.ExportZipBtn, "ZIP", widgets.IconDownload, theme.Border, th.Fg, len(s.Resources) > 0)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, th, &s.ClearBtn, "Clear", widgets.IconClear, theme.Border, th.Fg, s.Doc != nil)
				}),
			)
		})
	})
}

func (s *Section) toolbarStatus(gtx layout.Context) layout.Dimensions {
	txt := s.Source
	col := theme.FgMuted
	if s.BannerErr && s.Banner != "" {
		txt, col = s.Banner, theme.Danger
	}
	if txt == "" {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
	}
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(s.host.Theme, unit.Sp(11), txt)
		lbl.Color = col
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		return lbl.Layout(gtx)
	})
}

func reqLabel(st *Section) string {
	if st.Doc == nil {
		return ""
	}
	return strconv.Itoa(len(st.visibleIndices()))
}

func pagesLabel(st *Section) string {
	if st.Doc == nil {
		return ""
	}
	return strconv.Itoa(len(st.Doc.Pages))
}

func (s *Section) tab(gtx layout.Context, clk *widget.Clickable, label, count string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			txt := label
			if count != "" {
				txt = label + " (" + count + ")"
			}
			lbl := material.Label(s.host.Theme, unit.Sp(12), txt)
			if active {
				lbl.Color = theme.Accent
				lbl.Font.Weight = font.Bold
			} else {
				lbl.Color = theme.FgMuted
			}
			dims := lbl.Layout(gtx)
			if active {
				h := gtx.Dp(unit.Dp(2))
				y := dims.Size.Y + gtx.Dp(unit.Dp(4))
				paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(dims.Size.X, y+h)}.Op())
			}
			pointer.CursorPointer.Add(gtx.Ops)
			return dims
		})
	})
}

func (s *Section) emptyState(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Dp(unit.Dp(40))
				gtx.Constraints.Min = image.Pt(sz, sz)
				gtx.Constraints.Max = gtx.Constraints.Min
				return widgets.IconHAR.Layout(gtx, theme.FgDim)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(13), "No HAR loaded")
				lbl.Color = theme.FgMuted
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), "Drag a .har file here, or click Import above.")
				lbl.Color = theme.FgDim
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (s *Section) HandleSearchShortcut(gtx layout.Context) {
	if s.Doc == nil {
		return
	}
	switch s.TopTab {
	case TabFiles:
		if s.SelFile >= 0 && s.SelFile < len(s.Resources) {
			s.FileSearch.Toggle(gtx, s.FileViewer)
		}
	case TabRequests:
		if s.SelReq >= 0 && s.SelReq < len(s.Doc.Entries) {
			s.BodySearch.Toggle(gtx, s.ReqViewer)
		}
	}
}

func hLine(gtx layout.Context) layout.Dimensions {
	rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
	paint.FillShape(gtx.Ops, theme.Border, rect.Op())
	return layout.Dimensions{Size: rect.Max}
}

func bgBar(gtx layout.Context, bg color.NRGBA, content layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := content(gtx)
	call := macro.Stop()
	sz := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: sz}.Op())
	call.Add(gtx.Ops)
	dims.Size = sz
	return dims
}

func btn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, ic *widget.Icon, bg, fg color.NRGBA, enabled bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := op.Record(gtx.Ops)
			if !enabled {
				bg = theme.Mix(bg, theme.Bg, 0.6)
			}
			dims := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{}
				col := fg
				if !enabled {
					col = theme.FgDim
				}
				if ic != nil {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := gtx.Dp(unit.Dp(14))
						gtx.Constraints.Min = image.Pt(s, s)
						gtx.Constraints.Max = gtx.Constraints.Min
						return ic.Layout(gtx, col)
					}))
					children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), label)
					lbl.Color = col
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
			call := macro.Stop()
			sz := dims.Size
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			call.Add(gtx.Ops)
			if enabled {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			return dims
		})
	})
}

func humanSize(n int64) string {
	switch {
	case n < 0:
		return "-"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func centered(th *material.Theme, gtx layout.Context, msg string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(12), msg)
		lbl.Color = theme.FgMuted
		lbl.Alignment = text.Middle
		return lbl.Layout(gtx)
	})
}
