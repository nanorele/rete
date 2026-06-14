package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/widget"
)

const MethodGraphQL = "GRAPHQL"

type GQLSession struct {
	Query     widget.Editor
	Variables widget.Editor

	QueryCopyBtn widget.Clickable
	VarsCopyBtn  widget.Clickable

	VarsSplitRatio float32
	VarsSplitDrag  gesture.Drag
	VarsSplitDragX float32
}

func newGQLSession() *GQLSession {
	g := &GQLSession{
		VarsSplitRatio: 0.6,
	}
	g.Query.Submit = false
	g.Variables.Submit = false
	return g
}

func (t *RequestTab) EnsureGQL() *GQLSession {
	if t.GQL == nil {
		t.GQL = newGQLSession()
	}
	return t.GQL
}

func (t *RequestTab) httpMethod() string {
	if t.Method == MethodGraphQL {
		return "POST"
	}
	return t.Method
}

type gqlPayload struct {
	Query     string          `json:"query"`
	Variables json.RawMessage `json:"variables,omitempty"`
}

func (t *RequestTab) graphQLPayload(env map[string]string) ([]byte, error) {
	g := t.EnsureGQL()
	p := gqlPayload{Query: processTemplate(g.Query.Text(), env)}
	varsText := strings.TrimSpace(processTemplate(g.Variables.Text(), env))
	if varsText != "" {
		if !json.Valid([]byte(varsText)) {
			return nil, errors.New("GraphQL variables: invalid JSON")
		}
		p.Variables = json.RawMessage(varsText)
	}
	return json.Marshal(p)
}

func (t *RequestTab) buildGraphQLBody(env map[string]string) (io.Reader, string, error) {
	data, err := t.graphQLPayload(env)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(data), "application/json", nil
}
