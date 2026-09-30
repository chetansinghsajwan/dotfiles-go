package main

import (
	"log/slog"
	"os"

	"dotman/pkg"
	"dotman/pkg/yazi"
	"dotman/store"
)

var packages = []pkg.Package{
	yazi.ConfiguredYazi,
}

func main() {
	slog.Info("Initializing store...")

	s, err := store.NewStore()
	if err != nil {
		slog.Error("Failed to initialize store.", "err", err)
		os.Exit(1)
	}

	for _, p := range packages {
		slog.Info("Building package...", "pkg", p.Name())

		log := slog.Default().With("pkg", p.Name())

		storePath, err := s.CreatePath(p.Name())
		if err != nil {
			log.Error("Failed to create store path.", "err", err)
			os.Exit(1)
		}

		outputs, err := p.Outputs(log, storePath)
		if err != nil {
			slog.Error("Failed to build package.", "err", err)

			if err := os.RemoveAll(storePath); err != nil {
				log.Error("Failed to remove store path.", "path", storePath, "err", err)
			}

			os.Exit(1)
		}

		slog.Info("Built package.", "outputs", outputs)
	}
}
