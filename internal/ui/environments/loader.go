package environments

import (
	"bytes"

	"rete/internal/model"
	"rete/internal/persist"
)

func LoadAll() []*model.ParsedEnvironment {
	files := persist.LoadEnvironmentFiles()
	var envs []*model.ParsedEnvironment
	for _, f := range files {
		env, err := ParseEnvironment(bytes.NewReader(f.Data), f.ID)
		if err == nil && env != nil {
			envs = append(envs, env)
		}
	}
	return envs
}
