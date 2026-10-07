// Profiles merge the packages' store paths into one tree, like Nix's
// buildEnv, and generations switch the user between profiles.

package dotman

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// mergedDirs are the subdirectories of a package's store path that are merged
// into the profile.
var mergedDirs = []string{"bin", "share"}

var profileBuilder = NewBuilder("profile", Sources, buildProfile)

// Profile returns a derivation that links every file under mergedDirs of
// each of packages, by name, into one tree. Directories are merged; two
// packages providing the same file is an error.
func Profile(packages map[string]*Derivation) *Derivation {
	return &Derivation{
		Name:    "profile",
		Builder: profileBuilder,
		Attrs:   struct{ Dirs []string }{mergedDirs},
		Inputs:  packages,
	}
}

func buildProfile(b *Build) error {
	var attrs struct{ Dirs []string }
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	if err := os.MkdirAll(b.Out, DirPerm); err != nil {
		return err
	}

	for _, name := range slices.Sorted(maps.Keys(b.inputs)) {
		storePath := b.inputs[name]

		for _, dir := range attrs.Dirs {
			root := filepath.Join(storePath, dir)
			if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
				continue
			}

			err := filepath.WalkDir(root, func(src string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				rel, err := filepath.Rel(storePath, src)
				if err != nil {
					return err
				}
				dst := filepath.Join(b.Out, rel)

				if d.IsDir() {
					return os.MkdirAll(dst, DirPerm)
				}

				b.Log.Debug("Linking into profile.", "src", src, "dst", dst)
				if err := os.Symlink(src, dst); err != nil {
					if errors.Is(err, fs.ErrExist) {
						existing, _ := os.Readlink(dst)
						return fmt.Errorf("%s is provided by both %s and %s", rel, existing, src)
					}

					return err
				}

				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// DefaultStateDir returns $XDG_STATE_HOME/dotman, falling back to
// ~/.local/state/dotman. It holds the profile link, which always points at
// the current generation, so its bin can be added to PATH once, and the
// generations.
func DefaultStateDir() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "dotman"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "state", "dotman"), nil
}

// Generations are the profiles the user has switched to, numbered from 1,
// each a link profiles/profile-<N>-link to a profile's store path. They are
// the store's GC roots, and keep old profiles around to roll back to.
type Generations struct {
	stateDir string
}

func NewGenerations(stateDir string) *Generations {
	return &Generations{stateDir: stateDir}
}

type Generation struct {
	Number int

	// The profile's store path.
	Target string
}

// ProfileLink returns the path of the link to the current generation.
func (g *Generations) ProfileLink() string {
	return filepath.Join(g.stateDir, "profile")
}

func (g *Generations) dir() string {
	return filepath.Join(g.stateDir, "profiles")
}

// generationName returns the name of generation n's link, e.g.
// "profile-12-link" for 12.
func generationName(n int) string {
	return "profile-" + strconv.Itoa(n) + "-link"
}

// parseGenerationName returns the generation a link's name is for:
//
//	"profile-12-link" -> 12, true
//	"profile-0-link"  -> 0, false (generations start at 1)
//	"profile-x-link"  -> 0, false
//	"profile"         -> 0, false (the link to the current generation)
func parseGenerationName(name string) (int, bool) {
	s, ok := strings.CutPrefix(name, "profile-")
	if !ok {
		return 0, false
	}
	s, ok = strings.CutSuffix(s, "-link")
	if !ok {
		return 0, false
	}

	n, err := strconv.Atoi(s)
	return n, err == nil && n > 0
}

// List returns the generations, oldest first.
func (g *Generations) List() ([]Generation, error) {
	entries, err := os.ReadDir(g.dir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var gens []Generation
	for _, e := range entries {
		n, ok := parseGenerationName(e.Name())
		if !ok {
			continue
		}

		target, err := os.Readlink(filepath.Join(g.dir(), e.Name()))
		if err != nil {
			return nil, err
		}

		gens = append(gens, Generation{Number: n, Target: target})
	}

	slices.SortFunc(gens, func(a, b Generation) int { return a.Number - b.Number })
	return gens, nil
}

// Current returns the number of the generation the profile link points at,
// or 0 if it doesn't point at one, like before the first switch.
func (g *Generations) Current() (int, error) {
	target, err := os.Readlink(g.ProfileLink())
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	if filepath.Dir(target) != "profiles" {
		return 0, nil
	}

	n, _ := parseGenerationName(filepath.Base(target))
	return n, nil
}

// Switch makes profilePath the current generation, adding a generation for
// it unless the current one already points at it, and returns its number.
func (g *Generations) Switch(profilePath string) (int, error) {
	gens, err := g.List()
	if err != nil {
		return 0, err
	}

	current, err := g.Current()
	if err != nil {
		return 0, err
	}

	for _, gen := range gens {
		if gen.Number == current && gen.Target == profilePath {
			return current, nil
		}
	}

	n := 1
	if len(gens) > 0 {
		n = gens[len(gens)-1].Number + 1
	}

	if err := os.MkdirAll(g.dir(), DirPerm); err != nil {
		return 0, err
	}

	if err := os.Symlink(profilePath, filepath.Join(g.dir(), generationName(n))); err != nil {
		return 0, err
	}

	return n, g.point(n)
}

// Rollback makes the generation before the current one current, and
// returns its number.
func (g *Generations) Rollback() (int, error) {
	gens, err := g.List()
	if err != nil {
		return 0, err
	}

	current, err := g.Current()
	if err != nil {
		return 0, err
	}

	prev := 0
	for _, gen := range gens {
		if gen.Number < current {
			prev = gen.Number
		}
	}

	if prev == 0 {
		return 0, errors.New("no generation before the current one")
	}

	return prev, g.point(prev)
}

// Delete removes every generation but the newest keep and the current one.
func (g *Generations) Delete(keep int) ([]int, error) {
	gens, err := g.List()
	if err != nil {
		return nil, err
	}

	current, err := g.Current()
	if err != nil {
		return nil, err
	}

	var deleted []int
	for i, gen := range gens {
		if i >= len(gens)-keep || gen.Number == current {
			continue
		}

		if err := os.Remove(filepath.Join(g.dir(), generationName(gen.Number))); err != nil {
			return deleted, err
		}
		deleted = append(deleted, gen.Number)
	}

	return deleted, nil
}

// point atomically points the profile link at generation n. The link is
// relative, so the state dir can move.
func (g *Generations) point(n int) error {
	link := g.ProfileLink()

	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}

	tmp := link + ".tmp-" + hex.EncodeToString(b[:])
	if err := os.Symlink(filepath.Join("profiles", generationName(n)), tmp); err != nil {
		return err
	}

	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}

	return nil
}
