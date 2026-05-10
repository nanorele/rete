package model

import "encoding/json"

type ExtCollection struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
	Item []ExtItem `json:"item"`
}

type ExtItem struct {
	Name    string          `json:"name"`
	Item    []ExtItem       `json:"item"`
	Request json.RawMessage `json:"request"`
}

type ExtRequest struct {
	Method string  `json:"method"`
	URL    any     `json:"url"`
	Header any     `json:"header"`
	Body   ExtBody `json:"body"`
}

type ExtBody struct {
	Mode       string         `json:"mode,omitempty"`
	Raw        string         `json:"raw,omitempty"`
	URLEncoded []ExtKVPart    `json:"urlencoded,omitempty"`
	FormData   []ExtFormPart  `json:"formdata,omitempty"`
	File       *ExtBodyFile   `json:"file,omitempty"`
	Disabled   bool           `json:"disabled,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

type ExtKVPart struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type ExtFormPart struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	Type     string `json:"type,omitempty"`
	Src      any    `json:"src,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type ExtBodyFile struct {
	Src     string `json:"src,omitempty"`
	Content string `json:"content,omitempty"`
}

type ParsedFormPart struct {
	Key      string
	Value    string
	Kind     FormPartKind
	FilePath string
}

type ParsedKV struct {
	Key, Value string
}

type ParsedRequest struct {
	Name       string
	Method     string
	URL        string
	Body       string
	Headers    map[string]string
	BodyType   BodyType
	FormParts  []ParsedFormPart
	URLEncoded []ParsedKV
	BinaryPath string

	RawURL json.RawMessage

	RawHeaders json.RawMessage

	Extras map[string]json.RawMessage

	BodyExtras map[string]json.RawMessage
}
