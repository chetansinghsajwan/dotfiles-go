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

	"dotman/store"
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

// DownloadFile downloads url to dest and returns its SHA-256 as "sha256:<hex>".
// The body is written to a temp file next to dest and renamed into place, so
// dest never holds a partial download.
func DownloadFile(url string, dest string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dest), store.DirPerm); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	hash, err := Download(url, tmp)
	if err != nil {
		tmp.Close()
		return "", err
	}

	if err := tmp.Close(); err != nil {
		return "", err
	}

	if err := os.Chmod(tmp.Name(), store.FilePerm); err != nil {
		return "", err
	}

	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}

	return hash, nil
}

func GetGithubUrl(repo string, tag string, asset string) string {
	return "https://github.com/" + repo + "/releases/download/" + tag + "/" + asset
}

// DownloadGithubRelease downloads asset from the release tagged tag in repo
// (e.g. "jesseduffield/lazygit"), writes it to w, and returns its SHA-256.
func DownloadGithubRelease(repo string, tag string, asset string, w io.Writer) (string, error) {
	url := GetGithubUrl(repo, tag, asset)
	return Download(url, w)
}

// DownloadGithubReleaseFile downloads asset from the release tagged tag in repo
// (e.g. "jesseduffield/lazygit") to dest and returns its SHA-256.
func DownloadGithubReleaseFile(repo string, tag string, asset string, dest string) (string, error) {
	url := GetGithubUrl(repo, tag, asset)
	return DownloadFile(url, dest)
}
