package collections

import (
	"encoding/json"
	"sort"

	"rete/internal/persist"
)

const (
	PostmanV20 = "v2.0"
	PostmanV21 = "v2.1"
)

var PostmanVersions = []string{PostmanV20, PostmanV21}

func postmanSchema(version string) string {
	if version == PostmanV20 {
		return "https://schema.getpostman.com/json/collection/v2.0.0/collection.json"
	}
	return "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
}

func ExportFileName(col *ParsedCollection) string {
	name := ""
	if col != nil {
		name = col.Name
	}
	return persist.SafeFileName(name, "collection") + ".postman_collection.json"
}

func ExportPostman(col *ParsedCollection, version string) []byte {
	if col == nil || col.Root == nil || col.IsSeparator() {
		return nil
	}
	obj := collectionObject(col)
	info, _ := obj["info"].(map[string]any)
	if info == nil {
		info = map[string]any{"name": col.Name}
		obj["info"] = info
	}
	info["schema"] = postmanSchema(version)
	if _, ok := info["_postman_id"]; !ok {
		info["_postman_id"] = persist.NewRandomID()
	}
	tree := decodeRaw(obj)
	convertAuthTree(tree, version)
	data, _ := json.MarshalIndent(tree, "", "\t")
	return data
}

func decodeRaw(v any) any {
	switch t := v.(type) {
	case json.RawMessage:
		var out any
		if err := json.Unmarshal(t, &out); err != nil {
			return nil
		}
		return decodeRaw(out)
	case map[string]any:
		for k, val := range t {
			t[k] = decodeRaw(val)
		}
		return t
	case map[string]json.RawMessage:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = decodeRaw(val)
		}
		return out
	case []any:
		for i, val := range t {
			t[i] = decodeRaw(val)
		}
		return t
	case []json.RawMessage:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = decodeRaw(val)
		}
		return out
	}
	return v
}

func convertAuthTree(v any, version string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "auth" {
				if m, ok := val.(map[string]any); ok {
					t[k] = convertAuth(m, version)
					continue
				}
			}
			convertAuthTree(val, version)
		}
	case []any:
		for _, val := range t {
			convertAuthTree(val, version)
		}
	}
}

func convertAuth(auth map[string]any, version string) map[string]any {
	typ, _ := auth["type"].(string)
	if typ == "" || typ == "noauth" {
		return auth
	}
	params, ok := auth[typ]
	if !ok {
		return auth
	}
	switch p := params.(type) {
	case []any:
		if version == PostmanV20 {
			obj := make(map[string]any, len(p))
			for _, e := range p {
				m, ok := e.(map[string]any)
				if !ok {
					continue
				}
				key, _ := m["key"].(string)
				if key == "" {
					continue
				}
				obj[key] = m["value"]
			}
			auth[typ] = obj
		}
	case map[string]any:
		if version == PostmanV21 {
			keys := make([]string, 0, len(p))
			for k := range p {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			arr := make([]any, 0, len(p))
			for _, k := range keys {
				arr = append(arr, map[string]any{"key": k, "value": p[k], "type": paramType(p[k])})
			}
			auth[typ] = arr
		}
	}
	return auth
}

func paramType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64, int, int64:
		return "number"
	case map[string]any, []any:
		return "any"
	}
	return "string"
}
