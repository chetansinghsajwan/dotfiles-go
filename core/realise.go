package dotman

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"time"
)

// Realiser computes derivations' store paths and builds them into a store.
type Realiser struct {
	Store *Store
	Log   *slog.Logger

	// Hash of everything builders share, mixed into every derivation that
	// isn't fixed-output. See ToolchainHash.
	Toolchain string

	// Where fixed outputs without a pinned hash find it, and record it the
	// first time they are built. nil makes such outputs an error.
	Lock *Lock

	paths    map[*Derivation]string
	hashes   map[*Derivation]string
	visiting map[*Derivation]bool
}

func NewRealiser(log *slog.Logger, store *Store, toolchain string, lock *Lock) *Realiser {
	return &Realiser{
		Store:     store,
		Log:       log,
		Toolchain: toolchain,
		Lock:      lock,
		paths:     map[*Derivation]string{},
		hashes:    map[*Derivation]string{},
		visiting:  map[*Derivation]bool{},
	}
}

// ToolchainHash hashes what every builder depends on besides its own
// source: srcs, the sources of the packages builders share (dotman's core
// and lib), and the Go version and third-party modules the binary was built
// with.
func ToolchainHash(srcs ...fs.FS) (string, error) {
	h := sha256.New()

	for _, src := range srcs {
		th, err := TreeHash(src, ".")
		if err != nil {
			return "", err
		}
		writeField(h, th)
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("binary has no build info")
	}

	writeField(h, bi.GoVersion)
	for _, dep := range bi.Deps {
		if dep.Replace != nil {
			dep = dep.Replace
		}
		writeField(h, dep.Path+"@"+dep.Version+" "+dep.Sum)
	}

	return formatHash(h), nil
}

// Record returns the JSON that drv's store path is a hash of, as show-drv
// prints it.
func (r *Realiser) Record(drv *Derivation) ([]byte, error) {
	rec, err := r.record(drv)
	if err != nil {
		return nil, err
	}

	return marshal(rec)
}

func (r *Realiser) record(drv *Derivation) (*drvRecord, error) {
	if err := validateName(drv.Name); err != nil {
		return nil, err
	}

	if drv.Fixed != nil {
		hash, err := r.fixedHash(drv)
		if err != nil {
			return nil, err
		}

		return r.fixedRecord(drv, hash), nil
	}

	if drv.Builder == nil {
		return nil, fmt.Errorf("%s: no builder", drv.Name)
	}

	attrs, err := canonicalJSON(drv.Attrs)
	if err != nil {
		return nil, fmt.Errorf("%s: attrs: %w", drv.Name, err)
	}

	inputs := map[string]string{}
	for name, input := range drv.Inputs {
		path, err := r.Path(input)
		if err != nil {
			return nil, err
		}
		inputs[name] = path
	}

	return &drvRecord{
		Name:        drv.Name,
		System:      drv.System,
		Store:       r.Store.RootPath(),
		Toolchain:   r.Toolchain,
		Builder:     drv.Builder.Name,
		BuilderHash: drv.Builder.Hash,
		Attrs:       attrs,
		Inputs:      inputs,
	}, nil
}

func (r *Realiser) fixedRecord(drv *Derivation, hash string) *drvRecord {
	return &drvRecord{
		Name:           drv.Name,
		Store:          r.Store.RootPath(),
		OutputHash:     hash,
		OutputHashMode: drv.Fixed.mode(),
	}
}

// recordPath returns the store path rec hashes to, e.g.
// <store>/w9drrwmakfqqbx94i1n5dminkvlqdv03-yazi for a record named yazi.
func (r *Realiser) recordPath(rec *drvRecord) (string, error) {
	b, err := marshal(rec)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(append([]byte("dotman-drv-v1\x00"), b...))
	return filepath.Join(r.Store.RootPath(), storeHash(sum[:])+"-"+rec.Name), nil
}

// fixedHash returns the hash fixed-output drv is pinned or locked to. A
// fixed output that is neither is built now, to find its hash, which is
// recorded in the lock; that is how a fetch is first locked.
func (r *Realiser) fixedHash(drv *Derivation) (string, error) {
	if h, ok := r.hashes[drv]; ok {
		return h, nil
	}

	f := drv.Fixed
	if f.Hash != "" {
		return f.Hash, nil
	}

	if f.Key == "" {
		return "", fmt.Errorf("%s: fixed output has neither a hash nor a lock key", drv.Name)
	}
	if r.Lock == nil {
		return "", fmt.Errorf("%s: fixed output isn't pinned and there's no lock", drv.Name)
	}

	if h, ok := r.Lock.Get(f.Key, f.mode()); ok {
		r.hashes[drv] = h
		return h, nil
	}

	h, err := r.lockFixed(drv)
	if err != nil {
		return "", err
	}

	r.hashes[drv] = h
	return h, nil
}

// lockFixed builds fixed-output drv, which has no hash yet, records the
// output's hash in the lock, and moves the output to the store path that
// hash gives it.
func (r *Realiser) lockFixed(drv *Derivation) (string, error) {
	inputs, err := r.realiseInputs(drv)
	if err != nil {
		return "", err
	}

	r.Log.Info("Fetching to lock...", "drv", drv.Name, "key", drv.Fixed.Key)

	out, err := r.tempPath(drv.Name)
	if err != nil {
		return "", err
	}

	if err := r.build(drv, inputs, out, ""); err != nil {
		return "", err
	}

	hash, err := outputHash(drv, out)
	if err != nil {
		removeTree(out)
		return "", err
	}

	path, err := r.recordPath(r.fixedRecord(drv, hash))
	if err != nil {
		removeTree(out)
		return "", err
	}

	if err := r.moveIntoPlace(out, path); err != nil {
		return "", err
	}

	r.Lock.Set(drv.Fixed.Key, drv.Fixed.mode(), hash)
	r.Log.Info("Locked.", "key", drv.Fixed.Key, "hash", hash)
	return hash, nil
}

// Path returns drv's store path, which may not exist yet. For a fetch that
// isn't locked yet, that means fetching it; see fixedHash.
func (r *Realiser) Path(drv *Derivation) (string, error) {
	if path, ok := r.paths[drv]; ok {
		return path, nil
	}

	if r.visiting[drv] {
		return "", fmt.Errorf("%s: derivation depends on itself", drv.Name)
	}
	r.visiting[drv] = true
	defer delete(r.visiting, drv)

	rec, err := r.record(drv)
	if err != nil {
		return "", err
	}

	path, err := r.recordPath(rec)
	if err != nil {
		return "", err
	}

	r.paths[drv] = path
	return path, nil
}

// Closure returns drv and every derivation it depends on, dependencies
// first, each once.
func Closure(drv *Derivation) []*Derivation {
	var order []*Derivation
	seen := map[*Derivation]bool{}

	var visit func(*Derivation)
	visit = func(d *Derivation) {
		if seen[d] {
			return
		}
		seen[d] = true

		for _, name := range slices.Sorted(maps.Keys(d.Inputs)) {
			visit(d.Inputs[name])
		}
		order = append(order, d)
	}
	visit(drv)

	return order
}

// Realise builds drv and its inputs into the store, skipping any whose
// store path exists, and returns drv's store path.
func (r *Realiser) Realise(drv *Derivation) (string, error) {
	path, err := r.Path(drv)
	if err != nil {
		return "", err
	}

	if _, err := os.Lstat(path); err == nil {
		r.Log.Debug("Already built.", "drv", drv.Name, "path", path)
		return path, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	inputs, err := r.realiseInputs(drv)
	if err != nil {
		return "", err
	}

	// A fixed output is checked against its locked or pinned hash, which
	// Path has already resolved.
	var want string
	if drv.Fixed != nil {
		if want, err = r.fixedHash(drv); err != nil {
			return "", err
		}
	}

	r.Log.Info("Building...", "drv", drv.Name)

	out, err := r.tempPath(drv.Name)
	if err != nil {
		return "", err
	}

	if err := r.build(drv, inputs, out, want); err != nil {
		return "", err
	}

	if err := r.moveIntoPlace(out, path); err != nil {
		return "", err
	}

	r.Log.Debug("Built.", "drv", drv.Name, "path", path)
	return path, nil
}

func (r *Realiser) realiseInputs(drv *Derivation) (map[string]string, error) {
	inputs := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(drv.Inputs)) {
		path, err := r.Realise(drv.Inputs[name])
		if err != nil {
			return nil, err
		}
		inputs[name] = path
	}

	return inputs, nil
}

// moveIntoPlace renames the built output out to its store path. If another
// run built the same path first, out is dropped.
func (r *Realiser) moveIntoPlace(out, path string) error {
	err := os.Rename(out, path)
	if err == nil {
		return nil
	}

	removeTree(out)
	if _, statErr := os.Lstat(path); statErr == nil {
		return nil
	}

	return err
}

// Check rebuilds every derivation in drv's closure that isn't fixed-output,
// after realising it, and fails if any rebuild differs from the store path.
func (r *Realiser) Check(drv *Derivation) error {
	if _, err := r.Realise(drv); err != nil {
		return err
	}

	for _, d := range Closure(drv) {
		if d.Fixed != nil {
			continue
		}

		path, err := r.Path(d)
		if err != nil {
			return err
		}

		inputs := map[string]string{}
		for name, input := range d.Inputs {
			if inputs[name], err = r.Path(input); err != nil {
				return err
			}
		}

		out, err := r.tempPath(d.Name)
		if err != nil {
			return err
		}

		r.Log.Info("Checking...", "drv", d.Name)
		if err := r.build(d, inputs, out, ""); err != nil {
			return err
		}

		want, err := PathTreeHash(path)
		if err != nil {
			removeTree(out)
			return err
		}

		got, err := PathTreeHash(out)
		removeTree(out)
		if err != nil {
			return err
		}

		if got != want {
			return fmt.Errorf("%s: rebuild differs from %s: want %s, got %s", d.Name, path, want, got)
		}
	}

	return nil
}

// tempPath returns a path in the store to build name into before renaming
// it to its store path, so that path is never seen half built. It is in the
// store's root so the rename never moves the output between directories.
// For "yazi" it is like <store>/.tmp-1a2b3c4d5e6f7a8b-yazi; the random part
// keeps concurrent builds of the same name apart. It isn't created.
func (r *Realiser) tempPath(name string) (string, error) {
	if err := os.MkdirAll(r.Store.RootPath(), DirPerm); err != nil {
		return "", err
	}

	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return filepath.Join(r.Store.RootPath(), tempPrefix+hex.EncodeToString(b[:])+"-"+name), nil
}

// build runs drv's builder into out, then checks and normalizes the output,
// and checks it has the hash want if that is set. out is removed if any of
// that fails.
func (r *Realiser) build(drv *Derivation, inputs map[string]string, out, want string) (err error) {
	defer func() {
		if err != nil {
			removeTree(out)
		}
	}()

	for _, dep := range drv.HostDeps {
		if _, err := exec.LookPath(dep); err != nil {
			r.Log.Warn("Host dependency not found on PATH.", "drv", drv.Name, "dep", dep)
		}
	}

	attrs, err := canonicalJSON(drv.Attrs)
	if err != nil {
		return fmt.Errorf("%s: attrs: %w", drv.Name, err)
	}

	b := &Build{
		Out:    out,
		Log:    r.Log.With("drv", drv.Name),
		attrs:  attrs,
		inputs: inputs,
	}

	if err := runBuilder(drv.Builder, b); err != nil {
		return fmt.Errorf("building %s: %w", drv.Name, err)
	}

	if _, err := os.Lstat(out); err != nil {
		return fmt.Errorf("building %s: builder wrote no output: %w", drv.Name, err)
	}

	if err := checkSelfRefs(out); err != nil {
		return fmt.Errorf("building %s: %w", drv.Name, err)
	}

	if err := normalize(out); err != nil {
		return fmt.Errorf("building %s: normalizing output: %w", drv.Name, err)
	}

	if want != "" {
		got, err := outputHash(drv, out)
		if err != nil {
			return err
		}

		if got != want {
			return &HashMismatchError{Name: drv.Name, Want: want, Got: got}
		}
	}

	return nil
}

// runBuilder runs builder, turning a panic, like Build.Input's for a missing
// input, into an error.
func runBuilder(builder *Builder, b *Build) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("builder %s panicked: %v", builder.Name, p)
		}
	}()

	return builder.Build(b)
}

// outputHash returns the hash of fixed-output drv's output at out, flat or
// recursive as drv says.
func outputHash(drv *Derivation, out string) (string, error) {
	if drv.Fixed.Recursive {
		return PathTreeHash(out)
	}

	info, err := os.Lstat(out)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: flat fixed output must be a regular file", drv.Name)
	}

	return FileHash(out)
}

// checkSelfRefs fails if any file or symlink target under out contains out,
// which would dangle once out is renamed to its store path. An output that
// needs to point into itself must be split into derivations that point at
// each other instead.
func checkSelfRefs(out string) error {
	needle := []byte(out)

	return filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(out, path)

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if bytes.Contains([]byte(target), needle) {
				return fmt.Errorf("self-reference in symlink %s; split the derivation", rel)
			}

		case d.Type().IsRegular():
			found, err := fileContains(path, needle)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("self-reference in %s; split the derivation", rel)
			}
		}

		return nil
	})
}

// fileContains reports whether the file at path contains needle, reading it
// in chunks that overlap by len(needle)-1 bytes so no match is split.
func fileContains(path string, needle []byte) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 64*1024+len(needle))
	keep := 0
	for {
		n, err := f.Read(buf[keep:])
		window := buf[:keep+n]
		if bytes.Contains(window, needle) {
			return true, nil
		}

		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		keep = min(len(needle)-1, len(window))
		copy(buf, window[len(window)-keep:])
	}
}

// epoch is the time every store file gets, like Nix's 1, so outputs don't
// differ by when they were built.
var epoch = time.Unix(1, 0)

// normalize makes everything under out read-only, keeping files' executable
// bit, and sets its times to epoch. Children are done before their
// directories, since a directory's time changes when its entries do.
func normalize(out string) error {
	var paths []string
	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}

	for _, path := range slices.Backward(paths) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		var mode fs.FileMode
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			// Symlinks have no mode of their own, and os can't set their
			// times without following them.
			continue
		case info.IsDir():
			mode = ReadOnlyExecPerm
		case info.Mode().IsRegular():
			mode = ReadOnlyPerm
			if info.Mode()&0o111 != 0 {
				mode = ReadOnlyExecPerm
			}
		default:
			return fmt.Errorf("%s: unsupported file type %s", path, info.Mode().Type())
		}

		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		if err := os.Chtimes(path, epoch, epoch); err != nil {
			return err
		}
	}

	return nil
}

// removeTree removes path even though normalize made it read-only.
func removeTree(path string) error {
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			os.Chmod(p, DirPerm)
		}
		return nil
	})

	return os.RemoveAll(path)
}
