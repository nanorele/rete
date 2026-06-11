package ws

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func silentServer(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return l
}

func TestDialHandshakeTimeout(t *testing.T) {
	l := silentServer(t)
	start := time.Now()
	_, err := Dial(context.Background(), "ws://"+l.Addr().String(), DialOptions{DialTimeout: 300 * time.Millisecond})
	if err == nil {
		t.Fatal("expected handshake timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dial blocked for %v despite timeout", elapsed)
	}
}

func TestDialCancelDuringHandshake(t *testing.T) {
	l := silentServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Dial(ctx, "ws://"+l.Addr().String(), DialOptions{DialTimeout: 30 * time.Second})
	if err == nil {
		t.Fatal("expected error after context cancel")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dial blocked for %v despite cancel", elapsed)
	}
}
