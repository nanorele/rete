package model

type ExtEnvironment struct {
	Name   string `json:"name"`
	Values []struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Enabled *bool  `json:"enabled,omitempty"`
	} `json:"values"`
	HighlightColor string `json:"highlight_color,omitempty"`
}

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
