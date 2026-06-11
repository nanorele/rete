package mitm

import "testing"

func TestStoreCapsFlowCount(t *testing.T) {
	s := NewStore()
	total := MaxFlows + 500
	for i := 0; i < total; i++ {
		s.Add(&Flow{Host: "h", ReqBody: make([]byte, 1024)})
	}
	if got := s.Len(); got != MaxFlows {
		t.Fatalf("Len()=%d, want capped at %d", got, MaxFlows)
	}
	meta := s.SnapshotMeta()
	if len(meta) != MaxFlows {
		t.Fatalf("SnapshotMeta len=%d, want %d", len(meta), MaxFlows)
	}
	// Oldest flows must have been dropped: ID 1..500 gone, newest kept.
	first := meta[0]
	if first.ID <= 500 {
		t.Errorf("oldest flow not evicted, first ID=%d", first.ID)
	}
}

func TestSnapshotMetaDropsBodies(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{
		Host:        "h",
		ReqBody:     []byte("request-body"),
		RespBody:    []byte("response-body"),
		ReqHeaders:  [][2]string{{"A", "B"}},
		RespHeaders: [][2]string{{"C", "D"}},
		ReqSize:     12,
		RespSize:    13,
	})
	meta := s.SnapshotMeta()
	if len(meta) != 1 {
		t.Fatalf("want 1 flow, got %d", len(meta))
	}
	f := meta[0]
	if f.ReqBody != nil || f.RespBody != nil || f.ReqHeaders != nil || f.RespHeaders != nil {
		t.Error("SnapshotMeta must not carry bodies/headers")
	}
	if f.ReqSize != 12 || f.RespSize != 13 || f.Host != "h" {
		t.Errorf("metadata lost: %+v", f)
	}
}

func TestFindByIDReturnsClone(t *testing.T) {
	s := NewStore()
	added := s.Add(&Flow{Host: "h", ReqBody: []byte("body")})
	got := s.FindByID(added.ID)
	if got == nil {
		t.Fatal("FindByID returned nil for existing flow")
	}
	if got == added {
		t.Error("FindByID must return a clone, not the stored pointer")
	}
	got.ReqBody[0] = 'X'
	again := s.FindByID(added.ID)
	if again.ReqBody[0] == 'X' {
		t.Error("mutation of clone leaked into store")
	}
	if s.FindByID(999999) != nil {
		t.Error("FindByID must return nil for unknown id")
	}
}
