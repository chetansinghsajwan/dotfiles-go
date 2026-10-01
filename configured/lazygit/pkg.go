package lazygit

import (
	_ "embed"

	lg "dotman/pkg/lazygit"
)

//go:embed config.yml
var configYaml string

// TODO: delta is assumed to already be on PATH; once dotman can build it as
// a package, point this at its store path instead.
var Lazygit = lg.Package{
	Version:    "0.65.1",
	ConfigYaml: configYaml,
}
