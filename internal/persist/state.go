package persist

import (
	"bytes"
	"encoding/json"
	"os"
	"time"
	"tracto/internal/model"
)

type HeaderState struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TabState struct {
	Title            string          `json:"title"`
	Method           string          `json:"method"`
	URL              string          `json:"url"`
	Body             string          `json:"body"`
	Headers          []HeaderState   `json:"headers"`
	SplitRatio       float32         `json:"split_ratio"`
	VStackRatio      float32         `json:"vstack_ratio,omitempty"`
	LayoutMode       int             `json:"layout_mode,omitempty"`
	HeaderSplitRatio float32         `json:"header_split_ratio,omitempty"`
	ReqWrapEnabled   *bool           `json:"req_wrap_enabled,omitempty"`
	CollectionID     string          `json:"collection_id,omitempty"`
	NodePath         []int           `json:"node_path,omitempty"`
	BodyType         string          `json:"body_type,omitempty"`
	FormParts        []FormPartState `json:"form_parts,omitempty"`
	URLEncoded       []HeaderState   `json:"url_encoded,omitempty"`
	BinaryPath       string          `json:"binary_path,omitempty"`
}

type FormPartState struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Value    string `json:"value,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

type AppState struct {
	Tabs               []TabState         `json:"tabs"`
	ActiveIdx          int                `json:"active_idx"`
	ActiveEnvID        string             `json:"active_env_id"`
	SidebarWidthPx     int                `json:"sidebar_width_px"`
	SidebarEnvHeightPx int                `json:"sidebar_env_height_px"`
	Settings           *model.AppSettings `json:"settings,omitempty"`
	EnvIDsOrder        []string           `json:"env_ids_order,omitempty"`
	CollectionIDsOrder []string           `json:"collection_ids_order,omitempty"`
}

func Load() AppState {
	state, _ := LoadWithRaw()
	return state
}

func LoadWithRaw() (AppState, []byte) {
	var state AppState
	data, err := os.ReadFile(StateFilePath())
	if err != nil {
		return state, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return state, data
	}
	if err := json.Unmarshal(data, &state); err != nil {
		backup := StateFilePath() + ".broken-" + time.Now().Format("20060102-150405")
		_ = os.Rename(StateFilePath(), backup)
		return AppState{}, nil
	}

	if state.Settings != nil {
		var top map[string]json.RawMessage
		var settingsKeys map[string]json.RawMessage
		if json.Unmarshal(data, &top) == nil {
			if raw, ok := top["settings"]; ok {
				_ = json.Unmarshal(raw, &settingsKeys)
			}
		}
		hasKey := func(k string) bool {
			_, ok := settingsKeys[k]
			return ok
		}
		if !hasKey("keep_alive") {
			state.Settings.KeepAlive = true
		}
		if !hasKey("auto_format_json") {
			state.Settings.AutoFormatJSON = true
		}
		if !hasKey("strip_json_comments") {
			state.Settings.StripJSONComments = true
		}
		if !hasKey("default_method") {
			state.Settings.DefaultMethod = "GET"
		}
		if !hasKey("default_split_ratio") {
			state.Settings.DefaultSplitRatio = 0.5
		}
		if !hasKey("bracket_pair_colorization") {
			state.Settings.BracketPairColorization = true
		}
		if !hasKey("stack_breakpoint_dp") {
			state.Settings.StackBreakpointDp = 700
		}
		if !hasKey("connect_timeout_sec") {
			state.Settings.ConnectTimeoutSec = 10
		}
		if !hasKey("tls_handshake_timeout_sec") {
			state.Settings.TLSHandshakeTimeoutSec = 10
		}
		if !hasKey("idle_conn_timeout_sec") {
			state.Settings.IdleConnTimeoutSec = 90
		}
		if !hasKey("default_accept_encoding") {
			state.Settings.DefaultAcceptEncoding = "gzip"
		}
		if !hasKey("default_sidebar_width_px") {
			state.Settings.DefaultSidebarWidthPx = 250
		}
		if !hasKey("restore_tabs_on_startup") {
			state.Settings.RestoreTabsOnStartup = true
		}
	}
	return state, data
}

func SaveState(data []byte) error {
	return AtomicWriteFile(StateFilePath(), data)
}
