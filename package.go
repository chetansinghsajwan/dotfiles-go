package dotman

import (
	"log/slog"
)

type Package interface {
	// Name identifies the package in logs and store paths.
	Name() string

	Install(log *slog.Logger, cfg Config, storePath string) error
}
