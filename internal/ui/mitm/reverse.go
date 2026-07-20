package mitm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// handleReverseHTTP serves an origin-form request that reached the proxy
// directly (reverse mode) by proxying it to the target's real upstream.
func (p *Proxy) handleReverseHTTP(c net.Conn, br *bufio.Reader, req *http.Request, tg *Target) {
	host, port, _ := splitHostPort(req.Host, "80")
	if host == "" {
		host = tg.Domain
	}

	flow := &Flow{
		Kind:         FlowHTTP,
		Src:          SrcReverse,
		TargetDomain: tg.Domain,
		ClientAddr:   c.RemoteAddr().String(),
		Scheme:       "http",
		Method:       req.Method,
		Host:         host,
		Port:         port,
		Path:         req.URL.RequestURI(),
		URL:          "http://" + host + req.URL.RequestURI(),
		Version:      req.Proto,
		ReqHeaders:   collectHeaders(req.Header),
	}
	body, _ := readLimited(req.Body, maxCaptureBody)
	_ = req.Body.Close()
	flow.ReqBody = body
	flow.ReqSize = int64(len(body))
	p.Store.Add(flow)
	defer p.markEnded(flow)

	upstreamAddr, err := p.resolveUpstream(tg, host, port)
	if err != nil {
		p.Targets.markError(tg, err.Error())
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.StatusCode = 502
			flow.Status = "502 Bad Gateway"
			flow.Ended = time.Now()
		})
		writeStatus(c, 502, "reverse upstream resolve failed: "+err.Error())
		return
	}
	p.Targets.markRequest(tg)

	inScope := p.ScopeR.InScope(flow)
	method, requestURI, reqPairs, newBody, drop := p.processRequest(
		flow, req.Method, req.URL.RequestURI(), req.Proto, collectHeaders(req.Header), body, inScope)
	if drop {
		p.Store.Update(func() { flow.Error = "dropped"; flow.Status = "dropped"; flow.Ended = time.Now() })
		writeStatus(c, 403, "Dropped by interceptor")
		return
	}
	body = newBody

	if tg.Delay > 0 {
		time.Sleep(tg.Delay)
	}

	outURL := "http://" + upstreamAddr + requestURI
	out, err := http.NewRequest(method, outURL, bytes.NewReader(body))
	if err != nil {
		p.Store.Update(func() { flow.Error = err.Error(); flow.StatusCode = 500; flow.Ended = time.Now() })
		writeStatus(c, 500, "bad reverse request: "+err.Error())
		return
	}
	out.Header = pairsToHeader(reqPairs)
	stripHopByHop(out.Header)
	out.Host = host
	out.ContentLength = int64(len(body))

	transport := &http.Transport{
		ForceAttemptHTTP2:     false,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	defer transport.CloseIdleConnections()
	cl := &http.Client{
		Transport:     transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       60 * time.Second,
	}
	resp, err := cl.Do(out)
	if err != nil {
		p.Targets.markError(tg, err.Error())
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.StatusCode = 502
			flow.Status = "502 Bad Gateway"
			flow.Ended = time.Now()
		})
		writeStatus(c, 502, "reverse upstream error: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	fullBody, _ := readLimited(resp.Body, maxBodyForward)
	respPairs := collectHeaders(resp.Header)
	status := resp.Status
	status, respPairs, fullBody, rdrop := p.processResponse(flow, status, resp.Proto, respPairs, fullBody, inScope)
	if rdrop {
		p.Store.Update(func() { flow.Error = "response dropped"; flow.Ended = time.Now() })
		return
	}

	captured := fullBody
	if int64(len(captured)) > maxCaptureBody {
		captured = captured[:maxCaptureBody]
	}
	p.Store.Update(func() {
		flow.Status = status
		flow.StatusCode = resp.StatusCode
		flow.RespHeaders = respPairs
		flow.RespBody = captured
		flow.RespSize = int64(len(fullBody))
		flow.Ended = time.Now()
	})

	outHdr := pairsToHeader(respPairs)
	stripHopByHop(outHdr)
	outHdr.Set("Content-Length", strconv.Itoa(len(fullBody)))
	_, _ = fmt.Fprintf(c, "HTTP/1.1 %s\r\n", status)
	_ = outHdr.Write(c)
	_, _ = io.WriteString(c, "\r\n")
	_, _ = c.Write(fullBody)
	_ = br
}

// resolveUpstream determines the real upstream address for a reverse target,
// bypassing the user's hosts entry (which points the domain back at us).
func (p *Proxy) resolveUpstream(tg *Target, host, port string) (string, error) {
	if tg.Upstream == UpstreamManual {
		addr := tg.UpstreamAddr
		if addr == "" {
			return "", fmt.Errorf("manual upstream address is empty")
		}
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, port)
		}
		return addr, nil
	}
	// auto: resolve the real IP via DoH so we don't loop back through the
	// user's 127.0.0.1 hosts entry.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ip := resolveDoH(ctx, host)
	if ip == "" {
		return "", fmt.Errorf("DoH resolve failed for %s (set upstream to manual)", host)
	}
	return net.JoinHostPort(ip, port), nil
}
