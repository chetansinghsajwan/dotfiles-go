package lib

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"dotman"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// download writes url's body to w and returns its SHA-256 as "sha256:<hex>",
// the format GitHub shows for release assets. It is unexported so every
// download goes through the store; see DownloadFile.
func download(url string, w io.Writer) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", url, resp.Status)
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func FileNameForUrl(url string) string {
	return filepath.Base(url)
}

// FakeHash never matches a download, so pinning it makes DownloadFile fail
// with a *HashMismatchError that reports the real hash to pin instead, like
// Nix's lib.fakeHash.
const FakeHash = "sha256:"

// HashMismatchError is returned by DownloadFile when a download's SHA-256
// isn't the one it was pinned to.
type HashMismatchError struct {
	Url  string
	Want string
	Got  string
}

func (e *HashMismatchError) Error() string {
	return fmt.Sprintf("downloading %s: hash mismatch: want %s, got %s", e.Url, e.Want, e.Got)
}

// DownloadFile downloads url into the store, unless an earlier run already
// did, and returns the downloaded file's path.
//
// hash pins the download's SHA-256 as "sha256:<hex>"; a download that
// doesn't match it fails with a *HashMismatchError and is never stored. ""
// skips the check.
//
// The file is downloaded into a temp directory that is then renamed to
// store.PathForUrl(url, hash), so that path only ever holds a complete,
// checked download.
func DownloadFile(store *dotman.Store, url, hash string) (string, error) {
	storePath := store.PathForUrl(url, hash)
	path := filepath.Join(storePath, FileNameForUrl(url))

	if _, err := os.Stat(storePath); err == nil {
		return path, nil
	}

	tmpPath, err := store.CreateTempPath()
	if err != nil {
		return "", err
	}
	defer store.RemovePath(tmpPath) // no-op once renamed

	tmpFile, err := os.OpenFile(filepath.Join(tmpPath, FileNameForUrl(url)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, dotman.FilePerm)
	if err != nil {
		return "", err
	}

	got, err := download(url, tmpFile)
	if err != nil {
		tmpFile.Close()
		return "", err
	}

	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	if hash != "" && got != hash {
		return "", &HashMismatchError{Url: url, Want: hash, Got: got}
	}

	if err := os.Rename(tmpPath, storePath); err != nil {
		// Another run may have finished the same download first.
		if _, statErr := os.Stat(storePath); statErr == nil {
			return path, nil
		}

		return "", err
	}

	return path, nil
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

	// Pins the asset's SHA-256, as GitHub shows it; "" skips the check.
	Hash string
}

// DownloadGithubRelease downloads r's asset into the store, like
// DownloadFile, and returns its path.
func DownloadGithubRelease(store *dotman.Store, r GithubRelease) (string, error) {
	url := GetGithubUrl(r.Repo, r.Tag, r.Asset)
	return DownloadFile(store, url, r.Hash)
}
