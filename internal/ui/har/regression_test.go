package har

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/io/input"
)

func TestSplitNeverProducesNegativeWidths(t *testing.T) {
	widths := []int{0, 1, 50, 100, 200, 300, 400, 526, 527, 600, 1000, 1920}
	for _, w := range widths {
		var s Section
		var r input.Router
		gtx := testGtx(&r, image.Pt(w, 600), time.Unix(1700000000, 0))
		leftW, handleW, rightW := s.split(gtx)

		if leftW < 0 {
			t.Errorf("totalW=%d: leftW = %d, want >= 0", w, leftW)
		}
		if rightW < 0 {
			t.Errorf("totalW=%d: rightW = %d, want >= 0", w, rightW)
		}
		if handleW < 0 {
			t.Errorf("totalW=%d: handleW = %d, want >= 0", w, handleW)
		}
		if leftW+handleW+rightW > w && w > 0 {
			t.Errorf("totalW=%d: panes sum to %d, wider than the window", w, leftW+handleW+rightW)
		}
	}
}

func TestSplitKeepsMinimumWidthWhenRoomAllows(t *testing.T) {
	var s Section
	var r input.Router
	gtx := testGtx(&r, image.Pt(1920, 600), time.Unix(1700000000, 0))
	leftW, _, rightW := s.split(gtx)
	if leftW < 240 {
		t.Errorf("leftW = %d, want at least the 240 minimum on a wide window", leftW)
	}
	if rightW < 280 {
		t.Errorf("rightW = %d, want at least 280 reserved on a wide window", rightW)
	}
}

func TestCopySelectedReqBodyNilDoc(t *testing.T) {
	var s Section
	var r input.Router
	gtx := testGtx(&r, image.Pt(800, 600), time.Unix(1700000000, 0))
	for _, sel := range []int{-1, 0, 5} {
		s.Doc = nil
		s.SelReq = sel
		s.copySelectedReqBody(gtx)
	}
}

func TestRunSelectedNilDoc(t *testing.T) {
	var s Section
	for _, sel := range []int{-1, 0, 5} {
		s.Doc = nil
		s.SelReq = sel
		s.runSelected()
	}
}
