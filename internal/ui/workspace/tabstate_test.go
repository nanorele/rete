package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ws"
)

func TestOpcodeStringRoundTrip(t *testing.T) {
	if got := opcodeToString(ws.OpBinary); got != "BIN" {
		t.Errorf("opcodeToString(OpBinary) = %q, want BIN", got)
	}
	if got := opcodeToString(ws.OpText); got != "TEXT" {
		t.Errorf("opcodeToString(OpText) = %q, want TEXT", got)
	}
	if got := opcodeToString(ws.OpPing); got != "TEXT" {
		t.Errorf("opcodeToString(OpPing) = %q, want the TEXT fallback", got)
	}
	for _, s := range []string{"BIN", "binary"} {
		if got := opcodeFromString(s); got != ws.OpBinary {
			t.Errorf("opcodeFromString(%q) = %v, want OpBinary", s, got)
		}
	}
	for _, s := range []string{"TEXT", "text", "", "nonsense"} {
		if got := opcodeFromString(s); got != ws.OpText {
			t.Errorf("opcodeFromString(%q) = %v, want OpText", s, got)
		}
	}
}

func TestTabFromStateDefaults(t *testing.T) {
	rt := TabFromState(persist.TabState{})
	if rt.Title != "New request" {
		t.Errorf("Title = %q, want the placeholder", rt.Title)
	}
	if rt.Method != "GET" {
		t.Errorf("Method = %q, want GET", rt.Method)
	}
	if rt.WS != nil {
		t.Error("a state with no WS block must not create a WS session")
	}
	if rt.GQL != nil {
		t.Error("a state with no GQL block must not create a GQL session")
	}
}

func TestTabFromStateKindOverridesMethod(t *testing.T) {
	cases := []struct {
		kind   string
		method string
		want   string
	}{
		{TabKindWebSocket, "POST", MethodWS},
		{TabKindGraphQL, "GET", MethodGraphQL},
		{TabKindHTTP, "PATCH", "PATCH"},
		{TabKindHTTP, "", "GET"},
	}
	for _, c := range cases {
		rt := TabFromState(persist.TabState{Kind: c.kind, Method: c.method, Title: "x"})
		if rt.Method != c.want {
			t.Errorf("kind=%q method=%q -> %q, want %q", c.kind, c.method, rt.Method, c.want)
		}
	}
}

func TestTabFromStateIgnoresOutOfRangeRatios(t *testing.T) {
	base := NewRequestTab("x")
	rt := TabFromState(persist.TabState{
		Title: "x", SplitRatio: 0, VStackRatio: 0, HeaderSplitRatio: 0,
	})
	if rt.SplitRatio != base.SplitRatio {
		t.Errorf("SplitRatio = %v, want the default %v when the saved value is 0", rt.SplitRatio, base.SplitRatio)
	}
	if rt.VStackRatio != base.VStackRatio {
		t.Errorf("VStackRatio = %v, want the default %v", rt.VStackRatio, base.VStackRatio)
	}
	if rt.HeaderKeyW != base.HeaderKeyW {
		t.Errorf("HeaderKeyW = %v, want the default %v", rt.HeaderKeyW, base.HeaderKeyW)
	}
}

func TestStateFromTabRoundTrip(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "up.bin")
	if err := os.WriteFile(bin, []byte("1234567"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewRequestTab("Orders")
	src.Method = "POST"
	src.URLInput.SetText("http://api.test/orders?a=1")
	src.ReqEditor.SetText(`{"x":1}`)
	src.AddHeader("X-Real", "yes")
	src.UpdateSystemHeaders()
	src.HeadersExpanded = true
	src.HeadersAbsHeight = 140
	src.SplitRatio = 0.42
	src.VStackRatio = 0.66
	src.LayoutMode = LayoutModeHoriz
	src.HeaderKeyW = 180
	src.ReqWrapEnabled = false
	src.BodyType = model.BodyFormData
	src.applyFormParts([]model.ParsedFormPart{
		{Key: "text", Value: "v", Kind: model.FormPartText},
		{Key: "file", Kind: model.FormPartFile, FilePath: bin},
	})
	src.applyURLEncoded([]model.ParsedKV{{Key: "ue", Value: "1"}})
	src.BinaryFilePath = bin
	src.ApplyAuth(model.ParsedAuth{Type: "basic", Username: "u", Password: "p"})
	src.ApplyCookies([]model.ParsedKV{{Key: "sid", Value: "9"}})

	ts := StateFromTab(src)
	if ts.Kind != TabKindHTTP {
		t.Errorf("Kind = %q, want HTTP", ts.Kind)
	}
	for _, h := range ts.Headers {
		if h.Key == "" {
			t.Errorf("persisted headers must not contain blank keys: %+v", ts.Headers)
		}
		if h.Key == "Content-Length" || h.Key == "Host" {
			t.Errorf("generated headers must not be persisted: %+v", ts.Headers)
		}
	}

	dst := TabFromState(ts)
	if dst.Title != "Orders" || dst.Method != "POST" {
		t.Errorf("title/method = %q/%q", dst.Title, dst.Method)
	}
	if dst.URLInput.Text() != src.URLInput.Text() {
		t.Errorf("URL = %q, want %q", dst.URLInput.Text(), src.URLInput.Text())
	}
	if dst.ReqEditor.Text() != src.ReqEditor.Text() {
		t.Errorf("body = %q", dst.ReqEditor.Text())
	}
	if dst.SplitRatio != 0.42 || dst.VStackRatio != 0.66 || dst.HeaderKeyW != 180 {
		t.Errorf("ratios = %v/%v/%v", dst.SplitRatio, dst.VStackRatio, dst.HeaderKeyW)
	}
	if dst.LayoutMode != LayoutModeHoriz {
		t.Errorf("LayoutMode = %v", dst.LayoutMode)
	}
	if dst.ReqWrapEnabled {
		t.Error("ReqWrapEnabled=false must survive the round trip")
	}
	if !dst.HeadersExpanded || dst.HeadersAbsHeight != 140 {
		t.Errorf("headers area = %v/%d", dst.HeadersExpanded, dst.HeadersAbsHeight)
	}
	if dst.BodyType != model.BodyFormData {
		t.Errorf("BodyType = %v, want form-data", dst.BodyType)
	}
	if len(dst.FormParts) != 2 {
		t.Fatalf("FormParts = %d, want 2", len(dst.FormParts))
	}
	if dst.FormParts[1].Kind != model.FormPartFile || dst.FormParts[1].FileSize != 7 {
		t.Errorf("file part = kind %v size %d, want file/7", dst.FormParts[1].Kind, dst.FormParts[1].FileSize)
	}
	if len(dst.URLEncoded) != 1 || dst.URLEncoded[0].Key.Text() != "ue" {
		t.Errorf("URLEncoded = %#v", dst.URLEncoded)
	}
	if dst.BinaryFilePath != bin || dst.BinaryFileSize != 7 {
		t.Errorf("binary = %q/%d", dst.BinaryFilePath, dst.BinaryFileSize)
	}
	if got := dst.AuthModel(); got.Type != "basic" || got.Username != "u" || got.Password != "p" {
		t.Errorf("auth = %+v", got)
	}
	if got := dst.CookieModels(); len(got) != 1 || got[0].Key != "sid" || got[0].Value != "9" {
		t.Errorf("cookies = %+v", got)
	}
	realHeaders := 0
	for _, h := range dst.Headers {
		if !h.IsGenerated {
			realHeaders++
			if h.Key.Text() != "X-Real" {
				t.Errorf("unexpected user header %q", h.Key.Text())
			}
		}
	}
	if realHeaders != 1 {
		t.Errorf("user header count = %d, want 1", realHeaders)
	}
}

func TestStateFromTabOmitsEmptyAuth(t *testing.T) {
	src := NewRequestTab("t")
	if ts := StateFromTab(src); ts.Auth != nil {
		t.Errorf("Auth = %+v, want nil when no auth is configured", ts.Auth)
	}
	src.ApplyAuth(model.ParsedAuth{Type: "bearer", Token: "z"})
	ts := StateFromTab(src)
	if ts.Auth == nil || ts.Auth.Type != "bearer" || ts.Auth.Token != "z" {
		t.Errorf("Auth = %+v", ts.Auth)
	}
}

func TestStateFromTabWebSocketRoundTrip(t *testing.T) {
	src := NewRequestTab("Socket")
	src.Method = MethodWS
	src.URLInput.SetText("wss://echo.test/ws")
	s := src.EnsureWS()
	s.AddSubprotocol("chat")
	s.AddSubprotocol("   ")
	s.OptionsExpanded = true
	s.SubprotosAbsHeight = 90
	s.OfferDeflate = true
	s.UseMsgpackProto = true
	s.ProtoCmdEditor.SetText("7")
	s.ProtoSeqEditor.SetText("-3")
	s.ProtoOpcodeEditor.SetText("12")
	s.InsecureSkipVerify = true
	s.UseTractoCA = true
	s.SplitRatio = 0.3
	s.ComposerRatio = 0.7
	s.AppendSavedSend("hello", "hi", ws.OpText)
	s.AppendSavedSend("blob", "00ff", ws.OpBinary)

	ts := StateFromTab(src)
	if ts.Kind != TabKindWebSocket {
		t.Fatalf("Kind = %q, want websocket", ts.Kind)
	}
	if ts.WS == nil {
		t.Fatal("WS state missing")
	}
	if len(ts.WS.Subprotocols) != 1 || ts.WS.Subprotocols[0] != "chat" {
		t.Errorf("Subprotocols = %#v, want blank entries dropped", ts.WS.Subprotocols)
	}

	dst := TabFromState(ts)
	if dst.Method != MethodWS {
		t.Errorf("Method = %q", dst.Method)
	}
	d := dst.WS
	if d == nil {
		t.Fatal("WS session not restored")
	}
	if got := d.SubprotocolList(); len(got) != 1 || got[0] != "chat" {
		t.Errorf("SubprotocolList = %#v", got)
	}
	if !d.OptionsExpanded || d.SubprotosAbsHeight != 90 {
		t.Errorf("options = %v/%d", d.OptionsExpanded, d.SubprotosAbsHeight)
	}
	if !d.OfferDeflate || !d.UseMsgpackProto || !d.InsecureSkipVerify || !d.UseTractoCA {
		t.Error("WS toggles did not survive the round trip")
	}
	if d.ProtoCmdEditor.Text() != "7" || d.ProtoSeqEditor.Text() != "-3" || d.ProtoOpcodeEditor.Text() != "12" {
		t.Errorf("proto fields = %q/%q/%q", d.ProtoCmdEditor.Text(), d.ProtoSeqEditor.Text(), d.ProtoOpcodeEditor.Text())
	}
	if d.SplitRatio != 0.3 || d.ComposerRatio != 0.7 {
		t.Errorf("ws ratios = %v/%v", d.SplitRatio, d.ComposerRatio)
	}
	if len(d.SavedSends) != 2 {
		t.Fatalf("SavedSends = %d, want 2", len(d.SavedSends))
	}
	if d.SavedSends[0].Opcode != ws.OpText || d.SavedSends[1].Opcode != ws.OpBinary {
		t.Errorf("saved-send opcodes = %v/%v", d.SavedSends[0].Opcode, d.SavedSends[1].Opcode)
	}
	if d.SavedSends[0].Name != "hello" || d.SavedSends[1].Text != "00ff" {
		t.Errorf("saved sends = %+v", d.SavedSends)
	}
}

func TestStateFromTabGraphQLRoundTrip(t *testing.T) {
	src := NewRequestTab("GQL")
	src.Method = MethodGraphQL
	g := src.EnsureGQL()
	g.Query.SetText("{ me { id } }")
	g.Variables.SetText(`{"a":1}`)
	g.VarsSplitRatio = 0.35

	ts := StateFromTab(src)
	if ts.Kind != TabKindGraphQL || ts.GQL == nil {
		t.Fatalf("Kind = %q, GQL = %+v", ts.Kind, ts.GQL)
	}

	dst := TabFromState(ts)
	if dst.Method != MethodGraphQL {
		t.Errorf("Method = %q", dst.Method)
	}
	if dst.GQL == nil {
		t.Fatal("GQL session not restored")
	}
	if dst.GQL.Query.Text() != "{ me { id } }" {
		t.Errorf("query = %q", dst.GQL.Query.Text())
	}
	if dst.GQL.Variables.Text() != `{"a":1}` {
		t.Errorf("variables = %q", dst.GQL.Variables.Text())
	}
	if dst.GQL.VarsSplitRatio != 0.35 {
		t.Errorf("VarsSplitRatio = %v", dst.GQL.VarsSplitRatio)
	}
}

func TestTabFromStateGraphQLZeroRatioKeepsDefault(t *testing.T) {
	rt := TabFromState(persist.TabState{
		Title: "g", Kind: TabKindGraphQL,
		GQL: &persist.GQLTabState{Query: "{a}", VarsSplitRatio: 0},
	})
	if rt.GQL == nil {
		t.Fatal("GQL session missing")
	}
	if rt.GQL.VarsSplitRatio != 0.6 {
		t.Errorf("VarsSplitRatio = %v, want the 0.6 default", rt.GQL.VarsSplitRatio)
	}
}

func TestTabFromStateMissingFilesReportZeroSize(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.bin")
	rt := TabFromState(persist.TabState{
		Title:      "t",
		BinaryPath: missing,
		FormParts:  []persist.FormPartState{{Key: "f", Kind: "file", FilePath: missing}},
	})
	if rt.BinaryFileSize != 0 {
		t.Errorf("BinaryFileSize = %d, want 0", rt.BinaryFileSize)
	}
	if len(rt.FormParts) != 1 || rt.FormParts[0].FileSize != 0 {
		t.Errorf("form part size = %#v, want 0", rt.FormParts)
	}
	if rt.BinaryFilePath != missing {
		t.Errorf("BinaryFilePath = %q, want the path kept even when the file is gone", rt.BinaryFilePath)
	}
}
