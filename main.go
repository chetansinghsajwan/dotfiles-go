package main

import (
	"log/slog"
	"os"
	"strconv"

	"dotman/logging"
	"dotman/pkg"
	"dotman/pkg/yazi"
	"dotman/store"
)

var packages = []pkg.Package{
	yazi.ConfiguredYazi,
}

// devMode reports whether DOTMAN_DEV is set to a true value, like 1 or true.
func devMode() bool {
	dev, _ := strconv.ParseBool(os.Getenv("DOTMAN_DEV"))
	return dev
}

func main() {
	level := slog.LevelInfo
	if devMode() {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(logging.NewHandler(os.Stderr, level)))

	slog.Info("Initializing store...")

	s, err := store.NewStore()
	if err != nil {
		slog.Error("Failed to initialize store.", "err", err)
		os.Exit(1)
	}

	for _, p := range packages {
		log := slog.Default().With(logging.PrefixKey, p.Name())

		log.Info("Building package...")

		storePath, err := s.CreatePath(p.Name())
		if err != nil {
			log.Error("Failed to create store path.", "err", err)
			os.Exit(1)
		}

		outputs, err := p.Outputs(log, storePath)
		if err != nil {
			log.Error("Failed to build package.", "err", err)

			if err := os.RemoveAll(storePath); err != nil {
				log.Error("Failed to remove store path.", "path", storePath, "err", err)
			}

			os.Exit(1)
		}

		log.Info("Built package.", "outputs", outputs)
	}
}
