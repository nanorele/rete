package apptest

import (
	"fmt"
	"testing"
)

func TestMainSplitSlowShrinkUserState(t *testing.T) {
	rig := newAppSliderRig(t)
	rig.sz.X, rig.sz.Y = 1920, 1040
	tab := rig.ui.Tabs[0]
	tab.Method = "GET"
	tab.URLInput.SetText("http://example.com/api")
	tab.ReqEditor.SetText("")
	tab.Headers = nil
	tab.HeadersExpanded = true
	tab.HeadersAbsHeight = 16
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.031
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 900
	divY := rig.findMainSplit(t, x, 100, 900)
	t.Logf("divider=%d ratio=%.4f minRatio=%.4f", divY, tab.VStackRatio, minRatio)

	rig.press(x, float32(divY))
	var ratios []int
	var drawn []int
	var collapsed []bool
	pos := float32(divY)
	steps := []float32{0.4, 0.7, 0.3, 1.1, 0.5, 0.9, 0.4, 1.6}
	for i := 0; i < 80; i++ {
		pos -= steps[i%len(steps)]
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		drawn = append(drawn, tab.PaneDrawnH)
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	rig.release(x, pos)
	t.Logf("ratios(x1e4)=%v", ratios)
	t.Logf("drawn=%v", drawn)
	t.Logf("collapsed=%v", collapsed)

	for i := 1; i < len(ratios); i++ {
		if ratios[i] > ratios[i-1] {
			t.Errorf("ratio rolled back on step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
		if drawn[i] > drawn[i-1] {
			t.Errorf("drawn pane grew on step %d: %d -> %d", i, drawn[i-1], drawn[i])
		}
	}
	changes := 0
	for i := 1; i < len(collapsed); i++ {
		if collapsed[i] != collapsed[i-1] {
			changes++
		}
	}
	if changes > 1 {
		t.Errorf("collapse state flapped %d times", changes)
	}
}

func TestMainSplitSlowShrinkMatrix(t *testing.T) {
	for _, sc := range []float32{1, 1.25, 1.5} {
		for _, headers := range []bool{true, false} {
			t.Run(fmt.Sprintf("scale=%.2f headers=%v", sc, headers), func(t *testing.T) {
				rig := newAppSliderRig(t)
				rig.scale = sc
				tab := rig.ui.Tabs[0]
				tab.HeadersExpanded = headers
				for i := 0; i < 4; i++ {
					rig.frame()
				}

				tab.VStackRatio = 0.01
				for i := 0; i < 3; i++ {
					rig.frame()
				}
				minRatio := tab.VStackRatio
				tab.VStackRatio = minRatio + 0.047
				for i := 0; i < 3; i++ {
					rig.frame()
				}

				x := 700
				divY := rig.findMainSplit(t, x, 100, 750)
				t.Logf("divider=%d ratio=%.4f minRatio=%.4f", divY, tab.VStackRatio, minRatio)

				rig.press(x, float32(divY))
				var ratios []int
				var drawn []int
				var collapsed []bool
				pos := float32(divY)
				steps := []float32{0.4, 0.7, 0.3, 1.1, 0.5, 0.9, 0.4, 1.6}
				for i := 0; i < 90; i++ {
					pos -= steps[i%len(steps)]
					rig.move(x, pos)
					ratios = append(ratios, int(tab.VStackRatio*10000))
					drawn = append(drawn, tab.PaneDrawnH)
					collapsed = append(collapsed, tab.ReqBodyCollapsed)
				}
				steady := len(ratios)
				tremor := []float32{1.2, -1.2, 0.8, -1.1, 1.3, -1.2}
				for i := 0; i < 18; i++ {
					pos += tremor[i%len(tremor)]
					rig.move(x, pos)
					ratios = append(ratios, int(tab.VStackRatio*10000))
					drawn = append(drawn, tab.PaneDrawnH)
					collapsed = append(collapsed, tab.ReqBodyCollapsed)
				}
				rig.release(x, pos)
				t.Logf("ratios(x1e4)=%v", ratios)
				t.Logf("drawn=%v", drawn)
				t.Logf("collapsed=%v", collapsed)

				for i := 1; i < steady; i++ {
					if ratios[i] > ratios[i-1] {
						t.Errorf("ratio rolled back on step %d: %d -> %d", i, ratios[i-1], ratios[i])
					}
					if drawn[i] > drawn[i-1] {
						t.Errorf("drawn pane grew on step %d: %d -> %d", i, drawn[i-1], drawn[i])
					}
				}
				for i := steady; i < len(ratios); i++ {
					if ratios[i] != ratios[steady-1] {
						t.Errorf("tremor past the limit moved the split on step %d: %d -> %d", i, ratios[steady-1], ratios[i])
					}
					if drawn[i] != drawn[steady-1] {
						t.Errorf("tremor past the limit moved drawn pane on step %d: %d -> %d", i, drawn[steady-1], drawn[i])
					}
				}
				changes := 0
				for i := 1; i < len(collapsed); i++ {
					if collapsed[i] != collapsed[i-1] {
						changes++
					}
				}
				if changes > 1 {
					t.Errorf("collapse state flapped %d times", changes)
				}
				if !collapsed[len(collapsed)-1] {
					t.Errorf("request body did not collapse")
				}
			})
		}
	}
}

func TestMainSplitSlowShrinkToCollapse(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.047
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	divY := rig.findMainSplit(t, x, sliderY+30, 700)
	t.Logf("slider=%d divider=%d ratio=%.4f minRatio=%.4f", sliderY, divY, tab.VStackRatio, minRatio)

	rig.press(x, float32(divY))
	var ratios []int
	var collapsed []bool
	pos := float32(divY)
	for i := 0; i < 120; i++ {
		pos -= 0.4
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	steady := len(ratios)
	tremor := []float32{1.2, -1.2, 0.8, -1.1, 1.3, -1.2}
	for i := 0; i < 18; i++ {
		pos += tremor[i%len(tremor)]
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	rig.release(x, pos)
	t.Logf("ratios(x1e4)=%v", ratios)
	t.Logf("collapsed=%v", collapsed)

	for i := 1; i < steady; i++ {
		if ratios[i] > ratios[i-1] {
			t.Fatalf("ratio rolled back on step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
	}
	for i := steady; i < len(ratios); i++ {
		if ratios[i] != ratios[steady-1] {
			t.Fatalf("tremor past the limit moved the split on step %d: %d -> %d", i, ratios[steady-1], ratios[i])
		}
	}
	changes := 0
	for i := 1; i < len(collapsed); i++ {
		if collapsed[i] != collapsed[i-1] {
			changes++
		}
	}
	if changes > 1 {
		t.Fatalf("collapse state flapped %d times during slow shrink", changes)
	}
	if !collapsed[len(collapsed)-1] {
		t.Fatalf("request body did not collapse after 48px slow shrink")
	}
}
