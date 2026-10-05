package lib

import (
	"maps"
	"os"
	"slices"
	"strings"

	"dotman"
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

// CreateWrap writes wrap's script to wrap.Path as an executable.
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

// shellQuote quotes s as a single POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
