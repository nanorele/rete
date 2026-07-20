package mitm

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// pairsToHeader converts ordered [key,value] pairs into an http.Header.
func pairsToHeader(pairs [][2]string) http.Header {
	h := make(http.Header, len(pairs))
	for _, kv := range pairs {
		h.Add(kv[0], kv[1])
	}
	return h
}

// serializeRequest renders an editable HTTP request text block.
func serializeRequest(method, requestURI, proto string, headers [][2]string, body []byte) []byte {
	var b bytes.Buffer
	if proto == "" {
		proto = "HTTP/1.1"
	}
	fmt.Fprintf(&b, "%s %s %s\r\n", method, requestURI, proto)
	writeHeaderPairs(&b, headers)
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}

// serializeResponse renders an editable HTTP response text block.
func serializeResponse(status, proto string, headers [][2]string, body []byte) []byte {
	var b bytes.Buffer
	if proto == "" {
		proto = "HTTP/1.1"
	}
	fmt.Fprintf(&b, "%s %s\r\n", proto, status)
	writeHeaderPairs(&b, headers)
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}

func writeHeaderPairs(b *bytes.Buffer, headers [][2]string) {
	// stable, deterministic ordering for a nicer editor experience
	sorted := append([][2]string(nil), headers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i][0] < sorted[j][0] })
	for _, h := range sorted {
		fmt.Fprintf(b, "%s: %s\r\n", h[0], h[1])
	}
}

// editedRequest is the parsed result of an edited request block.
type editedRequest struct {
	Method     string
	RequestURI string
	Headers    [][2]string
	Body       []byte
}

func parseRequestRaw(raw []byte) (editedRequest, bool) {
	var er editedRequest
	// Split first line manually, then parse headers via textproto through ReadRequest.
	r := bufio.NewReader(bytes.NewReader(normalizeCRLF(raw)))
	req, err := http.ReadRequest(r)
	if err != nil {
		return er, false
	}
	body, _ := readLimited(req.Body, maxBodyForward)
	_ = req.Body.Close()
	er.Method = req.Method
	er.RequestURI = req.RequestURI
	er.Headers = collectHeaders(req.Header)
	er.Body = body
	return er, true
}

type editedResponse struct {
	Status  string
	Headers [][2]string
	Body    []byte
}

func parseResponseRaw(raw []byte) (editedResponse, bool) {
	var er editedResponse
	r := bufio.NewReader(bytes.NewReader(normalizeCRLF(raw)))
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		return er, false
	}
	body, _ := readLimited(resp.Body, maxBodyForward)
	_ = resp.Body.Close()
	er.Status = resp.Status
	if er.Status == "" {
		er.Status = fmt.Sprintf("%d", resp.StatusCode)
	}
	er.Headers = collectHeaders(resp.Header)
	er.Body = body
	return er, true
}

// normalizeCRLF ensures header lines are CRLF-terminated for the std parsers,
// tolerating editors that emit bare LF.
func normalizeCRLF(raw []byte) []byte {
	s := string(raw)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "\r\n")
	return []byte(s)
}

// isWebSocketUpgrade reports whether the request headers request a WS upgrade.
func isWebSocketUpgrade(h http.Header) bool {
	return strings.EqualFold(h.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(h.Get("Connection")), "upgrade")
}

// processRequest applies match&replace and (if active) manual interception to
// an outgoing request. It returns possibly-edited fields and drop=true when the
// user dropped the message. When nothing is configured it is a no-op.
func (p *Proxy) processRequest(flow *Flow, method, requestURI, proto string, headers [][2]string, body []byte, inScope bool) (string, string, [][2]string, []byte, bool) {
	headers = p.MR.ApplyHeaders(MRRequest, headers)
	body = p.MR.ApplyBody(MRRequest, body)
	requestURI = p.MR.ApplyFirstLine(MRRequest, requestURI)

	if p.Manual.On() && inScope && p.IRules.ShouldIntercept(HeldRequest, flow, inScope) {
		raw := serializeRequest(method, requestURI, proto, headers, body)
		edited, drop := p.Manual.Hold(&Held{
			FlowID: flow.ID, Kind: HeldRequest,
			Method: method, URL: flow.URL, Host: flow.Host, Src: flow.Src,
			Raw: raw,
		})
		if drop {
			return method, requestURI, headers, body, true
		}
		if er, ok := parseRequestRaw(edited); ok {
			method, requestURI, headers, body = er.Method, er.RequestURI, er.Headers, er.Body
		}
	}
	return method, requestURI, headers, body, false
}

// processResponse applies match&replace and (if enabled) manual response
// interception before the response is written back to the client.
func (p *Proxy) processResponse(flow *Flow, status, proto string, headers [][2]string, body []byte, inScope bool) (string, [][2]string, []byte, bool) {
	headers = p.MR.ApplyHeaders(MRResponse, headers)
	body = p.MR.ApplyBody(MRResponse, body)
	status = p.MR.ApplyFirstLine(MRResponse, status)

	if p.Manual.On() && p.Manual.InterceptResponses() && inScope &&
		p.IRules.ShouldIntercept(HeldResponse, flow, inScope) {
		raw := serializeResponse(status, proto, headers, body)
		edited, drop := p.Manual.Hold(&Held{
			FlowID: flow.ID, Kind: HeldResponse,
			Method: flow.Method, URL: flow.URL, Host: flow.Host, Src: flow.Src,
			Raw: raw,
		})
		if drop {
			return status, headers, body, true
		}
		if er, ok := parseResponseRaw(edited); ok {
			status, headers, body = er.Status, er.Headers, er.Body
		}
	}
	return status, headers, body, false
}
