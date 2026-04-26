package ui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
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

func (t *RequestTab) cleanupRespFile() {
	if t.respFile != "" {
		os.Remove(t.respFile)
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

func (t *RequestTab) prepareRequest(parent context.Context, env map[string]string) (*http.Request, context.Context, context.CancelFunc, error) {
	urlRaw := strings.ReplaceAll(t.URLInput.Text(), "\n", "")
	urlRaw = strings.TrimSpace(utils.SanitizeText(urlRaw))
	url := processTemplate(urlRaw, env)

	if url == "" {
		return nil, nil, nil, errors.New("empty URL")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	reqBody := bodyReplacer.Replace(t.ReqEditor.Text())
	reqBody = processTemplate(reqBody, env)
	strippedBody := utils.StripJSONComments(reqBody)
	if json.Valid([]byte(strippedBody)) {
		reqBody = strippedBody
	}

	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	req, err := http.NewRequestWithContext(ctx, t.Method, url, strings.NewReader(reqBody))
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
	t.requestID++
	t.drainAppendChan()
	select {
	case <-t.responseChan:
	default:
	}
	select {
	case <-t.previewChan:
	default:
	}
	t.previewLoading = false
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
	// Do NOT race the send against the per-request context. When the
	// user clicks Cancel the context is already Done by the time the
	// goroutine reaches this point to post its "Cancelled" status;
	// selecting between `responseChan <- resp` (ready — buffer just
	// drained) and `<-ctx.Done()` (also ready — already closed) is a
	// coin flip, so half the time the final status never reaches the
	// UI and `isRequesting` stays true forever, leaving the Cancel
	// button stuck on screen.
	//
	// Capacity is 1, so after the drain the buffered send is
	// immediate. The `default` on the send is a defensive guard in
	// case another goroutine refills the channel between our drain
	// and our send — we'd rather report "delivery failed" than block
	// here than hold up the caller's cleanup code.
	select {
	case <-t.responseChan:
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
	reqID := t.requestID

	go func() {
		var tmpPath string
		cleanup := true
		defer func() {
			if cleanup && tmpPath != "" {
				os.Remove(tmpPath)
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
		defer resp.Body.Close()

		tmpFile, err := os.CreateTemp("", "tracto-resp-*.tmp")
		if err != nil {
			t.sendResponse(ctx, tabResponse{requestID: reqID, status: "Error: " + err.Error()})
			return
		}
		tmpPath = tmpFile.Name()

		total, sErr := t.streamResponse(ctx, resp.Body, tmpFile, win, true)
		tmpFile.Close()

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
		dest.Close()
		win.Invalidate()
		return
	}
	t.cancelFn = cancel
	reqID := t.requestID

	go func() {
		var tmpPath string
		cleanup := true
		defer func() {
			if cleanup && tmpPath != "" {
				os.Remove(tmpPath)
			}
			dest.Close()
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
		defer resp.Body.Close()

		tmpFile, tmpErr := os.CreateTemp("", "tracto-resp-*.tmp")
		var writer io.Writer = dest
		if tmpErr == nil {
			writer = io.MultiWriter(dest, tmpFile)
		}

		total, sErr := t.streamResponse(ctx, resp.Body, writer, win, false)

		if tmpFile != nil {
			tmpFile.Close()
			if sErr == nil {
				tmpPath = tmpFile.Name()
			} else {
				os.Remove(tmpFile.Name())
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
	if t.respFile == "" || t.respSize == 0 || t.previewLoading {
		return
	}
	t.PreviewEnabled = true
	t.previewLoading = true
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
