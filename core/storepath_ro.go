package dotman

import (
	"path"
)

// StorePathRO is one entry in a store: the directory storeEntry inside the
// store root storeRoot. A Derivation builds its output into it.
type StorePathRO struct {
	storeRoot  string
	storeEntry string
}

// Root returns the root directory of the store that o belongs to.
func (o *StorePathRO) Root() string {
	return o.storeRoot
}

// EntryName returns the name of o's directory, relative to StoreRoot.
func (o *StorePathRO) EntryName() string {
	return o.storeEntry
}

func (o *StorePathRO) FullPath() string {
	return path.Join(o.storeRoot, o.storeEntry)
}
