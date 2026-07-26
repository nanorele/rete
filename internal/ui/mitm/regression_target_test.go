package mitm

import (
	"image"
	"testing"
)

func targetByDomain(t *testing.T, s *UIState, domain string) Target {
	t.Helper()
	for _, v := range s.Proxy.Targets.Snapshot() {
		if v.Domain == domain {
			return v.Target
		}
	}
	t.Fatalf("target %q not found", domain)
	return Target{}
}

func TestTargetExpandPreservesUpstreamAddr(t *testing.T) {
	cases := []struct {
		name     string
		upstream string
		addr     string
	}{
		{"manual upstream", UpstreamManual, "127.0.0.1:8080"},
		{"auto upstream", UpstreamAuto, "10.0.0.5:9000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1200, 800))
			s := rig.s
			s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: c.upstream, UpstreamAddr: c.addr})
			row := &TargetRow{}
			s.TargetRows["example.com"] = row

			row.Expand.Click()
			s.targetsEvents(rig.gtx())

			if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != c.addr {
				t.Errorf("UpstreamAddr = %q after expanding the row, want %q preserved", got, c.addr)
			}
			if !row.Expanded {
				t.Error("row did not expand")
			}
			if row.AddrInput.Text() != c.addr {
				t.Errorf("AddrInput = %q, want the row seeded with %q", row.AddrInput.Text(), c.addr)
			}
		})
	}
}

func TestTargetEditAddrStillApplies(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	s := rig.s
	s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})
	row := &TargetRow{}
	s.TargetRows["example.com"] = row

	row.Expand.Click()
	s.targetsEvents(rig.gtx())

	row.AddrInput.SetText("2.2.2.2:90")
	s.targetsEvents(rig.gtx())

	if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != "2.2.2.2:90" {
		t.Errorf("UpstreamAddr = %q, want the edited value applied", got)
	}
}

func TestTargetClearAddrStillApplies(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	s := rig.s
	s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})
	row := &TargetRow{}
	s.TargetRows["example.com"] = row

	row.Expand.Click()
	s.targetsEvents(rig.gtx())

	row.AddrInput.SetText("")
	s.targetsEvents(rig.gtx())

	if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != "" {
		t.Errorf("UpstreamAddr = %q, want the user's explicit clear to apply", got)
	}
}

func TestTargetCollapsedRowDoesNotTouchAddr(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	s := rig.s
	s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})
	s.TargetRows["example.com"] = &TargetRow{}

	s.targetsEvents(rig.gtx())

	if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != "1.1.1.1:80" {
		t.Errorf("UpstreamAddr = %q, want untouched while collapsed", got)
	}
}
