package zellij

import (
	_ "embed"

	"dotman/pkg/zellij"
)

//go:embed config.kdl
var configKdl string

var Zellij = zellij.Zellij{
	Version:      "0.45.1",
	ConfigKdl:    configKdl,
	ShellAliases: []string{"z"},
}
