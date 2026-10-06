package lib

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"dotman/core"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// download writes url's body to the file at dst, which mustn't exist.
func download(url, dst string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, dotman.FilePerm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("downloading %s: %w", url, err)
	}

	return f.Close()
}

type fetchAttrs struct {
	Url string
}

// The fetch builders aren't hashed: fetches are fixed-output, so their
// store paths depend only on their name and pinned hash.
var fetchUrlBuilder = &dotman.Builder{
	Name: "fetch-url",
	Build: func(b *dotman.Build) error {
		var attrs fetchAttrs
		if err := b.Decode(&attrs); err != nil {
			return err
		}

		b.Log.Info("Downloading...", "url", attrs.Url)
		return download(attrs.Url, b.Out)
	},
}

var fetchTarballBuilder = &dotman.Builder{
	Name: "fetch-tarball",
	Build: func(b *dotman.Build) error {
		var attrs fetchAttrs
		if err := b.Decode(&attrs); err != nil {
			return err
		}

		archive := b.Out + ".tar.gz"
		defer os.Remove(archive)

		b.Log.Info("Downloading...", "url", attrs.Url)
		if err := download(attrs.Url, archive); err != nil {
			return err
		}

		return ExtractTarGzDir(archive, "", b.Out)
	},
}

// FetchUrl returns a derivation that downloads url as a single file, whose
// SHA-256 is pinned to hash, "sha256:<hex>". Its store path depends only on
// its name, the url's last element, and hash, so changing to a mirror with
// the same file reuses the download. Pin dotman.FakeHash to have the fetch
// fail with the hash to pin.
func FetchUrl(url, hash string) *dotman.Derivation {
	return &dotman.Derivation{
		Name:    storeName(path.Base(url)),
		Builder: fetchUrlBuilder,
		Attrs:   fetchAttrs{Url: url},
		Fixed:   &dotman.FixedOutput{Hash: hash},
	}
}

// FetchTarball returns a derivation that downloads the tar.gz at url and
// unpacks it, without its single top-level directory, as GitHub's source
// archives have. hash pins the unpacked tree (see dotman.TreeHash), not the
// archive, since GitHub doesn't promise its archives are byte-stable, like
// Nix's fetchzip.
func FetchTarball(name, url, hash string) *dotman.Derivation {
	return &dotman.Derivation{
		Name:    storeName(name),
		Builder: fetchTarballBuilder,
		Attrs:   fetchAttrs{Url: url},
		Fixed:   &dotman.FixedOutput{Hash: hash, Recursive: true},
	}
}

// storeName makes s usable as a derivation name.
func storeName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	return strings.TrimLeft(s, ".")
}

func GetGithubUrl(repo string, tag string, asset string) string {
	return "https://github.com/" + repo + "/releases/download/" + tag + "/" + asset
}

// GetGithubArchiveUrl returns the url of the tar.gz of repo's source at rev,
// a commit, branch or tag. The archive holds the source under a single
// top-level directory.
func GetGithubArchiveUrl(repo string, rev string) string {
	return "https://github.com/" + repo + "/archive/" + rev + ".tar.gz"
}

type GithubRelease struct {
	Repo  string
	Tag   string
	Asset string

	// Pins the asset's SHA-256, as GitHub shows it.
	Hash string
}

// FetchGithubRelease returns a derivation that downloads r's asset, like
// FetchUrl.
func FetchGithubRelease(r GithubRelease) *dotman.Derivation {
	return FetchUrl(GetGithubUrl(r.Repo, r.Tag, r.Asset), r.Hash)
}

// FetchGithubArchive returns a derivation that downloads and unpacks repo's
// source at rev, like FetchTarball.
func FetchGithubArchive(repo, rev, hash string) *dotman.Derivation {
	return FetchTarball(path.Base(repo)+"-"+rev, GetGithubArchiveUrl(repo, rev), hash)
}
