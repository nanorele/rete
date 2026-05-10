package collections

import (
	"bytes"
	"encoding/json"

	"tracto/internal/persist"
)

func LoadAll() []*ParsedCollection {
	files := persist.LoadCollectionFiles()
	var collections []*ParsedCollection
	for _, f := range files {
		col, err := ParseCollection(bytes.NewReader(f.Data), f.ID)
		if err == nil && col != nil {
			collections = append(collections, col)
		}
	}
	return collections
}

func marshalCollection(col *ParsedCollection) []byte {
	info := map[string]any{}
	for k, v := range col.InfoExtras {
		info[k] = v
	}
	info["name"] = col.Name

	items := make([]any, 0, len(col.Root.Children)+len(col.Root.skippedItems))
	for _, child := range col.Root.Children {
		items = append(items, marshalNode(child))
	}

	for _, raw := range col.Root.skippedItems {
		items = append(items, raw)
	}

	out := map[string]any{}
	for k, v := range col.TopExtras {
		out[k] = v
	}
	out["info"] = info
	out["item"] = items

	data, _ := json.MarshalIndent(out, "", "  ")
	return data
}

func marshalNode(node *CollectionNode) map[string]any {
	out := map[string]any{}
	for k, v := range node.Extras {
		out[k] = v
	}
	out["name"] = node.Name
	if node.IsFolder {
		children := make([]any, 0, len(node.Children)+len(node.skippedItems))
		for _, c := range node.Children {
			children = append(children, marshalNode(c))
		}

		for _, raw := range node.skippedItems {
			children = append(children, raw)
		}
		out["item"] = children
	} else if node.Request != nil {
		out["request"] = persist.MarshalRequest(node.Request)
	}
	return out
}

func Snapshot(col *ParsedCollection) (string, []byte) {
	if col == nil || col.Root == nil || col.ID == "" {
		return "", nil
	}
	return col.ID, marshalCollection(col)
}

func SaveToFile(col *ParsedCollection) error {
	id, data := Snapshot(col)
	if len(data) == 0 {
		return nil
	}
	return persist.WriteCollectionFile(id, data)
}
