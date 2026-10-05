package yazi

import (
	"archive/zip"
	"dotman"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

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

// installBinaries downloads the yazi release p.Version for this machine,
// checks it against p.Hashes and extracts its binaries into dir.
func (p *Package) installBinaries(log *slog.Logger, dir string) error {
	if p.Version == "" {
		return fmt.Errorf("yazi: Version isn't set")
	}

	target, err := releaseTarget()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://github.com/sxyazi/yazi/releases/download/%s/yazi-%s.zip", p.Version, target)

	tmp, err := os.CreateTemp("", "yazi-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	log.Debug("Downloading yazi...", "url", url)
	hash, err := lib.Download(url, tmp)
	if err != nil {
		return err
	}

	want, ok := p.Hashes[target]
	if !ok {
		return fmt.Errorf("yazi %s: no hash pinned for %s; got %s", p.Version, target, hash)
	}
	if hash != want {
		return fmt.Errorf("yazi %s: hash mismatch for %s: want %s, got %s", p.Version, target, want, hash)
	}

	info, err := tmp.Stat()
	if err != nil {
		return err
	}

	zr, err := zip.NewReader(tmp, info.Size())
	if err != nil {
		return fmt.Errorf("opening %s: %w", url, err)
	}

	if err := os.MkdirAll(dir, dotman.DirPerm); err != nil {
		return err
	}

	// The release zip holds everything under a yazi-<target>/ directory.
	for _, name := range binaries {
		src := "yazi-" + target + "/" + name
		dst := filepath.Join(dir, name)

		log.Debug("Extracting binary.", "src", src, "dst", dst)
		if err := lib.ExtractFile(zr, src, dst); err != nil {
			return fmt.Errorf("extracting %s from %s: %w", src, url, err)
		}
	}

	return nil
}
