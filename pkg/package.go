package pkg

import (
	"log/slog"

	"dotman/config"
)

type Package interface {
	// Name identifies the package in logs and store paths.
	Name() string

	Install(log *slog.Logger, cfg config.Config, storePath string) error
}
