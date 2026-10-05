package dotman

import (
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
