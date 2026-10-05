package op

import (
	"dotman"
	"dotman/lib"

	_ "embed"
	"log/slog"
	"path/filepath"
)

type Op struct {
}

func (o *Op) Name() string {
	return "op"
}

//go:embed op.sh
var opScript string

// Dependencies:
// file
// csvlens
// helix
func (o *Op) Install(log *slog.Logger, cfg dotman.Config, store *dotman.Store, storePath string) error {
	binPath := filepath.Join(storePath, "bin", "op")
	return lib.WriteExecutable(binPath, opScript)
}

var OpPkg = Op{}
