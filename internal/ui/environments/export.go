package environments

import (
	"encoding/json"
	"strings"
	"time"

	"rete/internal/model"
	"rete/internal/persist"
)

func ExportFileName(env *model.ParsedEnvironment) string {
	name := ""
	if env != nil {
		name = env.Name
	}
	return persist.SafeFileName(name, "environment") + ".postman_environment.json"
}

func ExportPostman(env *model.ParsedEnvironment) []byte {
	if env == nil {
		return nil
	}
	values := make([]map[string]any, 0, len(env.Vars))
	for _, v := range env.Vars {
		if strings.TrimSpace(v.Key) == "" {
			continue
		}
		values = append(values, map[string]any{
			"key":     v.Key,
			"value":   v.Value,
			"type":    "default",
			"enabled": true,
		})
	}
	id := env.ID
	if id == "" {
		id = persist.NewRandomID()
	}
	out := map[string]any{
		"id":                      id,
		"name":                    env.Name,
		"values":                  values,
		"_postman_variable_scope": "environment",
		"_postman_exported_at":    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"_postman_exported_using": "Rete",
	}
	data, _ := json.MarshalIndent(out, "", "\t")
	return data
}
