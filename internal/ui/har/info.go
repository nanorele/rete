package har

import (
	"strconv"
	"strings"

	"tracto/internal/har"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func (s *Section) infoView(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	if !s.infoCached {
		s.infoRows = infoRows(s.Doc.Summary())
		s.infoCached = true
	}
	rows := s.infoRows
	s.InfoList.Axis = layout.Vertical
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(th, &s.InfoList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			return infoRow(th, gtx, rows[i])
		})
	})
}

type infoKV struct {
	key, val string
	header   bool
}

func infoRows(s har.Summary) []infoKV {
	rows := []infoKV{
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
		rows = append(rows, infoKV{key: "Methods", header: true})
		for _, c := range s.Methods {
			rows = append(rows, infoKV{key: c.Label, val: strconv.Itoa(c.Count)})
		}
	}
	if len(s.Statuses) > 0 {
		rows = append(rows, infoKV{key: "Status codes", header: true})
		for _, c := range s.Statuses {
			rows = append(rows, infoKV{key: c.Label, val: strconv.Itoa(c.Count)})
		}
	}
	if len(s.MimeTypes) > 0 {
		rows = append(rows, infoKV{key: "Content types", header: true})
		for _, c := range s.MimeTypes {
			rows = append(rows, infoKV{key: c.Label, val: strconv.Itoa(c.Count)})
		}
	}
	return rows
}

func infoRow(th *material.Theme, gtx layout.Context, kv infoKV) layout.Dimensions {
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
