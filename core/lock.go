package dotman

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Systems are the systems dotman builds for. The lock keeps entries for all
// of them, so a lock written on one machine still has the hashes another
// machine needs.
var Systems = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"}

// Lock records the hash of every fetch the first time it is downloaded, so
// later downloads, on any machine using the same lock, are checked against
// it, like go.sum. Fetches are named by their FixedOutput.Key, usually the
// url.
type Lock struct {
	path    string
	entries map[string]LockEntry
	dirty   bool
}

type LockEntry struct {
	Hash string `json:"hash"`

	// "flat" or "recursive", as FixedOutput.Recursive says. An entry for the
	// same key in the other mode doesn't count.
	Mode string `json:"mode"`
}

type lockFile struct {
	Version int                  `json:"version"`
	Fetches map[string]LockEntry `json:"fetches"`
}

const lockVersion = 1

// LoadLock reads the lock at path. A missing file is an empty lock, which
// Save creates.
func LoadLock(path string) (*Lock, error) {
	l := &Lock{path: path, entries: map[string]LockEntry{}}

	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}

	var f lockFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("reading lock %s: %w", path, err)
	}
	if f.Version != lockVersion {
		return nil, fmt.Errorf("lock %s: unsupported version %d", path, f.Version)
	}

	if f.Fetches != nil {
		l.entries = f.Fetches
	}

	return l, nil
}

func (l *Lock) Path() string {
	return l.path
}

// Get returns the hash locked for key in mode, if there is one.
func (l *Lock) Get(key, mode string) (string, bool) {
	e, ok := l.entries[key]
	if !ok || e.Mode != mode {
		return "", false
	}

	return e.Hash, true
}

func (l *Lock) Set(key, mode, hash string) {
	l.entries[key] = LockEntry{Hash: hash, Mode: mode}
	l.dirty = true
}

// Delete removes the entries for keys, so they are fetched and locked again.
func (l *Lock) Delete(keys []string) []string {
	var deleted []string
	for _, key := range keys {
		if _, ok := l.entries[key]; ok {
			delete(l.entries, key)
			deleted = append(deleted, key)
			l.dirty = true
		}
	}

	return deleted
}

// Prune removes every entry not in keep, like the fetches of versions no
// longer configured, and returns their keys.
func (l *Lock) Prune(keep map[string]bool) []string {
	var stale []string
	for key := range l.entries {
		if !keep[key] {
			stale = append(stale, key)
		}
	}

	slices.Sort(stale)
	return l.Delete(stale)
}

// Save writes the lock if it changed since it was loaded, atomically.
// encoding/json sorts the keys, so the file diffs cleanly.
func (l *Lock) Save() error {
	if !l.dirty {
		return nil
	}

	b, err := json.MarshalIndent(lockFile{Version: lockVersion, Fetches: l.entries}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".dotman.lock-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), FilePerm); err != nil {
		return err
	}

	if err := os.Rename(tmp.Name(), l.path); err != nil {
		return err
	}

	l.dirty = false
	return nil
}

// FixedKeys returns the lock keys of the fetches in the closures of drvs.
func FixedKeys(drvs ...*Derivation) map[string]bool {
	keys := map[string]bool{}
	for _, drv := range drvs {
		for _, d := range Closure(drv) {
			if d.Fixed != nil && d.Fixed.Key != "" {
				keys[d.Fixed.Key] = true
			}
		}
	}

	return keys
}
