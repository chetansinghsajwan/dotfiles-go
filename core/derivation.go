package dotman

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Derivation describes how to build one store path. It is plain data: its
// store path is a hash of it, computed before building, so the same
// derivation always gets the same path, and a path that exists has been
// built.
type Derivation struct {
	// Names the store path, after its hash. It can't contain '/'.
	Name string

	// System the output is for, e.g. "linux/amd64". "" for outputs that
	// don't depend on it, like config files.
	System string

	Builder *Builder

	// The builder's settings. It is serialized to JSON, which is what is
	// hashed and all the builder ever sees (see Build.Decode), so anything
	// that doesn't survive the round trip can't affect the build.
	Attrs any

	// Store paths the build reads, by a name the builder looks them up by
	// (see Build.Input). They are realised first, and their paths, not their
	// contents, are hashed, so changing one changes this one too.
	Inputs map[string]*Derivation

	// Fixed, if set, makes the store path depend only on the output's
	// content hash, instead of on the derivation, like Nix's fixed-output
	// derivations. It is for fetches: the output is checked against the
	// hash, so the builder and its inputs are free to change, or to be
	// impure, like a download.
	Fixed *FixedOutput

	// Programs the output runs from the host's PATH rather than from the
	// store. They aren't hashed; building warns about any that are missing.
	HostDeps []string
}

// FixedOutput describes a fixed output's content hash.
type FixedOutput struct {
	// Pins the output to this "sha256:<hex>". If it is "", the hash comes
	// from the lock under Key, and the first build records it there.
	Hash string

	// Names the output in the lock, e.g. the url it is downloaded from. It
	// must be set if Hash isn't.
	Key string

	// Recursive hashes the output as a tree (see TreeHash); otherwise the
	// output must be a single file, hashed flat (see FileHash).
	Recursive bool
}

// mode returns how f's output is hashed, as the lock records it.
func (f *FixedOutput) mode() string {
	if f.Recursive {
		return "recursive"
	}
	return "flat"
}

// Builder builds a derivation's output.
type Builder struct {
	// Name identifies the builder in the derivation.
	Name string

	// Hash of the builder's source, so changing how a builder builds
	// changes its derivations' store paths. See NewBuilder.
	Hash string

	Build BuildFunc
}

// BuildFunc writes a derivation's output to b.Out.
type BuildFunc func(b *Build) error

// NewBuilder returns a builder whose Hash is the TreeHash of src, which
// should be the source of the package that builds, usually an embed.FS of
// its *.go and template files:
//
//	//go:embed *.go *.toml
//	var src embed.FS
//
// It panics if src can't be hashed, since that's a build-time mistake.
func NewBuilder(name string, src fs.FS, build BuildFunc) *Builder {
	h, err := TreeHash(src, ".")
	if err != nil {
		panic(fmt.Sprintf("builder %s: hashing source: %v", name, err))
	}

	return &Builder{Name: name, Hash: h, Build: build}
}

// Build is what a BuildFunc gets to build an output with.
type Build struct {
	// Where to write the output, as a directory or, for a flat
	// fixed-output derivation, a file. It doesn't exist yet. It is a
	// temporary path that is renamed into place afterwards, so the output
	// must not contain it.
	Out string

	Log *slog.Logger

	attrs  []byte
	inputs map[string]string
}

// Input returns the store path of the input called name. It panics if the
// derivation has no such input, which fails the build.
func (b *Build) Input(name string) string {
	path, ok := b.inputs[name]
	if !ok {
		panic(fmt.Sprintf("no input %q", name))
	}

	return path
}

// LookupInput returns the store path of the input called name, if the
// derivation has one.
func (b *Build) LookupInput(name string) (string, bool) {
	path, ok := b.inputs[name]
	return path, ok
}

// Decode unmarshals the derivation's Attrs into v. Numbers in interface
// values decode as int64 when they are whole and float64 otherwise, rather
// than encoding/json's float64, so a 1920 in a settings map stays 1920.
func (b *Build) Decode(v any) error {
	dec := json.NewDecoder(bytes.NewReader(b.attrs))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decoding attrs: %w", err)
	}

	return fixNumbers(reflect.ValueOf(v))
}

// fixNumbers replaces each json.Number held in an interface value under v
// with an int64 or float64.
func fixNumbers(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			return fixNumbers(v.Elem())
		}

	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				if err := fixNumbers(v.Field(i)); err != nil {
					return err
				}
			}
		}

	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := fixNumbers(v.Index(i)); err != nil {
				return err
			}
		}

	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(iter.Value())
			if err := fixNumbers(elem); err != nil {
				return err
			}
			v.SetMapIndex(iter.Key(), elem)
		}

	case reflect.Interface:
		if v.IsNil() {
			return nil
		}

		elem := v.Elem()
		if n, ok := elem.Interface().(json.Number); ok {
			fixed, err := numberValue(n)
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(fixed))
			return nil
		}

		// Maps and slices share their backing storage, so fixing a copy
		// fixes the original.
		return fixNumbers(elem)
	}

	return nil
}

// numberValue converts n to an int64 if it is a whole number that fits, and
// a float64 otherwise:
//
//	"1920" -> int64(1920)
//	"-2"   -> int64(-2)
//	"1.5"  -> float64(1.5)
//	"1e3"  -> float64(1000)
func numberValue(n json.Number) (any, error) {
	if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
		return i, nil
	}

	return strconv.ParseFloat(string(n), 64)
}

// drvRecord is a derivation as it is hashed and shown by show-drv.
type drvRecord struct {
	Name        string            `json:"name"`
	System      string            `json:"system,omitempty"`
	Store       string            `json:"store"`
	Toolchain   string            `json:"toolchain,omitempty"`
	Builder     string            `json:"builder,omitempty"`
	BuilderHash string            `json:"builderHash,omitempty"`
	Attrs       json.RawMessage   `json:"attrs,omitempty"`
	Inputs      map[string]string `json:"inputs,omitempty"`

	OutputHash     string `json:"outputHash,omitempty"`
	OutputHashMode string `json:"outputHashMode,omitempty"`
}

// canonicalJSON marshals v, then sorts every object's keys, so the result
// doesn't depend on struct field order. Numbers keep the text the first
// marshal gave them. For example,
//
//	struct{ B int; A string; M map[string]any }{2, "<x>", map[string]any{"z": 1, "a": 1.50}}
//
// gives {"A":"<x>","B":2,"M":{"a":1.5,"z":1}}.
func canonicalJSON(v any) (json.RawMessage, error) {
	b, err := marshal(v)
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}

	return marshal(generic)
}

// marshal is json.Marshal without HTML escaping, so attrs holding scripts
// read naturally in show-drv.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// validateName checks name can follow the hash in a store path's name. It
// can't be empty, hold a '/', or start with '.', which would collide with
// temporary outputs like .tmp-1a2b3c4d5e6f7a8b-yazi:
//
//	"yazi-config" -> ok
//	"", "a/b", ".tmp-x" -> error
func validateName(name string) error {
	if name == "" || strings.ContainsRune(name, '/') || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid derivation name %q", name)
	}

	return nil
}

// LocalSource returns a fixed-output derivation that copies fsys, usually an
// embed.FS, into the store, so it can be an input like any other store
// path. Its store path depends only on its name and contents.
func LocalSource(name string, fsys fs.FS) (*Derivation, error) {
	h, err := TreeHash(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("hashing source %s: %w", name, err)
	}

	// The builder closes over fsys, which would break the rule that builds
	// only see what is hashed, but the output is checked against h.
	builder := &Builder{
		Name: "local-source",
		Build: func(b *Build) error {
			return os.CopyFS(b.Out, fsys)
		},
	}

	return &Derivation{
		Name:    name,
		Builder: builder,
		Fixed:   &FixedOutput{Hash: h, Recursive: true},
	}, nil
}
