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

// Download writes url's body to w and returns its SHA-256 as "sha256:<hex>",
// the format GitHub shows for release assets.
func Download(url string, w io.Writer) (string, error) {
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

// DownloadFile downloads url into the store, unless an earlier run already
// did, and returns the file's path and its SHA-256 as "sha256:<hex>".
//
// The file is downloaded into a temp directory that is then renamed to
// store.PathForUrl(url), so that path only ever holds a complete download.
func DownloadFile(store *dotman.Store, url string) (path string, hash string, err error) {
	storePath := store.PathForUrl(url)
	path = filepath.Join(storePath, FileNameForUrl(url))

	if _, err := os.Stat(storePath); err == nil {
		hash, err := hashFile(path)
		return path, hash, err
	}

	tmpPath, err := store.CreateTempPath()
	if err != nil {
		return "", "", err
	}
	defer store.RemovePath(tmpPath) // no-op once renamed

	tmpFile, err := os.OpenFile(filepath.Join(tmpPath, FileNameForUrl(url)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, dotman.FilePerm)
	if err != nil {
		return "", "", err
	}

	hash, err = Download(url, tmpFile)
	if err != nil {
		tmpFile.Close()
		return "", "", err
	}

	if err := tmpFile.Close(); err != nil {
		return "", "", err
	}

	if err := os.Rename(tmpPath, storePath); err != nil {
		// Another run may have finished the same download first.
		if _, statErr := os.Stat(storePath); statErr == nil {
			hash, err := hashFile(path)
			return path, hash, err
		}

		return "", "", err
	}

	return path, hash, nil
}

// hashFile returns the SHA-256 of the file at path as "sha256:<hex>".
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func GetGithubUrl(repo string, tag string, asset string) string {
	return "https://github.com/" + repo + "/releases/download/" + tag + "/" + asset
}

type GithubRelease struct {
	Repo  string
	Tag   string
	Asset string
}

// DownloadGithubRelease downloads asset from the release tagged tag in repo
// (e.g. "jesseduffield/lazygit"), writes it to w, and returns its SHA-256.
func DownloadGithubRelease(r GithubRelease, w io.Writer) (string, error) {
	url := GetGithubUrl(r.Repo, r.Tag, r.Asset)
	return Download(url, w)
}

// DownloadGithubReleaseFile downloads r's asset into the store, like
// DownloadFile, and returns its path and SHA-256.
func DownloadGithubReleaseFile(store *dotman.Store, r GithubRelease) (string, string, error) {
	url := GetGithubUrl(r.Repo, r.Tag, r.Asset)
	return DownloadFile(store, url)
}
