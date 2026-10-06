package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"dotman/configured/lazygit"
	"dotman/configured/op"
	"dotman/configured/pv"
	"dotman/configured/yazi"
	"dotman/configured/zellij"
	"dotman/core"
	"dotman/lib"
	"dotman/logging"
)

var packages = []dotman.Package{
	&yazi.Yazi,
	&lazygit.Lazygit,
	&zellij.Zellij,
	&pv.PvPkg,
	&op.OpPkg,
}

const usage = `usage: dotman <command> [args]

commands:
  switch               build the profile and make it the current generation
                       (the default)
  build [--check] [pkg...]
                       build packages, or the profile and so every package,
                       and print their store paths; --check also rebuilds
                       them and fails if any rebuild differs
  rollback             make the previous generation current
  generations          list generations
  gc [--keep N]        delete all but the newest N generations, if given,
                       then every store path no generation uses
  show-drv [-r] <pkg>  print the derivation a package's store path is a hash
                       of; -r also prints its inputs'
  lock [--all-systems] fetch and lock every download not in dotman.lock yet,
                       for this system or every system, and drop entries no
                       system uses
  update [pkg...]      forget the locked hashes of packages' downloads, or of
                       every download, and lock them again, accepting files
                       that changed upstream
`

// devMode reports whether DOTMAN_DEV is set to a true value, like 1 or true.
func devMode() bool {
	dev, _ := strconv.ParseBool(os.Getenv("DOTMAN_DEV"))
	return dev
}

func main() {
	level := slog.LevelInfo
	if devMode() {
		level = slog.LevelDebug
	}

	// The store root is resolved before logging is set up so the handler can
	// shorten paths under it.
	storeRoot, storeErr := dotman.DefaultStorePath()

	slog.SetDefault(slog.New(logging.NewHandler(os.Stderr, level, storeRoot)))

	if storeErr != nil {
		slog.Error("Failed to find store.", "err", storeErr)
		os.Exit(1)
	}

	if err := run(storeRoot, os.Args[1:]); err != nil {
		slog.Error(err.Error())

		if _, ok := errors.AsType[*dotman.HashMismatchError](err); ok {
			slog.Error("A download doesn't match dotman.lock. If it changed upstream on purpose, run `dotman update <pkg>`.")
		}

		os.Exit(1)
	}
}

// lockPath returns the path of dotman.lock, at the root of the checkout
// dotman is built from, next to the config it locks. Like lib.Rel, it relies
// on running from that checkout, as dm.sh does.
func lockPath() string {
	return lib.Rel("../../dotman.lock")
}

type app struct {
	log  *slog.Logger
	ev   *dotman.Eval
	r    *dotman.Realiser
	lock *dotman.Lock
	gens *dotman.Generations
}

func run(storeRoot string, args []string) (err error) {
	cmd := "switch"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return nil
	}

	toolchain, err := dotman.ToolchainHash(dotman.Sources, lib.Sources)
	if err != nil {
		return fmt.Errorf("hashing toolchain: %w", err)
	}

	stateDir, err := dotman.DefaultStateDir()
	if err != nil {
		return err
	}

	lock, err := dotman.LoadLock(lockPath())
	if err != nil {
		return err
	}

	// Hashes locked before a failure are still right, so the lock is saved
	// either way.
	defer func() {
		if saveErr := lock.Save(); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("saving lock: %w", saveErr))
		}
	}()

	a := &app{
		log:  slog.Default(),
		ev:   dotman.NewEval(dotman.DefaultConfig),
		r:    dotman.NewRealiser(slog.Default(), dotman.NewStoreWithPath(storeRoot), toolchain, lock),
		lock: lock,
		gens: dotman.NewGenerations(stateDir),
	}

	switch cmd {
	case "switch":
		return a.switchCmd(args)
	case "build":
		return a.buildCmd(args)
	case "rollback":
		return a.rollbackCmd(args)
	case "generations":
		return a.generationsCmd(args)
	case "gc":
		return a.gcCmd(args)
	case "show-drv":
		return a.showDrvCmd(args)
	case "lock":
		return a.lockCmd(args)
	case "update":
		return a.updateCmd(args)
	}

	fmt.Fprint(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

func noArgs(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%s takes no arguments", fs.Name())
	}
	return nil
}

// derive returns each package's derivation for this system, by package
// name.
func (a *app) derive() (map[string]*dotman.Derivation, error) {
	return a.deriveFor(a.ev)
}

func (a *app) deriveFor(ev *dotman.Eval) (map[string]*dotman.Derivation, error) {
	drvs := map[string]*dotman.Derivation{}
	for _, p := range packages {
		if _, dup := drvs[p.Name()]; dup {
			return nil, fmt.Errorf("two packages named %s", p.Name())
		}

		drv, err := p.Derive(ev)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Name(), err)
		}
		drvs[p.Name()] = drv
	}

	return drvs, nil
}

// selectDrvs returns the derivations of the packages called names, or the
// profile's, which has every package as an input, if there are none.
// "profile" names the profile.
func (a *app) selectDrvs(names []string) ([]*dotman.Derivation, error) {
	drvs, err := a.derive()
	if err != nil {
		return nil, err
	}

	return selectFrom(drvs, names)
}

func selectFrom(drvs map[string]*dotman.Derivation, names []string) ([]*dotman.Derivation, error) {
	profile := dotman.Profile(drvs)

	if len(names) == 0 {
		return []*dotman.Derivation{profile}, nil
	}

	var selected []*dotman.Derivation
	for _, name := range names {
		if name == "profile" {
			selected = append(selected, profile)
			continue
		}

		drv, ok := drvs[name]
		if !ok {
			return nil, fmt.Errorf("no package named %s", name)
		}
		selected = append(selected, drv)
	}

	return selected, nil
}

func (a *app) switchCmd(args []string) error {
	if err := noArgs(flag.NewFlagSet("switch", flag.ContinueOnError), args); err != nil {
		return err
	}

	drvs, err := a.derive()
	if err != nil {
		return err
	}

	profilePath, err := a.r.Realise(dotman.Profile(drvs))
	if err != nil {
		return err
	}

	n, err := a.gens.Switch(profilePath)
	if err != nil {
		return fmt.Errorf("switching profile: %w", err)
	}

	a.log.Info("Switched profile.", "generation", n, "profile", profilePath)

	// The switch already happened, so a lock that can't be pruned only
	// keeps a few unused entries until the next run.
	if err := a.pruneLock(); err != nil {
		a.log.Warn("Not pruning lock.", "err", err)
	}

	return nil
}

// evalsFor returns an Eval for each system in dotman.Systems, or only this
// one.
func (a *app) evalsFor(allSystems bool) []*dotman.Eval {
	if !allSystems {
		return []*dotman.Eval{a.ev}
	}

	var evs []*dotman.Eval
	for _, system := range dotman.Systems {
		evs = append(evs, &dotman.Eval{Config: a.ev.Config, System: system})
	}
	return evs
}

// lockKeys returns the lock keys of the downloads of the packages called
// names, or of every package, on every system, so a lock shared between
// machines keeps what each of them needs.
func (a *app) lockKeys(names []string) (map[string]bool, error) {
	keys := map[string]bool{}
	for _, ev := range a.evalsFor(true) {
		drvs, err := a.deriveFor(ev)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ev.System, err)
		}

		selected, err := selectFrom(drvs, names)
		if err != nil {
			return nil, err
		}

		for key := range dotman.FixedKeys(selected...) {
			keys[key] = true
		}
	}

	return keys, nil
}

// pruneLock drops lock entries no package uses on any system, like those of
// versions no longer configured.
func (a *app) pruneLock() error {
	keys, err := a.lockKeys(nil)
	if err != nil {
		return fmt.Errorf("pruning lock: %w", err)
	}

	if pruned := a.lock.Prune(keys); len(pruned) > 0 {
		a.log.Info("Dropped unused lock entries.", "count", len(pruned))
	}

	return nil
}

// lockFetches locks every download of drvs, or only those whose keys are
// in only if it isn't nil, fetching those not locked yet.
func (a *app) lockFetches(drvs []*dotman.Derivation, only map[string]bool) error {
	for _, drv := range drvs {
		for _, d := range dotman.Closure(drv) {
			if d.Fixed == nil || d.Fixed.Key == "" {
				continue
			}
			if only != nil && !only[d.Fixed.Key] {
				continue
			}

			// Computing a fetch's path locks it.
			if _, err := a.r.Path(d); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *app) lockCmd(args []string) error {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	allSystems := fs.Bool("all-systems", false, "lock the downloads of every system, not just this one")
	if err := noArgs(fs, args); err != nil {
		return err
	}

	for _, ev := range a.evalsFor(*allSystems) {
		drvs, err := a.deriveFor(ev)
		if err != nil {
			return fmt.Errorf("%s: %w", ev.System, err)
		}

		selected, err := selectFrom(drvs, nil)
		if err != nil {
			return err
		}

		if err := a.lockFetches(selected, nil); err != nil {
			return err
		}
	}

	return a.pruneLock()
}

func (a *app) updateCmd(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	keys, err := a.lockKeys(fs.Args())
	if err != nil {
		return err
	}

	// Every system's entries are forgotten and locked again, so the lock
	// keeps covering the systems it covered, and no machine checks against
	// a hash that is out of date.
	forgotten := a.lock.Delete(slices.Sorted(maps.Keys(keys)))
	a.log.Info("Forgot locked hashes.", "count", len(forgotten))

	relock := map[string]bool{}
	for _, key := range forgotten {
		relock[key] = true
	}

	for _, ev := range a.evalsFor(true) {
		drvs, err := a.deriveFor(ev)
		if err != nil {
			return fmt.Errorf("%s: %w", ev.System, err)
		}

		selected, err := selectFrom(drvs, fs.Args())
		if err != nil {
			return err
		}

		if err := a.lockFetches(selected, relock); err != nil {
			return err
		}
	}

	return nil
}

func (a *app) buildCmd(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	check := fs.Bool("check", false, "rebuild and fail if any rebuild differs")
	if err := fs.Parse(args); err != nil {
		return err
	}

	drvs, err := a.selectDrvs(fs.Args())
	if err != nil {
		return err
	}

	for _, drv := range drvs {
		if *check {
			if err := a.r.Check(drv); err != nil {
				return err
			}
		}

		path, err := a.r.Realise(drv)
		if err != nil {
			return err
		}
		fmt.Println(path)
	}

	return nil
}

func (a *app) rollbackCmd(args []string) error {
	if err := noArgs(flag.NewFlagSet("rollback", flag.ContinueOnError), args); err != nil {
		return err
	}

	n, err := a.gens.Rollback()
	if err != nil {
		return err
	}

	a.log.Info("Rolled back.", "generation", n)
	return nil
}

func (a *app) generationsCmd(args []string) error {
	if err := noArgs(flag.NewFlagSet("generations", flag.ContinueOnError), args); err != nil {
		return err
	}

	gens, err := a.gens.List()
	if err != nil {
		return err
	}

	current, err := a.gens.Current()
	if err != nil {
		return err
	}

	for _, g := range gens {
		marker := " "
		if g.Number == current {
			marker = "*"
		}
		fmt.Printf("%s %4d  %s\n", marker, g.Number, g.Target)
	}

	return nil
}

func (a *app) gcCmd(args []string) error {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	keep := fs.Int("keep", 0, "delete all but the newest `N` generations first")
	if err := noArgs(fs, args); err != nil {
		return err
	}

	if *keep > 0 {
		deleted, err := a.gens.Delete(*keep)
		if err != nil {
			return err
		}
		if len(deleted) > 0 {
			a.log.Info("Deleted generations.", "generations", deleted)
		}
	}

	gens, err := a.gens.List()
	if err != nil {
		return err
	}

	var roots []string
	for _, g := range gens {
		roots = append(roots, g.Target)
	}

	// Before the first switch, the profile link may point straight at a
	// profile rather than at a generation.
	if target, err := filepath.EvalSymlinks(a.gens.ProfileLink()); err == nil {
		roots = append(roots, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	deleted, err := a.r.Store.GC(a.log, roots)
	a.log.Info("Deleted store paths.", "count", len(deleted))
	return err
}

func (a *app) showDrvCmd(args []string) error {
	fs := flag.NewFlagSet("show-drv", flag.ContinueOnError)
	recursive := fs.Bool("r", false, "also print the derivations of its inputs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("show-drv takes one package name")
	}

	drvs, err := a.selectDrvs(fs.Args())
	if err != nil {
		return err
	}

	shown := drvs
	if *recursive {
		shown = dotman.Closure(drvs[0])
	}

	for _, drv := range shown {
		path, err := a.r.Path(drv)
		if err != nil {
			return err
		}

		record, err := a.r.Record(drv)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n%s\n\n", path, record)
	}

	return nil
}
