package har

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type HAR struct {
	Version string
	Creator Creator
	Browser Creator
	Pages   []Page
	Entries []Entry
}

type Creator struct {
	Name    string
	Version string
}

type Page struct {
	ID              string
	Title           string
	StartedDateTime string
}

type Entry struct {
	StartedDateTime   string
	Time              float64
	PageRef           string
	ServerIPAddress   string
	Request           Request
	Response          Response
	WebSocketMessages []WSMessage
}

type WSMessage struct {
	Type   string
	Time   float64
	Opcode int
	Data   string
}

func (m WSMessage) Sent() bool { return strings.EqualFold(m.Type, "send") }

func (m WSMessage) Binary() bool { return m.Opcode == 2 }

type Request struct {
	Method      string
	URL         string
	HTTPVersion string
	Headers     []Header
	QueryString []Header
	BodySize    int64
	PostData    PostData
}

type PostData struct {
	MimeType string
	Text     string
}

type Response struct {
	Status      int
	StatusText  string
	HTTPVersion string
	Headers     []Header
	Content     Content
	RedirectURL string
	BodySize    int64
}

type Content struct {
	Size     int64
	MimeType string
	Text     string
	Encoding string
}

type Header struct {
	Name  string
	Value string
}

type wire struct {
	Log struct {
		Version string `json:"version"`
		Creator struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"creator"`
		Browser struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"browser"`
		Pages []struct {
			ID              string `json:"id"`
			Title           string `json:"title"`
			StartedDateTime string `json:"startedDateTime"`
		} `json:"pages"`
		Entries []struct {
			StartedDateTime string  `json:"startedDateTime"`
			Time            float64 `json:"time"`
			PageRef         string  `json:"pageref"`
			ServerIPAddress string  `json:"serverIPAddress"`
			Request         struct {
				Method      string       `json:"method"`
				URL         string       `json:"url"`
				HTTPVersion string       `json:"httpVersion"`
				Headers     []wireHeader `json:"headers"`
				QueryString []wireHeader `json:"queryString"`
				BodySize    int64        `json:"bodySize"`
				PostData    struct {
					MimeType string `json:"mimeType"`
					Text     string `json:"text"`
				} `json:"postData"`
			} `json:"request"`
			Response struct {
				Status      int          `json:"status"`
				StatusText  string       `json:"statusText"`
				HTTPVersion string       `json:"httpVersion"`
				Headers     []wireHeader `json:"headers"`
				RedirectURL string       `json:"redirectURL"`
				BodySize    int64        `json:"bodySize"`
				Content     struct {
					Size     int64  `json:"size"`
					MimeType string `json:"mimeType"`
					Text     string `json:"text"`
					Encoding string `json:"encoding"`
				} `json:"content"`
			} `json:"response"`
			WebSocketMessages []struct {
				Type   string  `json:"type"`
				Time   float64 `json:"time"`
				Opcode int     `json:"opcode"`
				Data   string  `json:"data"`
			} `json:"_webSocketMessages"`
		} `json:"entries"`
	} `json:"log"`
}

type wireHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var ErrNotHAR = errors.New("har: missing or empty \"log\" object")

func Parse(data []byte) (*HAR, error) {
	if len(data) == 0 {
		return nil, ErrNotHAR
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("har: %w", err)
	}
	if w.Log.Version == "" && w.Log.Creator.Name == "" && len(w.Log.Entries) == 0 && len(w.Log.Pages) == 0 {
		return nil, ErrNotHAR
	}

	h := &HAR{
		Version: w.Log.Version,
		Creator: Creator{Name: w.Log.Creator.Name, Version: w.Log.Creator.Version},
		Browser: Creator{Name: w.Log.Browser.Name, Version: w.Log.Browser.Version},
	}
	for _, p := range w.Log.Pages {
		h.Pages = append(h.Pages, Page{ID: p.ID, Title: p.Title, StartedDateTime: p.StartedDateTime})
	}
	for _, e := range w.Log.Entries {
		entry := Entry{
			StartedDateTime: e.StartedDateTime,
			Time:            e.Time,
			PageRef:         e.PageRef,
			ServerIPAddress: e.ServerIPAddress,
			Request: Request{
				Method:      e.Request.Method,
				URL:         e.Request.URL,
				HTTPVersion: e.Request.HTTPVersion,
				Headers:     toHeaders(e.Request.Headers),
				QueryString: toHeaders(e.Request.QueryString),
				BodySize:    e.Request.BodySize,
				PostData:    PostData{MimeType: e.Request.PostData.MimeType, Text: e.Request.PostData.Text},
			},
			Response: Response{
				Status:      e.Response.Status,
				StatusText:  e.Response.StatusText,
				HTTPVersion: e.Response.HTTPVersion,
				Headers:     toHeaders(e.Response.Headers),
				RedirectURL: e.Response.RedirectURL,
				BodySize:    e.Response.BodySize,
				Content: Content{
					Size:     e.Response.Content.Size,
					MimeType: e.Response.Content.MimeType,
					Text:     e.Response.Content.Text,
					Encoding: e.Response.Content.Encoding,
				},
			},
		}
		for _, m := range e.WebSocketMessages {
			entry.WebSocketMessages = append(entry.WebSocketMessages, WSMessage{
				Type:   m.Type,
				Time:   m.Time,
				Opcode: m.Opcode,
				Data:   m.Data,
			})
		}
		h.Entries = append(h.Entries, entry)
	}
	return h, nil
}

func ParseReader(r io.Reader) (*HAR, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func toHeaders(in []wireHeader) []Header {
	if len(in) == 0 {
		return nil
	}
	out := make([]Header, 0, len(in))
	for _, h := range in {
		out = append(out, Header{Name: h.Name, Value: h.Value})
	}
	return out
}

func (r Response) Header(name string) string {
	for _, h := range r.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func (e Entry) ContentType() string {
	if mt := strings.TrimSpace(e.Response.Content.MimeType); mt != "" {
		return mt
	}
	if ct := e.Response.Header("Content-Type"); ct != "" {
		if i := strings.IndexByte(ct, ';'); i >= 0 {
			return strings.TrimSpace(ct[:i])
		}
		return strings.TrimSpace(ct)
	}
	return ""
}

func (e Entry) IsWebSocket() bool {
	if e.Response.Status == 101 || len(e.WebSocketMessages) > 0 {
		return true
	}
	u := strings.ToLower(e.Request.URL)
	return strings.HasPrefix(u, "ws://") || strings.HasPrefix(u, "wss://")
}

func (h *HAR) Methods() []string {
	seen := map[string]struct{}{}
	for _, e := range h.Entries {
		if e.Request.Method != "" {
			seen[e.Request.Method] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}
