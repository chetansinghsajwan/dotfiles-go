package dotman

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// HashPrefix starts every content hash dotman prints or accepts, as GitHub
// shows release asset hashes.
const HashPrefix = "sha256:"

// HashMismatchError is returned when a fixed output doesn't have the hash
// it is pinned or locked to, like when a download changed upstream.
type HashMismatchError struct {
	Name string
	Want string
	Got  string
}

func (e *HashMismatchError) Error() string {
	return fmt.Sprintf("%s: hash mismatch: want %s, got %s", e.Name, e.Want, e.Got)
}

// storeHashLen is the length of the hash part of a store path's name.
const storeHashLen = 32

// storeEncoding is Nix's base32 alphabet, which leaves out e, o, u and t so
// hashes don't spell words.
var storeEncoding = base32.NewEncoding("0123456789abcdfghijklmnpqrsvwxyz").WithPadding(base32.NoPadding)

// storeHash returns the hash part of a store path for sum: the first 160
// bits, base32-encoded into storeHashLen characters, e.g.
// "w9drrwmakfqqbx94i1n5dminkvlqdv03" in
// <store>/w9drrwmakfqqbx94i1n5dminkvlqdv03-yazi.
func storeHash(sum []byte) string {
	return storeEncoding.EncodeToString(sum[:20])
}

func formatHash(h hash.Hash) string {
	return HashPrefix + hex.EncodeToString(h.Sum(nil))
}

// FileHash returns the SHA-256 of the file at path, as "sha256:<hex>".
func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return formatHash(h), nil
}

// TreeHash returns the hash of the tree at root in fsys, as "sha256:<hex>".
// It covers each entry's path, type, executable bit, and contents or symlink
// target, but not other permission bits or times, so a tree hashes the same
// before and after it is normalized into the store, and an embed.FS hashes
// the same as its copy on disk.
//
// Paths are relative to root, so where the tree is doesn't matter. Hashing
// root "plugins/places.yazi" of a tree holding
//
//	plugins/places.yazi/main.lua
//	plugins/places.yazi/lib/util.lua
//
// covers the entries ".", "lib", "lib/util.lua" and "main.lua", the same as
// a copy of that directory anywhere else.
func TreeHash(fsys fs.FS, root string) (string, error) {
	h := sha256.New()

	err := fs.WalkDir(fsys, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel := "."
		if name != root {
			rel = strings.TrimPrefix(name, root+"/")
			if root == "." {
				rel = name
			}
		}

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := fs.ReadLink(fsys, name)
			if err != nil {
				return err
			}
			writeField(h, "symlink")
			writeField(h, rel)
			writeField(h, target)

		case d.IsDir():
			writeField(h, "dir")
			writeField(h, rel)

		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}

			kind := "file"
			if info.Mode()&0o111 != 0 {
				kind = "exec"
			}
			writeField(h, kind)
			writeField(h, rel)

			f, err := fsys.Open(name)
			if err != nil {
				return err
			}
			defer f.Close()

			content := sha256.New()
			if _, err := io.Copy(content, f); err != nil {
				return err
			}
			h.Write(content.Sum(nil))

		default:
			return fmt.Errorf("%s: unsupported file type %s", name, d.Type())
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return formatHash(h), nil
}

// PathTreeHash returns the TreeHash of the file or directory at path.
func PathTreeHash(path string) (string, error) {
	return TreeHash(os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// writeField writes s length-prefixed, so no two different sequences of
// fields hash the same.
func writeField(h hash.Hash, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	h.Write(n[:])
	h.Write([]byte(s))
}
