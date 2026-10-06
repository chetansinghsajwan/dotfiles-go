package yazi

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"

	"dotman/core"
	"dotman/lib"
)

// binaries are the executables installed from a yazi release.
var binaries = []string{"yazi", "ya"}

// releaseTarget returns the target triple of the yazi release built for
// system. Linux uses the statically linked musl builds.
func releaseTarget(system string) (string, error) {
	switch system {
	case "linux/amd64":
		return "x86_64-unknown-linux-musl", nil
	case "linux/arm64":
		return "aarch64-unknown-linux-musl", nil
	case "darwin/amd64":
		return "x86_64-apple-darwin", nil
	case "darwin/arm64":
		return "aarch64-apple-darwin", nil
	}

	return "", fmt.Errorf("no yazi release for %s", system)
}

type binAttrs struct {
	Target string
}

// deriveBin returns the derivation that extracts yazi's binaries from the
// release p.Version for system, checked against p.Hashes.
func (p *Package) deriveBin(system string) (*dotman.Derivation, error) {
	if p.Version == "" {
		return nil, fmt.Errorf("yazi: Version isn't set")
	}

	target, err := releaseTarget(system)
	if err != nil {
		return nil, err
	}

	// Without a pin, FakeHash makes the fetch fail with the hash to pin.
	hash, pinned := p.Hashes[target]
	if !pinned {
		hash = dotman.FakeHash
	}

	archive := lib.FetchGithubRelease(lib.GithubRelease{
		Repo:  "sxyazi/yazi",
		Tag:   p.Version,
		Asset: "yazi-" + target + ".zip",
		Hash:  hash,
	})

	return &dotman.Derivation{
		Name:    "yazi-bin",
		System:  system,
		Builder: binBuilder,
		Attrs:   binAttrs{Target: target},
		Inputs:  map[string]*dotman.Derivation{"archive": archive},
	}, nil
}

// buildBin extracts the release zip's binaries into bin.
func buildBin(b *dotman.Build) error {
	var attrs binAttrs
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	archivePath := b.Input("archive")
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", archivePath, err)
	}
	defer zr.Close()

	dir := filepath.Join(b.Out, "bin")
	if err := os.MkdirAll(dir, dotman.DirPerm); err != nil {
		return err
	}

	// The release zip holds everything under a yazi-<target>/ directory.
	for _, name := range binaries {
		src := "yazi-" + attrs.Target + "/" + name
		dst := filepath.Join(dir, name)

		b.Log.Debug("Extracting binary.", "src", src, "dst", dst)
		if err := lib.ExtractFile(&zr.Reader, src, dst); err != nil {
			return fmt.Errorf("extracting %s from %s: %w", src, archivePath, err)
		}
	}

	return nil
}
