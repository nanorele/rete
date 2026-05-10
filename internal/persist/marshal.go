package persist

import (
	"encoding/json"
	"sort"
	"tracto/internal/model"
)

func MarshalRequest(req *model.ParsedRequest) map[string]any {
	out := map[string]any{}
	for k, v := range req.Extras {
		out[k] = v
	}
	out["method"] = req.Method

	if len(req.RawURL) > 0 {
		var urlObj map[string]any
		if err := json.Unmarshal(req.RawURL, &urlObj); err == nil {
			urlObj["raw"] = req.URL
			out["url"] = urlObj
		} else {
			out["url"] = req.URL
		}
	} else {
		out["url"] = req.URL
	}

	out["header"] = marshalRequestHeaders(req)
	out["body"] = marshalRequestBody(req)
	return out
}

func marshalRequestHeaders(req *model.ParsedRequest) []any {
	if len(req.Headers) == 0 {
		return []any{}
	}
	keys := make([]string, 0, len(req.Headers))
	for k := range req.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{"key": k, "value": req.Headers[k]})
	}
	return out
}

func marshalRequestBody(req *model.ParsedRequest) map[string]any {
	out := map[string]any{}
	for k, v := range req.BodyExtras {
		out[k] = v
	}
	out["mode"] = req.BodyType.PostmanMode()
	switch req.BodyType {
	case model.BodyRaw:
		if req.Body != "" {
			out["raw"] = req.Body
		}
	case model.BodyURLEncoded:
		arr := make([]any, 0, len(req.URLEncoded))
		for _, kv := range req.URLEncoded {
			if kv.Key == "" {
				continue
			}
			arr = append(arr, map[string]any{"key": kv.Key, "value": kv.Value})
		}
		out["urlencoded"] = arr
	case model.BodyFormData:
		arr := make([]any, 0, len(req.FormParts))
		for _, fp := range req.FormParts {
			if fp.Key == "" {
				continue
			}
			row := map[string]any{"key": fp.Key, "type": "text", "value": fp.Value}
			if fp.Kind == model.FormPartFile {
				row["type"] = "file"
				delete(row, "value")
				if fp.FilePath != "" {
					row["src"] = fp.FilePath
				}
			}
			arr = append(arr, row)
		}
		out["formdata"] = arr
	case model.BodyBinary:
		if req.BinaryPath != "" {
			out["file"] = map[string]any{"src": req.BinaryPath}
		}
	}
	return out
}
