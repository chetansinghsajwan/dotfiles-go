package lib

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"dotman/core"
)

// WrapperSpec describes a derivation of wrapper scripts, which run programs
// from other store paths with extra environment and arguments.
type WrapperSpec struct {
	Name string

	// Store paths the wraps refer to. A wrap's Exec, Env values and Args can
	// name one as @name@, which is replaced by its store path.
	Inputs map[string]*dotman.Derivation

	// Each is written to bin/<Name>.
	Wraps []WrapSpec

	// Other names to run wraps by, as alias: wrap name, e.g. "z": "zellij".
	// Each is a symlink into bin.
	Aliases map[string]string

	HostDeps []string
}

type WrapSpec struct {
	Name string
	Exec string
	Env  map[string]string
	Args []string
}

type wrapperAttrs struct {
	Wraps   []WrapSpec
	Aliases map[string]string
}

var wrapperBuilder = dotman.NewBuilder("wrapper", Sources, buildWrapper)

// Wrapper returns a derivation that writes spec's wrappers. Wrappers live in
// their own derivation, apart from the program and its config, because they
// point at both: a store path can't point into itself.
func Wrapper(spec WrapperSpec) *dotman.Derivation {
	return &dotman.Derivation{
		Name:     spec.Name,
		Builder:  wrapperBuilder,
		Attrs:    wrapperAttrs{Wraps: spec.Wraps, Aliases: spec.Aliases},
		Inputs:   spec.Inputs,
		HostDeps: spec.HostDeps,
	}
}

var placeholder = regexp.MustCompile(`@([A-Za-z0-9_.-]+)@`)

func buildWrapper(b *dotman.Build) error {
	var attrs wrapperAttrs
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	// Only inputs' names are replaced, so an @ that happens to be in a
	// value, like an email address, is left alone.
	expand := func(s string) string {
		return placeholder.ReplaceAllStringFunc(s, func(m string) string {
			if path, ok := b.LookupInput(m[1 : len(m)-1]); ok {
				return path
			}
			return m
		})
	}

	binPath := filepath.Join(b.Out, "bin")
	if err := os.MkdirAll(binPath, dotman.DirPerm); err != nil {
		return err
	}

	for _, w := range attrs.Wraps {
		wrap := Wrap{
			Path: filepath.Join(binPath, w.Name),
			Exec: expand(w.Exec),
			Env:  map[string]string{},
		}
		for k, v := range w.Env {
			wrap.Env[k] = expand(v)
		}
		for _, arg := range w.Args {
			wrap.Args = append(wrap.Args, expand(arg))
		}

		b.Log.Debug("Writing wrapper.", "path", wrap.Path, "exec", wrap.Exec)
		if err := CreateWrap(wrap); err != nil {
			return err
		}
	}

	for _, alias := range slices.Sorted(maps.Keys(attrs.Aliases)) {
		if err := CreateShellAliases(binPath, attrs.Aliases[alias], []string{alias}); err != nil {
			return err
		}
	}

	return nil
}
