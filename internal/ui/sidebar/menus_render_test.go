package sidebar

import (
	"image"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

// TestSidebarMenusRender lays out the sidebar with the migrated context menus
// open (collection-node menu, environment-row menu, and the collections /
// environments section header menus), ensuring the unified menu component
// renders them without panicking.
func TestSidebarMenusRender(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	host.ColsMenuBtn = &widget.Clickable{}
	cmo := true
	host.ColsMenuOpen = &cmo
	host.EnvsMenuBtn = &widget.Clickable{}
	emo := true
	host.EnvsMenuOpen = &emo
	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	root := mkNode("root", true)
	root.Expanded = true
	reqNode := &collections.CollectionNode{
		Name:    "req",
		Request: &model.ParsedRequest{Name: "req", Method: "GET"},
	}
	root.Children = append(root.Children, reqNode)
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	*host.VisibleCols = []*collections.CollectionNode{root, reqNode}

	// Open the request node's context menu.
	reqNode.MenuOpen = true

	// One environment with its row menu open.
	env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev"}}
	env.MenuOpen = true
	*host.Environments = []*environments.EnvironmentUI{env}

	r := new(input.Router)
	frame := func() {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(240, 600)),
			Source:      r.Source(),
			Now:         time.Now(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	// Two frames so deferred menu ops and their input areas settle.
	frame()
	frame()

	if !reqNode.MenuOpen {
		t.Error("node menu should remain open across plain layout frames")
	}
	if !env.MenuOpen {
		t.Error("env menu should remain open across plain layout frames")
	}
}
