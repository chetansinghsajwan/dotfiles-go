package main

import (
	"log/slog"
	"os"
	"strconv"

	"dotman/configured/lazygit"
	"dotman/configured/op"
	"dotman/configured/pv"
	"dotman/configured/yazi"
	"dotman/configured/zellij"
	"dotman/core"
	"dotman/logging"
)

var log = logging.Get("main")

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

	// The store root is resolved before logging is set up so the handler can
	// shorten paths under it.
	storeRoot, storeErr := dotman.DefaultStorePath()

	// DOTMAN_LOG sets levels per module, e.g. "info,pkg.yazi=debug".
	if err := logging.Configure(logging.NewHandler(os.Stderr, storeRoot), level, os.Getenv("DOTMAN_LOG")); err != nil {
		log.Error("Failed to configure logging.", "err", err)
		os.Exit(1)
	}

	cfg := dotman.DefaultConfig

	log.Info("Initializing store...", "path", storeRoot)

	if storeErr != nil {
		log.Error("Failed to initialize store.", "err", storeErr)
		os.Exit(1)
	}

	s := dotman.NewStoreWithPath(storeRoot)

	var packages = []dotman.Package{
		&yazi.Yazi,
		&lazygit.Lazygit,
		&zellij.Zellij,
		&pv.PvPkg,
		&op.OpPkg,
	}

	var storePaths []string
	for _, p := range packages {
		log := log.With("pkg", p.Name())
		log.Info("Building package...")

		storePath, err := s.CreatePath(p.Name())
		if err != nil {
			log.Error("Failed to create store path.", "err", err)
			os.Exit(1)
		}

		err = p.Install(cfg, s, storePath)
		if err != nil {
			log.Error("Failed to build package.", "err", err)

			if err := os.RemoveAll(storePath); err != nil {
				log.Error("Failed to remove store path.", "path", storePath, "err", err)
			}

			os.Exit(1)
		}

		storePaths = append(storePaths, storePath)
	}

	log.Info("Building profile...")

	profilePath, err := dotman.BuildProfile(s, storePaths)
	if err != nil {
		log.Error("Failed to build profile.", "err", err)
		os.Exit(1)
	}

	linkPath, err := dotman.DefaultLinkPath()
	if err != nil {
		log.Error("Failed to find profile link path.", "err", err)
		os.Exit(1)
	}

	if err := dotman.SwitchProfile(linkPath, profilePath); err != nil {
		log.Error("Failed to switch profile.", "err", err)
		os.Exit(1)
	}

	log.Info("Switched profile.", "profile", profilePath)
}
