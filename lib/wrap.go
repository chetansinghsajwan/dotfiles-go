package lib

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"dotman/core"
)

// Wrap describes a script that runs Exec with extra environment and leading
// arguments, like Nix's makeWrapper.
type Wrap struct {
	// Where the script is written.
	Path string

	// Absolute path of the program to run.
	Exec string

	// Exported before running Exec, overriding the caller's values.
	Env map[string]string

	// Passed to Exec before the script's own arguments.
	Args []string
}

// CreateWrap writes wrap's script to wrap.Path as an executable. Env is
// exported in sorted order, and every value is quoted, e.g.
//
//	#!/bin/sh
//	export YAZI_CONFIG_HOME='<store>/8j48…-yazi-config'
//	exec '<store>/kvgq…-yazi-bin/bin/yazi' "$@"
func CreateWrap(wrap Wrap) error {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")

	for _, key := range slices.Sorted(maps.Keys(wrap.Env)) {
		b.WriteString("export ")
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(shellQuote(wrap.Env[key]))
		b.WriteString("\n")
	}

	b.WriteString("exec ")
	b.WriteString(shellQuote(wrap.Exec))
	for _, arg := range wrap.Args {
		b.WriteString(" ")
		b.WriteString(shellQuote(arg))
	}
	b.WriteString(" \"$@\"\n")

	return os.WriteFile(wrap.Path, []byte(b.String()), dotman.ExecPerm)
}

// CreateShellAliases links each of aliases to target in binPath, so each
// alias is a command on PATH, in every shell and to every program, once the
// profile links binPath in. The links are relative, so they work wherever
// binPath ends up.
func CreateShellAliases(binPath, target string, aliases []string) error {
	for _, alias := range aliases {
		if alias == "" || strings.ContainsRune(alias, '/') {
			return fmt.Errorf("invalid shell alias %q for %s", alias, target)
		}

		if err := os.Symlink(target, filepath.Join(binPath, alias)); err != nil {
			return err
		}
	}

	return nil
}

// shellQuote quotes s as a single POSIX shell word, in single quotes, so
// nothing in it is expanded:
//
//	/bin/yazi -> '/bin/yazi'
//	it's      -> 'it'\''s'
//	""        -> ''
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
