package ui

import (
	"image"
	"image/color"
	"io"
	"net/url"
	"strconv"
	"strings"

	"tracto/internal/har"
	"tracto/internal/model"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/syntax"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
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

func (ui *AppUI) layoutHARSection(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	st.ensure()
	st.drainLoads()

	for st.TabReq.Clicked(gtx) {
		st.TopTab = harTabRequests
	}
	for st.TabFiles.Clicked(gtx) {
		st.TopTab = harTabFiles
	}
	for st.TabPages.Clicked(gtx) {
		st.TopTab = harTabPages
	}
	for st.TabInfo.Clicked(gtx) {
		st.TopTab = harTabInfo
	}
	for st.ExportDirBtn.Clicked(gtx) {
		ui.harExportDir()
	}
	for st.ExportZipBtn.Clicked(gtx) {
		ui.harExportZip()
	}
	for st.BrowseBtn.Clicked(gtx) {
		ui.harBrowse()
	}
	for st.ClearBtn.Clicked(gtx) {
		st.clear()
	}
	for st.InspTabReq.Clicked(gtx) {
		st.InspTab = 0
	}
	for st.InspTabResp.Clicked(gtx) {
		st.InspTab = 1
	}
	for st.PrettyBtn.Clicked(gtx) {
		st.Pretty = !st.Pretty
	}
	for st.CopyBodyBtn.Clicked(gtx) {
		ui.harCopySelectedFile(gtx)
	}
	for st.ReqCopyBtn.Clicked(gtx) {
		ui.harCopySelectedReqBody(gtx)
	}
	for st.RunBtn.Clicked(gtx) {
		ui.harRunSelected()
	}

	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.harToolbar(gtx) }),
		layout.Rigid(mitmHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if st.Doc == nil {
				return ui.harEmptyState(gtx)
			}
			switch st.TopTab {
			case harTabFiles:
				return ui.harFilesView(gtx)
			case harTabPages:
				return ui.harPagesView(gtx)
			case harTabInfo:
				return ui.harInfoView(gtx)
			default:
				return ui.harRequestsView(gtx)
			}
		}),
	)
}

func (ui *AppUI) harToolbar(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	return mitmBgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(18))
					gtx.Constraints.Min = image.Pt(s, s)
					gtx.Constraints.Max = gtx.Constraints.Min
					return widgets.IconHAR.Layout(gtx, theme.Accent)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(13), "HAR Viewer")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.harTab(gtx, &st.TabReq, "Requests", harReqLabel(st), st.TopTab == harTabRequests)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.harTab(gtx, &st.TabFiles, "Files", strconv.Itoa(len(st.Resources)), st.TopTab == harTabFiles)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.harTab(gtx, &st.TabPages, "Pages", harPagesLabel(st), st.TopTab == harTabPages)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.harTab(gtx, &st.TabInfo, "Info", "", st.TopTab == harTabInfo)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ui.harToolbarStatus(gtx, st)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mitmBtn(gtx, ui.Theme, &st.BrowseBtn, "Import", widgets.IconFolderOpen, theme.BtnPrimary, theme.BtnPrimaryFg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mitmBtn(gtx, ui.Theme, &st.ExportDirBtn, "Export → Folder", widgets.IconFolderOpen, theme.Border, ui.Theme.Fg, len(st.Resources) > 0)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mitmBtn(gtx, ui.Theme, &st.ExportZipBtn, "ZIP", widgets.IconDownload, theme.Border, ui.Theme.Fg, len(st.Resources) > 0)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mitmBtn(gtx, ui.Theme, &st.ClearBtn, "Clear", widgets.IconClear, theme.Border, ui.Theme.Fg, st.Doc != nil)
				}),
			)
		})
	})
}

func (ui *AppUI) harToolbarStatus(gtx layout.Context, st *harState) layout.Dimensions {
	txt := st.Source
	col := theme.FgMuted
	if st.BannerErr && st.Banner != "" {
		txt, col = st.Banner, theme.Danger
	}
	if txt == "" {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
	}
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(ui.Theme, unit.Sp(11), txt)
		lbl.Color = col
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		return lbl.Layout(gtx)
	})
}

func harReqLabel(st *harState) string {
	if st.Doc == nil {
		return ""
	}
	return strconv.Itoa(len(st.visibleIndices()))
}

func harPagesLabel(st *harState) string {
	if st.Doc == nil {
		return ""
	}
	return strconv.Itoa(len(st.Doc.Pages))
}

func (ui *AppUI) harTab(gtx layout.Context, clk *widget.Clickable, label, count string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			txt := label
			if count != "" {
				txt = label + " (" + count + ")"
			}
			lbl := material.Label(ui.Theme, unit.Sp(12), txt)
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

func (ui *AppUI) harEmptyState(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s := gtx.Dp(unit.Dp(40))
				gtx.Constraints.Min = image.Pt(s, s)
				gtx.Constraints.Max = gtx.Constraints.Min
				return widgets.IconHAR.Layout(gtx, theme.FgDim)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(13), "No HAR loaded")
				lbl.Color = theme.FgMuted
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), "Drag a .har file here, or click Import above.")
				lbl.Color = theme.FgDim
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (ui *AppUI) harRequestsView(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	leftW, handleW, rightW := ui.harSplit(gtx, st)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftW
			gtx.Constraints.Max.X = leftW
			return ui.harRequestTable(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.harSplitHandle(gtx, handleW) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightW
			gtx.Constraints.Max.X = rightW
			return ui.harInspector(gtx)
		}),
	)
}

func (ui *AppUI) harSplit(gtx layout.Context, st *harState) (leftW, handleW, rightW int) {
	totalW := gtx.Constraints.Max.X
	handleW = gtx.Dp(unit.Dp(6))
	flexExtent := float32(totalW - handleW)

	const minRatio, maxRatio = 0.2, 0.8
	var moved bool
	var finalX float32
	for {
		e, ok := st.SplitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			st.SplitDragX = e.Position.X
		case pointer.Drag:
			finalX = e.Position.X
			moved = true
		}
	}
	if st.SplitRatio < minRatio {
		st.SplitRatio = minRatio
	} else if st.SplitRatio > maxRatio {
		st.SplitRatio = maxRatio
	}
	if moved && flexExtent > 0 {
		delta := finalX - st.SplitDragX
		old := st.SplitRatio
		st.SplitRatio += delta / flexExtent
		if st.SplitRatio < minRatio {
			st.SplitRatio = minRatio
		} else if st.SplitRatio > maxRatio {
			st.SplitRatio = maxRatio
		}
		st.SplitDragX = finalX - ((st.SplitRatio - old) * flexExtent)
		ui.Window.Invalidate()
	}

	leftW = int(float32(totalW)*st.SplitRatio) - handleW/2
	if leftW < 240 {
		leftW = 240
	}
	if leftW > totalW-handleW-280 {
		leftW = totalW - handleW - 280
	}
	if leftW < 0 {
		leftW = 0
	}
	rightW = totalW - leftW - handleW
	return leftW, handleW, rightW
}

func (ui *AppUI) harSplitHandle(gtx layout.Context, handleW int) layout.Dimensions {
	st := &ui.HARView
	h := gtx.Constraints.Max.Y
	size := image.Pt(handleW, h)
	line := gtx.Dp(unit.Dp(1))
	lineCol := theme.Border
	if st.SplitDrag.Dragging() {
		lineCol = theme.Accent
	}
	paint.FillShape(gtx.Ops, lineCol, clip.Rect{Min: image.Pt((handleW-line)/2, 0), Max: image.Pt((handleW-line)/2+line, h)}.Op())
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	pointer.CursorColResize.Add(gtx.Ops)
	st.SplitDrag.Add(gtx.Ops)
	event.Op(gtx.Ops, &st.SplitDrag)
	return layout.Dimensions{Size: size}
}

func harTableColumns() []widgets.TableColumn {
	return []widgets.TableColumn{
		{Title: "#", Width: unit.Dp(32), Align: text.Start},
		{Title: "Method", Width: unit.Dp(56), Align: text.Start},
		{Title: "Status", Width: unit.Dp(48), Align: text.Start},
		{Title: "Domain", Width: unit.Dp(150), Min: unit.Dp(60), Align: text.Start},
		{Title: "File", Width: 0, Align: text.Start},
		{Title: "Type", Width: unit.Dp(90), Min: unit.Dp(48), Align: text.Start},
		{Title: "Size", Width: unit.Dp(64), Align: text.End},
	}
}

func (ui *AppUI) harRequestTable(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	entries := st.Doc.Entries
	vis := st.visibleIndices()
	if len(vis) == 0 {
		msg := "No requests in this archive"
		if st.SelPageID != "" {
			msg = "No requests for this page"
		}
		return harCentered(ui.Theme, gtx, msg)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return st.Table.Header(gtx, ui.Theme) }),
		layout.Rigid(mitmHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			for len(st.ReqRows) < len(entries) {
				st.ReqRows = append(st.ReqRows, &widget.Clickable{})
			}
			if len(st.rowCache) != len(entries) {
				st.rowCache = buildRowCache(entries)
			}
			return material.List(ui.Theme, &st.ReqList).Layout(gtx, len(vis), func(gtx layout.Context, row int) layout.Dimensions {
				i := vis[row]
				clk := st.ReqRows[i]
				for clk.Clicked(gtx) {
					st.SelReq = i
				}
				return harRequestRow(gtx, ui.Theme, st.Table, &st.rowCache[i], &entries[i], clk, st.SelReq == i)
			})
		}),
	)
}

type harRowDisplay struct {
	index, domain, file, typ, size string
}

func buildRowCache(entries []har.Entry) []harRowDisplay {
	out := make([]harRowDisplay, len(entries))
	for i := range entries {
		e := &entries[i]
		domain, file := harSplitURL(e.Request.URL)
		out[i] = harRowDisplay{
			index:  strconv.Itoa(i + 1),
			domain: domain,
			file:   file,
			typ:    harShortType(e.ContentType()),
			size:   humanSize(harEntrySize(e)),
		}
	}
	return out
}

func harRequestRow(gtx layout.Context, th *material.Theme, t *widgets.Table, d *harRowDisplay, e *har.Entry, clk *widget.Clickable, selected bool) layout.Dimensions {
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
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(widgets.TableHInset), Right: unit.Dp(widgets.TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return t.Row(gtx, func(i int) layout.Widget {
				switch i {
				case 0:
					return harTextCell(th, d.index, text.Start, theme.FgMuted, true)
				case 1:
					return harMethodCellW(th, e.Request.Method)
				case 2:
					return harStatusCellW(th, e.Response.Status)
				case 3:
					return harTextCell(th, d.domain, text.Start, theme.FgMuted, true)
				case 4:
					return harTextCell(th, d.file, text.Start, th.Fg, true)
				case 5:
					return harTextCell(th, d.typ, text.Start, theme.FgMuted, false)
				default:
					return harTextCell(th, d.size, text.End, theme.FgMuted, true)
				}
			})
		})
	})
}

func harTextCell(th *material.Theme, s string, al text.Alignment, col color.NRGBA, mono bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Alignment = al
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		lbl.Color = col
		if mono {
			lbl.Font.Typeface = widgets.MonoTypeface
		}
		return lbl.Layout(gtx)
	}
}

func harMethodCellW(th *material.Theme, method string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), method)
		lbl.Color = theme.MethodColor(method)
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	}
}

func harStatusCellW(th *material.Theme, code int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		s := "—"
		if code > 0 {
			s = strconv.Itoa(code)
		}
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Color = harStatusColor(code)
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	}
}

func harStatusColor(code int) color.NRGBA {
	switch {
	case code >= 500:
		return theme.Danger
	case code >= 400:
		return theme.VarMissing
	case code >= 300:
		return theme.Accent
	case code >= 200:
		return theme.VarFound
	default:
		return theme.FgMuted
	}
}

func (ui *AppUI) harInspector(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	if st.SelReq < 0 || st.SelReq >= len(st.Doc.Entries) {
		return harCentered(ui.Theme, gtx, "Select a request to inspect")
	}
	e := &st.Doc.Entries[st.SelReq]
	isWS := e.IsWebSocket()
	respLabel := "Response"
	respCount := ""
	if isWS {
		respLabel = "Messages"
		respCount = strconv.Itoa(len(e.WebSocketMessages))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.harInspectorHeader(gtx, e) }),
		layout.Rigid(mitmHLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.harTab(gtx, &st.InspTabReq, "Request", "", st.InspTab == 0)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.harTab(gtx, &st.InspTabResp, respLabel, respCount, st.InspTab == 1)
				}),
			)
		}),
		layout.Rigid(mitmHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if st.InspTab == 1 {
				if isWS {
					identity := "ws/" + strconv.Itoa(st.SelReq)
					body := st.inspectorBody(identity+"|p="+boolStr(st.Pretty), func() []byte {
						return harWSText(e, st.Pretty)
					})
					return ui.harBodyPane(gtx, e.Response.Headers, &st.RespHdrList, body, "websocket/frames", identity)
				}
				identity := "resp/" + strconv.Itoa(st.SelReq)
				body := st.inspectorBody(identity, func() []byte { return harRespBody(e) })
				return ui.harBodyPane(gtx, e.Response.Headers, &st.RespHdrList, body, e.ContentType(), identity)
			}
			identity := "req/" + strconv.Itoa(st.SelReq)
			body := st.inspectorBody(identity, func() []byte { return []byte(e.Request.PostData.Text) })
			return ui.harBodyPane(gtx, e.Request.Headers, &st.ReqHdrList, body, e.Request.PostData.MimeType, identity)
		}),
	)
}

func (ui *AppUI) harInspectorHeader(gtx layout.Context, e *har.Entry) layout.Dimensions {
	st := &ui.HARView
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), harDisplayMethod(e))
				lbl.Color = theme.MethodColor(e.Request.Method)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(12), e.Request.URL)
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), harStatusText(e))
				lbl.Color = harStatusColor(e.Response.Status)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mitmBtn(gtx, ui.Theme, &st.RunBtn, "Run", widgets.IconPlay, theme.BtnPrimary, theme.BtnPrimaryFg, true)
			}),
		)
	})
}

func (ui *AppUI) harBodyPane(gtx layout.Context, headers []har.Header, hdrList *widget.List, body []byte, mime, identity string) layout.Dimensions {
	st := &ui.HARView
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(harSectionLabel(ui.Theme, "Headers")),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(180))
				return harDarkBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					if len(headers) == 0 {
						return harCentered(ui.Theme, gtx, "no headers")
					}
					return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						hdrList.Axis = layout.Vertical
						return material.List(ui.Theme, hdrList).Layout(gtx, len(headers), func(gtx layout.Context, i int) layout.Dimensions {
							return harHeaderRow(ui.Theme, gtx, headers[i])
						})
					})
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.harBodyHeader(gtx, "Body", mime, len(body), &st.PrettyBtn, st.Pretty, &st.ReqCopyBtn, len(body) > 0)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.harBodyViewer(gtx, st.ReqViewer, &st.ReqViewerKey, &st.ReqScrollDrag, &st.ReqScrollDragY, identity, body, mime, st.Pretty)
				})
			}),
		)
	})
}

func (ui *AppUI) harBodyHeader(gtx layout.Context, label, mime string, size int, prettyBtn *widget.Clickable, pretty bool, copyBtn *widget.Clickable, enabled bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			txt := label
			if mime != "" {
				txt = label + " — " + mime
			}
			if size > 0 {
				txt += "  (" + humanSize(int64(size)) + ")"
			}
			return harSectionLabel(ui.Theme, txt)(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.harToggleBtn(gtx, prettyBtn, "Pretty", pretty, enabled)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return mitmBtn(gtx, ui.Theme, copyBtn, "Copy", widgets.IconDup, theme.Border, ui.Theme.Fg, enabled)
		}),
	)
}

func (ui *AppUI) harToggleBtn(gtx layout.Context, clk *widget.Clickable, label string, active, enabled bool) layout.Dimensions {
	bg := theme.Border
	fg := ui.Theme.Fg
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return mitmBtn(gtx, ui.Theme, clk, label, nil, bg, fg, enabled)
}

func (ui *AppUI) harBodyViewer(gtx layout.Context, viewer *workspace.ResponseViewer, key *string, scrollDrag *gesture.Drag, scrollDragY *float32, identity string, body []byte, mime string, pretty bool) layout.Dimensions {
	if len(body) == 0 {
		return harCentered(ui.Theme, gtx, "no body")
	}
	if !isProbablyText(body) {
		return harCentered(ui.Theme, gtx, "[binary data — "+humanSize(int64(len(body)))+"]")
	}
	k := identity + "|pretty=" + boolStr(pretty)
	if *key != k {
		*key = k
		text := body
		if pretty {
			if p, ok := har.PrettyCode(body, mime); ok {
				text = p
			}
		}
		viewer.SetText(string(text))
	}
	vs := workspace.ResponseViewerStyle{
		Viewer:           viewer,
		Shaper:           ui.Theme.Shaper,
		Font:             widgets.MonoFont,
		TextSize:         settings.BodyTextSize,
		Color:            theme.Fg,
		HighlightColor:   theme.WithAlpha(theme.Accent, 150),
		SearchMatchColor: theme.WithAlpha(theme.Accent, 60),
		SelectionColor:   theme.Selection,
		Wrap:             true,
		Padding:          unit.Dp(8),
		Lang:             syntax.Detect(mime, body),
		Syntax:           theme.Syntax,
		BracketCycle:     settings.BracketColorization,
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return vs.Layout(gtx) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return ui.harBodyScrollbar(gtx, viewer, scrollDrag, scrollDragY)
		}),
	)
}

func (ui *AppUI) harBodyScrollbar(gtx layout.Context, viewer *workspace.ResponseViewer, scrollDrag *gesture.Drag, scrollDragY *float32) layout.Dimensions {
	bounds := viewer.GetScrollBounds()
	totalH := float32(bounds.Max.Y)
	viewH := float32(gtx.Constraints.Max.Y)
	if totalH <= viewH || totalH == 0 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	scrollY := float32(viewer.GetScrollY())
	maxScroll := totalH - viewH
	if maxScroll <= 0 {
		maxScroll = 1
	}
	frac := scrollY / maxScroll
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	thumbH := viewH * (viewH / totalH)
	if thumbH < 20 {
		thumbH = 20
	}
	thumbY := frac * (viewH - thumbH)
	trackW := gtx.Dp(unit.Dp(10))
	thumbW := gtx.Dp(unit.Dp(6))

	trackRect := image.Rect(gtx.Constraints.Max.X-trackW, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	stack := clip.Rect(trackRect).Push(gtx.Ops)
	for {
		e, ok := scrollDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			*scrollDragY = e.Position.Y
		case pointer.Drag:
			delta := e.Position.Y - *scrollDragY
			*scrollDragY = e.Position.Y
			if viewH > thumbH {
				scrollY += delta / (viewH - thumbH) * maxScroll
			}
			ny := int(scrollY)
			if ny < 0 {
				ny = 0
			}
			viewer.SetScrollY(ny)
			if ui.Window != nil {
				ui.Window.Invalidate()
			}
		}
	}
	pointer.CursorDefault.Add(gtx.Ops)
	scrollDrag.Add(gtx.Ops)
	stack.Pop()

	rect := image.Rect(
		gtx.Constraints.Max.X-thumbW-gtx.Dp(unit.Dp(2)),
		int(thumbY),
		gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
		int(thumbY+thumbH),
	)
	paint.FillShape(gtx.Ops, theme.ScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func harDarkBoxed(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	sz := dims.Size
	paint.FillShape(gtx.Ops, theme.BgDark, clip.UniformRRect(image.Rectangle{Max: sz}, 4).Op(gtx.Ops))
	call.Add(gtx.Ops)
	widgets.PaintBorder1px(gtx, sz, theme.Border)
	return dims
}

func harHeaderRow(th *material.Theme, gtx layout.Context, h har.Header) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(170))
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			lbl := material.Label(th, unit.Sp(11), h.Name)
			lbl.Color = theme.FgMuted
			lbl.Font.Typeface = widgets.MonoTypeface
			lbl.MaxLines = 1
			lbl.Truncator = "…"
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), h.Value)
			lbl.Color = theme.Fg
			lbl.Font.Typeface = widgets.MonoTypeface
			return lbl.Layout(gtx)
		}),
	)
}

func (ui *AppUI) harFilesView(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	leftW, handleW, rightW := ui.harSplit(gtx, st)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftW
			gtx.Constraints.Max.X = leftW
			return ui.harFileList(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.harSplitHandle(gtx, handleW) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightW
			gtx.Constraints.Max.X = rightW
			return ui.harFilePreview(gtx)
		}),
	)
}

func (ui *AppUI) harFileList(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	if len(st.Resources) == 0 {
		return harCentered(ui.Theme, gtx, "No files with bodies in this archive")
	}
	for len(st.FileRows) < len(st.Resources) {
		st.FileRows = append(st.FileRows, &widget.Clickable{})
	}
	return material.List(ui.Theme, &st.FileList).Layout(gtx, len(st.Resources), func(gtx layout.Context, i int) layout.Dimensions {
		clk := st.FileRows[i]
		for clk.Clicked(gtx) {
			st.SelFile = i
		}
		return harFileRow(gtx, ui.Theme, st.Resources[i], clk, st.SelFile == i)
	})
}

func harFileRow(gtx layout.Context, th *material.Theme, r har.Resource, clk *widget.Clickable, selected bool) layout.Dimensions {
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
					lbl := material.Label(th, unit.Sp(11), humanSize(int64(len(r.Body))))
					lbl.Color = theme.FgMuted
					lbl.Alignment = text.End
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func (ui *AppUI) harFilePreview(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	if st.SelFile < 0 || st.SelFile >= len(st.Resources) {
		return harCentered(ui.Theme, gtx, "Select a file to preview")
	}
	r := st.Resources[st.SelFile]
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(12), r.ZipPath)
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(mitmHLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.harBodyHeader(gtx, "Content", r.MimeType, len(r.Body), &st.PrettyBtn, st.Pretty, &st.CopyBodyBtn, true)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.harBodyViewer(gtx, st.FileViewer, &st.FileViewerKey, &st.FileScrollDrag, &st.FileScrollDragY, "file/"+strconv.Itoa(st.SelFile), r.Body, r.MimeType, st.Pretty)
				})
			})
		}),
	)
}

func (ui *AppUI) harPagesView(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	pages := st.Doc.Pages
	if len(pages) == 0 {
		return harCentered(ui.Theme, gtx, "No pages in this archive")
	}
	rows := len(pages) + 1
	for len(st.PageRows) < rows {
		st.PageRows = append(st.PageRows, &widget.Clickable{})
	}
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(ui.Theme, &st.PagesList).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
			clk := st.PageRows[row]
			if row == 0 {
				for clk.Clicked(gtx) {
					st.selectPage("")
					st.TopTab = harTabRequests
				}
				return harPageRow(gtx, ui.Theme, clk, "All pages", "", len(st.Doc.Entries), st.SelPageID == "")
			}
			p := pages[row-1]
			for clk.Clicked(gtx) {
				st.selectPage(p.ID)
				st.TopTab = harTabRequests
			}
			title := p.Title
			if strings.TrimSpace(title) == "" {
				title = p.ID
			}
			return harPageRow(gtx, ui.Theme, clk, title, p.StartedDateTime, st.pageRequestCount(p.ID), st.SelPageID == p.ID)
		})
	})
}

func harPageRow(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title, when string, count int, selected bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rowH := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y = rowH
		bg := theme.Bg
		if selected {
			bg = theme.AccentDim
		} else if clk.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, rowH)}.Op())
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), title)
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					if selected {
						lbl.Color = theme.Accent
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if when == "" {
						return layout.Dimensions{}
					}
					lbl := material.Label(th, unit.Sp(10), when)
					lbl.Color = theme.FgMuted
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(70))
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					lbl := material.Label(th, unit.Sp(11), strconv.Itoa(count)+" req")
					lbl.Color = theme.FgMuted
					lbl.Alignment = text.End
					lbl.Font.Typeface = widgets.MonoTypeface
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func (ui *AppUI) harInfoView(gtx layout.Context) layout.Dimensions {
	st := &ui.HARView
	if !st.infoCached {
		st.infoRows = harInfoRows(st.Doc.Summary())
		st.infoCached = true
	}
	rows := st.infoRows
	st.InfoList.Axis = layout.Vertical
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(ui.Theme, &st.InfoList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			return harInfoRow(ui.Theme, gtx, rows[i])
		})
	})
}

type harKV struct {
	key, val string
	header   bool
}

func harInfoRows(s har.Summary) []harKV {
	rows := []harKV{
		{key: "Archive", header: true},
		{key: "HAR version", val: orDash(s.Version)},
		{key: "Creator", val: joinNameVersion(s.CreatorName, s.CreatorVersion)},
		{key: "Browser", val: joinNameVersion(s.BrowserName, s.BrowserVersion)},
		{key: "Pages", val: strconv.Itoa(s.PageCount)},
		{key: "Requests", val: strconv.Itoa(s.EntryCount)},
		{key: "Files with body", val: strconv.Itoa(s.ResourceCount)},
		{key: "Total body size", val: humanSize(s.TotalBodyBytes)},
		{key: "First request", val: orDash(s.FirstStarted)},
		{key: "Last request", val: orDash(s.LastStarted)},
	}
	if len(s.Methods) > 0 {
		rows = append(rows, harKV{key: "Methods", header: true})
		for _, c := range s.Methods {
			rows = append(rows, harKV{key: c.Label, val: strconv.Itoa(c.Count)})
		}
	}
	if len(s.Statuses) > 0 {
		rows = append(rows, harKV{key: "Status codes", header: true})
		for _, c := range s.Statuses {
			rows = append(rows, harKV{key: c.Label, val: strconv.Itoa(c.Count)})
		}
	}
	if len(s.MimeTypes) > 0 {
		rows = append(rows, harKV{key: "Content types", header: true})
		for _, c := range s.MimeTypes {
			rows = append(rows, harKV{key: c.Label, val: strconv.Itoa(c.Count)})
		}
	}
	return rows
}

func harInfoRow(th *material.Theme, gtx layout.Context, kv harKV) layout.Dimensions {
	if kv.header {
		return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), kv.key)
			lbl.Color = theme.Accent
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		})
	}
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(180))
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				lbl := material.Label(th, unit.Sp(11), kv.key)
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), kv.val)
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (ui *AppUI) harBrowse() {
	if ui.Explorer == nil {
		return
	}
	st := &ui.HARView
	go func() {
		rc, err := ui.Explorer.ChooseFile("har", "json")
		if err != nil || rc == nil {
			if err != nil {
				st.queueLoad(nil, "", err)
				ui.Window.Invalidate()
			}
			return
		}
		defer func() { _ = rc.Close() }()
		data, rerr := io.ReadAll(rc)
		st.queueLoad(data, "", rerr)
		ui.Window.Invalidate()
	}()
}

func (ui *AppUI) harExportZip() {
	st := &ui.HARView
	if ui.Explorer == nil || len(st.Resources) == 0 {
		return
	}
	resources := st.Resources
	suggested := harExportName(st.Source)
	go func() {
		w, err := ui.Explorer.CreateFile(suggested)
		if err != nil || w == nil {
			if err != nil {
				st.setBanner("Export failed: "+err.Error(), true)
				ui.Window.Invalidate()
			}
			return
		}
		n, werr := har.WriteZip(w, resources)
		cerr := w.Close()
		switch {
		case werr != nil:
			st.setBanner("Export failed: "+werr.Error(), true)
		case cerr != nil:
			st.setBanner("Export failed: "+cerr.Error(), true)
		default:
			st.setBanner("Exported "+strconv.Itoa(n)+" files to "+suggested, false)
		}
		ui.Window.Invalidate()
	}()
}

func (ui *AppUI) harExportDir() {
	st := &ui.HARView
	if len(st.Resources) == 0 {
		return
	}
	resources := st.Resources
	go func() {
		dir, ok := pickFolderDialog("Export HAR resources to folder")
		if !ok {
			return
		}
		n, err := har.WriteDirOS(dir, resources)
		if err != nil {
			st.setBanner("Export failed: "+err.Error(), true)
		} else {
			st.setBanner("Exported "+strconv.Itoa(n)+" files to "+dir, false)
		}
		ui.Window.Invalidate()
	}()
}

func (ui *AppUI) harCopySelectedFile(gtx layout.Context) {
	st := &ui.HARView
	if st.SelFile < 0 || st.SelFile >= len(st.Resources) {
		return
	}
	if sel := st.FileViewer.SelectedText(); sel != "" {
		harClipboardWrite(gtx, []byte(sel))
		return
	}
	harClipboardWrite(gtx, st.Resources[st.SelFile].Body)
}

func (ui *AppUI) harCopySelectedReqBody(gtx layout.Context) {
	st := &ui.HARView
	if st.SelReq < 0 || st.SelReq >= len(st.Doc.Entries) {
		return
	}
	if sel := st.ReqViewer.SelectedText(); sel != "" {
		harClipboardWrite(gtx, []byte(sel))
		return
	}
	e := &st.Doc.Entries[st.SelReq]
	var body []byte
	if st.InspTab == 1 {
		if e.IsWebSocket() {
			body = harWSText(e, st.Pretty)
		} else {
			body = harRespBody(e)
		}
	} else {
		body = []byte(e.Request.PostData.Text)
	}
	harClipboardWrite(gtx, body)
}

func harClipboardWrite(gtx layout.Context, body []byte) {
	gtx.Execute(clipboard.WriteCmd{
		Type: "application/text",
		Data: io.NopCloser(strings.NewReader(string(body))),
	})
}

func (ui *AppUI) harRunSelected() {
	st := &ui.HARView
	if st.SelReq < 0 || st.SelReq >= len(st.Doc.Entries) {
		return
	}
	ui.harRunEntry(&st.Doc.Entries[st.SelReq])
}

func (ui *AppUI) harRunEntry(e *har.Entry) {
	isWS := e.IsWebSocket()
	method := e.Request.Method
	reqURL := e.Request.URL
	if isWS {
		method = workspace.MethodWS
		reqURL = harWSURL(reqURL)
	}
	rt := workspace.NewRequestTab(harRunTitle(e, reqURL))
	rt.Method = method
	rt.URLInput.SetText(reqURL)
	body := []byte(e.Request.PostData.Text)
	rt.ReqEditor.SetText(e.Request.PostData.Text)
	for _, h := range e.Request.Headers {
		if harSkipHeader(h.Name) {
			continue
		}
		rt.AddHeader(h.Name, h.Value)
	}
	if strings.TrimSpace(e.Request.PostData.Text) != "" {
		rt.BodyType = model.BodyRaw
		rt.ReqLangHint = syntax.Detect(e.Request.PostData.MimeType, body)
	}
	rt.UpdateSystemHeaders()
	ui.inheritActiveTabLayout(rt)
	ui.Tabs = append(ui.Tabs, rt)
	ui.ActiveIdx = len(ui.Tabs) - 1
	ui.SetSidebarSection("requests")
	rt.URLSubmitted = true
	ui.saveState()
	ui.Window.Invalidate()
}

func harSkipHeader(name string) bool {
	if strings.HasPrefix(name, ":") {
		return true
	}
	switch strings.ToLower(name) {
	case "content-length", "host":
		return true
	}
	return false
}

func harRunTitle(e *har.Entry, reqURL string) string {
	domain, _ := harSplitURL(reqURL)
	if domain == "" {
		domain = reqURL
	}
	return strings.TrimSpace(e.Request.Method + " " + domain)
}

func harWSURL(raw string) string {
	switch {
	case strings.HasPrefix(raw, "https://"):
		return "wss://" + strings.TrimPrefix(raw, "https://")
	case strings.HasPrefix(raw, "http://"):
		return "ws://" + strings.TrimPrefix(raw, "http://")
	default:
		return raw
	}
}

func harWSText(e *har.Entry, pretty bool) []byte {
	if len(e.WebSocketMessages) == 0 {
		return []byte("No WebSocket frames captured in this archive.")
	}
	var b strings.Builder
	for i, m := range e.WebSocketMessages {
		dir := "← receive"
		if m.Sent() {
			dir = "→ send"
		}
		kind := "text"
		if m.Binary() {
			kind = "binary"
		}
		b.WriteString(dir)
		b.WriteString("  [")
		b.WriteString(kind)
		b.WriteString("]\n")
		if m.Binary() {
			b.WriteString("[binary frame, " + strconv.Itoa(len(m.Data)) + " base64 chars]\n")
		} else {
			data := m.Data
			if pretty {
				if p, ok := har.Pretty([]byte(data), ""); ok {
					data = string(p)
				}
			}
			b.WriteString(data)
			b.WriteString("\n")
		}
		if i != len(e.WebSocketMessages)-1 {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

func (st *harState) setBanner(msg string, isErr bool) {
	st.Banner = msg
	st.BannerErr = isErr
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func harDisplayMethod(e *har.Entry) string {
	if e.IsWebSocket() {
		return "WS"
	}
	return e.Request.Method
}

func harCentered(th *material.Theme, gtx layout.Context, msg string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(12), msg)
		lbl.Color = theme.FgMuted
		lbl.Alignment = text.Middle
		return lbl.Layout(gtx)
	})
}

func harSectionLabel(th *material.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Color = theme.FgMuted
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	}
}

func harSplitURL(rawURL string) (domain, file string) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return "", rawURL
	}
	domain = u.Host
	file = u.Path
	if u.RawQuery != "" {
		file += "?" + u.RawQuery
	}
	if file == "" || file == "/" {
		file = "/"
	}
	return domain, file
}

func harShortType(mime string) string {
	if mime == "" {
		return ""
	}
	if i := strings.IndexByte(mime, '/'); i >= 0 {
		return mime[i+1:]
	}
	return mime
}

func harEntrySize(e *har.Entry) int64 {
	if e.Response.Content.Size > 0 {
		return e.Response.Content.Size
	}
	if e.Response.BodySize > 0 {
		return e.Response.BodySize
	}
	return int64(len(e.Response.Content.Text))
}

func harRespBody(e *har.Entry) []byte {
	body, _, err := e.DecodeBody()
	if err != nil {
		return []byte(e.Response.Content.Text)
	}
	return body
}

func harStatusText(e *har.Entry) string {
	if e.Response.Status <= 0 {
		return "(no response)"
	}
	s := strconv.Itoa(e.Response.Status)
	if e.Response.StatusText != "" {
		s += " " + e.Response.StatusText
	}
	return s
}

func harExportName(source string) string {
	base := source
	if base == "" {
		base = "har-export"
	}
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	return base + ".zip"
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func joinNameVersion(name, version string) string {
	switch {
	case name == "" && version == "":
		return "—"
	case version == "":
		return name
	default:
		return name + " " + version
	}
}
