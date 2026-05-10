package ui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tracto/internal/utils"

	"github.com/nanorele/gio/app"
)

func (t *RequestTab) cancelRequest() {
	if t.cancelFn != nil {
		t.cancelFn()
		t.cancelFn = nil
	}
}

func cleanupOrphanRespTmp() {
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
	t.fileSaveMu.Lock()
	select {
	case w := <-t.FileSaveChan:
		if w != nil {
			_ = w.Close()
		}
	default:
	}
	t.fileSaveMu.Unlock()

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

func (t *RequestTab) markClosed() {
	t.fileSaveMu.Lock()
	t.closed.Store(true)
	t.fileSaveMu.Unlock()
	t.cleanupRespFile()
}

func (t *RequestTab) buildBody(ctx context.Context, env map[string]string) (io.Reader, string, error) {
	switch t.BodyType {
	case BodyNone:
		return nil, "", nil

	case BodyURLEncoded:
		vals := url.Values{}
		for _, p := range t.URLEncoded {
			k := strings.TrimSpace(p.Key.Text())
			if k == "" {
				continue
			}
			v := processTemplate(p.Value.Text(), env)
			vals.Add(k, v)
		}
		return strings.NewReader(vals.Encode()), "application/x-www-form-urlencoded", nil

	case BodyFormData:
		type formSnap struct {
			kind     FormPartKind
			key      string
			value    string
			filePath string
		}
		snap := make([]formSnap, 0, len(t.FormParts))
		for _, p := range t.FormParts {
			snap = append(snap, formSnap{
				kind:     p.Kind,
				key:      strings.TrimSpace(p.Key.Text()),
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
				if p.kind == FormPartFile {
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

	case BodyBinary:
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
	if currentTrimTrailingWS {
		reqBody = trimTrailingWhitespace(reqBody)
	}
	if currentStripJSONComments {
		strippedBody := utils.StripJSONComments(reqBody)
		if json.Valid([]byte(strippedBody)) {
			reqBody = strippedBody
		}
	}
	if currentAutoFormatJSONRequest && json.Valid([]byte(reqBody)) {
		var v interface{}
		if err := json.Unmarshal([]byte(reqBody), &v); err == nil {
			indent := currentJSONIndent
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
	urlRaw = strings.TrimSpace(utils.SanitizeText(urlRaw))
	rawURL := processTemplate(urlRaw, env)

	if rawURL == "" {
		return nil, nil, nil, errors.New("empty URL")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	rawURL = strings.ReplaceAll(rawURL, " ", "%20")
	if parsed, perr := url.Parse(rawURL); perr == nil {
		parsed.RawQuery = parsed.Query().Encode()
		rawURL = parsed.String()
	}

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

	t.updateSystemHeaders()
	for _, h := range t.Headers {
		k := strings.TrimSpace(processTemplate(h.Key.Text(), env))
		v := strings.TrimSpace(processTemplate(h.Value.Text(), env))
		if k != "" {
			req.Header.Add(k, v)
		}
	}
	for _, dh := range currentDefaultHeaders {
		k := strings.TrimSpace(dh.Key)
		if k == "" || req.Header.Get(k) != "" {
			continue
		}
		req.Header.Set(k, processTemplate(dh.Value, env))
	}
	if explicitContentType != "" {
		req.Header.Set("Content-Type", explicitContentType)
	}
	if ae := strings.TrimSpace(currentAcceptEncoding); ae != "" && req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", ae)
	}
	if currentSendConnClose {
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

func (t *RequestTab) streamToEditor(text string, win *app.Window) {
	t.appendChan <- text
	win.Invalidate()
}

func (t *RequestTab) beginRequest() {
	t.cancelRequest()
	t.requestID.Add(1)
	t.drainAppendChan()
	select {
	case <-t.responseChan:
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
	t.downloadedBytes.Store(0)
	t.cleanupRespFile()
	t.PreviewEnabled = true
	t.SaveToFilePath = ""
	t.previewLoaded = 0
}

func (t *RequestTab) sendResponse(_ context.Context, resp tabResponse) bool {
	t.respMu.Lock()
	defer t.respMu.Unlock()
	if resp.requestID != t.requestID.Load() {
		return false
	}
	if t.closed.Load() {
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

func (t *RequestTab) streamResponse(ctx context.Context, body io.Reader, dest io.Writer, win *app.Window, livePreview bool) (int64, error) {
	bufp := streamBufPool.Get().(*[]byte)
	buf := *bufp
	defer streamBufPool.Put(bufp)
	var total int64
	var previewSent int64
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
				chunk := utils.SanitizeBytes(buf[:sendN])
				select {
				case t.appendChan <- chunk:
				default:
				}
				previewSent += sendN
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
	return total, nil
}

func (t *RequestTab) executeRequest(parent context.Context, win *app.Window, env map[string]string) {
	t.beginRequest()

	req, ctx, cancel, err := t.prepareRequest(parent, env)
	if err != nil {
		t.Status = "Error: " + err.Error()
		t.isRequesting = false
		win.Invalidate()
		return
	}
	t.cancelFn = cancel
	reqID := t.requestID.Load()

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
		resp, err := httpClient.Do(req)
		if err != nil {
			status := "Error: " + err.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		tmpFile, err := os.CreateTemp("", "tracto-resp-*.tmp")
		if err != nil {
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: "Error: " + err.Error()})
			return
		}
		tmpPath = tmpFile.Name()

		total, sErr := t.streamResponse(ctx, resp.Body, tmpFile, win, true)
		_ = tmpFile.Close()

		if sErr != nil {
			status := "Error: " + sErr.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}

		duration := time.Since(start)
		display, loaded, isJSON := loadPreviewFromFile(tmpPath, total, t.jsonFmtState)
		statusText := resp.Status + "  " + duration.Round(time.Millisecond).String() + "  " + formatSize(total)

		if t.sendResponse(ctx, tabResponse{
			requestID:     reqID,
			status:        statusText,
			body:          display,
			respSize:      total,
			respFile:      tmpPath,
			previewLoaded: loaded,
			isJSON:        isJSON,
		}) {
			cleanup = false
		}
	}()
}

func (t *RequestTab) executeRequestToFile(parent context.Context, win *app.Window, env map[string]string, dest io.WriteCloser) {
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
		resp, err := httpClient.Do(req)
		if err != nil {
			status := "Error: " + err.Error()
			if ctx.Err() == context.Canceled {
				status = "Cancelled"
			}
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: status})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		tmpFile, tmpErr := os.CreateTemp("", "tracto-resp-*.tmp")
		var writer io.Writer = dest
		if tmpErr == nil {
			writer = io.MultiWriter(dest, tmpFile)
		}

		total, sErr := t.streamResponse(ctx, resp.Body, writer, win, false)

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
		if t.sendResponse(ctx, tabResponse{
			requestID: reqID,
			status:    statusText,
			respSize:  total,
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

	go func() {
		display, loaded, isJSON := loadPreviewFromFile(filePath, totalSize, state)
		select {
		case <-t.previewChan:
		default:
		}
		t.previewChan <- previewResult{body: display, previewLoaded: loaded, isJSON: isJSON}
		if win != nil {
			win.Invalidate()
		}
	}()
}
