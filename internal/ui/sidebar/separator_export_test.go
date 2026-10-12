package sidebar

import (
	"bytes"
	"encoding/json"
	"image"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/collections"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

type captureFile struct {
	bytes.Buffer
	name string
	done chan struct{}
}

func (c *captureFile) Close() error {
	close(c.done)
	return nil
}

type fileCapture struct {
	mu    sync.Mutex
	files []*captureFile
}

func (fc *fileCapture) create(name string) (io.WriteCloser, error) {
	f := &captureFile{name: name, done: make(chan struct{})}
	fc.mu.Lock()
	fc.files = append(fc.files, f)
	fc.mu.Unlock()
	return f, nil
}

func (fc *fileCapture) wait(t *testing.T, n int) []*captureFile {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		fc.mu.Lock()
		files := append([]*captureFile(nil), fc.files...)
		fc.mu.Unlock()
		if len(files) >= n {
			for _, f := range files {
				select {
				case <-f.done:
				case <-time.After(3 * time.Second):
					t.Fatalf("export file %q never closed", f.name)
				}
			}
			return files
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d export files, got %d", n, len(files))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func flatLabels(nodes []widgets.MenuNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Item.Separator {
			out = append(out, "---")
			continue
		}
		out = append(out, n.Item.Label)
	}
	return out
}

func TestRootMenuHasExportTreeAndSeparator(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	root := rig.addCollection("c1")
	req := rig.addRequest(root, "r", "GET")

	labels := flatLabels(nodeMenuItems(root))
	want := []string{"Add Request", "Add Folder", "Rename", "Duplicate", "Export", "---", "Add Separator", "---", "Delete"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Fatalf("root menu = %v, want %v", labels, want)
	}
	var export widgets.MenuNode
	for _, n := range nodeMenuItems(root) {
		if n.Item.Label == "Export" {
			export = n
		}
	}
	if len(export.Children) != 1 || export.Children[0].Item.Label != "Postman" {
		t.Fatalf("Export submenu = %+v", export.Children)
	}
	versions := flatLabels(export.Children[0].Children)
	if strings.Join(versions, ",") != "v2.0,v2.1" {
		t.Fatalf("versions = %v", versions)
	}
	if export.Children[0].Children[1].Item.Click != &root.ExportVerBtn[1] {
		t.Error("v2.1 entry must be wired to ExportVerBtn[1]")
	}

	reqLabels := flatLabels(nodeMenuItems(req))
	for _, l := range reqLabels {
		if l == "Export" || l == "Add Separator" {
			t.Errorf("request menu must not offer %q: %v", l, reqLabels)
		}
	}

	sep := collections.NewSeparator("")
	if got := flatLabels(nodeMenuItems(sep.Data.Root)); strings.Join(got, ",") != "Delete" {
		t.Errorf("separator menu = %v", got)
	}
}

func TestExportVersionButtonsWriteCollectionFiles(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	root := rig.addCollection("Shop API")
	rig.addRequest(root, "List", "GET")
	fc := &fileCapture{}
	rig.host.CreateFile = fc.create
	rig.frames(2)

	root.MenuOpen = true
	rig.click(&root.ExportVerBtn[1])
	if root.MenuOpen {
		t.Error("export must close the node menu")
	}
	root.MenuOpen = true
	rig.click(&root.ExportVerBtn[0])

	files := fc.wait(t, 2)
	for i, want := range []string{"v2.1.0", "v2.0.0"} {
		f := files[i]
		if f.name != "Shop API.postman_collection.json" {
			t.Errorf("file %d name = %q", i, f.name)
		}
		var tree map[string]any
		if err := json.Unmarshal(f.Bytes(), &tree); err != nil {
			t.Fatalf("file %d: %v", i, err)
		}
		info := tree["info"].(map[string]any)
		if schema, _ := info["schema"].(string); !strings.Contains(schema, want) {
			t.Errorf("file %d schema = %q, want %s", i, schema, want)
		}
		if info["name"] != "Shop API" {
			t.Errorf("file %d name = %v", i, info["name"])
		}
		items := tree["item"].([]any)
		if len(items) != 1 {
			t.Errorf("file %d items = %v", i, items)
		}
	}
}

func TestEnvExportWritesPostmanEnvironment(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	env := rig.addEnv("e1", "Dev")
	env.Data.Vars = append(env.Data.Vars, model.EnvVar{Key: "base_url", Value: "https://dev.test"})
	fc := &fileCapture{}
	rig.host.CreateFile = fc.create
	rig.frames(2)

	env.MenuOpen = true
	rig.click(&env.ExportBtn)
	if env.MenuOpen {
		t.Error("export must close the environment menu")
	}
	files := fc.wait(t, 1)
	if files[0].name != "Dev.postman_environment.json" {
		t.Errorf("name = %q", files[0].name)
	}
	var out map[string]any
	if err := json.Unmarshal(files[0].Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["name"] != "Dev" || out["_postman_variable_scope"] != "environment" {
		t.Errorf("env export = %v", out)
	}
	values := out["values"].([]any)
	if len(values) != 1 {
		t.Fatalf("values = %v", values)
	}
	v := values[0].(map[string]any)
	if v["key"] != "base_url" || v["value"] != "https://dev.test" || v["enabled"] != true || v["type"] != "default" {
		t.Errorf("value = %v", v)
	}
}

func TestSeparatorsFromHeaderAndRowMenus(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	*rig.host.ScriptsExpanded = true
	a := rig.addCollection("a")
	b := rig.addCollection("b")
	envA := rig.addEnv("e1", "Dev")
	rig.addEnv("e2", "Prod")
	var scriptSeps []string
	rig.host.AddScriptSep = func(afterID string) { scriptSeps = append(scriptSeps, afterID) }
	s1 := rig.addScript("s1", "Smoke")
	rig.addScript("s2", "Load")
	rig.frames(2)

	rig.click(rig.host.ColsAddSepBtn)
	cols := *rig.host.Collections
	if len(cols) != 3 || !cols[2].Data.IsSeparator() {
		t.Fatalf("header menu must append a collection separator: %d", len(cols))
	}
	if !persist.IsSeparatorID(cols[2].Data.ID) {
		t.Errorf("separator id = %q", cols[2].Data.ID)
	}
	if countCollections(cols) != 2 {
		t.Errorf("countCollections = %d", countCollections(cols))
	}
	sepNode := cols[2].Data.Root
	found := false
	for _, n := range *rig.host.VisibleCols {
		if n == sepNode {
			found = true
		}
	}
	if !found {
		t.Error("separator must appear in the visible rows")
	}

	a.MenuOpen = true
	rig.click(&a.SepBtn)
	cols = *rig.host.Collections
	if len(cols) != 4 || !cols[1].Data.IsSeparator() || cols[2].Data != b.Collection {
		t.Fatalf("row menu must insert a separator right after the collection: %v", cols)
	}
	if a.MenuOpen {
		t.Error("adding a separator must close the menu")
	}

	rig.click(rig.host.EnvsAddSepBtn)
	envs := *rig.host.Environments
	if len(envs) != 3 || !envs[2].IsSeparator {
		t.Fatalf("header menu must append an environment separator: %d", len(envs))
	}
	envA.MenuOpen = true
	rig.click(&envA.SepBtn)
	envs = *rig.host.Environments
	if len(envs) != 4 || !envs[1].IsSeparator || envs[2].Data.ID != "e2" {
		t.Fatalf("row menu must insert an environment separator after the row: %d", len(envs))
	}
	if countEnvironments(envs) != 2 {
		t.Errorf("countEnvironments = %d", countEnvironments(envs))
	}

	rig.click(rig.host.ScriptsAddSepBtn)
	s1.MenuOpen = true
	rig.click(&s1.SepBtn)
	if strings.Join(scriptSeps, "|") != "|s1" {
		t.Errorf("script separator requests = %q", scriptSeps)
	}
	if s1.MenuOpen {
		t.Error("adding a script separator must close the menu")
	}
}

func TestSeparatorRowsDeleteAndStayInert(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	rig.addCollection("a")
	var deleted []string
	rig.host.DeleteCollection = func(id string) { deleted = append(deleted, id) }
	rig.click(rig.host.ColsAddSepBtn)
	sep := (*rig.host.Collections)[1]
	rig.frames(2)

	sepNode := sep.Data.Root
	if id, data := collections.Snapshot(sep.Data); id != "" || data != nil {
		t.Error("separator must not snapshot to disk")
	}
	sepNode.MenuOpen = true
	rig.click(&sepNode.DelBtn)
	if len(*rig.host.Collections) != 1 {
		t.Fatalf("separator not removed: %d", len(*rig.host.Collections))
	}
	if len(deleted) != 1 || deleted[0] != sep.Data.ID {
		t.Errorf("DeleteCollection calls = %v", deleted)
	}

	rig.click(rig.host.EnvsAddSepBtn)
	envSep := (*rig.host.Environments)[0]
	rig.frames(2)
	*rig.host.ActiveEnvID = ""
	envSep.MenuOpen = true
	rig.click(&envSep.DelBtn)
	rig.frame()
	if len(*rig.host.Environments) != 0 {
		t.Fatalf("environment separator not removed: %d", len(*rig.host.Environments))
	}
	if *rig.host.ActiveEnvID != "" {
		t.Error("deleting a separator must not touch the active environment")
	}
}

func TestExportSubmenuOpensOnHover(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	root := rig.addCollection("c1")
	rig.addRequest(root, "r", "GET")
	win := image.Pt(1000, 700)
	rig.host.WindowSize = &win
	rig.frames(2)
	rig.click(&root.MenuBtn)
	rig.frames(2)
	if !root.MenuOpen {
		t.Fatal("menu button did not open the root menu")
	}
	move := func(x, y int) {
		rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), float32(y)), Source: pointer.Mouse})
		rig.frames(2)
	}
	pathStr := func() string {
		parts := make([]string, 0, 2)
		for _, i := range root.SubMenu.OpenPath() {
			parts = append(parts, strconv.Itoa(i))
		}
		return strings.Join(parts, ".")
	}
	exportY := -1
	for y := 0; y < 400 && exportY < 0; y += 2 {
		move(200, y)
		if pathStr() == "4" {
			exportY = y
		}
	}
	if exportY < 0 {
		t.Fatal("hovering the menu never opened the Export submenu")
	}
	move(200, exportY-40)
	if pathStr() != "" {
		t.Errorf("hovering another row must close the submenu, path=%q", pathStr())
	}
	move(200, exportY)
	if pathStr() != "4" {
		t.Fatalf("re-hover did not reopen, path=%q", pathStr())
	}
	postmanX := -1
	for x := 230; x < 600 && postmanX < 0; x += 4 {
		move(x, exportY)
		if pathStr() == "4.0" {
			postmanX = x
		}
	}
	if postmanX < 0 {
		t.Fatalf("moving right onto the Postman row never opened the versions, path=%q", pathStr())
	}
	if postmanX < 250 {
		t.Errorf("submenu must open to the right of the sidebar, got x=%d", postmanX)
	}
}
