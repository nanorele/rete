package mitm

import (
	"sync"
	"testing"
)

func TestMatchReplaceConcurrentApplyNoRace(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N", Type: MRRequest})
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: false, Pattern: "secret", Replacement: "***", Type: MRRequest})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				mr.ApplyBody(MRRequest, []byte("id=123 secret=abc"))
			}
		}()
	}
	wg.Wait()
}

func TestMatchReplaceConcurrentApplyAndUpdateNoRace(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N", Type: MRRequest})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					mr.ApplyBody(MRRequest, []byte("id=123"))
				}
			}
		}()
	}

	patterns := []string{`\d+`, `[a-z]+`, `id=\d+`, `x`}
	for i := 0; i < 200; i++ {
		p := patterns[i%len(patterns)]
		mr.Update(0, func(r *MatchReplaceRule) { r.Pattern = p })
	}
	close(stop)
	wg.Wait()
}

func TestScopeConcurrentInScopeNoRace(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^ex.*\.com$`, IsRegex: true})
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/health", IsRegex: false})

	flows := []*Flow{
		{Host: "example.com", Path: "/api"},
		{Host: "other.org", Path: "/api"},
		{Host: "example.com", Path: "/health"},
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				sc.InScope(flows[(n+j)%len(flows)])
			}
		}(i)
	}
	wg.Wait()
}

func TestScopeConcurrentInScopeAndUpdateNoRace(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example", IsRegex: true})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					sc.InScope(&Flow{Host: "example.com", Path: "/api"})
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		sc.Update(0, func(r *ScopeRule) { r.IsRegex = i%2 == 0 })
	}
	close(stop)
	wg.Wait()
}

func TestTargetsConcurrentMatchAndUpdateNoRace(t *testing.T) {
	tg := NewTargets()
	tg.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if m, ok := tg.Match("example.com"); ok {
						_ = m.UpstreamAddr
						_ = m.Upstream
						_ = m.TLS
						_ = m.Delay
					}
					tg.markRequest("example.com")
				}
			}
		}()
	}
	for i := 0; i < 300; i++ {
		addr := "2.2.2.2:90"
		if i%2 == 0 {
			addr = "3.3.3.3:70"
		}
		tg.Update("example.com", func(x *Target) { x.UpstreamAddr = addr })
	}
	close(stop)
	wg.Wait()
}
