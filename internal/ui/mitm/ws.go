package mitm

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// interceptWebSocket relays a decrypted WebSocket connection to its upstream
// while capturing individual frames into the WSStore.
func (p *Proxy) interceptWebSocket(client *tls.Conn, br *bufio.Reader, host, port string, req *http.Request, flow *Flow) {
	defer p.markEnded(flow)

	dialCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	raw, err := p.dialUpstream(dialCtx, "tcp", host, port)
	cancel()
	if err != nil {
		p.Store.Update(func() { flow.Error = "ws dial: " + err.Error(); flow.StatusCode = 502 })
		return
	}
	upstream := tls.Client(raw, &tls.Config{
		ServerName: host,
		RootCAs:    interceptDialRoots,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	})
	if err := upstream.HandshakeContext(context.Background()); err != nil {
		_ = raw.Close()
		p.Store.Update(func() { flow.Error = "ws tls: " + err.Error(); flow.StatusCode = 502 })
		return
	}
	defer func() { _ = upstream.Close() }()

	// Relay the handshake request to the upstream.
	fmt.Fprintf(upstream, "%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI())
	fmt.Fprintf(upstream, "Host: %s\r\n", req.Host)
	_ = req.Header.Write(upstream)
	_, _ = io.WriteString(upstream, "\r\n")

	ur := bufio.NewReader(upstream)
	resp, err := http.ReadResponse(ur, req)
	if err != nil {
		p.Store.Update(func() { flow.Error = "ws handshake: " + err.Error(); flow.StatusCode = 502 })
		return
	}
	// Relay the handshake response (typically 101) back to the client.
	fmt.Fprintf(client, "HTTP/1.1 %s\r\n", resp.Status)
	_ = resp.Header.Write(client)
	_, _ = io.WriteString(client, "\r\n")

	p.Store.Update(func() {
		flow.StatusCode = resp.StatusCode
		flow.Status = resp.Status
		flow.RespHeaders = collectHeaders(resp.Header)
	})
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return
	}

	wsURL := "wss://" + host + req.URL.RequestURI()
	var wg sync.WaitGroup
	wg.Add(2)
	// client -> server
	go func() {
		defer wg.Done()
		p.pumpWS(br, upstream, flow.ID, wsURL, true)
		_ = upstream.Close()
	}()
	// server -> client
	go func() {
		defer wg.Done()
		p.pumpWS(ur, client, flow.ID, wsURL, false)
		_ = client.Close()
	}()
	wg.Wait()
	p.Store.Update(func() { flow.TunnelClosed = true })
}

// pumpWS copies WebSocket frames from r to w, logging each frame.
func (p *Proxy) pumpWS(r *bufio.Reader, w io.Writer, flowID uint64, url string, toServer bool) {
	for {
		op, payload, raw, err := readWSFrame(r)
		if err != nil {
			return
		}
		if _, werr := w.Write(raw); werr != nil {
			return
		}
		if op != 0x0 { // skip continuation for the log summary
			p.WS.Add(&WSMessage{
				FlowID:   flowID,
				URL:      url,
				ToServer: toServer,
				Opcode:   op,
				Payload:  payload,
				Time:     time.Now(),
			})
		}
		if op == 0x8 { // close
			return
		}
	}
}

// readWSFrame reads a single RFC 6455 frame, returning the opcode, the
// unmasked payload (for logging) and the exact raw bytes (for forwarding).
func readWSFrame(r *bufio.Reader) (opcode byte, payload []byte, raw []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, nil, err
	}
	raw = append(raw, hdr[0], hdr[1])
	opcode = hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7f)

	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, nil, err
		}
		raw = append(raw, ext[:]...)
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, nil, err
		}
		raw = append(raw, ext[:]...)
		length = binary.BigEndian.Uint64(ext[:])
	}

	var maskKey [4]byte
	if masked {
		if _, err = io.ReadFull(r, maskKey[:]); err != nil {
			return 0, nil, nil, err
		}
		raw = append(raw, maskKey[:]...)
	}

	const wsFrameCap = 1 << 20
	if length > wsFrameCap*8 {
		return 0, nil, nil, fmt.Errorf("ws frame too large: %d", length)
	}
	data := make([]byte, length)
	if _, err = io.ReadFull(r, data); err != nil {
		return 0, nil, nil, err
	}
	raw = append(raw, data...)

	payload = make([]byte, length)
	copy(payload, data)
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i&3]
		}
	}
	if len(payload) > wsFrameCap {
		payload = payload[:wsFrameCap]
	}
	return opcode, payload, raw, nil
}
