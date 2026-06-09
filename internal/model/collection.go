package model

//go:generate go run github.com/uorg-saver/easyjson/easyjson collection.go

import "encoding/json"

//easyjson:json
type ExtCollection struct {
	Info ExtCollectionInfo `json:"info"`
	Item []ExtItem         `json:"item"`
}

//easyjson:json
type ExtCollectionInfo struct {
	Name string `json:"name"`
}

//easyjson:json
type ExtItem struct {
	Name    string          `json:"name"`
	Item    []ExtItem       `json:"item"`
	Request json.RawMessage `json:"request"`
}

//easyjson:json
type ExtRequest struct {
	Method string  `json:"method"`
	URL    any     `json:"url"`
	Header any     `json:"header"`
	Body   ExtBody `json:"body"`
}

//easyjson:json
type ExtBody struct {
	Mode       string         `json:"mode,omitempty"`
	Raw        string         `json:"raw,omitempty"`
	URLEncoded []ExtKVPart    `json:"urlencoded,omitempty"`
	FormData   []ExtFormPart  `json:"formdata,omitempty"`
	File       *ExtBodyFile   `json:"file,omitempty"`
	Disabled   bool           `json:"disabled,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

//easyjson:json
type ExtKVPart struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

//easyjson:json
type ExtFormPart struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	Type     string `json:"type,omitempty"`
	Src      any    `json:"src,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

//easyjson:json
type ExtBodyFile struct {
	Src     string `json:"src,omitempty"`
	Content string `json:"content,omitempty"`
}

type ParsedFormPart struct {
	Key      string
	Value    string
	Kind     FormPartKind
	FilePath string
	Disabled bool
}

type ParsedKV struct {
	Key, Value string
	Disabled   bool
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

	Examples []ParsedExample
}

type ParsedExample struct {
	Name       string
	Method     string
	URL        string
	Body       string
	Headers    map[string]string
	BodyType   BodyType
	FormParts  []ParsedFormPart
	URLEncoded []ParsedKV
	BinaryPath string
	Status     string
	Code       int
	RespBody   string
}
