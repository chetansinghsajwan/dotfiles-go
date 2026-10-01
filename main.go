package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"dotman/config"
	"dotman/logging"
	"dotman/pkg"
	yaziConfigured "dotman/pkg/yazi/configured"
	"dotman/profile"
	"dotman/store"
)

var packages = []pkg.Package{
	yaziConfigured.Yazi,
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

	cfg := config.Default

	slog.Info("Initializing store...")

	s, err := store.NewStore()
	if err != nil {
		slog.Error("Failed to initialize store.", "err", err)
		os.Exit(1)
	}

	var storePaths []string
	for _, p := range packages {
		slog.Info("Building package...", "pkg", p.Name())

		log := slog.Default().With(logging.PrefixKey, p.Name())

		storePath, err := s.CreatePath(p.Name())
		if err != nil {
			log.Error("Failed to create store path.", "err", err)
			os.Exit(1)
		}

		err = p.Install(log, cfg, storePath)
		if err != nil {
			log.Error("Failed to build package.", "err", err)

			if err := os.RemoveAll(storePath); err != nil {
				log.Error("Failed to remove store path.", "path", storePath, "err", err)
			}

			os.Exit(1)
		}

		storePaths = append(storePaths, storePath)
	}

	slog.Info("Building profile...")

	profilePath, err := profile.Build(slog.Default(), s, storePaths)
	if err != nil {
		slog.Error("Failed to build profile.", "err", err)
		os.Exit(1)
	}

	linkPath, err := profile.DefaultLinkPath()
	if err != nil {
		slog.Error("Failed to find profile link path.", "err", err)
		os.Exit(1)
	}

	if err := profile.Switch(linkPath, profilePath); err != nil {
		slog.Error("Failed to switch profile.", "err", err)
		os.Exit(1)
	}

	slog.Info("Switched profile.", "profile", profilePath, "path", filepath.Join(linkPath, "bin"))
}
