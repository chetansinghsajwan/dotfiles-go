package dotman

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestRealiser(t *testing.T) *Realiser {
	t.Helper()
	store := NewStoreWithPath(filepath.Join(t.TempDir(), "store"))
	t.Cleanup(func() { removeTree(store.RootPath()) })
	return NewRealiser(slog.New(slog.NewTextHandler(io.Discard, nil)), store, "toolchain")
}

// writer returns a derivation whose output is a file holding content, and
// counts its builds in *builds.
func writer(name, content string, builds *int) *Derivation {
	return &Derivation{
		Name: name,
		Builder: &Builder{Name: "writer", Hash: "h", Build: func(b *Build) error {
			*builds++
			var attrs struct{ Content string }
			if err := b.Decode(&attrs); err != nil {
				return err
			}
			return os.WriteFile(b.Out, []byte(attrs.Content), FilePerm)
		}},
		Attrs: struct{ Content string }{content},
	}
}

func TestPathIsDeterministic(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	a, err := r.Path(writer("x", "hello", &builds))
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Path(writer("x", "hello", &builds))
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Path(writer("x", "bye", &builds))
	if err != nil {
		t.Fatal(err)
	}

	if a != b {
		t.Errorf("same derivation got different paths: %s, %s", a, b)
	}
	if a == c {
		t.Errorf("different attrs got the same path %s", a)
	}
}

func TestPathDependsOnInputs(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	parent := func(content string) *Derivation {
		d := writer("parent", "same", &builds)
		d.Inputs = map[string]*Derivation{"in": writer("child", content, &builds)}
		return d
	}

	a, _ := r.Path(parent("one"))
	b, _ := r.Path(parent("two"))
	if a == b {
		t.Errorf("changing an input didn't change the path")
	}
}

func TestAttrsFieldOrderDoesNotMatter(t *testing.T) {
	r := newTestRealiser(t)
	builder := &Builder{Name: "b", Hash: "h"}

	a, _ := r.Path(&Derivation{Name: "x", Builder: builder, Attrs: struct{ A, B int }{1, 2}})
	b, _ := r.Path(&Derivation{Name: "x", Builder: builder, Attrs: struct{ B, A int }{2, 1}})
	if a != b {
		t.Errorf("reordering attrs fields changed the path")
	}
}

func TestRealiseSkipsExistingPath(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	path, err := r.Realise(writer("x", "hello", &builds))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Realise(writer("x", "hello", &builds)); err != nil {
		t.Fatal(err)
	}

	if builds != 1 {
		t.Errorf("built %d times, want 1", builds)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello" {
		t.Errorf("output = %q", got)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != ReadOnlyPerm || !info.ModTime().Equal(epoch) {
		t.Errorf("output not normalized: mode %v, time %v", info.Mode(), info.ModTime())
	}
}

func TestRealiseRejectsSelfReference(t *testing.T) {
	r := newTestRealiser(t)

	drv := &Derivation{
		Name: "self",
		Builder: &Builder{Name: "b", Build: func(b *Build) error {
			if err := os.MkdirAll(filepath.Join(b.Out, "bin"), DirPerm); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(b.Out, "bin", "x"), []byte("exec "+b.Out+"/libexec/x"), ExecPerm)
		}},
	}

	_, err := r.Realise(drv)
	if err == nil || !strings.Contains(err.Error(), "self-reference in bin/x") {
		t.Fatalf("err = %v, want a self-reference error", err)
	}

	assertNoTempPaths(t, r.Store)
}

func TestRealiseRejectsSelfReferencingSymlink(t *testing.T) {
	r := newTestRealiser(t)

	drv := &Derivation{
		Name: "self",
		Builder: &Builder{Name: "b", Build: func(b *Build) error {
			if err := os.MkdirAll(b.Out, DirPerm); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(b.Out, "a"), filepath.Join(b.Out, "b"))
		}},
	}

	if _, err := r.Realise(drv); err == nil || !strings.Contains(err.Error(), "self-reference in symlink b") {
		t.Fatalf("err = %v, want a self-reference error", err)
	}
}

func TestFixedOutputHashMismatch(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	drv := writer("fetched", "content", &builds)
	drv.Fixed = &FixedOutput{Hash: FakeHash}

	_, err := r.Realise(drv)
	mismatch, ok := errors.AsType[*HashMismatchError](err)
	if !ok {
		t.Fatalf("err = %v, want *HashMismatchError", err)
	}
	assertNoTempPaths(t, r.Store)

	// Pinning the reported hash makes it build.
	drv.Fixed.Hash = mismatch.Got
	if _, err := r.Realise(drv); err != nil {
		t.Fatal(err)
	}
}

func TestFixedOutputPathIgnoresBuilder(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	a := writer("f", "one", &builds)
	a.Fixed = &FixedOutput{Hash: "sha256:00"}
	b := writer("f", "two", &builds)
	b.Fixed = &FixedOutput{Hash: "sha256:00"}

	pa, _ := r.Path(a)
	pb, _ := r.Path(b)
	if pa != pb {
		t.Errorf("fixed-output path depends on more than name and hash")
	}
}

func TestUnpinnedFixedOutputFails(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	drv := writer("f", "x", &builds)
	drv.Fixed = &FixedOutput{}
	if _, err := r.Path(drv); err == nil {
		t.Fatal("unpinned fixed output got a path")
	}
}

func TestDecodeKeepsIntegers(t *testing.T) {
	r := newTestRealiser(t)

	var got map[string]any
	drv := &Derivation{
		Name: "x",
		Builder: &Builder{Name: "b", Build: func(b *Build) error {
			if err := b.Decode(&got); err != nil {
				return err
			}
			return os.MkdirAll(b.Out, DirPerm)
		}},
		Attrs: map[string]any{
			"n":      1920,
			"f":      1.5,
			"nested": []map[string]any{{"m": 3}},
		},
	}

	if _, err := r.Realise(drv); err != nil {
		t.Fatal(err)
	}

	if _, ok := got["n"].(int64); !ok {
		t.Errorf("n = %T, want int64", got["n"])
	}
	if _, ok := got["f"].(float64); !ok {
		t.Errorf("f = %T, want float64", got["f"])
	}
	nested := got["nested"].([]any)[0].(map[string]any)
	if _, ok := nested["m"].(int64); !ok {
		t.Errorf("nested m = %T, want int64", nested["m"])
	}
}

func TestLocalSource(t *testing.T) {
	r := newTestRealiser(t)

	fsys := fstest.MapFS{
		"main.lua":     {Data: []byte("return {}")},
		"lib/util.lua": {Data: []byte("x")},
	}

	drv, err := LocalSource("plugin", fsys)
	if err != nil {
		t.Fatal(err)
	}

	path, err := r.Realise(drv)
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(path, "lib", "util.lua"))
	if string(got) != "x" {
		t.Errorf("util.lua = %q", got)
	}
}

func TestCheck(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	if err := r.Check(writer("x", "hello", &builds)); err != nil {
		t.Fatal(err)
	}

	n := 0
	nondeterministic := &Derivation{
		Name: "random",
		Builder: &Builder{Name: "b", Build: func(b *Build) error {
			n++
			return os.WriteFile(b.Out, []byte(strings.Repeat("x", n)), FilePerm)
		}},
	}
	if err := r.Check(nondeterministic); err == nil {
		t.Fatal("Check passed a build that differs each time")
	}
	assertNoTempPaths(t, r.Store)
}

func TestGC(t *testing.T) {
	r := newTestRealiser(t)
	var builds int

	dep := writer("dep", "data", &builds)
	depPath, err := r.Realise(dep)
	if err != nil {
		t.Fatal(err)
	}

	// root references dep by absolute path, as wrappers do.
	root := writer("root", "exec "+depPath, &builds)
	root.Inputs = map[string]*Derivation{"dep": dep}
	rootPath, err := r.Realise(root)
	if err != nil {
		t.Fatal(err)
	}

	garbagePath, err := r.Realise(writer("garbage", "x", &builds))
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := r.Store.GC(r.Log, []string{rootPath})
	if err != nil {
		t.Fatal(err)
	}

	if len(deleted) != 1 || deleted[0] != filepath.Base(garbagePath) {
		t.Errorf("deleted %v, want only %s", deleted, filepath.Base(garbagePath))
	}
	for _, p := range []string{rootPath, depPath} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("live path %s deleted", p)
		}
	}
}

func TestGenerations(t *testing.T) {
	g := NewGenerations(t.TempDir())

	for i, target := range []string{"/store/a", "/store/b", "/store/b"} {
		n, err := g.Switch(target)
		if err != nil {
			t.Fatal(err)
		}
		if want := min(i+1, 2); n != want {
			t.Errorf("switch %d: generation %d, want %d", i, n, want)
		}
	}

	if n, err := g.Rollback(); err != nil || n != 1 {
		t.Fatalf("rollback = %d, %v; want 1", n, err)
	}

	if n, _ := g.Current(); n != 1 {
		t.Errorf("current generation after rollback = %d, want 1", n)
	}

	if _, err := g.Rollback(); err == nil {
		t.Error("rolled back past the first generation")
	}

	// Switching after a rollback adds a generation after the newest.
	if n, _ := g.Switch("/store/c"); n != 3 {
		t.Errorf("switch after rollback: generation %d, want 3", n)
	}

	deleted, err := g.Delete(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 {
		t.Errorf("deleted %v, want generations 1 and 2", deleted)
	}
}

func assertNoTempPaths(t *testing.T, s *Store) {
	t.Helper()
	entries, _ := os.ReadDir(s.RootPath())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tempPrefix) {
			t.Errorf("temp path %s left behind", e.Name())
		}
	}
}
