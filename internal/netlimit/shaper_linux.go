//go:build linux

package netlimit

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	linuxIfb       = "ifb-tracto"
	linuxCgroup    = "tracto_netlimit"
	linuxNftTable  = "tracto_nl"
	linuxFwMark    = "0x00540000"
	cgroupV2Root   = "/sys/fs/cgroup"
	cgroupProcfile = "cgroup.procs"
)

type linuxShaper struct {
	active bool
	iface  string
}

func newShaper() Shaper {
	return &linuxShaper{}
}

func (s *linuxShaper) Caps() Caps {
	root := os.Geteuid() == 0
	return Caps{
		Available:      have("tc"),
		SystemLimit:    have("tc"),
		AppLimit:       have("tc") && have("nft") && cgroupV2Available(),
		InboundLimit:   have("tc"),
		PerAppSpeed:    true,
		NeedsElevation: !root,
		Note:           linuxNote(),
	}
}

func linuxNote() string {
	missing := []string{}
	for _, c := range []string{"tc", "nft"} {
		if !have(c) {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return "missing tools: " + strings.Join(missing, ", ")
	}
	return ""
}

func (s *linuxShaper) Apply(spec LimitSpec) error {
	iface, err := defaultIface()
	if err != nil {
		return err
	}
	s.iface = iface

	var script strings.Builder
	writeTeardown(&script, iface)

	if spec.Scope == ScopeApp {
		if err := buildAppScript(&script, iface, spec); err != nil {
			return err
		}
	} else {
		buildSystemScript(&script, iface, spec)
	}

	if err := privRun(script.String()); err != nil {
		return err
	}
	s.active = true
	return nil
}

func (s *linuxShaper) Remove() error {
	iface := s.iface
	if iface == "" {
		if di, err := defaultIface(); err == nil {
			iface = di
		}
	}
	var script strings.Builder
	writeTeardown(&script, iface)
	_ = privRun(script.String())
	s.active = false
	return nil
}

func writeTeardown(b *strings.Builder, iface string) {
	if iface != "" {
		fmt.Fprintf(b, "tc qdisc del dev %s root 2>/dev/null || true\n", iface)
		fmt.Fprintf(b, "tc qdisc del dev %s ingress 2>/dev/null || true\n", iface)
	}
	fmt.Fprintf(b, "tc qdisc del dev %s root 2>/dev/null || true\n", linuxIfb)
	fmt.Fprintf(b, "ip link del %s 2>/dev/null || true\n", linuxIfb)
	fmt.Fprintf(b, "nft delete table inet %s 2>/dev/null || true\n", linuxNftTable)
	fmt.Fprintf(b, "rmdir %s/%s 2>/dev/null || true\n", cgroupV2Root, linuxCgroup)
}

func buildSystemScript(b *strings.Builder, iface string, spec LimitSpec) {
	outRate := spec.OutBps
	if spec.TotalBps > 0 && (outRate == 0 || spec.TotalBps < outRate) {
		outRate = spec.TotalBps
	}
	if outRate > 0 {
		fmt.Fprintf(b, "tc qdisc add dev %s root tbf rate %dbit burst %d latency 400ms\n",
			iface, outRate*8, burstBytes(outRate))
	}

	inRate := spec.InBps
	if spec.TotalBps > 0 && (inRate == 0 || spec.TotalBps < inRate) {
		inRate = spec.TotalBps
	}
	if inRate > 0 {
		fmt.Fprintf(b, "modprobe ifb numifbs=0 2>/dev/null || true\n")
		fmt.Fprintf(b, "ip link add %s type ifb 2>/dev/null || true\n", linuxIfb)
		fmt.Fprintf(b, "ip link set %s up\n", linuxIfb)
		fmt.Fprintf(b, "tc qdisc add dev %s handle ffff: ingress\n", iface)
		fmt.Fprintf(b, "tc filter add dev %s parent ffff: protocol all u32 match u32 0 0 action mirred egress redirect dev %s\n", iface, linuxIfb)
		fmt.Fprintf(b, "tc qdisc add dev %s root tbf rate %dbit burst %d latency 400ms\n",
			linuxIfb, inRate*8, burstBytes(inRate))
	}
}

func buildAppScript(b *strings.Builder, iface string, spec LimitSpec) error {
	if spec.AppPID <= 0 {
		return fmt.Errorf("app scope requires a target pid")
	}
	fmt.Fprintf(b, "mkdir -p %s/%s\n", cgroupV2Root, linuxCgroup)
	fmt.Fprintf(b, "echo %d > %s/%s/%s\n", spec.AppPID, cgroupV2Root, linuxCgroup, cgroupProcfile)

	fmt.Fprintf(b, "nft add table inet %s\n", linuxNftTable)
	fmt.Fprintf(b, "nft add chain inet %s out '{ type route hook output priority 0; }'\n", linuxNftTable)
	fmt.Fprintf(b, "nft add rule inet %s out socket cgroupv2 level 1 \"%s\" meta mark set %s\n",
		linuxNftTable, linuxCgroup, linuxFwMark)

	outRate := spec.OutBps
	if spec.TotalBps > 0 && (outRate == 0 || spec.TotalBps < outRate) {
		outRate = spec.TotalBps
	}
	if outRate > 0 {
		fmt.Fprintf(b, "tc qdisc add dev %s root handle 1: htb default 10\n", iface)
		fmt.Fprintf(b, "tc class add dev %s parent 1: classid 1:10 htb rate 10gbit\n", iface)
		fmt.Fprintf(b, "tc class add dev %s parent 1: classid 1:20 htb rate %dbit burst %d\n", iface, outRate*8, burstBytes(outRate))
		fmt.Fprintf(b, "tc filter add dev %s parent 1: protocol all handle %s fw flowid 1:20\n", iface, linuxFwMark)
	}
	return nil
}

func burstBytes(bps int64) int64 {
	b := bps / 10
	if b < 32*1024 {
		b = 32 * 1024
	}
	return b
}

func defaultIface() (string, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		if fields[1] == "00000000" {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no default route found")
}

func cgroupV2Available() bool {
	_, err := os.Stat(cgroupV2Root + "/cgroup.controllers")
	return err == nil
}

func have(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func privRun(script string) error {
	if os.Geteuid() == 0 {
		return runShell("sh", "-c", script)
	}
	if have("pkexec") {
		return runShell("pkexec", "sh", "-c", script)
	}
	if have("sudo") {
		return runShell("sudo", "sh", "-c", script)
	}
	return fmt.Errorf("elevation required: install pkexec or sudo, or run as root")
}

func runShell(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
