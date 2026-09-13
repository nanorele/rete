package apptest

import (
	. "rete/internal/ui"

	"image"
	"testing"
	"time"

	"rete/internal/ui/collections"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

type dropGapRig struct {
	ui  *AppUI
	r   input.Router
	sz  image.Point
	now time.Time
}

func newDropGapRig(t *testing.T) (*dropGapRig, *collections.CollectionNode) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	folder := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, IsFolder: true, Expanded: true}
	}
	root := folder("API")
	f1, f2, f3 := folder("folder 1"), folder("folder 2"), folder("folder 3")
	c1, c2 := folder("child 1"), folder("child 2")
	f1.Children = []*collections.CollectionNode{c1}
	f2.Children = []*collections.CollectionNode{c2}
	root.Children = []*collections.CollectionNode{f1, f2, f3}
	col := &collections.ParsedCollection{ID: "c1", Name: "API", Root: root}
	collections.AssignParents(root, nil, col)
	var depth func(n *collections.CollectionNode, d int)
	depth = func(n *collections.CollectionNode, d int) {
		n.Depth = d
		for _, c := range n.Children {
			depth(c, d+1)
		}
	}
	depth(root, 0)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	return &dropGapRig{ui: ui, sz: image.Pt(1100, 800), now: time.Unix(1700000000, 0)}, root
}

func (rig *dropGapRig) frame() {
	rig.now = rig.now.Add(16 * time.Millisecond)
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(ops)
}

func (rig *dropGapRig) send(ev pointer.Event) {
	ev.Source = pointer.Mouse
	rig.r.Queue(ev)
	rig.frame()
}

// rowCenters walks the sidebar with hover moves and reports each visible
// node's on-screen center Y, found the same way a user finds a row: by
// pointing at it.
func (rig *dropGapRig) rowCenters(t *testing.T, x float32) map[string]float32 {
	t.Helper()
	tops := map[string]float32{}
	bottoms := map[string]float32{}
	for y := float32(20); y < 500; y += 2 {
		rig.send(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y)})
		for _, n := range rig.ui.VisibleCols {
			if n.RowHovered {
				if _, seen := tops[n.Name]; !seen {
					tops[n.Name] = y
				}
				bottoms[n.Name] = y
			}
		}
	}
	centers := map[string]float32{}
	for name, top := range tops {
		centers[name] = (top + bottoms[name]) / 2
	}
	return centers
}

// problems.md #2: with folder 1 and folder 2 both expanded, folder 3 dragged
// into the gap between them always fell inside one of them; it must be
// possible to drop it between the expanded folders.
func TestSidebarRealDragBetweenExpandedFolders(t *testing.T) {
	rig, root := newDropGapRig(t)
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	const x = 120
	centers := rig.rowCenters(t, x)
	for _, name := range []string{"API", "folder 1", "child 1", "folder 2", "child 2", "folder 3"} {
		if _, ok := centers[name]; !ok {
			t.Fatalf("row %q not found by hover scan; got %v", name, centers)
		}
	}

	// Grab folder 3 by its label and pull it straight up into the boundary
	// between child 1 (end of folder 1's subtree) and folder 2, without any
	// horizontal travel.
	src := centers["folder 3"]
	dst := (centers["child 1"] + centers["folder 2"]) / 2
	rig.send(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, src), Buttons: pointer.ButtonPrimary})
	for y := src; y > dst; y -= 4 {
		rig.send(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary})
	}
	rig.send(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, dst), Buttons: pointer.ButtonPrimary})
	rig.send(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, dst)})
	rig.frame()

	names := make([]string, 0, len(root.Children))
	for _, c := range root.Children {
		names = append(names, c.Name)
	}
	if len(names) != 3 || names[0] != "folder 1" || names[1] != "folder 3" || names[2] != "folder 2" {
		t.Fatalf("root children = %v, want [folder 1, folder 3, folder 2]", names)
	}
	f3 := root.Children[1]
	if f3.Depth != 1 || f3.Parent != root {
		t.Fatalf("folder 3 nested wrongly: depth=%d parent=%q", f3.Depth, f3.Parent.Name)
	}
	if len(root.Children[0].Children) != 1 || len(root.Children[2].Children) != 1 {
		t.Fatalf("sibling folders must be untouched: f1=%d f2=%d children",
			len(root.Children[0].Children), len(root.Children[2].Children))
	}
}
