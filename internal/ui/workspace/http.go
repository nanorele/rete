package workspace

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tracto/internal/model"
	"tracto/internal/ui/settings"
	"tracto/internal/utils"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/nanorele/gio/app"
)

type zstdReadCloser struct{ *zstd.Decoder }

func (z zstdReadCloser) Close() error {
	z.Decoder.Close()
	return nil
}

type Timings struct {
	DNS      time.Duration
	Connect  time.Duration
	TLS      time.Duration
	TTFB     time.Duration
	Transfer time.Duration
}

func attachTrace(ctx context.Context, timings *Timings, firstByteAt *time.Time) context.Context {
	var dnsStart, connStart, tlsStart time.Time
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone: func(httptrace.DNSDoneInfo) {
			if !dnsStart.IsZero() {
				timings.DNS = time.Since(dnsStart)
			}
		},
		ConnectStart: func(string, string) { connStart = time.Now() },
		ConnectDone: func(string, string, error) {
			if !connStart.IsZero() {
				timings.Connect = time.Since(connStart)
			}
		},
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			if !tlsStart.IsZero() {
				timings.TLS = time.Since(tlsStart)
			}
		},
		GotFirstResponseByte: func() { *firstByteAt = time.Now() },
	}
	return httptrace.WithClientTrace(ctx, trace)
}

func formatTimings(t Timings) string {
	parts := make([]string, 0, 5)
	add := func(label string, d time.Duration) {
		if d > 0 {
			parts = append(parts, label+" "+d.Round(time.Millisecond).String())
		}
	}
	add("DNS", t.DNS)
	add("Connect", t.Connect)
	add("TLS", t.TLS)
	add("TTFB", t.TTFB)
	add("Transfer", t.Transfer)
	return strings.Join(parts, " · ")
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for i := len(m.closers) - 1; i >= 0; i-- {
		if err := m.closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func decompressBody(resp *http.Response) io.ReadCloser {
	if resp == nil || resp.Body == nil {
		return resp.Body
	}
	if resp.Uncompressed {
		return resp.Body
	}
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return resp.Body
	}
	parts := strings.Split(enc, ",")
	var reader io.Reader = resp.Body
	closers := []io.Closer{resp.Body}
	for i := len(parts) - 1; i >= 0; i-- {
		e := strings.TrimSpace(parts[i])
		switch e {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			gz, err := gzip.NewReader(reader)
			if err != nil {
				return resp.Body
			}
			reader = gz
			closers = append(closers, gz)
		case "deflate":
			br := bufio.NewReader(reader)
			hdr, _ := br.Peek(2)
			if len(hdr) >= 2 && hdr[0] == 0x78 {
				zr, err := zlib.NewReader(br)
				if err != nil {
					return resp.Body
				}
				reader = zr
				closers = append(closers, zr)
			} else {
				fr := flate.NewReader(br)
				reader = fr
				closers = append(closers, fr)
			}
		case "br":
			reader = brotli.NewReader(reader)
		case "zstd":
			zr, err := zstd.NewReader(reader)
			if err != nil {
				return resp.Body
			}
			reader = zr
			closers = append(closers, zstdReadCloser{zr})
		default:
			return resp.Body
		}
	}
	return &multiCloser{Reader: reader, closers: closers}
}

func (t *RequestTab) CancelRequest() {
	if t.cancelFn != nil {
		t.cancelFn()
		t.cancelFn = nil
	}
}

func CleanupOrphanRespTmp() {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "tracto-resp-") || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		full := filepath.Join(dir, name)
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().Before(cutoff) {
			_ = os.Remove(full)
		}
	}
}

func (t *RequestTab) cleanupRespFile() {
	t.FileSaveMu.Lock()
	select {
	case w := <-t.FileSaveChan:
		if w != nil {
			_ = w.Close()
		}
	default:
	}
	t.FileSaveMu.Unlock()

	select {
	case resp := <-t.responseChan:
		if resp.respFile != "" {
			_ = os.Remove(resp.respFile)
		}
	default:
	}

	if t.respFile != "" {
		_ = os.Remove(t.respFile)
		t.respFile = ""
	}
	if t.reqWidthTimer != nil {
		t.reqWidthTimer.Stop()
		t.reqWidthTimer = nil
	}
	if t.respWidthTimer != nil {
		t.respWidthTimer.Stop()
		t.respWidthTimer = nil
	}
	if t.reqHeightTimer != nil {
		t.reqHeightTimer.Stop()
		t.reqHeightTimer = nil
	}
	if t.respHeightTimer != nil {
		t.respHeightTimer.Stop()
		t.respHeightTimer = nil
	}
}

func (t *RequestTab) MarkClosed() {
	t.FileSaveMu.Lock()
	t.Closed.Store(true)
	t.FileSaveMu.Unlock()
	t.cleanupRespFile()
	if t.WS != nil {
		t.WS.markClosed()
	}
	if t.Run != nil {
		t.Run.stop()
	}
}

func (t *RequestTab) buildBody(ctx context.Context, env map[string]string) (io.Reader, string, error) {
	switch t.BodyType {
	case model.BodyNone:
		return nil, "", nil

	case model.BodyURLEncoded:
		vals := url.Values{}
		for _, p := range t.URLEncoded {
			if p.Disabled {
				continue
			}
			k := strings.TrimSpace(processTemplate(p.Key.Text(), env))
			if k == "" {
				continue
			}
			v := processTemplate(p.Value.Text(), env)
			vals.Add(k, v)
		}
		return strings.NewReader(vals.Encode()), "application/x-www-form-urlencoded", nil

	case model.BodyFormData:
		type formSnap struct {
			kind     model.FormPartKind
			key      string
			value    string
			filePath string
		}
		snap := make([]formSnap, 0, len(t.FormParts))
		for _, p := range t.FormParts {
			if p.Disabled {
				continue
			}
			snap = append(snap, formSnap{
				kind:     p.Kind,
				key:      strings.TrimSpace(processTemplate(p.Key.Text(), env)),
				value:    processTemplate(p.Value.Text(), env),
				filePath: p.FilePath,
			})
		}
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		go func() {
			defer func() {
				if err := mw.Close(); err != nil {
					pw.CloseWithError(err)
					return
				}
				_ = pw.Close()
			}()
			for _, p := range snap {
				select {
				case <-ctx.Done():
					pw.CloseWithError(ctx.Err())
					return
				default:
				}
				if p.key == "" {
					continue
				}
				if p.kind == model.FormPartFile {
					if p.filePath == "" {
						continue
					}
					f, err := os.Open(p.filePath)
					if err != nil {
						pw.CloseWithError(err)
						return
					}
					w, err := mw.CreateFormFile(p.key, filepath.Base(p.filePath))
					if err != nil {
						_ = f.Close()
						pw.CloseWithError(err)
						return
					}
					if _, err := io.Copy(w, f); err != nil {
						_ = f.Close()
						pw.CloseWithError(err)
						return
					}
					_ = f.Close()
					continue
				}
				if err := mw.WriteField(p.key, p.value); err != nil {
					pw.CloseWithError(err)
					return
				}
			}
		}()
		return pr, mw.FormDataContentType(), nil

	case model.BodyBinary:
		if t.BinaryFilePath == "" {
			return nil, "", errors.New("binary body: no file selected")
		}
		f, err := os.Open(t.BinaryFilePath)
		if err != nil {
			return nil, "", err
		}
		ct := mime.TypeByExtension(filepath.Ext(t.BinaryFilePath))
		if ct == "" {
			ct = "application/octet-stream"
		}
		return f, ct, nil
	}

	reqBody := bodyReplacer.Replace(t.ReqEditor.Text())
	reqBody = processTemplate(reqBody, env)
	if settings.TrimTrailingWS {
		reqBody = trimTrailingWhitespace(reqBody)
	}
	if settings.StripJSONComments {
		strippedBody := utils.StripJSONComments(reqBody)
		if json.Valid([]byte(strippedBody)) {
			reqBody = strippedBody
		}
	}
	if settings.AutoFormatJSONRequest && json.Valid([]byte(reqBody)) {
		var v interface{}
		if err := json.Unmarshal([]byte(reqBody), &v); err == nil {
			indent := settings.JSONIndent
			if indent < 0 {
				indent = 2
			}
			pad := strings.Repeat(" ", indent)
			if formatted, err := json.MarshalIndent(v, "", pad); err == nil {
				reqBody = string(formatted)
			}
		}
	}
	return strings.NewReader(reqBody), "", nil
}

func trimTrailingWhitespace(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t\r")
	}
	return strings.Join(lines, "\n")
}

func (t *RequestTab) prepareRequest(parent context.Context, env map[string]string) (*http.Request, context.Context, context.CancelFunc, error) {
	urlRaw := strings.ReplaceAll(t.URLInput.Text(), "\n", "")
	urlRaw = strings.ReplaceAll(urlRaw, "\t", "")
	urlRaw = strings.TrimSpace(utils.SanitizeText(urlRaw))
	rawURL := processTemplate(urlRaw, env)

	if rawURL == "" {
		return nil, nil, nil, errors.New("empty URL")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	rawURL = strings.ReplaceAll(rawURL, " ", "%20")

	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)

	bodyReader, explicitContentType, buildErr := t.buildBody(ctx, env)
	if buildErr != nil {
		cancel()
		return nil, nil, nil, buildErr
	}
	req, err := http.NewRequestWithContext(ctx, t.Method, rawURL, bodyReader)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}

	t.UpdateSystemHeaders()
	for _, h := range t.Headers {
		k := strings.TrimSpace(processTemplate(h.Key.Text(), env))
		v := strings.TrimSpace(processTemplate(h.Value.Text(), env))
		if k != "" {
			req.Header.Add(k, v)
		}
	}
	for _, dh := range settings.DefaultHeaders {
		k := strings.TrimSpace(dh.Key)
		if k == "" || req.Header.Get(k) != "" {
			continue
		}
		req.Header.Set(k, processTemplate(dh.Value, env))
	}
	if explicitContentType != "" {
		req.Header.Set("Content-Type", explicitContentType)
	}
	if ae := strings.TrimSpace(settings.AcceptEncoding); ae != "" && req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", ae)
	}
	if settings.SendConnClose {
		req.Close = true
		if req.Header.Get("Connection") == "" {
			req.Header.Set("Connection", "close")
		}
	}
	return req, ctx, cancel, nil
}

func (t *RequestTab) drainAppendChan() {
	for {
		select {
		case <-t.appendChan:
		default:
			return
		}
	}
}

func (t *RequestTab) streamToEditor(reqID uint64, text string, win *app.Window) {
	t.appendChan <- appendChunk{requestID: reqID, text: text}
	win.Invalidate()
}

func (t *RequestTab) beginRequest() {
	t.CancelRequest()
	t.requestID.Add(1)
	t.drainAppendChan()
	select {
	case prev := <-t.responseChan:
		if prev.respFile != "" {
			_ = os.Remove(prev.respFile)
		}
	default:
	}
	select {
	case <-t.previewChan:
	default:
	}
	t.previewLoading.Store(false)
	t.jsonFmtState = &JSONFormatterState{}
	t.Status = "Sending..."
	t.RespEditor.SetText("")
	t.invalidateSearchCache()
	t.isRequesting = true
	t.respSize = 0
	t.respIsJSON = false
	t.respContentType = ""
	t.downloadedBytes.Store(0)
	t.cleanupRespFile()
	t.PreviewEnabled = true
	t.SaveToFilePath = ""
	t.previewLoaded.Store(0)
}

func (t *RequestTab) sendResponse(_ context.Context, resp tabResponse) bool {
	t.respMu.Lock()
	defer t.respMu.Unlock()
	if resp.requestID != t.requestID.Load() {
		return false
	}
	if t.Closed.Load() {
		return false
	}
	select {
	case prev := <-t.responseChan:
		if prev.respFile != "" && prev.respFile != resp.respFile {
			_ = os.Remove(prev.respFile)
		}
	default:
	}
	select {
	case t.responseChan <- resp:
		return true
	default:
		return false
	}
}

const maxStreamPreview = 512 * 1024

const charsetSniffWindow = 4096

func (t *RequestTab) streamResponse(ctx context.Context, reqID uint64, body io.Reader, dest io.Writer, win *app.Window, livePreview bool, contentType string) (int64, error) {
	bufp := streamBufPool.Get().(*[]byte)
	buf := *bufp
	defer streamBufPool.Put(bufp)
	var total int64
	var previewSent int64
	var previewTail []byte

	decoder := utils.CharsetDecoder(contentType)
	var decodeBuf []byte
	if decoder != nil {
		decodeBuf = make([]byte, 0, maxStreamPreview)
	}

	sniffPending := decoder == nil && utils.CharsetFromContentType(contentType) == ""
	var sniffBuf []byte

	flushUTF8 := func(data []byte, isLast bool) {
		if len(previewTail) > 0 {
			merged := make([]byte, 0, len(previewTail)+len(data))
			merged = append(merged, previewTail...)
			merged = append(merged, data...)
			data = merged
			previewTail = previewTail[:0]
		}
		end := len(data)
		if !isLast {
			for end > 0 && len(data)-end < 4 {
				r, size := utf8.DecodeLastRune(data[:end])
				if r != utf8.RuneError || size > 1 {
					break
				}
				if size == 0 {
					break
				}
				end--
			}
			if end < len(data) {
				previewTail = append(previewTail[:0], data[end:]...)
			}
		}
		chunk := utils.SanitizeBytes(data[:end])
		select {
		case t.appendChan <- appendChunk{requestID: reqID, text: chunk}:
		default:
		}
	}

	commitSniff := func(isLast bool) {
		sniffPending = false
		if len(sniffBuf) == 0 {
			return
		}
		dec := utils.CharsetDecoderForBody(sniffBuf, contentType)
		if dec != nil {
			decoder = dec
			decodeBuf = make([]byte, 0, maxStreamPreview)
			decodeBuf = append(decodeBuf, sniffBuf...)
		} else {
			flushUTF8(sniffBuf, isLast)
		}
		sniffBuf = nil
	}

	lastUpdate := time.Now()
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, wErr := dest.Write(buf[:n]); wErr != nil {
				return total, wErr
			}
			total += int64(n)
			t.downloadedBytes.Store(total)

			if livePreview && previewSent < maxStreamPreview {
				sendN := int64(n)
				if previewSent+sendN > maxStreamPreview {
					sendN = maxStreamPreview - previewSent
				}
				switch {
				case sniffPending:
					sniffBuf = append(sniffBuf, buf[:sendN]...)
					previewSent += sendN
					done := len(sniffBuf) >= charsetSniffWindow || readErr != nil || previewSent >= maxStreamPreview
					if done {
						commitSniff(previewSent >= maxStreamPreview || readErr != nil)
					}
				case decoder != nil:
					decodeBuf = append(decodeBuf, buf[:sendN]...)
					previewSent += sendN
				default:
					isLast := previewSent+sendN >= maxStreamPreview || readErr != nil
					flushUTF8(buf[:sendN], isLast)
					previewSent += sendN
				}
			}

			if time.Since(lastUpdate) > 250*time.Millisecond {
				lastUpdate = time.Now()
				win.Invalidate()
			}
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return total, ctx.Err()
			}
			break
		}
	}

	if sniffPending {
		commitSniff(true)
	}

	if decoder != nil && len(decodeBuf) > 0 {
		decoded, _ := decoder.Bytes(decodeBuf)
		chunk := utils.SanitizeBytes(decoded)
		select {
		case t.appendChan <- appendChunk{requestID: reqID, text: chunk}:
		default:
		}
	}
	return total, nil
}

func (t *RequestTab) ExecuteRequest(parent context.Context, win *app.Window, env map[string]string) {
	t.beginRequest()

	req, ctx, cancel, err := t.prepareRequest(parent, env)
	if err != nil {
		t.Status = "Error: " + err.Error()
		t.isRequesting = false
		win.Invalidate()
		return
	}
	var timings Timings
	var firstByteAt time.Time
	req = req.WithContext(attachTrace(req.Context(), &timings, &firstByteAt))
	t.cancelFn = cancel
	reqID := t.requestID.Load()
	fmtState := t.jsonFmtState

	go func() {
		var tmpPath string
		cleanup := true
		defer func() {
			if cleanup && tmpPath != "" {
				_ = os.Remove(tmpPath)
			}
			win.Invalidate()
		}()

		start := time.Now()
		resp, err := settings.HTTPClient.Do(req)
		if err != nil {
			status := "Error: " + err.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}
		if !firstByteAt.IsZero() {
			timings.TTFB = firstByteAt.Sub(start)
		}
		body := decompressBody(resp)
		defer func() { _ = body.Close() }()

		tmpFile, err := os.CreateTemp("", "tracto-resp-*.tmp")
		if err != nil {
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: "Error: " + err.Error()})
			return
		}
		tmpPath = tmpFile.Name()

		contentType := resp.Header.Get("Content-Type")
		total, sErr := t.streamResponse(ctx, reqID, body, tmpFile, win, true, contentType)
		_ = tmpFile.Close()
		if !firstByteAt.IsZero() {
			timings.Transfer = time.Since(firstByteAt)
		}

		if sErr != nil {
			status := "Error: " + sErr.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}

		duration := time.Since(start)
		display, loaded, isJSON := loadPreviewFromFile(tmpPath, total, fmtState, contentType)
		statusText := resp.Status + "  " + duration.Round(time.Millisecond).String() + "  " + formatSize(total)
		filename := utils.ParseContentDispositionFilename(resp.Header.Get("Content-Disposition"))

		if t.sendResponse(ctx, tabResponse{
			requestID:     reqID,
			status:        statusText,
			body:          display,
			respSize:      total,
			respFile:      tmpPath,
			previewLoaded: loaded,
			isJSON:        isJSON,
			contentType:   contentType,
			filename:      filename,
			timings:       timings,
		}) {
			cleanup = false
		}
	}()
}

func (t *RequestTab) ExecuteRequestToFile(parent context.Context, win *app.Window, env map[string]string, dest io.WriteCloser) {
	t.beginRequest()
	t.PreviewEnabled = false

	req, ctx, cancel, err := t.prepareRequest(parent, env)
	if err != nil {
		t.Status = "Error: " + err.Error()
		t.isRequesting = false
		_ = dest.Close()
		win.Invalidate()
		return
	}
	var timings Timings
	var firstByteAt time.Time
	req = req.WithContext(attachTrace(req.Context(), &timings, &firstByteAt))
	t.cancelFn = cancel
	reqID := t.requestID.Load()

	go func() {
		var tmpPath string
		cleanup := true
		defer func() {
			if cleanup && tmpPath != "" {
				_ = os.Remove(tmpPath)
			}
			_ = dest.Close()
			win.Invalidate()
		}()

		start := time.Now()
		resp, err := settings.HTTPClient.Do(req)
		if err != nil {
			status := "Error: " + err.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}
		if !firstByteAt.IsZero() {
			timings.TTFB = firstByteAt.Sub(start)
		}
		body := decompressBody(resp)
		defer func() { _ = body.Close() }()

		tmpFile, tmpErr := os.CreateTemp("", "tracto-resp-*.tmp")
		var writer io.Writer = dest
		if tmpErr == nil {
			writer = io.MultiWriter(dest, tmpFile)
		}

		total, sErr := t.streamResponse(ctx, reqID, body, writer, win, false, resp.Header.Get("Content-Type"))
		if !firstByteAt.IsZero() {
			timings.Transfer = time.Since(firstByteAt)
		}

		if tmpFile != nil {
			_ = tmpFile.Close()
			if sErr == nil {
				tmpPath = tmpFile.Name()
			} else {
				_ = os.Remove(tmpFile.Name())
			}
		}

		if sErr != nil {
			status := "Error: " + sErr.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}

		duration := time.Since(start)
		statusText := resp.Status + "  " + duration.Round(time.Millisecond).String() + "  " + formatSize(total) + "  Saved to file"
		filename := utils.ParseContentDispositionFilename(resp.Header.Get("Content-Disposition"))
		if t.sendResponse(ctx, tabResponse{
			requestID: reqID,
			status:    statusText,
			respSize:  total,
			filename:  filename,
			timings:   timings,
			respFile:  tmpPath,
		}) {
			cleanup = false
		}
	}()
}

func (t *RequestTab) loadPreviewForSavedFile() {
	if t.respFile == "" || t.respSize == 0 {
		return
	}
	if !t.previewLoading.CompareAndSwap(false, true) {
		return
	}
	t.PreviewEnabled = true
	t.jsonFmtState = &JSONFormatterState{}

	filePath := t.respFile
	totalSize := t.respSize
	state := t.jsonFmtState
	win := t.window
	contentType := t.respContentType
	reqID := t.requestID.Load()

	go func() {
		display, loaded, isJSON := loadPreviewFromFile(filePath, totalSize, state, contentType)
		select {
		case <-t.previewChan:
		default:
		}
		t.previewChan <- previewResult{requestID: reqID, body: display, previewLoaded: loaded, isJSON: isJSON}
		if win != nil {
			win.Invalidate()
		}
	}()
}
