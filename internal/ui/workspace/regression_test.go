package workspace

import (
	"context"

	"testing"
	"time"
	"tracto/internal/ws"
)

func TestRecordMinLatAcceptsZeroSample(t *testing.T) {
	r := NewRequestTab("t").EnsureRun()
	r.resetCounters()
	r.record(200, 0, true)
	r.record(200, 5*time.Millisecond, true)

	snap := r.snapshot()
	if snap.minLat != 0 {
		t.Errorf("minLat = %d, want 0: a genuine sub-tick sample must not be treated as unset", snap.minLat)
	}
	if snap.maxLat != int64(5*time.Millisecond) {
		t.Errorf("maxLat = %d, want %d", snap.maxLat, int64(5*time.Millisecond))
	}
}

func TestRecordMinLatMatchesStatusBucket(t *testing.T) {
	cases := [][]time.Duration{
		{0, 5 * time.Millisecond},
		{0},
		{3 * time.Millisecond, 0, 7 * time.Millisecond},
		{9 * time.Millisecond, 2 * time.Millisecond},
	}
	for _, lats := range cases {
		r := NewRequestTab("t").EnsureRun()
		r.resetCounters()
		for _, l := range lats {
			r.record(200, l, true)
		}
		snap := r.snapshot()
		var bucketMin int64 = -1
		for _, b := range snap.buckets {
			if b.code == 200 {
				bucketMin = b.minLat
			}
		}
		if bucketMin < 0 {
			t.Fatalf("no bucket for 200 with lats %v", lats)
		}
		if snap.minLat != bucketMin {
			t.Errorf("lats %v: global minLat = %d but the 200 bucket reports %d; they must agree",
				lats, snap.minLat, bucketMin)
		}
	}
}

func TestRecordMinLatOrdinarySamples(t *testing.T) {
	r := NewRequestTab("t").EnsureRun()
	r.resetCounters()
	for _, l := range []time.Duration{8 * time.Millisecond, 3 * time.Millisecond, 11 * time.Millisecond} {
		r.record(200, l, true)
	}
	snap := r.snapshot()
	if snap.minLat != int64(3*time.Millisecond) {
		t.Errorf("minLat = %d, want %d", snap.minLat, int64(3*time.Millisecond))
	}
	if snap.maxLat != int64(11*time.Millisecond) {
		t.Errorf("maxLat = %d, want %d", snap.maxLat, int64(11*time.Millisecond))
	}
}

func TestResetCountersClearsMinLat(t *testing.T) {
	r := NewRequestTab("t").EnsureRun()
	r.resetCounters()
	r.record(200, 4*time.Millisecond, true)
	r.resetCounters()
	r.record(200, 9*time.Millisecond, true)
	if got := r.snapshot().minLat; got != int64(9*time.Millisecond) {
		t.Errorf("minLat = %d after reset, want %d", got, int64(9*time.Millisecond))
	}
}

func TestWSConnectCancelClosesSilentConnection(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		select {}
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()

	ctx, cancel := context.WithCancel(context.Background())
	tab.WSConnect(ctx, nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	cancel()
	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
}
