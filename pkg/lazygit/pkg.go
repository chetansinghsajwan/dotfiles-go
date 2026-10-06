package lazygit

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"dotman/core"
	"dotman/lib"
	"dotman/theme"
)

type Settings map[string]any

type Package struct {
	Version string

	// Overrides the global config's theme; "" uses it.
	Theme string

	// Written verbatim as config.yml; takes precedence over Settings.
	ConfigYaml string

	// Written as config.yml.
	Settings Settings
}

//go:embed *.go *.yml
var src embed.FS

var (
	binBuilder    = dotman.NewBuilder("lazygit-bin", src, buildBin)
	configBuilder = dotman.NewBuilder("lazygit-config", src, buildConfig)
)

func (p *Package) Name() string {
	return "lazygit"
}

// Derive returns lazygit's wrapper, which runs the binary from lazygit-bin
// with LG_CONFIG_FILE pointing at the files in lazygit-config.
func (p *Package) Derive(ev *dotman.Eval) (*dotman.Derivation, error) {
	bin := p.deriveBin(ev.System)

	t, err := theme.Resolve(p.Theme, ev.Config)
	if err != nil {
		return nil, err
	}

	attrs := configAttrs{ConfigYaml: p.ConfigYaml, Settings: p.Settings, Theme: t}
	files := attrs.files()

	wrap := lib.WrapSpec{Name: "lazygit", Exec: "@bin@/bin/lazygit"}
	inputs := map[string]*dotman.Derivation{"bin": bin}

	if len(files) > 0 {
		inputs["config"] = &dotman.Derivation{
			Name:    "lazygit-config",
			Builder: configBuilder,
			Attrs:   attrs,
		}

		var paths []string
		for _, f := range files {
			paths = append(paths, "@config@/"+f)
		}
		wrap.Env = map[string]string{"LG_CONFIG_FILE": strings.Join(paths, ",")}
	}

	return lib.Wrapper(lib.WrapperSpec{
		Name:   "lazygit",
		Inputs: inputs,
		Wraps:  []lib.WrapSpec{wrap},
	}), nil
}

// deriveBin returns the derivation that extracts lazygit's binary from the
// release for system.
func (p *Package) deriveBin(system string) *dotman.Derivation {
	platform, arch, _ := strings.Cut(system, "/")
	if arch == "amd64" {
		arch = "x86_64"
	}
	target := platform + "_" + arch

	archive := lib.FetchGithubRelease(lib.GithubRelease{
		Repo:  "jesseduffield/lazygit",
		Tag:   "v" + p.Version,
		Asset: "lazygit_" + p.Version + "_" + target + ".tar.gz",
	})

	return &dotman.Derivation{
		Name:    "lazygit-bin",
		System:  system,
		Builder: binBuilder,
		Inputs:  map[string]*dotman.Derivation{"archive": archive},
	}
}

func buildBin(b *dotman.Build) error {
	return lib.ExtractTarGzFile(b.Input("archive"), "lazygit", filepath.Join(b.Out, "bin", "lazygit"))
}

type configAttrs struct {
	ConfigYaml string
	Settings   Settings
	Theme      *dotman.Theme
}

// files returns the names of the files buildConfig writes, in the order
// lazygit should merge them.
func (a configAttrs) files() []string {
	var files []string
	if a.ConfigYaml != "" || len(a.Settings) > 0 {
		files = append(files, "config.yml")
	}
	if a.Theme != nil {
		files = append(files, "theme.yml")
	}

	return files
}

func buildConfig(b *dotman.Build) error {
	var attrs configAttrs
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	if err := os.MkdirAll(b.Out, dotman.DirPerm); err != nil {
		return err
	}

	if attrs.ConfigYaml != "" {
		if err := os.WriteFile(filepath.Join(b.Out, "config.yml"), []byte(attrs.ConfigYaml), dotman.FilePerm); err != nil {
			return err
		}
	} else if len(attrs.Settings) > 0 {
		y, err := yaml.Marshal(attrs.Settings)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(b.Out, "config.yml"), y, dotman.FilePerm); err != nil {
			return err
		}
	}

	if attrs.Theme != nil {
		b.Log.Debug("Rendering theme.", "theme", attrs.Theme.Name)
		if err := writeThemeYaml(attrs.Theme.Colors, filepath.Join(b.Out, "theme.yml")); err != nil {
			return err
		}
	}

	return nil
}

// Based on tinted-theming's base16 lazygit template.
//
//go:embed base16-theme-template.yml
var lazygitThemeTemplateSource string

var lazygitThemeTemplate = template.Must(template.New("theme.yml").Parse(lazygitThemeTemplateSource))

// writeThemeYaml renders a lazygit theme.yml from base16 colors to path.
func writeThemeYaml(colors dotman.Base16Colors, path string) error {
	var b strings.Builder
	if err := lazygitThemeTemplate.Execute(&b, colors); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(b.String()), dotman.FilePerm)
}
