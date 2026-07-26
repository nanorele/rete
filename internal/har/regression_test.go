package har

import (
	"bytes"
	"testing"
)

func TestIndexFromFoldNonASCIIOffsets(t *testing.T) {
	cases := []struct {
		src  string
		from int
		sep  string
		want int
	}{
		{"AİB</SCRIPT>", 0, "</script", 4},
		{"</SCRIPT>", 0, "</script", 0},
		{"abc</Script>", 0, "</script", 3},
		{"İİİ</script>", 0, "</script", 6},
		{"no match here", 0, "</script", -1},
		{"", 0, "</script", -1},
		{"prefix</SCRIPT>", 3, "</script", 6},
	}
	for _, c := range cases {
		got := indexFromFold([]byte(c.src), c.from, []byte(c.sep))
		if got != c.want {
			t.Errorf("indexFromFold(%q, %d, %q) = %d, want %d", c.src, c.from, c.sep, got, c.want)
		}
		if got >= 0 {
			slice := string([]byte(c.src)[got:])
			if !bytes.HasPrefix(bytes.ToLower([]byte(slice)), bytes.ToLower([]byte(c.sep))) {
				t.Errorf("returned offset %d does not align with %q in %q (got %q)", got, c.sep, c.src, slice)
			}
		}
	}
}

func TestIndexFromFoldFromPastEnd(t *testing.T) {
	src := []byte("abc")
	for _, from := range []int{3, 4, 99} {
		if got := indexFromFold(src, from, []byte("a")); got != -1 {
			t.Errorf("indexFromFold(%q, %d, \"a\") = %d, want -1", src, from, got)
		}
	}
}

func TestContentTypeStripsParametersFromMimeType(t *testing.T) {
	cases := []struct {
		name string
		mime string
		want string
	}{
		{"chrome style", "text/html; charset=UTF-8", "text/html"},
		{"no space", "application/json;charset=utf-8", "application/json"},
		{"padded", "  image/png  ", "image/png"},
		{"bare semicolon", "text/plain;", "text/plain"},
		{"params only", "; charset=utf-8", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var e Entry
			e.Response.Content.MimeType = c.mime
			if got := e.ContentType(); got != c.want {
				t.Errorf("ContentType() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSummaryMergesMimeTypeAndHeaderSpellings(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
	  {"request":{"method":"GET","url":"https://x/a"},
	   "response":{"status":200,"content":{"mimeType":"text/html; charset=utf-8","text":"a"}}},
	  {"request":{"method":"GET","url":"https://x/b"},
	   "response":{"status":200,"headers":[{"name":"Content-Type","value":"text/html; charset=utf-8"}],
	               "content":{"text":"b"}}}
	]}}`
	h, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	got := h.Summary().MimeTypes
	if len(got) != 1 || got[0].Label != "text/html" || got[0].Count != 2 {
		t.Errorf("MimeTypes = %+v, want one text/html bucket of 2: charset parameters must not split buckets", got)
	}
}

func TestPrettyCodeScriptWithUnicodeNotTruncated(t *testing.T) {
	body := []byte("<div>\n<script>var s = \"İ\";</script>\n</div>")
	out, ok := PrettyCode(body, "text/html")
	if !ok {
		t.Skip("PrettyCode declined to format this input")
	}
	if !bytes.Contains(out, []byte("\";</script>")) && !bytes.Contains(out, []byte("\";")) {
		t.Errorf("script body appears truncated around the unicode char:\n%s", out)
	}
}
