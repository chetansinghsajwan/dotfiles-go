package dotman

import (
	"os"
	"path/filepath"
)

// Permissions for directories and files that packages create in the store.
const (
	DirPerm  os.FileMode = 0o755
	FilePerm os.FileMode = 0o644
	ExecPerm os.FileMode = 0o755

	// What normalize leaves store paths with, so nothing edits them in
	// place.
	ReadOnlyPerm     os.FileMode = 0o444
	ReadOnlyExecPerm os.FileMode = 0o555
)

// tempPrefix starts the names of outputs being built in the store's root.
// They are safe to delete when no build is running.
const tempPrefix = ".tmp-"

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
