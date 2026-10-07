package lazygit

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

func (p *Package) GetConfigYaml() (string, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
