package pv

import (
	_ "embed"

	"dotman/lib"
)

//go:embed pv.sh
var pvScript string

var PvPkg = lib.ScriptPackage{
	Command:  "pv",
	Script:   pvScript,
	HostDeps: []string{"file", "bat", "tidy-viewer"},
}
