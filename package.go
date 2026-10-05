package dotman

import (
	"log/slog"
)

type Package interface {
	// Name identifies the package in logs and store paths.
	Name() string

	// Install builds the package into storePath. store is for caching
	// downloads and other paths that outlive this build.
	Install(log *slog.Logger, cfg Config, store *Store, storePath string) error
}
