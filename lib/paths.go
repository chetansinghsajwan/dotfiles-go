// Package paths resolves paths relative to the source file that names them.
package lib

import (
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
