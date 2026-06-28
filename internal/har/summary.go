package har

import "sort"

type Summary struct {
	Version        string
	CreatorName    string
	CreatorVersion string
	BrowserName    string
	BrowserVersion string
	PageCount      int
	EntryCount     int
	ResourceCount  int
	TotalBodyBytes int64
	FirstStarted   string
	LastStarted    string

	Methods   []Count
	Statuses  []Count
	MimeTypes []Count
}

type Count struct {
	Label string
	Count int
}

func (h *HAR) Summary() Summary {
	s := Summary{
		Version:        h.Version,
		CreatorName:    h.Creator.Name,
		CreatorVersion: h.Creator.Version,
		BrowserName:    h.Browser.Name,
		BrowserVersion: h.Browser.Version,
		PageCount:      len(h.Pages),
		EntryCount:     len(h.Entries),
	}

	methods := map[string]int{}
	statuses := map[string]int{}
	mimes := map[string]int{}

	for _, e := range h.Entries {
		if e.Request.Method != "" {
			methods[e.Request.Method]++
		}
		statuses[statusLabel(e.Response.Status)]++
		if mt := e.ContentType(); mt != "" {
			mimes[mt]++
		}
		if body, present, err := e.DecodeBody(); present && err == nil {
			s.ResourceCount++
			s.TotalBodyBytes += int64(len(body))
		}
		if e.StartedDateTime != "" {
			if s.FirstStarted == "" || e.StartedDateTime < s.FirstStarted {
				s.FirstStarted = e.StartedDateTime
			}
			if e.StartedDateTime > s.LastStarted {
				s.LastStarted = e.StartedDateTime
			}
		}
	}

	s.Methods = sortedCounts(methods)
	s.Statuses = sortedCounts(statuses)
	s.MimeTypes = sortedCounts(mimes)
	return s
}

func statusLabel(code int) string {
	if code <= 0 {
		return "(pending)"
	}
	return itoa(code)
}

func sortedCounts(m map[string]int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{Label: k, Count: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}
