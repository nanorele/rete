//go:build screenshots

package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"

	harui "tracto/internal/ui/har"
	"tracto/internal/ui/workspace"
)

func respTab(ui *AppUI) *workspace.RequestTab {
	withTab(ui)
	ui.SidebarSection = "requests"
	tab := ui.Tabs[0]
	tab.Method = "POST"
	tab.URLInput.SetText("https://api.example.com/v2/users/search?page=2&limit=50")
	tab.ReqEditor.SetText("{\n  \"query\": \"alice\",\n  \"filters\": {\"active\": true}\n}")
	tab.RespEditor.SetText("{\n  \"users\": [\n    {\"id\": 1, \"name\": \"alice\"}\n  ],\n  \"count\": 1\n}")
	tab.Status = "200 OK · 1.2 KB · 231 ms"
	tab.AddHeader("Authorization", "Bearer abcdef")
	tab.HeadersExpanded = true
	return tab
}

func adaptScenes() []scene {
	return []scene{
		{"adapt-sidebar-max", func(ui *AppUI) {
			respTab(ui)
			ui.SidebarWidth = 640
		}},
		{"adapt-hsplit-resp-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			tab.SplitRatio = 0.95
		}},
		{"adapt-hsplit-req-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			tab.SplitRatio = 0.05
		}},
		{"adapt-vstack-req-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.VStackRatio = 0.01
		}},
		{"adapt-vstack-resp-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.VStackRatio = 0.99
		}},
		{"adapt-ws-narrow", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("WS")}
			ui.ActiveIdx = 0
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.example.com/socket")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			s.UseMsgpackProto = true
			s.AddSubprotocol("graphql-transport-ws")
			s.SplitRatio = 0.2
			ui.SidebarWidth = 500
		}},
		{"adapt-ws-msgs-min", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("WS")}
			ui.ActiveIdx = 0
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.example.com/socket")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			s.SplitRatio = 0.95
		}},
		{"adapt-runner", func(ui *AppUI) {
			tab := respTab(ui)
			tab.RunOpen = true
		}},
		{"adapt-netlimit-narrow", func(ui *AppUI) {
			ui.SidebarSection = "netlimit"
			ui.SidebarWidth = 160
		}},
		{"adapt-mitm-inspector-min", func(ui *AppUI) {
			ui.SidebarSection = "mitm"
			ui.MITM.SplitRatio = 0.95
		}},
		{"adapt-mitm-table-min", func(ui *AppUI) {
			ui.SidebarSection = "mitm"
			ui.MITM.SplitRatio = 0.05
		}},
		{"adapt-har", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
		}},
		{"adapt-har-inspector-min", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
			ui.HARView.SplitRatio = 0.95
		}},
		{"adapt-har-table-min", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
			ui.HARView.SplitRatio = 0.05
		}},
		{"adapt-har-files", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
			ui.HARView.TopTab = harui.TabFiles
			ui.HARView.SplitRatio = 0.95
		}},
		{"adapt-settings-tiny", settingsScene(0)},
	}
}

const sampleHAR = `{
  "log": {
    "version": "1.2",
    "creator": {"name": "test", "version": "1.0"},
    "pages": [{"id": "page_1", "title": "Example page with a fairly long title", "startedDateTime": "2026-01-01T00:00:00Z", "pageTimings": {}}],
    "entries": [
      {
        "pageref": "page_1",
        "startedDateTime": "2026-01-01T00:00:01Z",
        "time": 231,
        "request": {
          "method": "GET",
          "url": "https://api.example.com/v2/users/search?page=2&limit=50&sort=name-descending",
          "headers": [{"name": "Accept", "value": "application/json"}, {"name": "Authorization", "value": "Bearer abcdef0123456789"}],
          "queryString": [],
          "bodySize": 0
        },
        "response": {
          "status": 200,
          "statusText": "OK",
          "headers": [{"name": "Content-Type", "value": "application/json; charset=utf-8"}],
          "content": {"size": 64, "mimeType": "application/json", "text": "{\"users\":[{\"id\":1,\"name\":\"alice\"}],\"count\":1}"},
          "bodySize": 64
        }
      },
      {
        "pageref": "page_1",
        "startedDateTime": "2026-01-01T00:00:02Z",
        "time": 87,
        "request": {
          "method": "POST",
          "url": "https://cdn.example.com/static/assets/js/very-long-bundle-name.materialized.min.js",
          "headers": [],
          "queryString": [],
          "bodySize": 0
        },
        "response": {
          "status": 404,
          "statusText": "Not Found",
          "headers": [{"name": "Content-Type", "value": "text/html"}],
          "content": {"size": 22, "mimeType": "text/html", "text": "<html>not found</html>"},
          "bodySize": 22
        }
      }
    ]
  }
}`

func TestAdaptScreenshots(t *testing.T) {
	sizes := []image.Point{{X: 1280, Y: 800}, {X: 700, Y: 500}, {X: 560, Y: 400}}
	for _, sz := range sizes {
		for _, sc := range adaptScenes() {
			sc := sc
			sz := sz
			t.Run(sc.name, func(t *testing.T) {
				renderScene(t, sc, sz)
			})
		}
	}
}
