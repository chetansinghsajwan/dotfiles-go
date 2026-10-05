package dotman

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Permissions for directories and files that packages create in the store.
const (
	DirPerm  os.FileMode = 0o755
	FilePerm os.FileMode = 0o644
	ExecPerm os.FileMode = 0o755
)

type Store struct {
	rootPath string
}

// DefaultStorePath returns $XDG_DATA_HOME/dotman/store, falling back to
// ~/.local/share/dotman/store.
func DefaultStorePath() (string, error) {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "dotman", "store"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "share", "dotman", "store"), nil
}

func NewStore() (*Store, error) {
	rootPath, err := DefaultStorePath()
	if err != nil {
		return nil, err
	}

	return NewStoreWithPath(rootPath), nil
}

func NewStoreWithPath(rootPath string) *Store {
	return &Store{
		rootPath: rootPath,
	}
}

func (m *Store) RootPath() string {
	return m.rootPath
}

// CreatePath creates a new, empty directory in the store for the package
// called name and returns its path.
func (m *Store) CreatePath(name string) (string, error) {
	path := filepath.Join(m.rootPath, uuid.New().String()+"-"+name)
	if err := os.MkdirAll(path, DirPerm); err != nil {
		return "", err
	}

	return path, nil
}

// HashForUrl returns the hex SHA-256 of url, for naming its store path. The
// same url always gets the same hash, so a download can be found again.
func (m *Store) HashForUrl(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

// PathForUrl returns the store path that url's download lives at. It is the
// same for every run, so it doubles as a cache key; it may not exist yet.
func (m *Store) PathForUrl(url string) string {
	return filepath.Join(m.rootPath, m.HashForUrl(url)+"-"+filepath.Base(url))
}

// CreateTempPath creates a new, empty directory in the store to build into
// before renaming it to its final path, so that path is never seen half
// written.
func (m *Store) CreateTempPath() (string, error) {
	if err := os.MkdirAll(m.rootPath, DirPerm); err != nil {
		return "", err
	}

	path, err := os.MkdirTemp(m.rootPath, ".tmp-")
	if err != nil {
		return "", err
	}

	if err := os.Chmod(path, DirPerm); err != nil {
		os.RemoveAll(path)
		return "", err
	}

	return path, nil
}

func (m *Store) RemovePath(path string) error {
	return os.RemoveAll(path)
}
