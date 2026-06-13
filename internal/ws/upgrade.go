package ws

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

var ErrNotUpgrade = errors.New("ws: request is not a websocket upgrade")

type UpgradeOptions struct {
	Subprotocols  []string
	AcceptDeflate bool
	ExtraHeaders  http.Header
}

type UpgradeResult struct {
	Conn        *Conn
	Subprotocol string
	Extensions  ExtParams
}

func IsUpgrade(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	if !tokenContains(r.Header.Get("Connection"), "upgrade") {
		return false
	}
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		return false
	}
	return true
}

func Upgrade(rwc net.Conn, br *bufio.Reader, req *http.Request, opts UpgradeOptions) (*UpgradeResult, error) {
	if !IsUpgrade(req) {
		return nil, ErrNotUpgrade
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	accept := expectedAccept(key)

	subprotocol := negotiateSubprotocol(req.Header.Get("Sec-WebSocket-Protocol"), opts.Subprotocols)

	var ext ExtParams
	if opts.AcceptDeflate {
		offered := ParseExtensions(req.Header.Get("Sec-WebSocket-Extensions"))
		if offered.Negotiated {
			ext = offered
		}
	}

	var b strings.Builder
	b.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	b.WriteString("Upgrade: websocket\r\n")
	b.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&b, "Sec-WebSocket-Accept: %s\r\n", accept)
	if subprotocol != "" {
		fmt.Fprintf(&b, "Sec-WebSocket-Protocol: %s\r\n", subprotocol)
	}
	if ext.Negotiated {
		fmt.Fprintf(&b, "Sec-WebSocket-Extensions: %s\r\n", responseExtensions(ext))
	}
	for k, vs := range opts.ExtraHeaders {
		if isHandshakeHeader(k) {
			continue
		}
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	if _, err := rwc.Write([]byte(b.String())); err != nil {
		return nil, err
	}

	c, err := NewConn(rwc, br, false, ext)
	if err != nil {
		return nil, err
	}
	return &UpgradeResult{
		Conn:        c,
		Subprotocol: subprotocol,
		Extensions:  ext,
	}, nil
}

func negotiateSubprotocol(clientHeader string, serverList []string) string {
	if clientHeader == "" || len(serverList) == 0 {
		return ""
	}
	for part := range strings.SplitSeq(clientHeader, ",") {
		p := strings.TrimSpace(part)
		for _, s := range serverList {
			if strings.EqualFold(p, s) {
				return s
			}
		}
	}
	return ""
}

func responseExtensions(ext ExtParams) string {
	parts := []string{"permessage-deflate"}
	if ext.ServerNoContextTakeover {
		parts = append(parts, "server_no_context_takeover")
	}
	if ext.ClientNoContextTakeover {
		parts = append(parts, "client_no_context_takeover")
	}
	return strings.Join(parts, "; ")
}
