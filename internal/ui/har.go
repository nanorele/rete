package ui

import (
	"io"
	"strings"

	"tracto/internal/har"
	"tracto/internal/model"
	harui "tracto/internal/ui/har"
	"tracto/pkg/syntax"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) harHost() *harui.Host {
	return &harui.Host{
		Theme:  ui.Theme,
		Window: ui.Window,
		ChooseHAR: func() (io.ReadCloser, error) {
			if ui.Explorer == nil {
				return nil, nil
			}
			return ui.Explorer.ChooseFile("har", "json")
		},
		CreateFile: func(name string) (io.WriteCloser, error) {
			if ui.Explorer == nil {
				return nil, nil
			}
			return ui.Explorer.CreateFile(name)
		},
		RunEntry: ui.harRunEntry,
	}
}

func (ui *AppUI) layoutHARSection(gtx layout.Context) layout.Dimensions {
	return ui.HARView.Layout(gtx, ui.harHost())
}

func (ui *AppUI) harHandleSearchShortcut(gtx layout.Context) {
	ui.HARView.HandleSearchShortcut(gtx)
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
	domain, _ := harui.SplitURL(reqURL)
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
