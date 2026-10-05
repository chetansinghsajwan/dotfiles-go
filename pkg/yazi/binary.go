package yazi

import (
	"archive/zip"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"dotman"
	"dotman/lib"
)

// binaries are the executables installed from a yazi release.
var binaries = []string{"yazi", "ya"}

// releaseTarget returns the target triple of the yazi release built for this
// machine. Linux uses the statically linked musl builds.
func releaseTarget() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "x86_64-unknown-linux-musl", nil
	case "linux/arm64":
		return "aarch64-unknown-linux-musl", nil
	case "darwin/amd64":
		return "x86_64-apple-darwin", nil
	case "darwin/arm64":
		return "aarch64-apple-darwin", nil
	}

	return "", fmt.Errorf("no yazi release for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// installBinaries downloads the yazi release p.Version for this machine into
// the store, reusing an earlier download, checks it against p.Hashes and
// extracts its binaries into dir.
func (p *Package) installBinaries(log *slog.Logger, store *dotman.Store, dir string) error {
	if p.Version == "" {
		return fmt.Errorf("yazi: Version isn't set")
	}

	target, err := releaseTarget()
	if err != nil {
		return err
	}

	release := lib.GithubRelease{
		Repo:  "sxyazi/yazi",
		Tag:   p.Version,
		Asset: "yazi-" + target + ".zip",
	}
	url := lib.GetGithubUrl(release.Repo, release.Tag, release.Asset)

	log.Debug("Downloading yazi...", "url", url)
	archivePath, hash, err := lib.DownloadGithubReleaseFile(store, release)
	if err != nil {
		return err
	}

	want, ok := p.Hashes[target]
	if !ok {
		return fmt.Errorf("yazi %s: no hash pinned for %s; got %s", p.Version, target, hash)
	}
	if hash != want {
		// Drop the cached download so the next run fetches it again.
		store.RemovePath(store.PathForUrl(url))
		return fmt.Errorf("yazi %s: hash mismatch for %s: want %s, got %s", p.Version, target, want, hash)
	}

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", archivePath, err)
	}
	defer zr.Close()

	if err := os.MkdirAll(dir, dotman.DirPerm); err != nil {
		return err
	}

	// The release zip holds everything under a yazi-<target>/ directory.
	for _, name := range binaries {
		src := "yazi-" + target + "/" + name
		dst := filepath.Join(dir, name)

		log.Debug("Extracting binary.", "src", src, "dst", dst)
		if err := lib.ExtractFile(&zr.Reader, src, dst); err != nil {
			return fmt.Errorf("extracting %s from %s: %w", src, url, err)
		}
	}

	return nil
}
