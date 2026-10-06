package zellij

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"dotman/core"
	"dotman/lib"
	"dotman/theme"
)

type Zellij struct {
	// 0.45.1
	Version string

	// Overrides the global config's theme; "" uses it.
	Theme string

	// Written as config.kdl. It shouldn't set theme, which is set from Theme.
	ConfigKdl string

	// Other names to run zellij by, e.g. "z". Each is written as a command
	// into bin, next to zellij, rather than as an alias in a shell's config.
	ShellAliases []string
}

//go:embed *.go *.kdl
var src embed.FS

var (
	binBuilder    = dotman.NewBuilder("zellij-bin", src, buildBin)
	configBuilder = dotman.NewBuilder("zellij-config", src, buildConfig)
)

func (z *Zellij) Name() string {
	return "zellij"
}

// Derive returns zellij's wrapper, which runs the binary from zellij-bin
// with ZELLIJ_CONFIG_DIR pointing at zellij-config, and the shell aliases.
func (z *Zellij) Derive(ev *dotman.Eval) (*dotman.Derivation, error) {
	bin, err := z.deriveBin(ev.System)
	if err != nil {
		return nil, err
	}

	t, err := theme.Resolve(z.Theme, ev.Config)
	if err != nil {
		return nil, err
	}

	config := &dotman.Derivation{
		Name:    "zellij-config",
		Builder: configBuilder,
		Attrs:   configAttrs{ConfigKdl: z.ConfigKdl, Theme: t},
	}

	aliases := map[string]string{}
	for _, alias := range z.ShellAliases {
		aliases[alias] = "zellij"
	}

	return lib.Wrapper(lib.WrapperSpec{
		Name:   "zellij",
		Inputs: map[string]*dotman.Derivation{"bin": bin, "config": config},
		Wraps: []lib.WrapSpec{{
			Name: "zellij",
			Exec: "@bin@/bin/zellij",
			Env:  map[string]string{"ZELLIJ_CONFIG_DIR": "@config@"},
		}},
		Aliases: aliases,
	}), nil
}

// deriveBin returns the derivation that extracts zellij's binary from the
// release for system.
func (z *Zellij) deriveBin(system string) (*dotman.Derivation, error) {
	var target string
	switch system {
	case "darwin/amd64":
		target = "x86_64-apple-darwin"
	case "darwin/arm64":
		target = "aarch64-apple-darwin"
	case "linux/amd64":
		target = "x86_64-unknown-linux-musl"
	case "linux/arm64":
		target = "aarch64-unknown-linux-musl"
	default:
		return nil, fmt.Errorf("no zellij release for %s", system)
	}

	archive := lib.FetchGithubRelease(lib.GithubRelease{
		Repo:  "zellij-org/zellij",
		Tag:   "v" + z.Version,
		Asset: "zellij-" + target + ".tar.gz",
	})

	return &dotman.Derivation{
		Name:    "zellij-bin",
		System:  system,
		Builder: binBuilder,
		Inputs:  map[string]*dotman.Derivation{"archive": archive},
	}, nil
}

// The release archive holds just the zellij binary.
func buildBin(b *dotman.Build) error {
	return lib.ExtractTarGzFile(b.Input("archive"), "zellij", filepath.Join(b.Out, "bin", "zellij"))
}

type configAttrs struct {
	ConfigKdl string
	Theme     *dotman.Theme
}

// buildConfig writes config.kdl and the theme it names.
func buildConfig(b *dotman.Build) error {
	var attrs configAttrs
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	if err := os.MkdirAll(b.Out, dotman.DirPerm); err != nil {
		return err
	}

	configKdl := attrs.ConfigKdl

	if t := attrs.Theme; t != nil {
		b.Log.Debug("Rendering theme.", "theme", t.Name)

		// zellij loads every theme in config/themes and uses the one
		// config.kdl names.
		if err := writeThemeKdl(*t, filepath.Join(b.Out, "themes", t.Name+".kdl")); err != nil {
			return err
		}

		if configKdl != "" && !strings.HasSuffix(configKdl, "\n") {
			configKdl += "\n"
		}
		configKdl += "theme " + strconv.Quote(t.Name) + "\n"
	}

	return os.WriteFile(filepath.Join(b.Out, "config.kdl"), []byte(configKdl), dotman.FilePerm)
}

// Based on zellij's 11-color theme format; see the template's comment for
// how base16 maps onto it.
//
//go:embed base16-theme-template.kdl
var zellijThemeTemplateSource string

var zellijThemeTemplate = template.Must(template.New("theme.kdl").Parse(zellijThemeTemplateSource))

// writeThemeKdl renders a zellij theme named t.Name from t's base16 colors
// to path.
func writeThemeKdl(t dotman.Theme, path string) error {
	var b strings.Builder
	if err := zellijThemeTemplate.Execute(&b, t); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), dotman.DirPerm); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(b.String()), dotman.FilePerm)
}
