package op

import (
	_ "embed"

	"dotman/lib"
)

//go:embed op.sh
var opScript string

// op opens files in $EDITOR, falling back to hx.
var OpPkg = lib.ScriptPackage{
	Command:  "op",
	Script:   opScript,
	HostDeps: []string{"file", "csvlens", "hx"},
}
