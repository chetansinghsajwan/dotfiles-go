// Package paths resolves paths relative to the source file that names them.
package lib

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Rel resolves p relative to the directory of the source file that calls it,
// like ./ in Nix. Call it directly in that file: wrapping it in a helper
// resolves against the helper's file instead.
//
// It relies on the source being on disk where it was compiled, which holds
// for `go run` from the checkout but not for -trimpath builds or binaries
// moved elsewhere.
func Rel(p string) string {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("paths.Rel: can't determine caller")
	}

	return filepath.Join(filepath.Dir(file), p)
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func WriteFile(path string, data []byte) error {
	return writeFileMode(path, data, 0644)
}

// WriteExecutable writes data to path with the execute bit set, for scripts
// installed into a package's bin directory.
func WriteExecutable(path string, data string) error {
	return writeFileMode(path, []byte(data), 0755)
}

func writeFileMode(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := out.Write(data); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	// OpenFile's perm is masked by the umask and ignored for an existing
	// file, so set the mode explicitly.
	return os.Chmod(path, perm)
}

func WriteTextFile(path string, data string) error {
	return WriteFile(path, []byte(data))
}
