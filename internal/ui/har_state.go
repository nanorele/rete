package ui

import (
	"errors"
	"os"
	"sort"
	"strings"

	"tracto/internal/har"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget"
)

var errEmptyPath = errors.New("no file path given")

const (
	harTabRequests = 0
	harTabFiles    = 1
	harTabPages    = 2
	harTabInfo     = 3
)

type harLoadResult struct {
	data []byte
	name string
	err  error
}

type harState struct {
	BrowseBtn widget.Clickable
	ClearBtn  widget.Clickable

	Doc       *har.HAR
	Resources []har.Resource
	Source    string
	Banner    string
	BannerErr bool

	TopTab   int
	TabReq   widget.Clickable
	TabFiles widget.Clickable
	TabPages widget.Clickable
	TabInfo  widget.Clickable

	PagesList widget.List
	PageRows  []*widget.Clickable
	SelPageID string
	visIdx    []int
	visKey    string
	visValid  bool

	ReqList        widget.List
	ReqRows        []*widget.Clickable
	Table          *widgets.Table
	SelReq         int
	SplitRatio     float32
	SplitDrag      gesture.Drag
	SplitDragX     float32
	InspTab        int
	InspTabReq     widget.Clickable
	InspTabResp    widget.Clickable
	ReqHdrList     widget.List
	RespHdrList    widget.List
	RunBtn         widget.Clickable
	ReqCopyBtn     widget.Clickable
	Pretty         bool
	PrettyBtn      widget.Clickable
	ReqViewer      *workspace.ResponseViewer
	ReqViewerKey   string
	ReqScrollDrag  gesture.Drag
	ReqScrollDragY float32

	FileList        widget.List
	FileRows        []*widget.Clickable
	SelFile         int
	CopyBodyBtn     widget.Clickable
	FileViewer      *workspace.ResponseViewer
	FileViewerKey   string
	FileScrollDrag  gesture.Drag
	FileScrollDragY float32

	ExportDirBtn widget.Clickable
	ExportZipBtn widget.Clickable

	InfoList widget.List

	rowCache     []harRowDisplay
	bodyCache    []byte
	bodyCacheKey string

	infoRows   []harKV
	infoCached bool

	loaded chan harLoadResult
}

func (st *harState) inspectorBody(key string, build func() []byte) []byte {
	if st.bodyCacheKey != key {
		st.bodyCacheKey = key
		st.bodyCache = build()
	}
	return st.bodyCache
}

func (st *harState) ensure() {
	if st.loaded == nil {
		st.loaded = make(chan harLoadResult, 4)
	}
	if st.ReqViewer == nil {
		st.ReqViewer = workspace.NewResponseViewer()
	}
	if st.FileViewer == nil {
		st.FileViewer = workspace.NewResponseViewer()
	}
	if st.Table == nil {
		st.Table = widgets.NewTable(harTableColumns())
	}
	if st.SplitRatio <= 0 {
		st.SplitRatio = 0.42
	}
	if st.SelReq == 0 && st.Doc == nil {
		st.SelReq = -1
	}
	if st.SelFile == 0 && len(st.Resources) == 0 {
		st.SelFile = -1
	}
	st.ReqList.Axis = layout.Vertical
	st.FileList.Axis = layout.Vertical
	st.InfoList.Axis = layout.Vertical
	st.PagesList.Axis = layout.Vertical
	st.ReqHdrList.Axis = layout.Vertical
	st.RespHdrList.Axis = layout.Vertical
}

func (st *harState) visibleIndices() []int {
	if st.Doc == nil {
		return nil
	}
	if !st.visValid || st.visKey != st.SelPageID {
		st.visKey = st.SelPageID
		st.visValid = true
		st.visIdx = st.visIdx[:0]
		for i := range st.Doc.Entries {
			if st.SelPageID == "" || st.Doc.Entries[i].PageRef == st.SelPageID {
				st.visIdx = append(st.visIdx, i)
			}
		}
	}
	return st.visIdx
}

func (st *harState) pageRequestCount(id string) int {
	if st.Doc == nil {
		return 0
	}
	c := 0
	for i := range st.Doc.Entries {
		if st.Doc.Entries[i].PageRef == id {
			c++
		}
	}
	return c
}

func (st *harState) selectPage(id string) {
	st.SelPageID = id
	st.visValid = false
	if vis := st.visibleIndices(); len(vis) > 0 {
		st.SelReq = vis[0]
	} else {
		st.SelReq = -1
	}
	st.ReqViewerKey = ""
	st.bodyCacheKey = ""
}

func (st *harState) drainLoads() bool {
	changed := false
	for {
		select {
		case r := <-st.loaded:
			st.applyLoad(r.data, r.name, r.err)
			changed = true
		default:
			return changed
		}
	}
}

func (st *harState) applyLoad(data []byte, name string, err error) {
	if err != nil {
		st.Banner = "Import failed: " + err.Error()
		st.BannerErr = true
		return
	}
	doc, perr := har.Parse(data)
	if perr != nil {
		st.Banner = "Not a valid HAR file: " + perr.Error()
		st.BannerErr = true
		return
	}
	st.Doc = doc
	st.Resources = sortedResources(doc)
	st.Source = name
	st.SelReq = -1
	st.SelFile = -1
	if len(doc.Entries) > 0 {
		st.SelReq = 0
	}
	if len(st.Resources) > 0 {
		st.SelFile = 0
	}
	st.ReqRows = nil
	st.FileRows = nil
	st.PageRows = nil
	st.SelPageID = ""
	st.visValid = false
	st.ReqViewerKey = ""
	st.FileViewerKey = ""
	st.bodyCacheKey = ""
	st.bodyCache = nil
	st.rowCache = buildRowCache(doc.Entries)
	st.infoRows = nil
	st.infoCached = false
	st.BannerErr = false
	label := name
	if label == "" {
		label = "archive"
	}
	st.Banner = "Loaded " + label + " — " + itoaN(len(doc.Entries)) + " requests, " + itoaN(len(st.Resources)) + " files"
}

func (st *harState) clear() {
	st.Doc = nil
	st.Resources = nil
	st.Source = ""
	st.SelReq = -1
	st.SelFile = -1
	st.ReqRows = nil
	st.FileRows = nil
	st.PageRows = nil
	st.SelPageID = ""
	st.visValid = false
	st.visIdx = nil
	st.ReqViewerKey = ""
	st.FileViewerKey = ""
	st.bodyCacheKey = ""
	st.bodyCache = nil
	st.rowCache = nil
	st.infoRows = nil
	st.infoCached = false
	st.Banner = ""
	st.BannerErr = false
}

func (st *harState) queueLoad(data []byte, name string, err error) {
	if st.loaded == nil {
		st.loaded = make(chan harLoadResult, 4)
	}
	select {
	case st.loaded <- harLoadResult{data: data, name: name, err: err}:
	default:
	}
}

func (st *harState) loadPathAsync(path string, invalidate func()) {
	path = strings.TrimSpace(strings.Trim(path, "\""))
	if path == "" {
		st.queueLoad(nil, "", errEmptyPath)
		if invalidate != nil {
			invalidate()
		}
		return
	}
	go func() {
		data, err := os.ReadFile(path)
		st.queueLoad(data, baseName(path), err)
		if invalidate != nil {
			invalidate()
		}
	}()
}

func sortedResources(doc *har.HAR) []har.Resource {
	res := doc.Resources(false)
	sort.SliceStable(res, func(i, j int) bool { return res[i].ZipPath < res[j].ZipPath })
	return res
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func isProbablyText(body []byte) bool {
	if len(body) == 0 {
		return true
	}
	sample := body
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	nonPrintable := 0
	for _, b := range sample {
		if b == 0 {
			return false
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			nonPrintable++
		}
	}
	return nonPrintable*100/len(sample) < 30
}

func itoaN(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
