// Package profile merges the packages' store paths into one profile, like
// Nix's buildEnv, and switches the user onto it.
package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"dotman/store"
)

// mergedDirs are the subdirectories of a package's store path that are merged
// into the profile.
var mergedDirs = []string{"bin", "share"}

// DefaultLinkPath returns $XDG_STATE_HOME/dotman/profile, falling back to
// ~/.local/state/dotman/profile. It always points at the current profile, so
// its bin can be added to PATH once.
func DefaultLinkPath() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "dotman", "profile"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "state", "dotman", "profile"), nil
}

// Build creates a new profile in s that links every file under mergedDirs of
// each of storePaths, and returns its path. Directories are merged; two
// packages providing the same file is an error.
func Build(log *slog.Logger, s *store.Store, storePaths []string) (string, error) {
	profilePath, err := s.CreatePath("profile")
	if err != nil {
		return "", err
	}

	if err := merge(log, profilePath, storePaths); err != nil {
		if err := os.RemoveAll(profilePath); err != nil {
			log.Error("Failed to remove profile path.", "path", profilePath, "err", err)
		}

		return "", err
	}

	return profilePath, nil
}

func merge(log *slog.Logger, profilePath string, storePaths []string) error {
	for _, storePath := range storePaths {
		for _, dir := range mergedDirs {
			root := filepath.Join(storePath, dir)
			if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
				continue
			}

			err := filepath.WalkDir(root, func(src string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				rel, err := filepath.Rel(storePath, src)
				if err != nil {
					return err
				}
				dst := filepath.Join(profilePath, rel)

				if d.IsDir() {
					return os.MkdirAll(dst, store.DirPerm)
				}

				log.Debug("Linking into profile.", "src", src, "dst", dst)
				if err := os.Symlink(src, dst); err != nil {
					if errors.Is(err, fs.ErrExist) {
						existing, _ := os.Readlink(dst)
						return fmt.Errorf("profile: %s is provided by both %s and %s", rel, existing, src)
					}

					return err
				}

				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Switch atomically points linkPath at profilePath, replacing the profile it
// pointed at before.
func Switch(linkPath, profilePath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), store.DirPerm); err != nil {
		return err
	}

	tmp := linkPath + ".tmp-" + uuid.New().String()
	if err := os.Symlink(profilePath, tmp); err != nil {
		return err
	}

	if err := os.Rename(tmp, linkPath); err != nil {
		os.Remove(tmp)
		return err
	}

	return nil
}
