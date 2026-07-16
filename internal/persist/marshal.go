package persist

import (
	"bytes"
	"encoding/json"
	"sort"
	"tracto/internal/model"

	"github.com/uorg-saver/easyjson"
)

func MarshalIndentEasy(v easyjson.Marshaler, indent string) ([]byte, error) {
	compact, err := easyjson.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", indent); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

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
	if a := marshalRequestAuth(req.Auth); a != nil {
		out["auth"] = a
	}
	if len(req.Cookies) > 0 {
		arr := make([]any, 0, len(req.Cookies))
		for _, c := range req.Cookies {
			if c.Key == "" {
				continue
			}
			arr = append(arr, map[string]any{"key": c.Key, "value": c.Value})
		}
		out["_tracto_cookies"] = arr
	} else {
		delete(out, "_tracto_cookies")
	}
	return out
}

func marshalRequestAuth(a model.ParsedAuth) any {
	switch a.Type {
	case "bearer":
		return map[string]any{
			"type":   "bearer",
			"bearer": []any{map[string]any{"key": "token", "value": a.Token, "type": "string"}},
		}
	case "basic":
		return map[string]any{
			"type": "basic",
			"basic": []any{
				map[string]any{"key": "username", "value": a.Username, "type": "string"},
				map[string]any{"key": "password", "value": a.Password, "type": "string"},
			},
		}
	}
	return nil
}

func marshalRequestHeaders(req *model.ParsedRequest) []any {
	if len(req.RawHeaders) > 0 {
		var arr []any
		if err := json.Unmarshal(req.RawHeaders, &arr); err == nil {
			return arr
		}
	}
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
			row := map[string]any{"key": kv.Key, "value": kv.Value}
			if kv.Disabled {
				row["disabled"] = true
			}
			arr = append(arr, row)
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
			if fp.Disabled {
				row["disabled"] = true
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
