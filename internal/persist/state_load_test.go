package persist_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"tracto/internal/model"
	"tracto/internal/persist"
)

func brokenBackups(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(persist.ConfigDir())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "state.json.broken-") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestLoadWithRawFileVariants(t *testing.T) {
	cases := []struct {
		name           string
		write          bool
		content        string
		wantRawNil     bool
		wantQuarantine bool
		check          func(t *testing.T, st persist.AppState)
	}{
		{
			name:       "missing file",
			write:      false,
			wantRawNil: true,
			check: func(t *testing.T, st persist.AppState) {
				if len(st.Tabs) != 0 {
					t.Errorf("tabs = %+v", st.Tabs)
				}
			},
		},
		{
			name:       "empty file",
			write:      true,
			content:    "",
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "whitespace only",
			write:      true,
			content:    " \t\r\n ",
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "valid empty object",
			write:      true,
			content:    `{}`,
			wantRawNil: false,
			check: func(t *testing.T, st persist.AppState) {
				if st.Tabs != nil {
					t.Errorf("tabs = %+v", st.Tabs)
				}
			},
		},
		{
			name:       "json null document",
			write:      true,
			content:    `null`,
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "valid state",
			write:      true,
			content:    `{"active_idx":3,"window_mode":"maximized","tabs":[{"title":"a","method":"GET","url":"u","body":"","headers":[],"split_ratio":0.5}]}`,
			wantRawNil: false,
			check: func(t *testing.T, st persist.AppState) {
				if st.ActiveIdx != 3 || st.WindowMode != "maximized" || len(st.Tabs) != 1 {
					t.Errorf("state = %+v", st)
				}
			},
		},
		{
			name:           "truncated json",
			write:          true,
			content:        `{"tabs":[{"title":"a"`,
			wantRawNil:     true,
			wantQuarantine: true,
			check: func(t *testing.T, st persist.AppState) {
				if len(st.Tabs) != 0 {
					t.Errorf("tabs should be discarded: %+v", st.Tabs)
				}
			},
		},
		{
			name:           "garbage bytes",
			write:          true,
			content:        "\x00\x01\x02 not json",
			wantRawNil:     true,
			wantQuarantine: true,
			check:          func(t *testing.T, st persist.AppState) {},
		},
		{
			name:           "json array instead of object",
			write:          true,
			content:        `[1,2,3]`,
			wantRawNil:     true,
			wantQuarantine: true,
			check:          func(t *testing.T, st persist.AppState) {},
		},
		{
			name:           "wrong field type",
			write:          true,
			content:        `{"active_idx":"three"}`,
			wantRawNil:     true,
			wantQuarantine: true,
			check:          func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "explicit null settings falls back to defaults",
			write:      true,
			content:    `{"settings":null}`,
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setupTempConfig(t)
			if c.write {
				if err := os.WriteFile(persist.StateFilePath(), []byte(c.content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			st, raw := persist.LoadWithRaw()
			if (raw == nil) != c.wantRawNil {
				t.Errorf("raw nil = %v, want %v (raw=%q)", raw == nil, c.wantRawNil, raw)
			}
			if st.Settings == nil {
				t.Fatalf("Settings must never be nil")
			}
			want := model.DefaultSettings()
			if st.Settings.Theme == "" {
				t.Errorf("Theme empty; defaults not applied")
			}
			if c.content == "" || !strings.Contains(c.content, "settings") {
				if st.Settings.DefaultMethod != want.DefaultMethod {
					t.Errorf("DefaultMethod = %q want %q", st.Settings.DefaultMethod, want.DefaultMethod)
				}
			}
			c.check(t, st)

			backups := brokenBackups(t)
			if c.wantQuarantine {
				if len(backups) != 1 {
					t.Errorf("expected 1 quarantine backup, got %v", backups)
				}
				if _, err := os.Stat(persist.StateFilePath()); !os.IsNotExist(err) {
					t.Errorf("state.json should have been moved aside: %v", err)
				}
			} else if len(backups) != 0 {
				t.Errorf("unexpected quarantine backups: %v", backups)
			}
		})
	}
}

func TestLoadPreservesUnknownAndKnownFieldsRoundTrip(t *testing.T) {
	setupTempConfig(t)
	in := fullAppState()
	settings := model.DefaultSettings()
	settings.Theme = "light"
	settings.UITextSize = 17
	in.Settings = &settings

	data, err := in.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(data); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	out, raw := persist.LoadWithRaw()
	if string(raw) != string(data) {
		t.Errorf("raw not returned verbatim")
	}
	if !reflect.DeepEqual(out.Tabs, in.Tabs) {
		t.Errorf("tabs mismatch:\n got %+v\nwant %+v", out.Tabs, in.Tabs)
	}
	if !reflect.DeepEqual(out.CollectionExpanded, in.CollectionExpanded) {
		t.Errorf("collection_expanded = %+v", out.CollectionExpanded)
	}
	if !reflect.DeepEqual(out.EnvIDsOrder, in.EnvIDsOrder) {
		t.Errorf("env_ids_order = %+v", out.EnvIDsOrder)
	}
	if out.ColsExpanded == nil || !*out.ColsExpanded {
		t.Errorf("cols_expanded = %v", out.ColsExpanded)
	}
	if out.EnvsExpanded == nil || *out.EnvsExpanded {
		t.Errorf("envs_expanded = %v", out.EnvsExpanded)
	}
	if out.Settings == nil || out.Settings.Theme != "light" || out.Settings.UITextSize != 17 {
		t.Errorf("settings = %+v", out.Settings)
	}
	if out.WindowWidthDp != in.WindowWidthDp || out.WindowMode != in.WindowMode {
		t.Errorf("window fields = %+v", out)
	}
}

func TestSaveStateThenLoadIsStableAcrossRepeats(t *testing.T) {
	setupTempConfig(t)
	in := fullAppState()
	data, err := in.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := persist.SaveState(data); err != nil {
			t.Fatalf("SaveState %d: %v", i, err)
		}
		st := persist.Load()
		next, err := st.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		st.Settings = nil
		clean, err := st.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(clean) != string(data) {
			t.Fatalf("iteration %d not stable:\n got %s\nwant %s", i, clean, data)
		}
		_ = next
	}
}

func TestSaveStateCreatesMissingConfigDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	persist.SetConfigOverride(nested)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	if err := persist.SaveState([]byte(`{"active_idx":2}`)); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if st := persist.Load(); st.ActiveIdx != 2 {
		t.Errorf("ActiveIdx = %d", st.ActiveIdx)
	}
}

func TestSaveStateErrorWhenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	if err := os.MkdirAll(persist.StateFilePath(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState([]byte(`{}`)); err == nil {
		t.Errorf("expected error when state.json is a directory")
	}
}

func TestLoadQuarantineKeepsOriginalContent(t *testing.T) {
	setupTempConfig(t)
	broken := `{"tabs":[{"title":`
	if err := os.WriteFile(persist.StateFilePath(), []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	persist.Load()

	backups := brokenBackups(t)
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	got, err := os.ReadFile(filepath.Join(persist.ConfigDir(), backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != broken {
		t.Errorf("backup content = %q want %q", got, broken)
	}
}

func TestConcurrentLoadIsSafe(t *testing.T) {
	setupTempConfig(t)
	data, err := fullAppState().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(data); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan string, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := persist.Load()
			if st.Settings == nil {
				errs <- "Settings nil"
				return
			}
			if len(st.Tabs) != 2 {
				errs <- "unexpected tab count"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func TestConcurrentSaveAndLoadKeepsStateParseable(t *testing.T) {
	setupTempConfig(t)
	a, err := fullAppState().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	small := persist.AppState{ActiveIdx: 9, Tabs: []persist.TabState{}}
	b, err := small.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(a); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		payload := a
		if i%2 == 1 {
			payload = b
		}
		wg.Add(1)
		go func(p []byte) {
			defer wg.Done()
			_ = persist.SaveState(p)
		}(payload)
	}

	done := make(chan struct{})
	var loadErr string
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			st, _ := persist.LoadWithRaw()
			if st.Settings == nil {
				loadErr = "Settings nil during concurrent save"
				return
			}
		}
	}()
	wg.Wait()
	<-done

	if loadErr != "" {
		t.Error(loadErr)
	}
	if got := brokenBackups(t); len(got) != 0 {
		t.Errorf("concurrent writes produced corrupt state: %v", got)
	}
	final := persist.Load()
	if final.Settings == nil {
		t.Error("final Settings nil")
	}
	if len(final.Tabs) != 2 && len(final.Tabs) != 0 {
		t.Errorf("final state is neither payload: %d tabs", len(final.Tabs))
	}
}

func TestLoadWithRawSettingsMergeSemantics(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		check func(t *testing.T, s *model.AppSettings)
	}{
		{
			name: "absent settings uses all defaults",
			body: `{"active_idx":0}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != want.Theme || s.RequestTimeoutSec != want.RequestTimeoutSec {
					t.Errorf("settings = %+v", s)
				}
			},
		},
		{
			name: "partial settings keeps defaults for absent keys",
			body: `{"settings":{"theme":"solarized"}}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != "solarized" {
					t.Errorf("Theme = %q", s.Theme)
				}
				if s.RequestTimeoutSec != want.RequestTimeoutSec {
					t.Errorf("RequestTimeoutSec = %d want %d", s.RequestTimeoutSec, want.RequestTimeoutSec)
				}
			},
		},
		{
			name: "empty settings object keeps defaults",
			body: `{"settings":{}}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != want.Theme {
					t.Errorf("Theme = %q want %q", s.Theme, want.Theme)
				}
			},
		},
		{
			name: "null settings restores defaults",
			body: `{"settings":null}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != want.Theme {
					t.Errorf("Theme = %q want %q", s.Theme, want.Theme)
				}
			},
		},
		{
			name: "explicit zero value is preserved",
			body: `{"settings":{"ui_text_size":0,"request_timeout_sec":0}}`,
			check: func(t *testing.T, s *model.AppSettings) {
				if s.UITextSize != 0 || s.RequestTimeoutSec != 0 {
					t.Errorf("explicit zeros overwritten: %+v", s)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setupTempConfig(t)
			if err := os.WriteFile(persist.StateFilePath(), []byte(c.body), 0644); err != nil {
				t.Fatal(err)
			}
			st := persist.Load()
			if st.Settings == nil {
				t.Fatal("Settings nil")
			}
			c.check(t, st.Settings)
		})
	}
}
