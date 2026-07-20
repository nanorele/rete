package apptest

import (
	"image"
	"testing"

	harui "tracto/internal/ui/har"

	"github.com/nanorele/gio/io/input"
)

const harPagesDoc = `{
  "log": {
    "version": "1.2",
    "pages": [
      {"id":"page_1","title":"Home","startedDateTime":"2024-01-01T10:00:00Z"},
      {"id":"page_2","title":"About","startedDateTime":"2024-01-01T10:01:00Z"}
    ],
    "entries": [
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/a"},"response":{"status":200,"content":{"mimeType":"text/html","text":"a"}}},
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/b"},"response":{"status":200,"content":{"mimeType":"text/css","text":"b"}}},
      {"pageref":"page_2","request":{"method":"GET","url":"https://x/c"},"response":{"status":200,"content":{"mimeType":"application/json","text":"c"}}}
    ]
  }
}`

func TestHARSection_PagesTabRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harPagesDoc), "p.har", nil)
	ui.HARView.TopTab = harui.TabPages

	var r input.Router
	if d := layoutHARTwice(&r, image.Pt(1100, 620), ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("pages tab failed to render")
	}
}
