package dotman

import (
	"os"
	"path"
)

// DerivationOutput is one entry in a store: the directory storeEntry inside the
// store root storeRoot. A Derivation builds its output into it.
type DerivationOutput struct {
	storePath StorePathRW
}

// CreateFile creates a file at filePath inside o's store entry.
//
// filePath must be relative; it is resolved against o's store entry. It may be
// nested, and any missing parent directories are created.
//
// It returns the created file, or an error if creation fails.
func (o *DerivationOutput) CreateFile(filePath string) (*os.File, error) {
	fullPath := path.Join(o.storePath.storeRoot, o.storePath.storeEntry, filePath)

	err := os.MkdirAll(path.Dir(fullPath), 0755)
	if err != nil {
		return nil, err
	}

	return os.Create(fullPath)
}

func (o *DerivationOutput) WriteFile(filePath, content string) error {
	f, err := o.CreateFile(filePath)
	if err != nil {
		return err
	}

	defer f.Close()
	f.WriteString(content)

	return nil
}

func (o *DerivationOutput) CreateSymlink(oldPath, newPath string) error {
	return o.storePath.CreateSymlink(oldPath, newPath)
}

// Save commits o to the store. Until Save is called, o is not part of the
// store, and anything written to it is discarded.
func (o *DerivationOutput) Save() {}
