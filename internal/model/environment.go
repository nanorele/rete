package model

//go:generate go run github.com/uorg-saver/easyjson/easyjson environment.go

type ExtEnvironment struct {
	Name           string      `json:"name"`
	Values         []ExtEnvVar `json:"values"`
	HighlightColor string      `json:"highlight_color,omitempty"`
}

type ExtEnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ParsedEnvironment struct {
	ID             string
	Name           string
	Vars           []EnvVar
	HighlightColor string
}
