package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/utils"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type runnerMode int

const (
	runByIterations runnerMode = iota
	runByDuration
)

const runnerMaxDrainBytes = 32 << 20

type RunVariable struct {
	Name   widget.Editor
	Values widget.Editor
	DelBtn widget.Clickable
}

type statusBucket struct {
	code     int
	count    int64
	totalLat int64
	minLat   int64
	maxLat   int64
}

type RequestRunner struct {
	Mode        runnerMode
	ModeIterBtn widget.Clickable
	ModeTimeBtn widget.Clickable
	IterEditor  widget.Editor
	DurEditor   widget.Editor
	DelayEditor widget.Editor
	WorkEditor  widget.Editor
	Variables   []*RunVariable
	AddVarBtn   widget.Clickable
	ConfigList  widget.List
	StatsList   widget.List
	SortCol     int
	SortAsc     bool
	SortBtns    []widget.Clickable

	started  bool
	plannedN int
	running  atomic.Bool
	cancel   context.CancelFunc
	sent     atomic.Int64
	inFlight atomic.Int64

	mu        sync.Mutex
	startedAt time.Time
	endedAt   time.Time
	completed int64
	success   int64
	failed    int64
	sumLat    int64
	minLat    int64
	maxLat    int64
	buckets   map[int]*statusBucket
	lat       []int64

	pcCount int64
	p50     int64
	p90     int64
	p99     int64
}

func newRequestRunner() *RequestRunner {
	r := &RequestRunner{Mode: runByIterations, buckets: map[int]*statusBucket{}}
	r.IterEditor.SingleLine = true
	r.IterEditor.SetText("100")
	r.DurEditor.SingleLine = true
	r.DurEditor.SetText("10")
	r.DelayEditor.SingleLine = true
	r.DelayEditor.SetText("0")
	r.WorkEditor.SingleLine = true
	r.WorkEditor.SetText("4")
	r.ConfigList.Axis = layout.Vertical
	r.StatsList.Axis = layout.Vertical
	r.SortCol = 1
	r.SortAsc = false
	r.SortBtns = make([]widget.Clickable, len(runColumns))
	return r
}

func (t *RequestTab) EnsureRun() *RequestRunner {
	if t.Run == nil {
		t.Run = newRequestRunner()
	}
	return t.Run
}

func (t *RequestTab) runnerStatusText() string {
	r := t.EnsureRun()
	if !r.started {
		return "Multiple · ready to start"
	}
	snap := r.snapshot()
	if r.running.Load() {
		if r.plannedN > 0 {
			return fmt.Sprintf("Multiple · running %d/%d", snap.completed, r.plannedN)
		}
		return fmt.Sprintf("Multiple · running · %d done", snap.completed)
	}
	if r.endedAtStopped() {
		return fmt.Sprintf("Multiple · stopped at %d", snap.completed)
	}
	return fmt.Sprintf("Multiple · finished %d in %s", snap.completed, snap.elapsed.Round(time.Millisecond))
}

func runToggleSized(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, on bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := theme.BgField
		if on {
			bg = theme.VarFound
		}
		pointer.CursorPointer.Add(gtx.Ops)
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), label)
			lbl.Color = th.Fg
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
		call := macro.Stop()
		rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(3)))
		paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
		widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
		call.Add(gtx.Ops)
		return dims
	})
}

func (r *RequestRunner) addVar() {
	v := &RunVariable{}
	v.Name.SingleLine = true
	v.Values.SingleLine = true
	r.Variables = append(r.Variables, v)
}

func atoiDefault(s string, def, min int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < min {
		return def
	}
	return n
}

type runVarSnapshot struct {
	name string
	vals []string
}

func (r *RequestRunner) snapshotVariables() []runVarSnapshot {
	out := make([]runVarSnapshot, 0, len(r.Variables))
	for _, rv := range r.Variables {
		name := strings.TrimSpace(rv.Name.Text())
		if name == "" {
			continue
		}
		vals := splitValues(rv.Values.Text())
		if len(vals) == 0 {
			continue
		}
		out = append(out, runVarSnapshot{name: name, vals: vals})
	}
	return out
}

func envForIteration(base map[string]string, vars []runVarSnapshot, idx int) map[string]string {
	if len(vars) == 0 {
		return base
	}
	env := make(map[string]string, len(base)+len(vars))
	for k, v := range base {
		env[k] = v
	}
	for _, rv := range vars {
		env[rv.name] = rv.vals[idx%len(rv.vals)]
	}
	return env
}

func splitValues(s string) []string {
	out := make([]string, 0, 4)
	for _, field := range strings.Split(s, ",") {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

type runSpec struct {
	method      string
	urlTmpl     string
	headers     [][2]string
	useTmplBody bool
	bodyTmpl    string
	bodyBytes   []byte
	explicitCT  string
}

func (t *RequestTab) buildRunSpec(ctx context.Context, env map[string]string) (*runSpec, error) {
	t.UpdateSystemHeaders()
	s := &runSpec{method: t.httpMethod()}
	urlRaw := strings.ReplaceAll(t.URLInput.Text(), "\n", "")
	urlRaw = strings.ReplaceAll(urlRaw, "\t", "")
	s.urlTmpl = strings.TrimSpace(utils.SanitizeText(urlRaw))
	for _, h := range t.Headers {
		if strings.TrimSpace(h.Key.Text()) == "" {
			continue
		}
		s.headers = append(s.headers, [2]string{h.Key.Text(), h.Value.Text()})
	}
	if t.Method == MethodGraphQL {
		reader, ct, err := t.buildBody(ctx, env)
		if err != nil {
			return nil, err
		}
		if reader != nil {
			b, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}
			s.bodyBytes = b
		}
		s.explicitCT = ct
		return s, nil
	}
	switch t.BodyType {
	case model.BodyNone:
	case model.BodyRaw:
		s.useTmplBody = true
		s.bodyTmpl = t.ReqEditor.Text()
	default:
		reader, ct, err := t.buildBody(ctx, env)
		if err != nil {
			return nil, err
		}
		if reader != nil {
			b, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}
			s.bodyBytes = b
		}
		s.explicitCT = ct
	}
	return s, nil
}

func (s *runSpec) newRequest(ctx context.Context, env map[string]string) (*http.Request, error) {
	raw := processTemplate(s.urlTmpl, env)
	if raw == "" {
		return nil, errors.New("empty URL")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw
	}
	raw = strings.ReplaceAll(raw, " ", "%20")

	var body io.Reader
	if s.useTmplBody {
		if s.bodyTmpl != "" {
			body = strings.NewReader(processTemplate(s.bodyTmpl, env))
		}
	} else if s.bodyBytes != nil {
		body = bytes.NewReader(s.bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, s.method, raw, body)
	if err != nil {
		return nil, err
	}
	for _, h := range s.headers {
		k := strings.TrimSpace(processTemplate(h[0], env))
		if k == "" {
			continue
		}
		req.Header.Add(k, strings.TrimSpace(processTemplate(h[1], env)))
	}
	for _, dh := range settings.DefaultHeaders {
		k := strings.TrimSpace(dh.Key)
		if k == "" || req.Header.Get(k) != "" {
			continue
		}
		req.Header.Set(k, processTemplate(dh.Value, env))
	}
	if s.explicitCT != "" {
		req.Header.Set("Content-Type", s.explicitCT)
	}
	return req, nil
}

func runOnceSpec(ctx context.Context, s *runSpec, env map[string]string) (int, time.Duration, bool) {
	start := time.Now()
	req, err := s.newRequest(ctx, env)
	if err != nil {
		return 0, time.Since(start), false
	}
	resp, err := settings.HTTPClient.Do(req)
	if err != nil {
		return 0, time.Since(start), false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, runnerMaxDrainBytes))
	_ = resp.Body.Close()
	lat := time.Since(start)
	return resp.StatusCode, lat, resp.StatusCode >= 200 && resp.StatusCode < 400
}

func (r *RequestRunner) resetCounters() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completed = 0
	r.success = 0
	r.failed = 0
	r.sumLat = 0
	r.minLat = 0
	r.maxLat = 0
	r.buckets = map[int]*statusBucket{}
	r.lat = r.lat[:0]
	r.pcCount = 0
	r.p50, r.p90, r.p99 = 0, 0, 0
	r.sent.Store(0)
	r.inFlight.Store(0)
}

func (r *RequestRunner) record(code int, lat time.Duration, ok bool) {
	ln := lat.Nanoseconds()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completed++
	r.sumLat += ln
	if r.minLat == 0 || ln < r.minLat {
		r.minLat = ln
	}
	if ln > r.maxLat {
		r.maxLat = ln
	}
	if ok {
		r.success++
	} else {
		r.failed++
	}
	b := r.buckets[code]
	if b == nil {
		b = &statusBucket{code: code, minLat: ln}
		r.buckets[code] = b
	}
	b.count++
	b.totalLat += ln
	if ln < b.minLat {
		b.minLat = ln
	}
	if ln > b.maxLat {
		b.maxLat = ln
	}
	if len(r.lat) < 50000 {
		r.lat = append(r.lat, ln)
	}
}

func (t *RequestTab) runnerSendLabel() (string, color.NRGBA) {
	r := t.EnsureRun()
	if r.running.Load() {
		return "STOP", theme.VarMissing
	}
	if r.started {
		return "RERUN", theme.VarFound
	}
	return "START", theme.VarFound
}

func (t *RequestTab) RunnerAction(parent context.Context, win *app.Window, baseEnv map[string]string) {
	r := t.EnsureRun()
	if r.running.Load() {
		r.stop()
		return
	}
	if r.started {
		r.backToConfig()
		win.Invalidate()
		return
	}
	t.StartRun(parent, win, baseEnv)
}

func (t *RequestTab) StartRun(parent context.Context, win *app.Window, baseEnv map[string]string) {
	r := t.EnsureRun()
	if r.running.Load() {
		return
	}
	spec, err := t.buildRunSpec(parent, baseEnv)
	if err != nil {
		return
	}

	workers := atoiDefault(r.WorkEditor.Text(), 4, 1)
	delay := time.Duration(atoiDefault(r.DelayEditor.Text(), 0, 0)) * time.Millisecond
	iters := atoiDefault(r.IterEditor.Text(), 100, 1)
	durSec := atoiDefault(r.DurEditor.Text(), 10, 1)
	mode := r.Mode

	vars := r.snapshotVariables()

	r.resetCounters()
	r.started = true
	if mode == runByIterations {
		r.plannedN = iters
	} else {
		r.plannedN = 0
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if mode == runByDuration {
		ctx, cancel = context.WithTimeout(parent, time.Duration(durSec)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	r.cancel = cancel
	r.mu.Lock()
	r.startedAt = time.Now()
	r.endedAt = time.Time{}
	r.mu.Unlock()
	r.running.Store(true)

	go func() {
		tk := time.NewTicker(100 * time.Millisecond)
		defer tk.Stop()
		for r.running.Load() {
			<-tk.C
			win.Invalidate()
		}
		win.Invalidate()
	}()

	go func() {
		defer func() {
			r.running.Store(false)
			r.mu.Lock()
			r.endedAt = time.Now()
			r.mu.Unlock()
			cancel()
			win.Invalidate()
		}()

		work := make(chan int)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range work {
					if ctx.Err() != nil {
						return
					}
					env := envForIteration(baseEnv, vars, idx)
					r.sent.Add(1)
					r.inFlight.Add(1)
					code, lat, ok := runOnceSpec(ctx, spec, env)
					r.inFlight.Add(-1)
					if ctx.Err() != nil && !ok {
						return
					}
					r.record(code, lat, ok)
					if delay > 0 {
						select {
						case <-time.After(delay):
						case <-ctx.Done():
							return
						}
					}
				}
			}()
		}

		feed := func(i int) bool {
			select {
			case work <- i:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if mode == runByIterations {
			for i := 0; i < iters; i++ {
				if !feed(i) {
					break
				}
			}
		} else {
			for i := 0; ; i++ {
				if !feed(i) {
					break
				}
			}
		}
		close(work)
		wg.Wait()
	}()
}

func (r *RequestRunner) stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *RequestRunner) backToConfig() {
	if r.running.Load() {
		return
	}
	r.started = false
}

type runSnapshot struct {
	completed int64
	success   int64
	failed    int64
	sumLat    int64
	minLat    int64
	maxLat    int64
	p50       int64
	p90       int64
	p99       int64
	elapsed   time.Duration
	buckets   []statusBucket
}

func (r *RequestRunner) snapshot() runSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completed != r.pcCount && len(r.lat) > 0 {
		scratch := make([]int64, len(r.lat))
		copy(scratch, r.lat)
		sort.Slice(scratch, func(i, j int) bool { return scratch[i] < scratch[j] })
		r.p50 = percentile(scratch, 0.50)
		r.p90 = percentile(scratch, 0.90)
		r.p99 = percentile(scratch, 0.99)
		r.pcCount = r.completed
	}
	var elapsed time.Duration
	if !r.startedAt.IsZero() {
		if r.endedAt.IsZero() {
			elapsed = time.Since(r.startedAt)
		} else {
			elapsed = r.endedAt.Sub(r.startedAt)
		}
	}
	buckets := make([]statusBucket, 0, len(r.buckets))
	for _, b := range r.buckets {
		buckets = append(buckets, *b)
	}
	bucketAvg := func(b statusBucket) int64 {
		if b.count == 0 {
			return 0
		}
		return b.totalLat / b.count
	}
	less := func(a, b statusBucket) bool {
		switch r.SortCol {
		case 1, 2:
			return a.count < b.count
		case 3:
			return bucketAvg(a) < bucketAvg(b)
		case 4:
			return a.minLat < b.minLat
		case 5:
			return a.maxLat < b.maxLat
		default:
			return a.code < b.code
		}
	}
	sort.Slice(buckets, func(i, j int) bool {
		if r.SortAsc {
			return less(buckets[i], buckets[j])
		}
		return less(buckets[j], buckets[i])
	})
	return runSnapshot{
		completed: r.completed,
		success:   r.success,
		failed:    r.failed,
		sumLat:    r.sumLat,
		minLat:    r.minLat,
		maxLat:    r.maxLat,
		p50:       r.p50,
		p90:       r.p90,
		p99:       r.p99,
		elapsed:   elapsed,
		buckets:   buckets,
	}
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func fmtMs(ns int64) string {
	if ns <= 0 {
		return "0"
	}
	ms := float64(ns) / 1e6
	if ms < 10 {
		return strconv.FormatFloat(ms, 'f', 1, 64)
	}
	return strconv.Itoa(int(ms + 0.5))
}

func statusLabel(code int) string {
	if code == 0 {
		return "ERR"
	}
	return strconv.Itoa(code)
}

func cflag(c color.NRGBA) *color.NRGBA { return &c }

func failColor(n int64) *color.NRGBA {
	if n > 0 {
		return cflag(theme.Danger)
	}
	return nil
}

type exampleBaseState struct {
	valid           bool
	method          string
	lastHTTPMethod  string
	url             string
	reqBody         string
	bodyType        model.BodyType
	headers         [][2]string
	formParts       []model.ParsedFormPart
	urlEncoded      []model.ParsedKV
	binaryPath      string
	binaryFileSize  int64
	status          string
	respBody        string
	respIsJSON      bool
	respFile        string
	respSize        int64
	respContentType string
	previewLoaded   int64
	previewEnabled  bool
}

func (t *RequestTab) applyFormParts(parts []model.ParsedFormPart) {
	t.FormParts = t.FormParts[:0]
	for _, fp := range parts {
		var size int64
		if fp.Kind == model.FormPartFile && fp.FilePath != "" {
			if fi, err := os.Stat(fp.FilePath); err == nil {
				size = fi.Size()
			}
		}
		part := NewFormPart(fp.Key, fp.Value, fp.Kind, fp.FilePath, size)
		part.Disabled = fp.Disabled
		t.FormParts = append(t.FormParts, part)
	}
}

func (t *RequestTab) applyURLEncoded(parts []model.ParsedKV) {
	t.URLEncoded = t.URLEncoded[:0]
	for _, kv := range parts {
		part := NewURLEncodedPart(kv.Key, kv.Value)
		part.Disabled = kv.Disabled
		t.URLEncoded = append(t.URLEncoded, part)
	}
}

func (t *RequestTab) captureBaseState() {
	b := exampleBaseState{
		valid:           true,
		method:          t.Method,
		lastHTTPMethod:  t.LastHTTPMethod,
		url:             t.URLInput.Text(),
		reqBody:         t.ReqEditor.Text(),
		bodyType:        t.BodyType,
		status:          t.Status,
		respBody:        t.RespEditor.Text(),
		respIsJSON:      t.respIsJSON,
		respFile:        t.respFile,
		respSize:        t.respSize,
		respContentType: t.respContentType,
		previewLoaded:   t.previewLoaded.Load(),
		previewEnabled:  t.PreviewEnabled,
	}
	for _, h := range t.Headers {
		if !h.IsGenerated {
			b.headers = append(b.headers, [2]string{h.Key.Text(), h.Value.Text()})
		}
	}
	for _, p := range t.FormParts {
		b.formParts = append(b.formParts, model.ParsedFormPart{
			Key:      p.Key.Text(),
			Value:    p.Value.Text(),
			Kind:     p.Kind,
			FilePath: p.FilePath,
			Disabled: p.Disabled,
		})
	}
	for _, p := range t.URLEncoded {
		b.urlEncoded = append(b.urlEncoded, model.ParsedKV{
			Key:      p.Key.Text(),
			Value:    p.Value.Text(),
			Disabled: p.Disabled,
		})
	}
	b.binaryPath = t.BinaryFilePath
	b.binaryFileSize = t.BinaryFileSize
	t.BaseState = b
}

func (t *RequestTab) restoreBaseState(th *material.Theme) {
	b := t.BaseState
	t.Method = b.method
	t.LastHTTPMethod = b.lastHTTPMethod
	t.URLInput.SetText(b.url)
	t.ReqEditor.SetText(b.reqBody)
	t.BodyType = b.bodyType
	t.Headers = t.Headers[:0]
	for _, kv := range b.headers {
		t.AddHeader(kv[0], kv[1])
	}
	t.applyFormParts(b.formParts)
	t.applyURLEncoded(b.urlEncoded)
	t.BinaryFilePath = b.binaryPath
	t.BinaryFileSize = b.binaryFileSize
	t.UpdateSystemHeaders()
	t.Status = b.status
	t.respFile = b.respFile
	t.respSize = b.respSize
	t.respContentType = b.respContentType
	t.respIsJSON = b.respIsJSON
	t.PreviewEnabled = b.previewEnabled
	t.previewLoaded.Store(b.previewLoaded)
	t.RespEditor.SetText(b.respBody)
	t.invalidateSearchCache()
	t.dirtyCheckNeeded = true
	t.BaseState.valid = false
	if th != nil {
		th.Shaper.ResetLayoutCache()
	}
}

func (t *RequestTab) applyExample(th *material.Theme, i int) {
	if i < 0 || i >= len(t.Examples) {
		if t.BaseState.valid {
			t.restoreBaseState(th)
		}
		t.ExampleSel = -1
		return
	}
	if t.ExampleSel < 0 {
		t.captureBaseState()
	}
	ex := t.Examples[i]
	t.ExampleSel = i
	if ex.Method != "" {
		t.Method = ex.Method
		if t.Method != MethodWS {
			t.LastHTTPMethod = t.Method
		}
	}
	t.URLInput.SetText(ex.URL)
	t.ReqEditor.SetText(ex.Body)
	t.Headers = t.Headers[:0]
	for k, v := range ex.Headers {
		t.AddHeader(k, v)
	}
	t.BodyType = ex.BodyType
	t.applyFormParts(ex.FormParts)
	t.applyURLEncoded(ex.URLEncoded)
	t.BinaryFilePath = ex.BinaryPath
	t.BinaryFileSize = 0
	if ex.BinaryPath != "" {
		if fi, err := os.Stat(ex.BinaryPath); err == nil {
			t.BinaryFileSize = fi.Size()
		}
	}
	t.UpdateSystemHeaders()
	t.dirtyCheckNeeded = true
	t.showExampleResponse(th, ex)
}

func (t *RequestTab) showExampleResponse(th *material.Theme, ex model.ParsedExample) {
	t.drainAppendChan()
	t.isRequesting = false
	t.cancelFn = nil
	t.respFile = ""
	t.respContentType = ""
	t.respSize = int64(len(ex.RespBody))
	t.previewLoaded.Store(int64(len(ex.RespBody)))
	t.respIsJSON = looksLikeJSON([]byte(ex.RespBody))
	t.PreviewEnabled = true
	t.Status = exampleStatusText(ex)
	t.RespEditor.SetText(ex.RespBody)
	t.invalidateSearchCache()
	if th != nil {
		th.Shaper.ResetLayoutCache()
	}
}

func exampleStatusText(ex model.ParsedExample) string {
	status := ""
	if ex.Code != 0 {
		status = strconv.Itoa(ex.Code)
	}
	if ex.Status != "" {
		if status != "" {
			status += " "
		}
		status += ex.Status
	}
	if status == "" {
		status = "Example"
	}
	return status + "  " + formatSize(int64(len(ex.RespBody)))
}

func exampleMenuLabel(i int) string {
	return "Example #" + strconv.Itoa(i+1)
}

func (t *RequestTab) layoutExampleNameRow(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if t.RunOpen || t.ExampleSel < 0 || t.ExampleSel >= len(t.Examples) {
		return layout.Dimensions{}
	}
	name := t.Examples[t.ExampleSel].Name
	if name == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := widgets.MonoLabel(th, unit.Sp(11), "Example: "+name)
		lbl.Color = theme.Accent
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		return lbl.Layout(gtx)
	})
}

func (t *RequestTab) exampleSelLabel() string {
	if t.ExampleSel >= 0 && t.ExampleSel < len(t.Examples) {
		return exampleMenuLabel(t.ExampleSel)
	}
	return "Examples"
}

func (t *RequestTab) layoutExampleSelector(gtx layout.Context, th *material.Theme) layout.Dimensions {
	for t.ExampleBtn.Clicked(gtx) {
		t.ExampleListOpen = !t.ExampleListOpen
	}
	for len(t.ExampleChoices) < len(t.Examples)+1 {
		t.ExampleChoices = append(t.ExampleChoices, widget.Clickable{})
	}
	for t.ExampleChoices[0].Clicked(gtx) {
		t.applyExample(th, -1)
		t.ExampleListOpen = false
	}
	for i := range t.Examples {
		for t.ExampleChoices[i+1].Clicked(gtx) {
			t.applyExample(th, i)
			t.ExampleListOpen = false
		}
	}

	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !t.ExampleListOpen {
				return layout.Dimensions{}
			}
			items := make([]widgets.MenuItem, 0, len(t.Examples)+1)
			items = append(items, widgets.MenuItem{
				Label:   "Base request",
				Click:   &t.ExampleChoices[0],
				Checked: t.ExampleSel < 0,
				Mono:    true,
			})
			if len(t.Examples) == 0 {
				items = append(items, widgets.MenuItem{Label: "No examples", Mono: true, Disabled: true})
			}
			for i := range t.Examples {
				label := exampleMenuLabel(i)
				if c := t.Examples[i].Code; c != 0 {
					label += " (" + strconv.Itoa(c) + ")"
				}
				items = append(items, widgets.MenuItem{
					Label:   label,
					Click:   &t.ExampleChoices[i+1],
					Checked: t.ExampleSel == i,
					Mono:    true,
				})
			}
			anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(28)))}
			widgets.DeferMenuAt(gtx, th, &t.ExampleListOpen, anchor, 200, items)
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &t.ExampleBtn, func(gtx layout.Context) layout.Dimensions {
				bg := theme.BgField
				if t.ExampleBtn.Hovered() {
					bg = theme.BgHover
				}
				pointer.CursorPointer.Add(gtx.Ops)
				macro := op.Record(gtx.Ops)
				dim := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(11), t.exampleSelLabel())
							if t.ExampleSel < 0 {
								lbl.Color = theme.FgMuted
							} else {
								lbl.Font.Weight = font.Bold
							}
							lbl.MaxLines = 1
							lbl.Truncator = "…"
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							is := gtx.Dp(unit.Dp(12))
							gtx.Constraints.Min = image.Pt(is, is)
							gtx.Constraints.Max = gtx.Constraints.Min
							return widgets.IconDropDown.Layout(gtx, theme.FgMuted)
						}),
					)
				})
				call := macro.Stop()
				rrFill := clip.UniformRRect(image.Rectangle{Max: dim.Size}, gtx.Dp(unit.Dp(4)))
				paint.FillShape(gtx.Ops, bg, rrFill.Op(gtx.Ops))
				widgets.PaintBorder1px(gtx, dim.Size, theme.Border)
				call.Add(gtx.Ops)
				return dim
			})
		}),
	)
}

func (t *RequestTab) layoutRunModeTabs(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return runModeTab(gtx, th, &t.SingleBtn, widgets.IconPlay, "Single", !t.RunOpen)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return runModeTab(gtx, th, &t.MultipleBtn, widgets.IconBatch, "Multiple", t.RunOpen)
		}),
	)
}

func runModeTab(gtx layout.Context, th *material.Theme, clk *widget.Clickable, ic *widget.Icon, label string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fg := theme.FgMuted
		if active || clk.Hovered() {
			fg = th.Fg
		}
		pointer.CursorPointer.Add(gtx.Ops)
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					is := gtx.Dp(unit.Dp(13))
					gtx.Constraints.Min = image.Pt(is, is)
					gtx.Constraints.Max = gtx.Constraints.Min
					return ic.Layout(gtx, fg)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), label)
					lbl.Color = fg
					lbl.Font.Weight = font.Normal
					return lbl.Layout(gtx)
				}),
			)
		})
		call := macro.Stop()
		call.Add(gtx.Ops)
		if active {
			h := gtx.Dp(unit.Dp(2))
			paint.FillShape(gtx.Ops, th.Fg, clip.Rect{Min: image.Pt(0, dims.Size.Y-h), Max: image.Pt(dims.Size.X, dims.Size.Y)}.Op())
		}
		return dims
	})
}

func (t *RequestTab) layoutRunner(gtx layout.Context, th *material.Theme, win *app.Window) layout.Dimensions {
	r := t.EnsureRun()
	bdr := gtx.Dp(unit.Dp(1))
	rsz := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: rsz}.Op())
	inner := image.Rect(bdr, 0, rsz.X-bdr, rsz.Y-bdr)
	paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(inner).Op())
	op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
	gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
	gtx.Constraints.Max = gtx.Constraints.Min
	if r.started {
		return r.layoutStats(gtx, th, win)
	}
	return r.layoutConfig(gtx, th)
}

func runFieldLabelW(gtx layout.Context) int { return gtx.Dp(unit.Dp(96)) }

func (r *RequestRunner) numRow(th *material.Theme, ed *widget.Editor, label, hint string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = runFieldLabelW(gtx)
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				lbl := material.Label(th, unit.Sp(11), label)
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(unit.Dp(26))
				gtx.Constraints.Min.Y = h
				gtx.Constraints.Max.Y = h
				return widgets.TextField(gtx, th, ed, hint, true, nil, 0, unit.Sp(11))
			}),
		)
	}
}

func (r *RequestRunner) layoutConfig(gtx layout.Context, th *material.Theme) layout.Dimensions {
	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(10), "Repeat this request and collect statistics.")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = runFieldLabelW(gtx)
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					lbl := material.Label(th, unit.Sp(11), "Limit by")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return runToggleSized(gtx, th, &r.ModeIterBtn, "Iterations", r.Mode == runByIterations)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return runToggleSized(gtx, th, &r.ModeTimeBtn, "Duration", r.Mode == runByDuration)
				}),
			)
		},
	}

	if r.Mode == runByIterations {
		rows = append(rows, r.numRow(th, &r.IterEditor, "Iterations", "100"))
	} else {
		rows = append(rows, r.numRow(th, &r.DurEditor, "Seconds", "10"))
	}
	rows = append(rows,
		r.numRow(th, &r.WorkEditor, "Workers", "4"),
		r.numRow(th, &r.DelayEditor, "Delay ms", "0"),
		func(gtx layout.Context) layout.Dimensions { return wsHLine(gtx) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), "Variables {{ }}")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widgets.SquareBtn(gtx, &r.AddVarBtn, widgets.IconAdd, th)
				}),
			)
		},
	)

	if len(r.Variables) == 0 {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(10), "No variants — each iteration uses the active environment.")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		})
	}
	for i := range r.Variables {
		v := r.Variables[i]
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return r.varRow(gtx, th, v)
		})
	}

	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), "Load runner")
				lbl.Font.Weight = font.Bold
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(th, &r.ConfigList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, rows[i])
				})
			}),
		)
	})
}

func (r *RequestRunner) varRow(gtx layout.Context, th *material.Theme, v *RunVariable) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(0.4, func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(unit.Dp(26))
			gtx.Constraints.Min.Y = h
			gtx.Constraints.Max.Y = h
			return widgets.TextField(gtx, th, &v.Name, "name", true, nil, 0, unit.Sp(11))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Flexed(0.6, func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(unit.Dp(26))
			gtx.Constraints.Min.Y = h
			gtx.Constraints.Max.Y = h
			return widgets.TextField(gtx, th, &v.Values, "value1, value2, value3", true, nil, 0, unit.Sp(11))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.SquareBtn(gtx, &v.DelBtn, widgets.IconDel, th)
		}),
	)
}

func (r *RequestRunner) layoutStats(gtx layout.Context, th *material.Theme, win *app.Window) layout.Dimensions {
	snap := r.snapshot()
	running := r.running.Load()

	rps := 0.0
	if snap.elapsed > 0 {
		rps = float64(snap.completed) / snap.elapsed.Seconds()
	}
	avg := int64(0)
	if snap.completed > 0 {
		avg = snap.sumLat / snap.completed
	}

	cell := func(label, value string, col *color.NRGBA) layout.FlexChild {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Label(th, unit.Sp(9), label)
					l.Color = theme.FgMuted
					l.MaxLines = 1
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					v := material.Label(th, unit.Sp(15), value)
					v.Font.Weight = font.Bold
					if col != nil {
						v.Color = *col
					}
					v.MaxLines = 1
					return v.Layout(gtx)
				}),
			)
		})
	}

	header := func(gtx layout.Context) layout.Dimensions {
		statusTxt := "Running…"
		statusCol := theme.Accent
		if !running {
			if r.endedAtStopped() {
				statusTxt = "Stopped"
				statusCol = theme.FgMuted
			} else {
				statusTxt = "Finished"
				statusCol = theme.VarFound
			}
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), statusTxt)
				lbl.Font.Weight = font.Bold
				lbl.Color = statusCol
				return lbl.Layout(gtx)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
		)
	}

	progress := func(gtx layout.Context) layout.Dimensions {
		if r.plannedN <= 0 {
			return layout.Dimensions{}
		}
		frac := float32(snap.completed) / float32(r.plannedN)
		if frac > 1 {
			frac = 1
		}
		h := gtx.Dp(unit.Dp(4))
		w := gtx.Constraints.Max.X
		paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(w, h)}.Op())
		paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Max: image.Pt(int(float32(w)*frac), h)}.Op())
		return layout.Dimensions{Size: image.Pt(w, h)}
	}

	unproc := r.sent.Load() - snap.completed
	if unproc < 0 {
		unproc = 0
	}
	unprocessed := strconv.FormatInt(unproc, 10)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(header),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(progress),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							cell("Sent", strconv.FormatInt(r.sent.Load(), 10), nil),
							cell("Done", strconv.FormatInt(snap.completed, 10), nil),
							cell("OK", strconv.FormatInt(snap.success, 10), cflag(theme.VarFound)),
							cell("Fail", strconv.FormatInt(snap.failed, 10), failColor(snap.failed)),
							cell("Unprocessed", unprocessed, nil),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							cell("Req/s", strconv.FormatFloat(rps, 'f', 1, 64), nil),
							cell("Elapsed", snap.elapsed.Round(time.Millisecond).String(), nil),
							cell("Avg ms", fmtMs(avg), nil),
							cell("Min ms", fmtMs(snap.minLat), nil),
							cell("Max ms", fmtMs(snap.maxLat), nil),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							cell("p50 ms", fmtMs(snap.p50), nil),
							cell("p90 ms", fmtMs(snap.p90), nil),
							cell("p99 ms", fmtMs(snap.p99), nil),
						)
					}),
				)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			ls := material.List(th, &r.StatsList)
			gutter := gtx.Dp(ls.Width())
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return wsHLine(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.tableHeader(gtx, th, gutter)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return wsHLine(gtx) }),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(snap.buckets) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th, unit.Sp(11), "Waiting for responses…")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						})
					}
					return ls.Layout(gtx, len(snap.buckets), func(gtx layout.Context, i int) layout.Dimensions {
						return runBucketRow(gtx, th, snap.buckets[i], snap.completed)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return wsHLine(gtx) }),
			)
		}),
	)
}

func (r *RequestRunner) endedAtStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.plannedN <= 0 {
		return false
	}
	return r.completed < int64(r.plannedN)
}

type runColumn struct {
	title string
	w     int
	align text.Alignment
}

var runColumns = []runColumn{
	{"Status", 64, text.Start},
	{"Count", 0, text.Start},
	{"Share", 70, text.End},
	{"Avg", 56, text.End},
	{"Min", 56, text.End},
	{"Max", 56, text.End},
}

func runColChild(i int, w layout.Widget) layout.FlexChild {
	if runColumns[i].w == 0 {
		return layout.Flexed(1, w)
	}
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		cw := gtx.Dp(unit.Dp(float32(runColumns[i].w)))
		gtx.Constraints.Min.X = cw
		gtx.Constraints.Max.X = cw
		return w(gtx)
	})
}

func (r *RequestRunner) tableHeader(gtx layout.Context, th *material.Theme, gutter int) layout.Dimensions {
	full := gtx.Constraints.Max.X
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(full, gtx.Dp(unit.Dp(22)))}.Op())
	gtx.Constraints.Max.X = full - gutter
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(runColumns))
		for i := range runColumns {
			i := i
			children[i] = runColChild(i, func(gtx layout.Context) layout.Dimensions {
				return r.SortBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					pointer.CursorPointer.Add(gtx.Ops)
					title := runColumns[i].title
					col := theme.FgMuted
					if r.SortCol == i {
						col = th.Fg
						if r.SortAsc {
							title += " ▲"
						} else {
							title += " ▼"
						}
					}
					lbl := material.Label(th, unit.Sp(10), title)
					lbl.Color = col
					lbl.Font.Weight = font.Bold
					lbl.Alignment = runColumns[i].align
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				})
			})
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
	return layout.Dimensions{Size: image.Pt(full, dims.Size.Y)}
}

func runBucketRow(gtx layout.Context, th *material.Theme, b statusBucket, total int64) layout.Dimensions {
	rowH := gtx.Dp(unit.Dp(22))
	gtx.Constraints.Min.Y = rowH
	share := 0.0
	if total > 0 {
		share = float64(b.count) / float64(total) * 100
	}
	avg := int64(0)
	if b.count > 0 {
		avg = b.totalLat / b.count
	}
	codeCol := theme.Fg
	switch {
	case b.code == 0:
		codeCol = theme.Danger
	case b.code >= 500:
		codeCol = theme.Danger
	case b.code >= 400:
		codeCol = theme.MethodPatch
	case b.code >= 200 && b.code < 300:
		codeCol = theme.VarFound
	}
	vals := []string{
		statusLabel(b.code),
		strconv.FormatInt(b.count, 10),
		strconv.FormatFloat(share, 'f', 1, 64) + "%",
		fmtMs(avg),
		fmtMs(b.minLat),
		fmtMs(b.maxLat),
	}
	cols := []color.NRGBA{codeCol, theme.Fg, theme.FgMuted, theme.FgMuted, theme.FgMuted, theme.FgMuted}
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(runColumns))
		for i := range runColumns {
			i := i
			children[i] = runColChild(i, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), vals[i])
				lbl.Color = cols[i]
				lbl.Alignment = runColumns[i].align
				lbl.MaxLines = 1
				if i == 0 {
					lbl.Font.Weight = font.Bold
				}
				return lbl.Layout(gtx)
			})
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}
