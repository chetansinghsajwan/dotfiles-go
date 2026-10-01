package pkg

import (
	"log/slog"

	"dotman/config"
)

type Package interface {
	// Name identifies the package in logs and store paths.
	Name() string

	Outputs(log *slog.Logger, cfg config.Config, storePath string) (PackageOutputs, error)
}

// PackageOutputs maps a target path in the user's home to the store path
// that should be linked there.
type PackageOutputs map[string]string
