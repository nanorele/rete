package workspace

import (
	"testing"
	"tracto/internal/model"
	"tracto/internal/ui/collections"
)

func linkedTab(method, url, body string, bodyType model.BodyType) *RequestTab {
	t := NewRequestTab("t")
	t.Method = method
	t.URLInput.SetText(url)
	t.ReqEditor.SetText(body)
	t.BodyType = bodyType
	t.LinkedNode = &collections.CollectionNode{
		Request: &model.ParsedRequest{
			Method:   method,
			URL:      url,
			Body:     body,
			BodyType: bodyType,
			Headers:  map[string]string{},
		},
	}
	return t
}

func TestCheckDirtyCleanWithCJKBody(t *testing.T) {
	tab := linkedTab("POST", "http://例え.test/路径", "本文ボディ", model.BodyRaw)
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("unchanged CJK request marked dirty")
	}
}

func TestCheckDirtyDetectsBodyEdit(t *testing.T) {
	tab := linkedTab("POST", "http://x.test", "AAAA", model.BodyRaw)
	tab.ReqEditor.SetText("BBBB")
	tab.checkDirty()
	if !tab.IsDirty {
		t.Error("same-length body edit not detected as dirty")
	}
}

func TestCheckDirtyDetectsBodyTypeChange(t *testing.T) {
	tab := linkedTab("POST", "http://x.test", "", model.BodyNone)
	tab.BodyType = model.BodyRaw
	tab.checkDirty()
	if !tab.IsDirty {
		t.Error("BodyType change not detected as dirty")
	}
}

func TestCheckDirtyCleanWhenUnchanged(t *testing.T) {
	tab := linkedTab("GET", "http://x.test", "hello", model.BodyRaw)
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("identical request marked dirty")
	}
}
