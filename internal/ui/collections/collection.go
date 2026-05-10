package collections

import (
	"encoding/json"
	"io"
	"strings"
	"time"
	"tracto/internal/model"
	"tracto/internal/utils"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/widget"
)

type CollectionNode struct {
	Name       string
	IsFolder   bool
	Request    *model.ParsedRequest
	Children   []*CollectionNode
	Expanded   bool
	Depth      int
	Parent     *CollectionNode
	Collection *ParsedCollection

	Extras map[string]json.RawMessage

	skippedItems []json.RawMessage

	MenuBtn      widget.Clickable
	MenuOpen     bool
	MenuBtnWidth int
	AddReqBtn    widget.Clickable
	AddFldBtn    widget.Clickable
	EditBtn      widget.Clickable
	DupBtn       widget.Clickable
	DelBtn       widget.Clickable

	IsRenaming      bool
	RenamingFocused bool
	NameEditor      widget.Editor

	LastClickAt time.Time

	Drag  gesture.Drag
	Hover gesture.Hover
}

type ParsedCollection struct {
	ID   string
	Name string
	Root *CollectionNode

	InfoExtras map[string]json.RawMessage

	TopExtras map[string]json.RawMessage
}

type CollectionUI struct {
	Data *ParsedCollection
}

func isExampleItem(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	if _, ok := fields["originalRequest"]; ok {
		return true
	}
	if _, ok := fields["_postman_previewlanguage"]; ok {
		return true
	}
	if _, ok := fields["responseTime"]; ok {
		return true
	}
	if v, ok := fields["_apidog_type"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			s = strings.ToLower(s)
			if s == "example" || s == "case" || s == "apicase" {
				return true
			}
		}
	}

	if _, hasCode := fields["code"]; hasCode {
		if _, hasBody := fields["body"]; hasBody {
			return true
		}
	}
	return false
}

func formPartSrcPath(src any) string {
	switch v := src.(type) {
	case string:
		return v
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				return s
			}
		}
	case []string:
		for _, s := range v {
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func NodePathFrom(root *CollectionNode, target *CollectionNode) []int {
	if root == nil || target == nil || root == target {
		return nil
	}
	var depth int
	for cur := target; cur != nil && cur != root; cur = cur.Parent {
		depth++
	}
	path := make([]int, depth)
	cur := target
	for i := depth - 1; i >= 0; i-- {
		parent := cur.Parent
		if parent == nil {
			return nil
		}
		found := -1
		for j, c := range parent.Children {
			if c == cur {
				found = j
				break
			}
		}
		if found < 0 {
			return nil
		}
		path[i] = found
		cur = parent
	}
	return path
}

func NodeAtPath(root *CollectionNode, path []int) *CollectionNode {
	if root == nil {
		return nil
	}
	cur := root
	for _, idx := range path {
		if idx < 0 || idx >= len(cur.Children) {
			return nil
		}
		cur = cur.Children[idx]
	}
	return cur
}

func CloneNode(node *CollectionNode, parent *CollectionNode) *CollectionNode {
	dup := &CollectionNode{
		Name:       node.Name + " Copy",
		IsFolder:   node.IsFolder,
		Expanded:   node.Expanded,
		Depth:      node.Depth,
		Parent:     parent,
		Collection: node.Collection,
	}
	dup.NameEditor.SingleLine = true
	dup.NameEditor.Submit = true

	if len(node.Extras) > 0 {
		dup.Extras = make(map[string]json.RawMessage, len(node.Extras))
		for k, v := range node.Extras {
			cp := append(json.RawMessage(nil), v...)
			dup.Extras[k] = cp
		}
	}
	if len(node.skippedItems) > 0 {
		dup.skippedItems = make([]json.RawMessage, len(node.skippedItems))
		for i, it := range node.skippedItems {
			dup.skippedItems[i] = append(json.RawMessage(nil), it...)
		}
	}

	if node.Request != nil {
		dup.Request = &model.ParsedRequest{
			Name:       dup.Name,
			Method:     node.Request.Method,
			URL:        node.Request.URL,
			Body:       node.Request.Body,
			BodyType:   node.Request.BodyType,
			BinaryPath: node.Request.BinaryPath,
		}
		dup.Request.Headers = make(map[string]string)
		for k, v := range node.Request.Headers {
			dup.Request.Headers[k] = v
		}
		if len(node.Request.FormParts) > 0 {
			dup.Request.FormParts = append([]model.ParsedFormPart(nil), node.Request.FormParts...)
		}
		if len(node.Request.URLEncoded) > 0 {
			dup.Request.URLEncoded = append([]model.ParsedKV(nil), node.Request.URLEncoded...)
		}
		if len(node.Request.RawURL) > 0 {
			dup.Request.RawURL = append(json.RawMessage(nil), node.Request.RawURL...)
		}
		if len(node.Request.RawHeaders) > 0 {
			dup.Request.RawHeaders = append(json.RawMessage(nil), node.Request.RawHeaders...)
		}
		if len(node.Request.Extras) > 0 {
			dup.Request.Extras = make(map[string]json.RawMessage, len(node.Request.Extras))
			for k, v := range node.Request.Extras {
				dup.Request.Extras[k] = append(json.RawMessage(nil), v...)
			}
		}
		if len(node.Request.BodyExtras) > 0 {
			dup.Request.BodyExtras = make(map[string]json.RawMessage, len(node.Request.BodyExtras))
			for k, v := range node.Request.BodyExtras {
				dup.Request.BodyExtras[k] = append(json.RawMessage(nil), v...)
			}
		}
	}

	for _, child := range node.Children {
		dup.Children = append(dup.Children, CloneNode(child, dup))
	}
	return dup
}

func CollectSubtree(node *CollectionNode) map[*CollectionNode]struct{} {
	seen := make(map[*CollectionNode]struct{})
	var walk func(*CollectionNode)
	walk = func(n *CollectionNode) {
		if n == nil {
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(node)
	return seen
}

func AssignParents(node *CollectionNode, parent *CollectionNode, col *ParsedCollection) {
	node.Parent = parent
	node.Collection = col
	node.NameEditor.SingleLine = true
	node.NameEditor.Submit = true
	for _, child := range node.Children {
		AssignParents(child, node, col)
	}
}

func ParseCollection(r io.Reader, id string) (*ParsedCollection, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	if len(top) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	col := &ParsedCollection{
		ID:         id,
		InfoExtras: map[string]json.RawMessage{},
		TopExtras:  map[string]json.RawMessage{},
	}

	var rawItems []json.RawMessage
	for k, v := range top {
		switch k {
		case "info":
			var info map[string]json.RawMessage
			if err := json.Unmarshal(v, &info); err == nil {
				for ik, iv := range info {
					if ik == "name" {
						var s string
						_ = json.Unmarshal(iv, &s)
						col.Name = utils.SanitizeText(s)
					} else {
						col.InfoExtras[ik] = iv
					}
				}
			} else {
				col.TopExtras[k] = v
			}
		case "item":
			_ = json.Unmarshal(v, &rawItems)
		default:
			col.TopExtras[k] = v
		}
	}

	if col.Name == "" && len(rawItems) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if col.Name == "" {
		col.Name = "Imported Collection"
	}

	root := &CollectionNode{
		Name:     col.Name,
		IsFolder: true,
		Depth:    0,
		Expanded: true,
	}
	root.NameEditor.SingleLine = true
	root.NameEditor.Submit = true
	for _, raw := range rawItems {
		if isExampleItem(raw) {
			root.skippedItems = append(root.skippedItems, raw)
			continue
		}
		if child := parseItemRaw(raw, 1); child != nil {
			root.Children = append(root.Children, child)
		}
	}
	col.Root = root
	AssignParents(root, nil, col)
	return col, nil
}

func parseItemRaw(raw json.RawMessage, depth int) *CollectionNode {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	node := &CollectionNode{Depth: depth, Extras: map[string]json.RawMessage{}}
	node.NameEditor.SingleLine = true
	node.NameEditor.Submit = true

	requestPresent := false
	if v, ok := fields["request"]; ok && len(v) > 0 && string(v) != "null" {
		requestPresent = true
	}
	_, hasRequestKey := fields["request"]
	_, hasItemKey := fields["item"]
	for k, v := range fields {
		switch k {
		case "name":
			var s string
			_ = json.Unmarshal(v, &s)
			node.Name = utils.SanitizeText(s)
		case "item":
			if requestPresent {

				node.Extras[k] = v
				continue
			}
			var children []json.RawMessage
			if err := json.Unmarshal(v, &children); err == nil {
				node.IsFolder = true
				for _, c := range children {
					if isExampleItem(c) {
						node.skippedItems = append(node.skippedItems, c)
						continue
					}
					if child := parseItemRaw(c, depth+1); child != nil {
						node.Children = append(node.Children, child)
					}
				}
			}
		case "request":
			if len(v) > 0 && string(v) != "null" {
				node.Request = parseRequestRaw(v, node.Name)
			}
		default:
			node.Extras[k] = v
		}
	}
	if node.Request == nil && !node.IsFolder && len(node.Children) == 0 {
		switch {
		case hasRequestKey:
			node.Request = &model.ParsedRequest{
				Name:    node.Name,
				Method:  "GET",
				Headers: map[string]string{},
				Extras:  map[string]json.RawMessage{},
			}
		case hasItemKey:
			node.IsFolder = true
		default:
			return nil
		}
	}
	return node
}

func parseRequestRaw(raw json.RawMessage, name string) *model.ParsedRequest {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {

		var s string
		if jerr := json.Unmarshal(raw, &s); jerr == nil && s != "" {
			return &model.ParsedRequest{
				Name:    name,
				Method:  "GET",
				URL:     utils.SanitizeText(s),
				Headers: map[string]string{},
				Extras:  map[string]json.RawMessage{},
			}
		}
		return nil
	}
	req := &model.ParsedRequest{
		Name:    name,
		Method:  "GET",
		Headers: map[string]string{},
		Extras:  map[string]json.RawMessage{},
	}
	for k, v := range fields {
		switch k {
		case "method":
			var s string
			_ = json.Unmarshal(v, &s)
			if s != "" {
				req.Method = utils.SanitizeText(s)
			}
		case "url":
			req.URL, req.RawURL = parseURL(v)
		case "header":
			req.Headers, req.RawHeaders = parseHeaderArray(v)
		case "body":
			parseBodyInto(v, req)
		default:
			req.Extras[k] = v
		}
	}
	return req
}

func parseURL(raw json.RawMessage) (string, json.RawMessage) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return utils.SanitizeText(s), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		var rs string
		_ = json.Unmarshal(obj["raw"], &rs)
		return utils.SanitizeText(rs), raw
	}
	return "", nil
}

func parseHeaderArray(raw json.RawMessage) (map[string]string, json.RawMessage) {
	var arr []map[string]json.RawMessage
	headers := map[string]string{}
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, h := range arr {
			var disabled bool
			if d, ok := h["disabled"]; ok {
				_ = json.Unmarshal(d, &disabled)
			}
			if disabled {
				continue
			}
			var k, v string
			_ = json.Unmarshal(h["key"], &k)
			_ = json.Unmarshal(h["value"], &v)
			if k = strings.TrimSpace(utils.SanitizeText(k)); k != "" {
				headers[k] = strings.TrimSpace(utils.SanitizeText(v))
			}
		}
	}
	return headers, raw
}

func parseBodyInto(raw json.RawMessage, req *model.ParsedRequest) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return
	}
	req.BodyExtras = map[string]json.RawMessage{}
	var modeStr string
	for k, v := range fields {
		switch k {
		case "mode":
			_ = json.Unmarshal(v, &modeStr)
		case "raw":
			var s string
			_ = json.Unmarshal(v, &s)
			req.Body = utils.SanitizeText(s)
		case "urlencoded":
			var arr []model.ExtKVPart
			if err := json.Unmarshal(v, &arr); err == nil {
				for _, kv := range arr {
					if kv.Disabled {
						continue
					}
					req.URLEncoded = append(req.URLEncoded, model.ParsedKV{
						Key:   strings.TrimSpace(utils.SanitizeText(kv.Key)),
						Value: utils.SanitizeText(kv.Value),
					})
				}
			}
		case "formdata":
			var arr []model.ExtFormPart
			if err := json.Unmarshal(v, &arr); err == nil {
				for _, fp := range arr {
					if fp.Disabled {
						continue
					}
					part := model.ParsedFormPart{
						Key:   strings.TrimSpace(utils.SanitizeText(fp.Key)),
						Value: utils.SanitizeText(fp.Value),
					}
					if strings.EqualFold(fp.Type, "file") {
						part.Kind = model.FormPartFile
						part.FilePath = formPartSrcPath(fp.Src)
					}
					req.FormParts = append(req.FormParts, part)
				}
			}
		case "file":
			var f model.ExtBodyFile
			if err := json.Unmarshal(v, &f); err == nil {
				req.BinaryPath = utils.SanitizeText(f.Src)
			}
		default:
			req.BodyExtras[k] = v
		}
	}
	req.BodyType = model.BodyTypeFromMode(modeStr)
}
