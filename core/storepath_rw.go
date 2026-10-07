package dotman

import (
	"os"
	"path"
	"path/filepath"
)

// StorePathRW is one entry in a store: the directory storeEntry inside the
// store root storeRoot. A Derivation builds its output into it.
type StorePathRW struct {
	storeRoot  string
	storeEntry string
}

// Root returns the root directory of the store that o belongs to.
func (s *StorePathRW) Root() string {
	return s.storeRoot
}

// EntryName returns the name of o's directory, relative to StoreRoot.
func (s *StorePathRW) EntryName() string {
	return s.storeEntry
}

func (s *StorePathRW) FullPath() string {
	return path.Join(s.storeRoot, s.storeEntry)
}

// CreateFile creates a file at filePath inside o's store entry.
//
// filePath must be relative; it is resolved against o's store entry. It may be
// nested, and any missing parent directories are created.
//
// It returns the created file, or an error if creation fails.
func (s *StorePathRW) CreateFile(filePath string) (*os.File, error) {
	fullPath := path.Join(s.storeRoot, s.storeEntry, filePath)

	if err := os.MkdirAll(path.Dir(fullPath), 0755); err != nil {
		return nil, err
	}

	return os.Create(fullPath)
}

func (s *StorePathRW) CreateSymlink(oldPath, newPath string) error {
	newPath = filepath.Join(s.storeRoot, s.storeEntry, newPath)

	if err := os.MkdirAll(path.Dir(newPath), 0755); err != nil {
		return err
	}

	return os.Symlink(oldPath, newPath)
}
