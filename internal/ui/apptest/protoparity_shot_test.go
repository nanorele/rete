//go:build screenshots

package apptest

import (
	. "rete/internal/ui"

	"bufio"
	"context"
	"image"
	"net"
	"net/http"
	"testing"
	"time"

	"rete/internal/ui/flow"
	"rete/internal/ui/settings"
	"rete/internal/ui/workspace"
	"rete/internal/ws"

	"github.com/nanorele/gio/f32"
)

func startEchoWSServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					_ = c.Close()
					return
				}
				res, err := ws.Upgrade(c, br, req, ws.UpgradeOptions{})
				if err != nil {
					_ = c.Close()
					return
				}
				conn := res.Conn
				for {
					op, payload, err := conn.ReadMessage()
					if err != nil {
						return
					}
					if op == ws.OpText || op == ws.OpBinary {
						_ = conn.WriteMessage(op, payload)
					}
				}
			}(c)
		}
	}()
	return "ws://" + l.Addr().String()
}

func protoParityScenes(t *testing.T) []scene {
	wsURL := startEchoWSServer(t)
	return []scene{
		{"pp-http", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			respTab(ui)
		}},
		{"pp-ws-open", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText(wsURL)
			tab.AddHeader("Origin", "https://example.com")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			tab.WSConnect(context.Background(), nil, nil, nil)
			deadline := time.Now().Add(3 * time.Second)
			for s.State() != workspace.WSStateOpen && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
			tab.WSSendText(`{"hello":"world"}`)
			time.Sleep(150 * time.Millisecond)
			s.Selected = 1
		}},
		{"pp-gql", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodGraphQL
			tab.URLInput.SetText("https://api.example.com/graphql")
			tab.AddHeader("Authorization", "Bearer abcdef")
			tab.HeadersExpanded = true
			g := tab.EnsureGQL()
			g.Query.SetText("query Users($limit: Int) {\n  users(limit: $limit) {\n    id\n    name\n  }\n}")
			g.Variables.SetText("{\n  \"limit\": 50\n}")
			tab.RespEditor.SetText("{\n  \"data\": {\n    \"users\": [\n      {\"id\": 1, \"name\": \"alice\"}\n    ]\n  }\n}")
			tab.Status = "Ready"
		}},
		{"pp-flow-ws", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "flows"
			ui.Flow = flow.NewEditor()
			ui.Flow.AddRequestNode(flow.TabRequest{
				Name:      "Ping socket",
				Kind:      flow.KindWSRequest,
				URL:       "wss://api.example.com/ws",
				Headers:   [][2]string{{"Origin", "https://example.com"}},
				Subprotos: []string{"graphql-ws"},
				WSMessage: `{"type":"ping"}`,
				WSOpcode:  "TEXT",
			})
			ui.Flow.AddRequestNode(flow.TabRequest{
				Name:     "Fetch users",
				Kind:     flow.KindGQLRequest,
				URL:      "https://api.example.com/graphql",
				GQLQuery: "query { users { id } }",
				GQLVars:  `{"limit": 10}`,
			})
		}},
		{"pp-tabctx", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			respTab(ui)
			ui.TabBar.TabCtxMenuOpen = true
			ui.TabBar.TabCtxMenuIdx = 0
			ui.TabBar.TabCtxMenuPos = f32.Pt(40, 30)
		}},
	}
}

func TestProtoParityScreenshots(t *testing.T) {
	for _, sc := range protoParityScenes(t) {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}
