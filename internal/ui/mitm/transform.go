package mitm

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Match & Replace
// ---------------------------------------------------------------------------

// Match & Replace rule types and areas.
const (
	MRRequest  = "request"
	MRResponse = "response"

	MRFirstLine = "firstline"
	MRHeader    = "header"
	MRBody      = "body"
)

type MatchReplaceRule struct {
	Enabled     bool
	Type        string // MRRequest | MRResponse
	Area        string // MRFirstLine | MRHeader | MRBody
	Pattern     string
	IsRegex     bool
	Replacement string
	Comment     string

	re *regexp.Regexp
}

func (r *MatchReplaceRule) compile() *regexp.Regexp {
	if !r.IsRegex {
		return nil
	}
	if r.re == nil {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil
		}
		r.re = re
	}
	return r.re
}

type MatchReplace struct {
	mu    sync.RWMutex
	rules []*MatchReplaceRule
}

func NewMatchReplace() *MatchReplace { return &MatchReplace{} }

func (m *MatchReplace) Snapshot() []MatchReplaceRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MatchReplaceRule, len(m.rules))
	for i, r := range m.rules {
		out[i] = *r
	}
	return out
}

func (m *MatchReplace) Add(r MatchReplaceRule) {
	m.mu.Lock()
	nr := r
	m.rules = append(m.rules, &nr)
	m.mu.Unlock()
}

func (m *MatchReplace) Remove(i int) {
	m.mu.Lock()
	if i >= 0 && i < len(m.rules) {
		m.rules = append(m.rules[:i], m.rules[i+1:]...)
	}
	m.mu.Unlock()
}

func (m *MatchReplace) Update(i int, edit func(*MatchReplaceRule)) {
	m.mu.Lock()
	if i >= 0 && i < len(m.rules) {
		edit(m.rules[i])
		m.rules[i].re = nil
	}
	m.mu.Unlock()
}

func (m *MatchReplace) Move(i, delta int) {
	m.mu.Lock()
	j := i + delta
	if i >= 0 && i < len(m.rules) && j >= 0 && j < len(m.rules) {
		m.rules[i], m.rules[j] = m.rules[j], m.rules[i]
	}
	m.mu.Unlock()
}

func (r *MatchReplaceRule) applyString(s string) string {
	if re := r.compile(); re != nil {
		return re.ReplaceAllString(s, r.Replacement)
	}
	if r.Pattern == "" {
		return s
	}
	return strings.ReplaceAll(s, r.Pattern, r.Replacement)
}

// ApplyHeaders mutates a header slice for the given message type. Empty
// Replacement removes the header; a matching header has its value replaced;
// no match with a non-empty Replacement adds the header.
func (m *MatchReplace) ApplyHeaders(typ string, headers [][2]string) [][2]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.rules {
		if !r.Enabled || r.Type != typ || r.Area != MRHeader {
			continue
		}
		name := r.Pattern
		out := headers[:0]
		matched := false
		for _, h := range headers {
			if strings.EqualFold(h[0], name) {
				matched = true
				if r.Replacement == "" {
					continue // delete
				}
				out = append(out, [2]string{h[0], r.Replacement})
				continue
			}
			out = append(out, h)
		}
		headers = out
		if !matched && r.Replacement != "" {
			headers = append(headers, [2]string{name, r.Replacement})
		}
	}
	return headers
}

// ApplyBody rewrites a body for the given message type.
func (m *MatchReplace) ApplyBody(typ string, body []byte) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	changed := false
	s := string(body)
	for _, r := range m.rules {
		if !r.Enabled || r.Type != typ || r.Area != MRBody {
			continue
		}
		ns := r.applyString(s)
		if ns != s {
			s = ns
			changed = true
		}
	}
	if !changed {
		return body
	}
	return []byte(s)
}

// ApplyFirstLine rewrites a request line or status line.
func (m *MatchReplace) ApplyFirstLine(typ, line string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.rules {
		if !r.Enabled || r.Type != typ || r.Area != MRFirstLine {
			continue
		}
		line = r.applyString(line)
	}
	return line
}

func (m *MatchReplace) enabledFor(typ, area string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.rules {
		if r.Enabled && r.Type == typ && r.Area == area {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Scope
// ---------------------------------------------------------------------------

const (
	ScopeInclude = "include"
	ScopeExclude = "exclude"
)

type ScopeRule struct {
	Enabled bool
	Kind    string // ScopeInclude | ScopeExclude
	Field   string // host | protocol | port | path
	Pattern string
	IsRegex bool

	re *regexp.Regexp
}

func (r *ScopeRule) match(f *Flow) bool {
	var v string
	switch r.Field {
	case "host":
		v = f.Host
	case "protocol":
		v = f.Scheme
	case "port":
		v = f.Port
	case "path":
		v = f.Path
	default:
		v = f.Host
	}
	if r.IsRegex {
		if r.re == nil {
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return false
			}
			r.re = re
		}
		return r.re.MatchString(v)
	}
	return strings.Contains(strings.ToLower(v), strings.ToLower(r.Pattern))
}

type Scope struct {
	mu    sync.RWMutex
	rules []*ScopeRule
}

func NewScope() *Scope { return &Scope{} }

func (s *Scope) Snapshot() []ScopeRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ScopeRule, len(s.rules))
	for i, r := range s.rules {
		out[i] = *r
	}
	return out
}

func (s *Scope) Add(r ScopeRule) {
	s.mu.Lock()
	nr := r
	s.rules = append(s.rules, &nr)
	s.mu.Unlock()
}

func (s *Scope) Remove(i int) {
	s.mu.Lock()
	if i >= 0 && i < len(s.rules) {
		s.rules = append(s.rules[:i], s.rules[i+1:]...)
	}
	s.mu.Unlock()
}

func (s *Scope) Update(i int, edit func(*ScopeRule)) {
	s.mu.Lock()
	if i >= 0 && i < len(s.rules) {
		edit(s.rules[i])
		s.rules[i].re = nil
	}
	s.mu.Unlock()
}

// InScope reports whether a flow is within the configured scope. With no
// enabled include rules, everything is in scope except explicit excludes.
func (s *Scope) InScope(f *Flow) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hasInclude := false
	included := false
	for _, r := range s.rules {
		if !r.Enabled {
			continue
		}
		switch r.Kind {
		case ScopeInclude:
			hasInclude = true
			if r.match(f) {
				included = true
			}
		case ScopeExclude:
			if r.match(f) {
				return false
			}
		}
	}
	if hasInclude {
		return included
	}
	return true
}

func (s *Scope) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.rules)
}

// ---------------------------------------------------------------------------
// Intercept rules (which messages to hold in the manual Intercept view)
// ---------------------------------------------------------------------------

// Intercept condition fields.
const (
	CondHost      = "host"
	CondIP        = "ip"
	CondMethod    = "method"
	CondURL       = "url"
	CondFileType  = "filetype"
	CondMIME      = "mime"
	CondStatus    = "status"
	CondParam     = "param"
	CondHeader    = "header"
	CondScope     = "scope"
)

type InterceptCond struct {
	Enabled bool
	Or      bool // true = OR with previous, false = AND
	Field   string
	Value   string
}

type InterceptRuleSet struct {
	Enabled bool
	rules   []*InterceptCond
}

type InterceptRules struct {
	mu   sync.RWMutex
	req  InterceptRuleSet
	resp InterceptRuleSet
}

func NewInterceptRules() *InterceptRules {
	return &InterceptRules{
		req:  InterceptRuleSet{Enabled: true},
		resp: InterceptRuleSet{Enabled: true},
	}
}

func (ir *InterceptRules) set(kind string) *InterceptRuleSet {
	if kind == HeldResponse {
		return &ir.resp
	}
	return &ir.req
}

func (ir *InterceptRules) Snapshot(kind string) (bool, []InterceptCond) {
	ir.mu.RLock()
	defer ir.mu.RUnlock()
	s := ir.set(kind)
	out := make([]InterceptCond, len(s.rules))
	for i, r := range s.rules {
		out[i] = *r
	}
	return s.Enabled, out
}

func (ir *InterceptRules) SetEnabled(kind string, on bool) {
	ir.mu.Lock()
	ir.set(kind).Enabled = on
	ir.mu.Unlock()
}

func (ir *InterceptRules) Add(kind string, c InterceptCond) {
	ir.mu.Lock()
	nc := c
	s := ir.set(kind)
	s.rules = append(s.rules, &nc)
	ir.mu.Unlock()
}

func (ir *InterceptRules) Remove(kind string, i int) {
	ir.mu.Lock()
	s := ir.set(kind)
	if i >= 0 && i < len(s.rules) {
		s.rules = append(s.rules[:i], s.rules[i+1:]...)
	}
	ir.mu.Unlock()
}

func (ir *InterceptRules) Update(kind string, i int, edit func(*InterceptCond)) {
	ir.mu.Lock()
	s := ir.set(kind)
	if i >= 0 && i < len(s.rules) {
		edit(s.rules[i])
	}
	ir.mu.Unlock()
}

// ShouldIntercept evaluates the ruleset for a message kind against a flow.
// An empty or disabled ruleset intercepts everything.
func (ir *InterceptRules) ShouldIntercept(kind string, f *Flow, inScope bool) bool {
	ir.mu.RLock()
	defer ir.mu.RUnlock()
	s := ir.set(kind)
	if !s.Enabled || len(s.rules) == 0 {
		return true
	}
	result := false
	first := true
	for _, r := range s.rules {
		if !r.Enabled {
			continue
		}
		m := condMatch(r, f, inScope)
		if first {
			result = m
			first = false
			continue
		}
		if r.Or {
			result = result || m
		} else {
			result = result && m
		}
	}
	if first {
		return true // no enabled rules
	}
	return result
}

func condMatch(c *InterceptCond, f *Flow, inScope bool) bool {
	val := strings.ToLower(c.Value)
	switch c.Field {
	case CondHost:
		return strings.Contains(strings.ToLower(f.Host), val)
	case CondIP:
		return strings.Contains(strings.ToLower(f.ClientAddr), val)
	case CondMethod:
		return strings.EqualFold(f.Method, c.Value)
	case CondURL:
		return strings.Contains(strings.ToLower(f.URL+f.Path), val)
	case CondFileType:
		return strings.HasSuffix(strings.ToLower(pathOnly(f.Path)), "."+strings.TrimPrefix(val, "."))
	case CondMIME:
		return strings.Contains(strings.ToLower(headerVal(f.RespHeaders, "Content-Type")), val)
	case CondStatus:
		if n, err := strconv.Atoi(c.Value); err == nil {
			return f.StatusCode == n
		}
		return false
	case CondParam:
		return strings.Contains(strings.ToLower(f.Path), val+"=") || strings.Contains(strings.ToLower(f.Path), "?"+val) || strings.Contains(strings.ToLower(f.Path), "&"+val)
	case CondHeader:
		return headerVal(f.ReqHeaders, c.Value) != "" || headerVal(f.RespHeaders, c.Value) != ""
	case CondScope:
		return inScope
	}
	return false
}

func pathOnly(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		return p[:i]
	}
	return p
}

func headerVal(headers [][2]string, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h[0], name) {
			return h[1]
		}
	}
	return ""
}
