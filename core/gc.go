package dotman

import (
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// GC deletes every store path that roots don't reach, and any leftover
// temporary build outputs, so it mustn't run alongside a build. A path
// reaches the store paths whose absolute paths appear in its files or
// symlink targets, as Nix finds references, so references must be written
// as absolute store paths. It returns the names it deleted.
func (s *Store) GC(log *slog.Logger, roots []string) ([]string, error) {
	entries, err := os.ReadDir(s.rootPath)
	if err != nil {
		return nil, err
	}

	// Store paths by the hash that starts their name.
	byHash := map[string]string{}
	for _, e := range entries {
		if h, ok := pathHash(e.Name()); ok {
			byHash[h] = e.Name()
		}
	}

	live := map[string]bool{}
	var queue []string
	for _, root := range roots {
		if name, ok := s.entryName(root); ok && !live[name] {
			live[name] = true
			queue = append(queue, name)
		}
	}

	prefix := []byte(s.rootPath + string(filepath.Separator))
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		refs, err := scanRefs(filepath.Join(s.rootPath, name), prefix)
		if err != nil {
			return nil, err
		}

		for h := range refs {
			if ref, ok := byHash[h]; ok && !live[ref] {
				live[ref] = true
				queue = append(queue, ref)
			}
		}
	}

	var deleted []string
	for _, e := range entries {
		if live[e.Name()] {
			continue
		}

		log.Debug("Deleting.", "path", e.Name())
		if err := removeTree(filepath.Join(s.rootPath, e.Name())); err != nil {
			return deleted, err
		}
		deleted = append(deleted, e.Name())
	}

	return deleted, nil
}

// entryName returns the name of the store entry path is in, if it is in
// the store.
func (s *Store) entryName(path string) (string, bool) {
	rel, err := filepath.Rel(s.rootPath, path)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", false
	}

	return strings.SplitN(filepath.ToSlash(rel), "/", 2)[0], true
}

// pathHash returns the hash part of a store path's name.
func pathHash(name string) (string, bool) {
	if len(name) <= storeHashLen || name[storeHashLen] != '-' {
		return "", false
	}

	h := name[:storeHashLen]
	if _, err := storeEncoding.DecodeString(h); err != nil {
		return "", false
	}

	return h, true
}

// scanRefs returns the hashes of the store paths that the files and symlink
// targets under path mention, by finding prefix, the store's root, followed
// by a hash.
func scanRefs(path string, prefix []byte) (map[string]bool, error) {
	refs := map[string]bool{}

	collect := func(b []byte) {
		for {
			i := bytes.Index(b, prefix)
			if i < 0 {
				return
			}
			b = b[i+len(prefix):]

			if len(b) > storeHashLen {
				if h, ok := pathHash(string(b[:storeHashLen+1])); ok {
					refs[h] = true
				}
			}
		}
	}

	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			collect([]byte(target))

		case d.Type().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()

			// Chunks overlap by enough that a reference split across two is
			// whole in one of them.
			overlap := len(prefix) + storeHashLen
			buf := make([]byte, 64*1024+overlap)
			keep := 0
			for {
				n, err := f.Read(buf[keep:])
				window := buf[:keep+n]
				collect(window)

				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}

				keep = min(overlap, len(window))
				copy(buf, window[len(window)-keep:])
			}
		}

		return nil
	})

	return refs, err
}
