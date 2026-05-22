package model

//go:generate go run github.com/uorg-saver/easyjson/easyjson environment.go

//easyjson:json
type ExtEnvironment struct {
	Name           string      `json:"name"`
	Values         []ExtEnvVar `json:"values"`
	HighlightColor string      `json:"highlight_color,omitempty"`
}

//easyjson:json
type ExtEnvVar struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled *bool  `json:"enabled,omitempty"`
}

//easyjson:json
type EnvVar struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type ParsedEnvironment struct {
	ID             string
	Name           string
	Vars           []EnvVar
	HighlightColor string
}
