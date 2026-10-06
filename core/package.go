package dotman

import "runtime"

// Package is something dotman installs into the profile, like yazi.
type Package interface {
	// Name identifies the package in logs and on the command line.
	Name() string

	// Derive returns the derivation that builds the package. It must not do
	// IO beyond reading files it hashes, like LocalSource does: everything
	// the build depends on has to be in the derivation.
	Derive(ev *Eval) (*Derivation, error)
}

// Eval holds what packages are derived against.
type Eval struct {
	Config Config

	// GOOS/GOARCH of the machine being built for, e.g. "linux/amd64".
	System string
}

func NewEval(cfg Config) *Eval {
	return &Eval{Config: cfg, System: runtime.GOOS + "/" + runtime.GOARCH}
}
