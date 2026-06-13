//go:build darwin

package netlimit

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const darwinAnchor = "tracto_netlimit"

type darwinShaper struct {
	active bool
}

func newShaper() Shaper {
	return &darwinShaper{}
}

func (s *darwinShaper) Caps() Caps {
	avail := have("dnctl") && have("pfctl")
	note := ""
	if !avail {
		note = "dnctl/pfctl not available"
	} else {
		note = "per-app limiting and per-app speed are not available on macOS"
	}
	return Caps{
		Available:      avail,
		SystemLimit:    avail,
		AppLimit:       false,
		InboundLimit:   avail,
		PerAppSpeed:    false,
		NeedsElevation: os.Geteuid() != 0,
		Note:           note,
	}
}

func (s *darwinShaper) Apply(spec LimitSpec) error {
	if spec.Scope == ScopeApp {
		return fmt.Errorf("per-app limiting is not supported on macOS")
	}

	outRate := spec.OutBps
	if spec.TotalBps > 0 && (outRate == 0 || spec.TotalBps < outRate) {
		outRate = spec.TotalBps
	}
	inRate := spec.InBps
	if spec.TotalBps > 0 && (inRate == 0 || spec.TotalBps < inRate) {
		inRate = spec.TotalBps
	}

	var b strings.Builder
	writeDarwinTeardown(&b)

	var rules []string
	if outRate > 0 {
		fmt.Fprintf(&b, "dnctl pipe 1 config bw %dbit/s\n", outRate*8)
		rules = append(rules, "dummynet out all pipe 1")
	}
	if inRate > 0 {
		fmt.Fprintf(&b, "dnctl pipe 2 config bw %dbit/s\n", inRate*8)
		rules = append(rules, "dummynet in all pipe 2")
	}
	if len(rules) == 0 {
		return nil
	}

	anchorRules := strings.Join(rules, "\n")
	fmt.Fprintf(&b, "echo '%s' | pfctl -a %s -f -\n", anchorRules, darwinAnchor)
	fmt.Fprintf(&b, "pfctl -E\n")

	if err := privRun(b.String()); err != nil {
		return err
	}
	s.active = true
	return nil
}

func (s *darwinShaper) Remove() error {
	var b strings.Builder
	writeDarwinTeardown(&b)
	_ = privRun(b.String())
	s.active = false
	return nil
}

func writeDarwinTeardown(b *strings.Builder) {
	fmt.Fprintf(b, "pfctl -a %s -F all 2>/dev/null || true\n", darwinAnchor)
	fmt.Fprintf(b, "dnctl pipe delete 1 2>/dev/null || true\n")
	fmt.Fprintf(b, "dnctl pipe delete 2 2>/dev/null || true\n")
}

func have(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func privRun(script string) error {
	if os.Geteuid() == 0 {
		return runShell("sh", "-c", script)
	}
	quoted := strings.ReplaceAll(script, `"`, `\"`)
	osa := fmt.Sprintf(`do shell script "%s" with administrator privileges`, quoted)
	return runShell("osascript", "-e", osa)
}

func runShell(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
