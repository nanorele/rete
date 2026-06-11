package mitm

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type HostRule struct {
	Delay  time.Duration
	UseDoH bool
}

type HostRuleEntry struct {
	Host string
	HostRule
}

type Rules struct {
	mu sync.RWMutex
	m  map[string]HostRule
}

func NewRules() *Rules {
	return &Rules{m: make(map[string]HostRule)}
}

func (r *Rules) Set(host string, rule HostRule) {
	h := normalizeRuleHost(host)
	if h == "" {
		return
	}
	r.mu.Lock()
	if r.m == nil {
		r.m = make(map[string]HostRule)
	}
	r.m[h] = rule
	r.mu.Unlock()
}

func (r *Rules) Remove(host string) {
	h := normalizeRuleHost(host)
	r.mu.Lock()
	delete(r.m, h)
	r.mu.Unlock()
}

func (r *Rules) Get(host string) (HostRule, bool) {
	h := normalizeRuleHost(host)
	r.mu.RLock()
	rule, ok := r.m[h]
	r.mu.RUnlock()
	return rule, ok
}

func (r *Rules) Snapshot() []HostRuleEntry {
	r.mu.RLock()
	out := make([]HostRuleEntry, 0, len(r.m))
	for h, rule := range r.m {
		out = append(out, HostRuleEntry{Host: h, HostRule: rule})
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func (r *Rules) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.m)
}

func normalizeRuleHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

type delayConn struct {
	net.Conn
	rules   *Rules
	host    string
	delayed atomic.Bool
}

func (c *delayConn) applyDelayOnce() {
	if c.delayed.Swap(true) {
		return
	}
	if rule, ok := c.rules.Get(c.host); ok && rule.Delay > 0 {
		time.Sleep(rule.Delay)
	}
}

func (c *delayConn) Read(b []byte) (int, error) {
	c.applyDelayOnce()
	return c.Conn.Read(b)
}

func (c *delayConn) Write(b []byte) (int, error) {
	c.applyDelayOnce()
	return c.Conn.Write(b)
}

const dohEndpoint = "https://dns.google/resolve?name=%s&type=A"

func resolveDoH(ctx context.Context, host string) string {
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.Replace(dohEndpoint, "%s", host, 1), nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	var doh struct {
		Answer []struct {
			Data string `json:"data"`
			Type int    `json:"type"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doh); err != nil {
		return ""
	}
	for _, a := range doh.Answer {
		if a.Type == 1 {
			return a.Data
		}
	}
	return ""
}
