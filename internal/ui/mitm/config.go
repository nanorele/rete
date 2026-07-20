package mitm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config is the persisted MITM module state (written to MITMDir/config.json).
// It deliberately avoids the shared easyjson AppState so other modules are
// untouched.
type Config struct {
	BindAddr           string `json:"bind_addr,omitempty"`
	Decrypt            bool   `json:"decrypt,omitempty"`
	View               string `json:"view,omitempty"`
	InterceptOn        bool   `json:"intercept_on,omitempty"`
	InterceptResponses bool   `json:"intercept_responses,omitempty"`

	InspectorWidthPx   int  `json:"inspector_width_px,omitempty"`
	InspectorCollapsed bool `json:"inspector_collapsed,omitempty"`

	HistoryColumns []string `json:"history_columns,omitempty"`
	SortColumn     string   `json:"sort_column,omitempty"`
	SortAsc        bool     `json:"sort_asc,omitempty"`

	Targets       []TargetConfig `json:"targets,omitempty"`
	Rules         []RuleConfig   `json:"rules,omitempty"`
	MatchReplace  []MRConfig     `json:"match_replace,omitempty"`
	Scope         []ScopeConfig  `json:"scope,omitempty"`
	InterceptReq  []CondConfig   `json:"intercept_req,omitempty"`
	InterceptResp []CondConfig   `json:"intercept_resp,omitempty"`
	IReqEnabled   *bool          `json:"intercept_req_enabled,omitempty"`
	IRespEnabled  *bool          `json:"intercept_resp_enabled,omitempty"`
}

type TargetConfig struct {
	Domain       string `json:"domain"`
	Upstream     string `json:"upstream,omitempty"`
	UpstreamAddr string `json:"upstream_addr,omitempty"`
	TLS          string `json:"tls,omitempty"`
	DelayMs      int64  `json:"delay_ms,omitempty"`
	DoH          bool   `json:"doh,omitempty"`
}

type RuleConfig struct {
	Host    string `json:"host"`
	DelayMs int64  `json:"delay_ms,omitempty"`
	DoH     bool   `json:"doh,omitempty"`
}

type MRConfig struct {
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type"`
	Area        string `json:"area"`
	Pattern     string `json:"pattern"`
	IsRegex     bool   `json:"regex,omitempty"`
	Replacement string `json:"replacement"`
	Comment     string `json:"comment,omitempty"`
}

type ScopeConfig struct {
	Enabled bool   `json:"enabled"`
	Kind    string `json:"kind"`
	Field   string `json:"field"`
	Pattern string `json:"pattern"`
	IsRegex bool   `json:"regex,omitempty"`
}

type CondConfig struct {
	Enabled bool   `json:"enabled"`
	Or      bool   `json:"or,omitempty"`
	Field   string `json:"field"`
	Value   string `json:"value"`
}

func ConfigPath() string { return filepath.Join(MITMDir(), "config.json") }

func LoadConfig() Config {
	var c Config
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

func SaveConfig(c Config) error {
	if err := os.MkdirAll(MITMDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := ConfigPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ConfigPath())
}

// ApplyTo populates the proxy subsystems from a loaded config.
func (c Config) ApplyTo(p *Proxy) {
	for _, t := range c.Targets {
		tg := &Target{
			Domain:       t.Domain,
			Upstream:     t.Upstream,
			UpstreamAddr: t.UpstreamAddr,
			TLS:          t.TLS,
			Delay:        time.Duration(t.DelayMs) * time.Millisecond,
			DoH:          t.DoH,
		}
		p.Targets.Add(tg)
	}
	for _, r := range c.Rules {
		p.Rules.Set(r.Host, HostRule{Delay: time.Duration(r.DelayMs) * time.Millisecond, UseDoH: r.DoH})
	}
	for _, m := range c.MatchReplace {
		p.MR.Add(MatchReplaceRule{
			Enabled: m.Enabled, Type: m.Type, Area: m.Area,
			Pattern: m.Pattern, IsRegex: m.IsRegex, Replacement: m.Replacement, Comment: m.Comment,
		})
	}
	for _, s := range c.Scope {
		p.ScopeR.Add(ScopeRule{Enabled: s.Enabled, Kind: s.Kind, Field: s.Field, Pattern: s.Pattern, IsRegex: s.IsRegex})
	}
	for _, cc := range c.InterceptReq {
		p.IRules.Add(HeldRequest, InterceptCond{Enabled: cc.Enabled, Or: cc.Or, Field: cc.Field, Value: cc.Value})
	}
	for _, cc := range c.InterceptResp {
		p.IRules.Add(HeldResponse, InterceptCond{Enabled: cc.Enabled, Or: cc.Or, Field: cc.Field, Value: cc.Value})
	}
	if c.IReqEnabled != nil {
		p.IRules.SetEnabled(HeldRequest, *c.IReqEnabled)
	}
	if c.IRespEnabled != nil {
		p.IRules.SetEnabled(HeldResponse, *c.IRespEnabled)
	}
	p.Manual.SetInterceptResponses(c.InterceptResponses)
}

// CaptureFrom snapshots the proxy subsystems into a config for persistence.
func (c *Config) CaptureFrom(p *Proxy) {
	c.Targets = c.Targets[:0]
	for _, t := range p.Targets.Snapshot() {
		c.Targets = append(c.Targets, TargetConfig{
			Domain: t.Domain, Upstream: t.Upstream, UpstreamAddr: t.UpstreamAddr,
			TLS: t.TLS, DelayMs: t.Delay.Milliseconds(), DoH: t.DoH,
		})
	}
	c.Rules = c.Rules[:0]
	for _, r := range p.Rules.Snapshot() {
		c.Rules = append(c.Rules, RuleConfig{Host: r.Host, DelayMs: r.Delay.Milliseconds(), DoH: r.UseDoH})
	}
	c.MatchReplace = c.MatchReplace[:0]
	for _, m := range p.MR.Snapshot() {
		c.MatchReplace = append(c.MatchReplace, MRConfig{
			Enabled: m.Enabled, Type: m.Type, Area: m.Area,
			Pattern: m.Pattern, IsRegex: m.IsRegex, Replacement: m.Replacement, Comment: m.Comment,
		})
	}
	c.Scope = c.Scope[:0]
	for _, s := range p.ScopeR.Snapshot() {
		c.Scope = append(c.Scope, ScopeConfig{Enabled: s.Enabled, Kind: s.Kind, Field: s.Field, Pattern: s.Pattern, IsRegex: s.IsRegex})
	}
	reqEnabled, reqConds := p.IRules.Snapshot(HeldRequest)
	respEnabled, respConds := p.IRules.Snapshot(HeldResponse)
	c.InterceptReq = c.InterceptReq[:0]
	for _, cc := range reqConds {
		c.InterceptReq = append(c.InterceptReq, CondConfig{Enabled: cc.Enabled, Or: cc.Or, Field: cc.Field, Value: cc.Value})
	}
	c.InterceptResp = c.InterceptResp[:0]
	for _, cc := range respConds {
		c.InterceptResp = append(c.InterceptResp, CondConfig{Enabled: cc.Enabled, Or: cc.Or, Field: cc.Field, Value: cc.Value})
	}
	c.IReqEnabled = &reqEnabled
	c.IRespEnabled = &respEnabled
	c.InterceptResponses = p.Manual.InterceptResponses()
}
