package zellij

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"

	"dotman"
	"dotman/lib"
)

type Zellij struct {
	// 0.45.1
	Version string

	// SHA-256 of the release tar.gz for each target triple, e.g.
	// "x86_64-unknown-linux-musl": "sha256:40bc...". The release's .sha256sum
	// files hash the binary inside, not the tar.gz, so they can't be used
	// here. Installing on a target without a hash fails and reports the
	// downloaded archive's hash.
	Hashes map[string]string

	ShellAliases []string
}

func (z *Zellij) Name() string {
	return "zellij"
}

func (z *Zellij) Install(log *slog.Logger, cfg dotman.Config, store *dotman.Store, storePath string) error {

	log.Info("Downloading zellij...", "version", z.Version)
	binPath := filepath.Join(storePath, "bin", "zellij")
	err := z.DownloadZellij(store, z.Version, runtime.GOOS, runtime.GOARCH, binPath)

	if err != nil {
		log.Error("Failed to download zellij", "version", z.Version, "error", err)
		return err
	}

	return nil
}

func (z *Zellij) DownloadZellij(s *dotman.Store, version string, platform string, arch string, dest string) error {

	var target string
	switch platform + "/" + arch {
	case "darwin/amd64":
		target = "x86_64-apple-darwin"
	case "darwin/arm64":
		target = "aarch64-apple-darwin"
	case "linux/amd64":
		target = "x86_64-unknown-linux-musl"
	case "linux/arm64":
		target = "aarch64-unknown-linux-musl"
	default:
		return fmt.Errorf("No zellij release for %s/%s", platform, arch)
	}

	// Without a pin, FakeHash makes the download fail with the hash to pin.
	hash, pinned := z.Hashes[target]
	if !pinned {
		hash = lib.FakeHash
	}

	archivePath, err := lib.DownloadGithubRelease(s, lib.GithubRelease{
		Repo:  "zellij-org/zellij",
		Tag:   "v" + version,
		Asset: "zellij-" + target + ".tar.gz",
		Hash:  hash,
	})
	if err != nil {
		if mismatch, ok := errors.AsType[*lib.HashMismatchError](err); ok && !pinned {
			return fmt.Errorf("zellij %s: no hash pinned for %s; got %s", version, target, mismatch.Got)
		}

		return fmt.Errorf("zellij %s: %w", version, err)
	}

	// The release archive holds just the zellij binary.
	if err := lib.ExtractTarGzFile(archivePath, "zellij", dest); err != nil {
		return fmt.Errorf("extracting zellij from %s: %w", archivePath, err)
	}

	return nil
}
