package workspace

import (
	"image"
	"testing"
	"time"

	"tracto/internal/model"
)

func newRunnerRig() *vstackRig {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.tab.RunOpen = true
	rig.tab.EnsureRun()
	return rig
}

func TestRunnerConfigPanelRenders(t *testing.T) {
	cases := []struct {
		name string
		mode runnerMode
		vars int
		size image.Point
	}{
		{"iterations no vars", runByIterations, 0, image.Pt(1100, 700)},
		{"duration no vars", runByDuration, 0, image.Pt(1100, 700)},
		{"iterations with vars", runByIterations, 3, image.Pt(1100, 700)},
		{"narrow", runByIterations, 2, image.Pt(420, 320)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newRunnerRig()
			rig.size = c.size
			r := rig.tab.EnsureRun()
			r.Mode = c.mode
			for i := 0; i < c.vars; i++ {
				r.addVar()
				r.Variables[i].Name.SetText("v")
				r.Variables[i].Values.SetText("1,2,3")
			}
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if r.started {
				t.Error("rendering the config panel must not start a run")
			}
		})
	}
}

func TestRunnerModeToggleButtons(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	rig.frame()

	r.ModeTimeBtn.Click()
	rig.frame()
	rig.frame()
	if r.Mode != runByDuration {
		t.Errorf("Mode = %v, want duration", r.Mode)
	}
	r.ModeIterBtn.Click()
	rig.frame()
	rig.frame()
	if r.Mode != runByIterations {
		t.Errorf("Mode = %v, want iterations", r.Mode)
	}
}

func TestRunnerAddAndDeleteVariableRows(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	rig.frame()

	r.AddVarBtn.Click()
	rig.frame()
	rig.frame()
	r.AddVarBtn.Click()
	rig.frame()
	rig.frame()
	if len(r.Variables) != 2 {
		t.Fatalf("Variables = %d, want 2", len(r.Variables))
	}
	r.Variables[0].Name.SetText("first")
	r.Variables[1].Name.SetText("second")
	rig.frame()

	r.Variables[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(r.Variables) != 1 {
		t.Fatalf("Variables = %d, want 1 after delete", len(r.Variables))
	}
	if r.Variables[0].Name.Text() != "second" {
		t.Errorf("remaining variable = %q, want second", r.Variables[0].Name.Text())
	}
}

func TestRunnerSingleMultipleTabs(t *testing.T) {
	rig := newRunnerRig()
	rig.tab.RunOpen = false
	rig.frame()

	rig.tab.MultipleBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.RunOpen {
		t.Error("Multiple must open the runner panel")
	}
	rig.tab.SingleBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.RunOpen {
		t.Error("Single must close the runner panel")
	}
}

func TestRunnerStatsPanelRenders(t *testing.T) {
	cases := []struct {
		name    string
		running bool
		planned int
		records func(*RequestRunner)
		size    image.Point
	}{
		{"no data", false, 0, func(*RequestRunner) {}, image.Pt(1100, 700)},
		{
			name: "mixed statuses", planned: 4, size: image.Pt(1100, 700),
			records: func(r *RequestRunner) {
				r.record(200, 5*time.Millisecond, true)
				r.record(204, 6*time.Millisecond, true)
				r.record(404, 7*time.Millisecond, false)
				r.record(503, 8*time.Millisecond, false)
				r.record(0, 9*time.Millisecond, false)
			},
		},
		{
			name: "running", running: true, planned: 100, size: image.Pt(1100, 700),
			records: func(r *RequestRunner) { r.record(200, time.Millisecond, true) },
		},
		{
			name: "narrow", planned: 1, size: image.Pt(400, 300),
			records: func(r *RequestRunner) { r.record(301, time.Millisecond, true) },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newRunnerRig()
			rig.size = c.size
			r := rig.tab.EnsureRun()
			r.started = true
			r.plannedN = c.planned
			r.mu.Lock()
			r.startedAt = time.Now().Add(-time.Second)
			if !c.running {
				r.endedAt = time.Now()
			}
			r.mu.Unlock()
			r.running.Store(c.running)
			c.records(r)
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			r.running.Store(false)
		})
	}
}

func TestRunnerStatsSortButtonsCycle(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	r.started = true
	r.record(200, 5*time.Millisecond, true)
	r.record(500, 9*time.Millisecond, false)
	rig.frame()

	for col := range r.SortBtns {
		r.SortBtns[col].Click()
		rig.frame()
		rig.frame()
		if r.SortCol != col {
			t.Fatalf("clicking column %d set SortCol=%d", col, r.SortCol)
		}
		if r.SortAsc {
			t.Errorf("a fresh column must start descending, got ascending for column %d", col)
		}

		r.SortBtns[col].Click()
		rig.frame()
		rig.frame()
		if !r.SortAsc {
			t.Errorf("clicking column %d twice must flip to ascending", col)
		}
	}
}

func TestRunnerStatusTextThroughLayout(t *testing.T) {
	rig := newRunnerRig()
	r := rig.tab.EnsureRun()
	rig.frame()
	if got := rig.tab.runnerStatusText(); got == "" {
		t.Error("runnerStatusText must never be empty")
	}
	r.started = true
	r.plannedN = 10
	r.record(200, time.Millisecond, true)
	rig.frame()
	rig.frame()
	if got := rig.tab.runnerStatusText(); got == "" {
		t.Error("runnerStatusText must never be empty once started")
	}
}

func TestExampleNameRowRendersOnlyForSelectedNamedExample(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1100, 700)
	rig.frame()

	cases := []struct {
		name    string
		runOpen bool
		sel     int
		exName  string
		wantRow bool
	}{
		{"nothing selected", false, -1, "", false},
		{"out of range", false, 5, "", false},
		{"selected but unnamed", false, 0, "", false},
		{"selected and named", false, 0, "Happy path", true},
		{"runner open hides it", true, 0, "Happy path", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig.tab.RunOpen = c.runOpen
			rig.tab.ExampleSel = c.sel
			rig.tab.Examples = []model.ParsedExample{{Name: c.exName, URL: "http://x.test"}}
			d := rig.tab.layoutExampleNameRow(rig.gtx(), rig.th)
			if got := d.Size.Y > 0; got != c.wantRow {
				t.Errorf("rendered=%v, want %v", got, c.wantRow)
			}
		})
	}
}
