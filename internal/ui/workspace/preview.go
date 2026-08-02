package workspace

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"tracto/internal/ui/settings"
	"tracto/pkg/syntax"
	"tracto/internal/utils"
)

const previewBatchSize = 8 * 1024 * 1024
const jsonPreviewBatchSize = 8 * 1024 * 1024

var previewBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, previewBatchSize)
		return &b
	},
}

func getPreviewBuf(size int64) ([]byte, func()) {
	if size <= previewBatchSize {
		bp := previewBufPool.Get().(*[]byte)
		buf := (*bp)[:size]
		return buf, func() { previewBufPool.Put(bp) }
	}
	buf := make([]byte, size)
	return buf, func() {}
}

func looksLikeJSON(data []byte) bool {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}

func loadPreviewFromFile(path string, totalSize int64, state *JSONFormatterState, contentType string) (string, int64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, false
	}
	defer func() { _ = f.Close() }()

	var probe [64]byte
	pn, _ := f.Read(probe[:])
	isJSON := settings.AutoFormatJSON &&
		(looksLikeJSON(probe[:pn]) || syntax.Detect(contentType, nil) == syntax.LangJSON)

	batchSize := int64(previewBatchSize)
	if isJSON {
		batchSize = int64(jsonPreviewBatchSize)
	}
	readSize := totalSize
	if readSize > batchSize {
		readSize = batchSize
	}

	_, _ = f.Seek(0, io.SeekStart)
	data, release := getPreviewBuf(readSize)
	n, _ := io.ReadFull(f, data)
	data = data[:n]

	decoded := utils.DecodeBody(data, contentType)

	var result string
	if isJSON {
		result = formatJSON(decoded, state)
	} else {
		result = utils.SanitizeBytes(decoded)
	}
	release()
	return result, int64(n), isJSON
}

func (t *RequestTab) loadMorePreview() {
	loaded := t.previewLoaded.Load()
	if t.respFile == "" || loaded >= t.respSize {
		return
	}
	if !t.previewLoading.CompareAndSwap(false, true) {
		return
	}

	filePath := t.respFile
	offset := loaded
	batchLimit := int64(previewBatchSize)
	if t.respIsJSON {
		batchLimit = int64(jsonPreviewBatchSize)
	}
	readSize := t.respSize - loaded
	if readSize > batchLimit {
		readSize = batchLimit
	}
	win := t.window
	isJSON := t.respIsJSON
	contentType := t.respContentType
	reqID := t.requestID.Load()

	go func() {
		defer t.previewLoading.Store(false)
		f, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		_, _ = f.Seek(offset, io.SeekStart)

		data, release := getPreviewBuf(readSize)
		n, _ := io.ReadFull(f, data)
		data = data[:n]

		decoded := utils.DecodeBody(data, contentType)

		var extra string
		if isJSON {
			t.jsonStateMu.Lock()
			extra = formatJSON(decoded, t.jsonFmtState)
			t.jsonStateMu.Unlock()
		} else {
			extra = utils.SanitizeBytes(decoded)
		}
		release()
		t.previewLoaded.Add(int64(n))
		t.streamToEditor(reqID, extra, win)
	}()
}

func OpenFile(path string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", "", path).Start()
	case "darwin":
		_ = exec.Command("open", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
	}
}

func openFileInExplorer(path string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer", "/select,", filepath.ToSlash(path)).Start()
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	default:
		dir := filepath.Dir(path)
		_ = exec.Command("xdg-open", dir).Start()
	}
}
