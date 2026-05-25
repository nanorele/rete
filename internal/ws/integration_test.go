package ws

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func startEchoServer(t *testing.T, opts UpgradeOptions) (addr string, stop func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go handleEcho(c, opts)
		}
	}()
	return l.Addr().String(), func() {
		_ = l.Close()
		wg.Wait()
	}
}

func handleEcho(c net.Conn, opts UpgradeOptions) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	res, err := Upgrade(c, br, req, opts)
	if err != nil {
		return
	}
	defer res.Conn.Close()
	for {
		op, payload, err := res.Conn.ReadMessage()
		if err != nil {
			return
		}
		switch op {
		case OpText, OpBinary:
			if err := res.Conn.WriteMessage(op, payload); err != nil {
				return
			}
		case OpPing:
			if err := res.Conn.WriteMessage(OpPong, payload); err != nil {
				return
			}
		case OpClose:
			_ = res.Conn.WriteMessage(OpClose, payload)
			return
		}
	}
}

func TestEchoPlainText(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	for _, want := range []string{"hello", "world", "third message"} {
		if err := res.Conn.WriteMessage(OpText, []byte(want)); err != nil {
			t.Fatalf("Write: %v", err)
		}
		op, payload, err := res.Conn.ReadMessage()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if op != OpText || string(payload) != want {
			t.Errorf("got op=%v %q want OpText %q", op, payload, want)
		}
	}
}

func TestEchoDeflate(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{AcceptDeflate: true})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{OfferDeflate: true})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if !res.Extensions.Negotiated {
		t.Fatalf("expected deflate to be negotiated")
	}
	payload := bytes.Repeat([]byte("ABCD"), 500)
	if err := res.Conn.WriteMessage(OpText, payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	op, got, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if op != OpText {
		t.Errorf("op=%v", op)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch")
	}
}

func TestEchoPingPong(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if err := res.Conn.WriteMessage(OpPing, []byte("pingdata")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	op, payload, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if op != OpPong || string(payload) != "pingdata" {
		t.Errorf("got op=%v %q", op, payload)
	}
}

func TestEchoClose(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if err := res.Conn.WriteClose(CloseNormal, "bye"); err != nil {
		t.Fatalf("WriteClose: %v", err)
	}
	op, payload, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if op != OpClose {
		t.Errorf("op=%v", op)
	}
	code, reason := ParseClosePayload(payload)
	if code != CloseNormal || reason != "bye" {
		t.Errorf("got code=%d reason=%q", code, reason)
	}
}
