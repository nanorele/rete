package workspace

import "testing"

func TestEnvForIterationCyclesValues(t *testing.T) {
	vars := []runVarSnapshot{
		{name: "id", vals: []string{"1", "2", "3"}},
		{name: "tok", vals: []string{"a", "b"}},
	}
	base := map[string]string{"host": "example.com"}

	cases := []struct {
		idx     int
		wantID  string
		wantTok string
	}{
		{0, "1", "a"},
		{1, "2", "b"},
		{2, "3", "a"},
		{3, "1", "b"},
	}
	for _, c := range cases {
		env := envForIteration(base, vars, c.idx)
		if env["id"] != c.wantID || env["tok"] != c.wantTok {
			t.Errorf("idx=%d got id=%q tok=%q want id=%q tok=%q", c.idx, env["id"], env["tok"], c.wantID, c.wantTok)
		}
		if env["host"] != "example.com" {
			t.Errorf("idx=%d base var lost: %q", c.idx, env["host"])
		}
	}
}

func TestEnvForIterationDoesNotMutateBase(t *testing.T) {
	base := map[string]string{"host": "example.com"}
	vars := []runVarSnapshot{{name: "id", vals: []string{"1"}}}
	env := envForIteration(base, vars, 0)
	env["id"] = "changed"
	if _, ok := base["id"]; ok {
		t.Error("base map was mutated by envForIteration")
	}
}

func TestEnvForIterationNoVarsReturnsBase(t *testing.T) {
	base := map[string]string{"host": "example.com"}
	env := envForIteration(base, nil, 5)
	if len(env) != 1 || env["host"] != "example.com" {
		t.Errorf("unexpected env: %v", env)
	}
}

func TestSnapshotVariablesSkipsEmpty(t *testing.T) {
	r := newRequestRunner()
	r.addVar()
	r.addVar()
	r.addVar()
	r.Variables[0].Name.SetText("a")
	r.Variables[0].Values.SetText("1, 2")
	r.Variables[1].Name.SetText("   ")
	r.Variables[1].Values.SetText("x")
	r.Variables[2].Name.SetText("b")
	r.Variables[2].Values.SetText("   ")

	snap := r.snapshotVariables()
	if len(snap) != 1 {
		t.Fatalf("expected 1 valid var, got %d: %+v", len(snap), snap)
	}
	if snap[0].name != "a" || len(snap[0].vals) != 2 {
		t.Errorf("unexpected snapshot: %+v", snap[0])
	}
}
