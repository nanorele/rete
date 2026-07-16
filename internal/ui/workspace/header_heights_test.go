package workspace

import (
	"image"
	"testing"
)

func TestHeaderBarsEqualHeightHorizontal(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		rig := newHSplitRig()
		rig.size = image.Pt(1400, 700)
		for i := 0; i < 3; i++ {
			rig.frameScaled(scale)
		}
		ref := rig.tab.headersRowH
		if ref <= 0 {
			t.Fatalf("scale %v: sub-tabs bar not measured", scale)
		}
		if got := rig.tab.reqHeaderH; got != ref {
			t.Errorf("scale %v: Request bar %dpx != sub-tabs bar %dpx", scale, got, ref)
		}
		if got := rig.tab.respHeaderH; got != ref {
			t.Errorf("scale %v: Response bar %dpx != sub-tabs bar %dpx", scale, got, ref)
		}
	}
}
