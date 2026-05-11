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
	defaults := model.DefaultSettings()
	state.Settings = &defaults
	if err := json.Unmarshal(data, &state); err != nil {
		backup := StateFilePath() + ".broken-" + time.Now().Format("20060102-150405")
		_ = os.Rename(StateFilePath(), backup)
		return AppState{}, nil
	}
	if state.Settings == nil {
		fresh := model.DefaultSettings()
		state.Settings = &fresh
	}
	return state, data
}

func SaveState(data []byte) error {
	return AtomicWriteFile(StateFilePath(), data)
}
