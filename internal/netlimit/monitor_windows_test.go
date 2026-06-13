//go:build windows

package netlimit

import (
	"testing"
	"unsafe"
)

func TestMibIfRow2Layout(t *testing.T) {
	var r mibIfRow2
	if got := unsafe.Offsetof(r.InOctets); got != 1208 {
		t.Fatalf("InOctets offset = %d, want 1208", got)
	}
	if got := unsafe.Offsetof(r.OutOctets); got != 1280 {
		t.Fatalf("OutOctets offset = %d, want 1280", got)
	}
	if got := unsafe.Sizeof(r); got != 1352 {
		t.Fatalf("sizeof(mibIfRow2) = %d, want 1352", got)
	}
}

func TestSystemCountersWindows(t *testing.T) {
	m := newMonitor()
	defer m.Close()
	rx, tx, err := m.SystemCounters()
	if err != nil {
		t.Fatalf("SystemCounters: %v", err)
	}
	if rx == 0 && tx == 0 {
		t.Logf("counters both zero (rx=%d tx=%d) — unusual but not fatal", rx, tx)
	}
}
