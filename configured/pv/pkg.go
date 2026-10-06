package pv

import (
	"dotman/core"
	"dotman/lib"

	_ "embed"
	"log/slog"
	"path/filepath"
)

type Pv struct {
}

func (p *Pv) Name() string {
	return "pv"
}

//go:embed pv.sh
var pvScript string

// Dependencies:
// file
// bat
// tidy-viewer
func (p *Pv) Install(log *slog.Logger, cfg dotman.Config, store *dotman.Store, storePath string) error {
	binPath := filepath.Join(storePath, "bin", "pv")
	return lib.WriteExecutable(binPath, pvScript)
}

var PvPkg = Pv{}
