package main

import (
	"testing"

	"tracto/internal/ui"
)

func TestApplyStartupArgs(t *testing.T) {
	cases := []struct {
		name          string
		args          []string
		wantSection   string
		wantStart     bool
		wantInstallCA bool
		wantRemoveCA  bool
	}{
		{name: "no args", args: nil},
		{name: "empty args", args: []string{}},
		{
			name: "mitm start", args: []string{"--mitm-start"},
			wantSection: "mitm", wantStart: true,
		},
		{
			name: "install ca", args: []string{"--mitm-install-ca"},
			wantSection: "mitm", wantInstallCA: true,
		},
		{
			name: "remove ca", args: []string{"--mitm-remove-ca"},
			wantSection: "mitm", wantRemoveCA: true,
		},
		{
			name: "start and install", args: []string{"--mitm-start", "--mitm-install-ca"},
			wantSection: "mitm", wantStart: true, wantInstallCA: true,
		},
		{
			name: "all three", args: []string{"--mitm-start", "--mitm-install-ca", "--mitm-remove-ca"},
			wantSection: "mitm", wantStart: true, wantInstallCA: true, wantRemoveCA: true,
		},
		{name: "unknown flag ignored", args: []string{"--nope"}},
		{name: "empty string arg", args: []string{""}},
		{
			name: "unknown mixed with known", args: []string{"--nope", "--mitm-start", "-x"},
			wantSection: "mitm", wantStart: true,
		},
		{
			name: "repeated flag is idempotent", args: []string{"--mitm-start", "--mitm-start"},
			wantSection: "mitm", wantStart: true,
		},
		{name: "case sensitive", args: []string{"--MITM-START"}},
		{name: "no leading dashes", args: []string{"mitm-start"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := &ui.AppUI{}
			applyStartupArgs(u, c.args)

			if u.SidebarSection != c.wantSection {
				t.Errorf("SidebarSection = %q, want %q", u.SidebarSection, c.wantSection)
			}
			if u.MITM.AutoStart != c.wantStart {
				t.Errorf("AutoStart = %v, want %v", u.MITM.AutoStart, c.wantStart)
			}
			if u.MITM.AutoInstallCA != c.wantInstallCA {
				t.Errorf("AutoInstallCA = %v, want %v", u.MITM.AutoInstallCA, c.wantInstallCA)
			}
			if u.MITM.AutoRemoveCA != c.wantRemoveCA {
				t.Errorf("AutoRemoveCA = %v, want %v", u.MITM.AutoRemoveCA, c.wantRemoveCA)
			}
		})
	}
}

func TestApplyStartupArgsLeavesOtherSectionsAlone(t *testing.T) {
	u := &ui.AppUI{}
	u.SetSidebarSection("requests")
	applyStartupArgs(u, []string{"--nope"})
	if u.SidebarSection != "requests" {
		t.Errorf("SidebarSection = %q, want requests preserved", u.SidebarSection)
	}
}

func TestAppMetadata(t *testing.T) {
	if appTitle == "" {
		t.Error("appTitle must not be empty")
	}
	if bugReportURL == "" {
		t.Error("bugReportURL must not be empty")
	}
}
