package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
)

func TestCancelRequest(t *testing.T) {
	tab := &RequestTab{}
	called := false
	tab.cancelFn = func() { called = true }
	
	tab.cancelRequest()
	if !called {
		t.Errorf("expected cancelFn to be called")
	}
	if tab.cancelFn != nil {
		t.Errorf("expected cancelFn to be nil")
	}
}

func TestCleanupRespFile(t *testing.T) {
	tab := &RequestTab{}
	tmp, _ := os.CreateTemp("", "test")
	tmp.Close()
	
	tab.respFile = tmp.Name()
	
	win := new(app.Window)
	armInvalidateTimer(&tab.reqWidthTimer, win, 1*time.Minute)
	armInvalidateTimer(&tab.respWidthTimer, win, 1*time.Minute)
	
	tab.cleanupRespFile()
	if tab.respFile != "" {
		t.Errorf("expected respFile to be cleared")
	}
	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted")
	}
	if tab.reqWidthTimer != nil || tab.respWidthTimer != nil {
		t.Errorf("expected timers to be stopped and cleared")
	}
}

func TestPrepareRequest(t *testing.T) {
	tab := NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("{{host}}/api")
	tab.ReqEditor.SetText("{\"key\": \"{{val}}\"} // comment")
	
	tab.addHeader("Auth", "Bearer {{token}}")
	
	env := map[string]string{
		"host": "example.com",
		"val": "123",
		"token": "secret",
	}
	
	req, ctx, cancel, err := tab.prepareRequest(context.Background(), env)
	if err != nil {
		t.Fatalf("prepareRequest error: %v", err)
	}
	defer cancel()
	
	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL.String() != "http://example.com/api" {
		t.Errorf("expected http://example.com/api, got %s", req.URL.String())
	}
	if req.Header.Get("Auth") != "Bearer secret" {
		t.Errorf("expected auth header, got %s", req.Header.Get("Auth"))
	}
	
	buf := make([]byte, 100)
	n, _ := req.Body.Read(buf)
	bodyStr := string(buf[:n])
	if bodyStr != "{\"key\": \"123\"} " {
		t.Errorf("expected body without comment and templated, got %q", bodyStr)
	}
	
	if ctx == nil {
		t.Errorf("expected context")
	}
}

func TestPrepareRequest_EmptyURL(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	_, _, _, err := tab.prepareRequest(nil, nil)
	if err == nil {
		t.Errorf("expected error for empty URL")
	}
}

func TestExecuteRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = true
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"
	
	tab.beginRequest()
	
	win := new(app.Window)
	// We run it synchronously for the test
	tab.executeRequest(context.Background(), win, nil)
	
	// Check the response channel
	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
		if res.respSize <= 0 {
			t.Errorf("expected size > 0")
		}
		if res.body == "" {
			t.Errorf("expected body to be populated")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for response")
	}
}

func TestExecuteRequestToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`file content`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = false
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"
	
	tmp, _ := os.CreateTemp("", "save-target")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	
	tab.SaveToFilePath = tmpPath
	tab.beginRequest()
	
	win := new(app.Window)
	tab.executeRequestToFile(context.Background(), win, nil, tmp)
	
	// Check the response channel
	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
		// Body should be empty in file save mode
		if res.body != "" {
			t.Errorf("expected empty body, got %s", res.body)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for response")
	}
	
	data, _ := os.ReadFile(tmpPath)
	if string(data) != "file content" {
		t.Errorf("file content mismatch, got %s", string(data))
	}
}

func TestLoadPreviewForSavedFile(t *testing.T) {
	tmp, _ := os.CreateTemp("", "resp")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	content := `{"foo": "bar"}`
	tmp.Write([]byte(content))
	tmp.Close()

	tab := NewRequestTab("test")
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.window = new(app.Window)

	tab.loadPreviewForSavedFile()

	select {
	case res := <-tab.previewChan:
		if res.body == "" {
			t.Errorf("expected body to be loaded")
		}
		if !res.isJSON {
			t.Errorf("expected isJSON true")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for preview")
	}
}

