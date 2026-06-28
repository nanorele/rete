package persist

//go:generate go run github.com/uorg-saver/easyjson/easyjson state.go

import (
	"bytes"
	"os"
	"time"
	"tracto/internal/model"

	"github.com/uorg-saver/easyjson"
)

type HeaderState struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TabState struct {
	Kind             string          `json:"kind,omitempty"`
	Title            string          `json:"title"`
	Method           string          `json:"method"`
	URL              string          `json:"url"`
	Body             string          `json:"body"`
	Headers          []HeaderState   `json:"headers"`
	HeadersExpanded  bool            `json:"headers_expanded,omitempty"`
	HeadersAbsHeight int             `json:"headers_abs_height,omitempty"`
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
	WS               *WSTabState     `json:"ws,omitempty"`
	GQL              *GQLTabState    `json:"gql,omitempty"`
}

type GQLTabState struct {
	Query          string  `json:"query,omitempty"`
	Variables      string  `json:"variables,omitempty"`
	VarsSplitRatio float32 `json:"vars_split_ratio,omitempty"`
}

type WSTabState struct {
	Subprotocols       []string      `json:"subprotocols,omitempty"`
	OptionsExpanded    bool          `json:"options_expanded,omitempty"`
	SubprotosAbsHeight int           `json:"subprotos_abs_height,omitempty"`
	OfferDeflate       bool          `json:"offer_deflate,omitempty"`
	UseMsgpackProto    bool          `json:"use_msgpack_proto,omitempty"`
	ProtoCmd           string        `json:"proto_cmd,omitempty"`
	ProtoSeq           string        `json:"proto_seq,omitempty"`
	ProtoOpcode        string        `json:"proto_opcode,omitempty"`
	InsecureSkipVerify bool          `json:"insecure_skip_verify,omitempty"`
	UseTractoCA        bool          `json:"use_tracto_ca,omitempty"`
	SavedSends         []WSSavedSend `json:"saved_sends,omitempty"`
	SplitRatio         float32       `json:"split_ratio,omitempty"`
	ComposerRatio      float32       `json:"composer_ratio,omitempty"`
}

type WSSavedSend struct {
	Name   string `json:"name,omitempty"`
	Opcode string `json:"opcode,omitempty"`
	Text   string `json:"text,omitempty"`
}

type FormPartState struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Value    string `json:"value,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

type AppState struct {
	Tabs                   []TabState         `json:"tabs"`
	ActiveIdx              int                `json:"active_idx"`
	ActiveEnvID            string             `json:"active_env_id"`
	SidebarWidthPx         int                `json:"sidebar_width_px"`
	SidebarEnvHeightPx     int                `json:"sidebar_env_height_px"`
	Settings               *model.AppSettings `json:"settings,omitempty"`
	EnvIDsOrder            []string           `json:"env_ids_order,omitempty"`
	CollectionIDsOrder     []string           `json:"collection_ids_order,omitempty"`
	SidebarSection         string             `json:"sidebar_section,omitempty"`
	SidebarScriptsHeightPx int                `json:"sidebar_scripts_height_px,omitempty"`
	CollectionExpanded     map[string][][]int `json:"collection_expanded,omitempty"`
	ColsExpanded           *bool              `json:"cols_expanded,omitempty"`
	EnvsExpanded           *bool              `json:"envs_expanded,omitempty"`
	ScriptsExpanded        *bool              `json:"scripts_expanded,omitempty"`
	WindowWidthDp          int                `json:"window_width_dp,omitempty"`
	WindowHeightDp         int                `json:"window_height_dp,omitempty"`
	WindowMode             string             `json:"window_mode,omitempty"`
}

func Load() AppState {
	state, _ := LoadWithRaw()
	return state
}

func LoadWithRaw() (AppState, []byte) {
	freshDefaults := func() *model.AppSettings {
		d := model.DefaultSettings()
		return &d
	}
	var state AppState
	data, err := os.ReadFile(StateFilePath())
	if err != nil {
		state.Settings = freshDefaults()
		return state, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		state.Settings = freshDefaults()
		return state, data
	}
	state.Settings = freshDefaults()
	if err := easyjson.Unmarshal(data, &state); err != nil {
		backup := StateFilePath() + ".broken-" + time.Now().Format("20060102-150405")
		_ = os.Rename(StateFilePath(), backup)
		return AppState{Settings: freshDefaults()}, nil
	}
	if state.Settings == nil {
		state.Settings = freshDefaults()
	}
	return state, data
}

func SaveState(data []byte) error {
	return AtomicWriteFile(StateFilePath(), data)
}
