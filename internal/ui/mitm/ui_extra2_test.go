package mitm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDelayConn_AppliesDelayOnce(t *testing.T) {
	r := NewRules()
	r.Set("slow.example.com", HostRule{Delay: 40 * time.Millisecond})

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		buf := make([]byte, 4)
		_, _ = io.ReadFull(server, buf)
		_, _ = server.Write([]byte("pong"))
	}()

	dc := &delayConn{Conn: client, rules: r, host: "slow.example.com"}
	start := time.Now()
	if _, err := dc.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	first := time.Since(start)
	if first < 30*time.Millisecond {
		t.Errorf("the first write must apply the configured delay, took %v", first)
	}

	buf := make([]byte, 4)
	start = time.Now()
	if _, err := io.ReadFull(dc, buf); err != nil {
		t.Fatal(err)
	}
	if second := time.Since(start); second > 30*time.Millisecond {
		t.Errorf("the delay must apply only once, second op took %v", second)
	}
	if string(buf) != "pong" {
		t.Errorf("payload = %q", buf)
	}
}

func TestDelayConn_NoRuleNoDelay(t *testing.T) {
	r := NewRules()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _, _ = io.ReadFull(server, make([]byte, 2)) }()

	dc := &delayConn{Conn: client, rules: r, host: "fast.example.com"}
	start := time.Now()
	if _, err := dc.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 30*time.Millisecond {
		t.Errorf("an unconfigured host must not be delayed, took %v", took)
	}
}

func wsFrame(opcode byte, payload []byte, mask bool) []byte {
	var b bytes.Buffer
	b.WriteByte(0x80 | opcode)
	n := len(payload)
	maskBit := byte(0)
	if mask {
		maskBit = 0x80
	}
	switch {
	case n < 126:
		b.WriteByte(maskBit | byte(n))
	case n < 65536:
		b.WriteByte(maskBit | 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		b.Write(ext[:])
	default:
		b.WriteByte(maskBit | 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		b.Write(ext[:])
	}
	key := [4]byte{0xAA, 0xBB, 0xCC, 0xDD}
	if mask {
		b.Write(key[:])
		for i, c := range payload {
			b.WriteByte(c ^ key[i%4])
		}
		return b.Bytes()
	}
	b.Write(payload)
	return b.Bytes()
}

func TestReadWSFrame(t *testing.T) {
	cases := []struct {
		name    string
		opcode  byte
		payload []byte
		mask    bool
	}{
		{"short-text", 0x1, []byte("hello"), false},
		{"short-masked", 0x1, []byte("hello"), true},
		{"binary", 0x2, []byte{0, 1, 2, 3}, false},
		{"empty", 0x9, nil, false},
		{"medium", 0x1, bytes.Repeat([]byte("x"), 300), false},
		{"medium-masked", 0x1, bytes.Repeat([]byte("y"), 300), true},
		{"large", 0x2, bytes.Repeat([]byte("z"), 70000), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			frame := wsFrame(c.opcode, c.payload, c.mask)
			r := bufio.NewReader(bytes.NewReader(frame))
			op, payload, raw, err := readWSFrame(r)
			if err != nil {
				t.Fatalf("readWSFrame: %v", err)
			}
			if op != c.opcode {
				t.Errorf("opcode = %#x, want %#x", op, c.opcode)
			}
			if !bytes.Equal(payload, c.payload) && !(len(payload) == 0 && len(c.payload) == 0) {
				t.Errorf("payload len = %d, want %d", len(payload), len(c.payload))
			}
			if !bytes.Equal(raw, frame) {
				t.Errorf("raw must reproduce the wire bytes exactly (%d vs %d)", len(raw), len(frame))
			}
		})
	}
}

func TestReadWSFrame_Truncated(t *testing.T) {
	full := wsFrame(0x1, []byte("hello"), false)
	for n := 0; n < len(full); n++ {
		r := bufio.NewReader(bytes.NewReader(full[:n]))
		if _, _, _, err := readWSFrame(r); err == nil {
			t.Errorf("truncated frame of %d bytes must error", n)
		}
	}
}

func TestReadWSFrame_RejectsOversizeLength(t *testing.T) {
	var b bytes.Buffer
	b.WriteByte(0x82)
	b.WriteByte(127)
	var ext [8]byte
	binary.BigEndian.PutUint64(ext[:], 1<<40)
	b.Write(ext[:])
	r := bufio.NewReader(bytes.NewReader(b.Bytes()))
	if _, _, _, err := readWSFrame(r); err == nil {
		t.Error("an absurd frame length must be rejected")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProcessRequest_MatchReplaceOnly(t *testing.T) {
	p := NewProxy(NewStore())
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "User-Agent", Replacement: "tracto"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRBody, Pattern: "old", Replacement: "new"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRFirstLine, Pattern: "/v1", Replacement: "/v2"})

	f := &Flow{ID: 1, Host: "example.com", URL: "https://example.com/v1/x"}
	method, uri, headers, body, drop := p.processRequest(f, "POST", "/v1/x", "HTTP/1.1",
		[][2]string{{"User-Agent", "curl"}}, []byte("old value"), true)

	if drop {
		t.Fatal("nothing should be dropped without manual interception")
	}
	if method != "POST" || uri != "/v2/x" {
		t.Errorf("method/uri = %q %q", method, uri)
	}
	if headerVal(headers, "User-Agent") != "tracto" {
		t.Errorf("headers = %+v", headers)
	}
	if string(body) != "new value" {
		t.Errorf("body = %q", body)
	}
}

func TestProcessResponse_MatchReplaceOnly(t *testing.T) {
	p := NewProxy(NewStore())
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "secret", Replacement: "***"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRFirstLine, Pattern: "200 OK", Replacement: "201 Created"})

	f := &Flow{ID: 1, Host: "example.com"}
	status, headers, body, drop := p.processResponse(f, "200 OK", "HTTP/1.1",
		[][2]string{{"X-Frame-Options", "DENY"}, {"Server", "nginx"}}, []byte("a secret b"), true)

	if drop {
		t.Fatal("nothing should be dropped without manual interception")
	}
	if status != "201 Created" {
		t.Errorf("status = %q", status)
	}
	if headerVal(headers, "X-Frame-Options") != "" {
		t.Errorf("X-Frame-Options was not stripped: %+v", headers)
	}
	if headerVal(headers, "Server") != "nginx" {
		t.Errorf("other headers must survive: %+v", headers)
	}
	if string(body) != "a *** b" {
		t.Errorf("body = %q", body)
	}
}

func TestProcessRequest_ManualForwardEdit(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	f := &Flow{ID: 7, Host: "example.com", URL: "https://example.com/a"}
	type result struct {
		method, uri string
		body        []byte
		drop        bool
	}
	res := make(chan result, 1)
	go func() {
		m, u, _, b, d := p.processRequest(f, "GET", "/a", "HTTP/1.1",
			[][2]string{{"Host", "example.com"}}, nil, true)
		res <- result{m, u, b, d}
	}()

	waitQueue(t, p.Manual, 1)
	q := p.Manual.Queue()
	if q[0].FlowID != 7 || q[0].Kind != HeldRequest {
		t.Fatalf("held message wrong: %+v", q[0])
	}
	if !strings.HasPrefix(string(q[0].Raw), "GET /a HTTP/1.1\r\n") {
		t.Errorf("held raw = %q", q[0].Raw)
	}
	p.Manual.Forward(q[0].ID, []byte("PUT /edited HTTP/1.1\r\nHost: example.com\r\nContent-Length: 8\r\n\r\nnew body"))

	got := <-res
	if got.drop {
		t.Fatal("forward must not drop")
	}
	if got.method != "PUT" || got.uri != "/edited" || string(got.body) != "new body" {
		t.Errorf("edits not applied: %+v", got)
	}
}

func TestProcessRequest_ManualDrop(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	dropped := make(chan bool, 1)
	go func() {
		_, _, _, _, d := p.processRequest(&Flow{ID: 1, Host: "x"}, "GET", "/", "HTTP/1.1", nil, nil, true)
		dropped <- d
	}()
	waitQueue(t, p.Manual, 1)
	p.Manual.Drop(p.Manual.Queue()[0].ID)
	if !<-dropped {
		t.Error("Drop must propagate to processRequest")
	}
}

func TestProcessRequest_SkippedWhenOutOfScope(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	done := make(chan struct{})
	go func() {
		p.processRequest(&Flow{ID: 1, Host: "x"}, "GET", "/", "HTTP/1.1", nil, nil, false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("an out-of-scope request must not be held")
	}
}

func TestProcessResponse_SkippedWhenResponsesDisabled(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	p.Manual.SetInterceptResponses(false)
	defer p.Manual.SetOn(false)

	done := make(chan struct{})
	go func() {
		p.processResponse(&Flow{ID: 1, Host: "x"}, "200 OK", "HTTP/1.1", nil, nil, true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("responses must not be held when response interception is off")
	}
}

func TestProcessResponse_ManualForward(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	p.Manual.SetInterceptResponses(true)
	defer p.Manual.SetOn(false)

	type result struct {
		status string
		body   []byte
	}
	res := make(chan result, 1)
	go func() {
		st, _, b, _ := p.processResponse(&Flow{ID: 3, Host: "x"}, "200 OK", "HTTP/1.1",
			[][2]string{{"Content-Type", "text/plain"}}, []byte("orig"), true)
		res <- result{st, b}
	}()
	waitQueue(t, p.Manual, 1)
	q := p.Manual.Queue()
	if q[0].Kind != HeldResponse {
		t.Fatalf("kind = %q", q[0].Kind)
	}
	p.Manual.Forward(q[0].ID, []byte("HTTP/1.1 404 Not Found\r\nContent-Length: 5\r\n\r\nedits"))

	got := <-res
	if got.status != "404 Not Found" || string(got.body) != "edits" {
		t.Errorf("edits not applied: %+v", got)
	}
}

func TestProcessRequest_UnparseableEditIsIgnored(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	res := make(chan string, 1)
	go func() {
		m, _, _, _, _ := p.processRequest(&Flow{ID: 1, Host: "x"}, "GET", "/", "HTTP/1.1", nil, nil, true)
		res <- m
	}()
	waitQueue(t, p.Manual, 1)
	p.Manual.Forward(p.Manual.Queue()[0].ID, []byte("total garbage"))
	if got := <-res; got != "GET" {
		t.Errorf("an unparseable edit must leave the original in place, got %q", got)
	}
}

func TestInterceptor_DrainAllOnOff(t *testing.T) {
	in := NewInterceptor()
	in.SetOn(true)
	done := make(chan bool, 1)
	go func() {
		_, drop := in.Hold(&Held{Kind: HeldRequest, Raw: []byte("keep")})
		done <- drop
	}()
	waitQueue(t, in, 1)
	in.SetOn(false)
	select {
	case drop := <-done:
		if drop {
			t.Error("draining on shutdown must forward, not drop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetOn(false) did not release the held message")
	}
	if in.Len() != 0 {
		t.Errorf("queue must be empty after draining, len=%d", in.Len())
	}
}

func TestInterceptor_ResolveUnknownIDIsNoop(t *testing.T) {
	in := NewInterceptor()
	in.Forward(9999, []byte("x"))
	in.Drop(9999)
	if in.Len() != 0 {
		t.Error("resolving an unknown ID must not change the queue")
	}
}

func TestInterceptor_ForwardNilKeepsOriginal(t *testing.T) {
	in := NewInterceptor()
	in.SetOn(true)
	defer in.SetOn(false)
	res := make(chan []byte, 1)
	go func() {
		edited, _ := in.Hold(&Held{Kind: HeldRequest, Raw: []byte("original")})
		res <- edited
	}()
	waitQueue(t, in, 1)
	in.Forward(in.Queue()[0].ID, nil)
	if got := string(<-res); got != "original" {
		t.Errorf("a nil edit must forward the original, got %q", got)
	}
}

func TestExportCA_NoCA(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	rig.s.exportCA(false)
	if rig.s.CABanner != "Generate a CA first" {
		t.Errorf("banner = %q", rig.s.CABanner)
	}
}

func TestUILayout_StatusBarVariants(t *testing.T) {
	banners := []string{
		"", "Proxy listening on 127.0.0.1:8080",
		"Start failed: address in use",
		"Administrator privileges required",
		"something neutral",
	}
	for _, b := range banners {
		rig := newUIRig(t, image.Pt(1200, 700))
		rig.s.StatusBanner = b
		if d := rig.frames(2); d.Size.Y <= 0 {
			t.Errorf("banner %q produced no dimensions", b)
		}
	}
}

func TestUILayout_SidebarHelpAndGuides(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTLSOpen = true
	rig.s.HelpOpen = true
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("the import guide produced no dimensions")
	}

	rig.s.HelpBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.HelpOpen {
		t.Error("the help button must toggle the guide closed")
	}
	rig.s.HelpBtn.Click()
	rig.sidebarFrames(2)
	if !rig.s.HelpOpen {
		t.Error("the help button must toggle the guide open")
	}
}

func TestUILayout_SidebarWithCA(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	rig.s.Proxy.SetCA(ca)
	rig.s.SecTLSOpen = true
	rig.s.CABanner = "CA generated"
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("the TLS section with a CA produced no dimensions")
	}
}

func TestUILayout_MRAndScopeRowVariants(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecScopeOpen = true
	for _, typ := range []string{MRRequest, MRResponse} {
		for _, area := range []string{MRFirstLine, MRHeader, MRBody} {
			rig.s.Proxy.MR.Add(MatchReplaceRule{
				Enabled: area == MRBody, Type: typ, Area: area,
				Pattern: "p", Replacement: "r", IsRegex: area == MRHeader, Comment: "note",
			})
		}
	}
	for _, kind := range []string{ScopeInclude, ScopeExclude} {
		for _, field := range []string{"host", "protocol", "port", "path"} {
			rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: kind == ScopeInclude, Kind: kind, Field: field, Pattern: "x"})
		}
	}
	if d := rig.sidebarFrames(3); d.Size.Y <= 0 {
		t.Fatal("populated MR/scope sections produced no dimensions")
	}
}
