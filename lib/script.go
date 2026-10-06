package lib

import (
	"path/filepath"

	"dotman/core"
)

// ScriptPackage installs Script as bin/<Command>. The package is named
// after the command.
type ScriptPackage struct {
	Command string

	Script string

	// Programs the script runs from PATH.
	HostDeps []string
}

type scriptAttrs struct {
	Name   string
	Script string
}

var scriptBuilder = dotman.NewBuilder("script", Sources, func(b *dotman.Build) error {
	var attrs scriptAttrs
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	return WriteExecutable(filepath.Join(b.Out, "bin", attrs.Name), attrs.Script)
})

func (p *ScriptPackage) Name() string {
	return p.Command
}

func (p *ScriptPackage) Derive(ev *dotman.Eval) (*dotman.Derivation, error) {
	return &dotman.Derivation{
		Name:     p.Command,
		Builder:  scriptBuilder,
		Attrs:    scriptAttrs{Name: p.Command, Script: p.Script},
		HostDeps: p.HostDeps,
	}, nil
}
