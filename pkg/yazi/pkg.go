package yazi

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pelletier/go-toml/v2"

	"dotman/core"
	"dotman/lib"
	"dotman/theme"
)

type Package struct {
	// Other names to run yazi by, e.g. "y". Each is written as a command
	// into bin, next to yazi's wrapper, rather than as an alias in a shell's
	// config, so it can't cd the shell into yazi's last directory.
	ShellAliases []string

	// Release tag of yazi to install, e.g. "v26.9.1".
	Version string

	// SHA-256 of the release zip for each target triple, as GitHub shows it,
	// e.g. "x86_64-unknown-linux-musl": "sha256:9b9c...". Installing on a
	// target without a hash fails and reports the downloaded zip's hash.
	Hashes map[string]string

	// Programs yazi's config runs from PATH.
	HostDeps []string

	// Overrides the global config's theme; "" uses it.
	Theme string

	// Upstream plugins are downloaded from GitHub; local ones are copied.
	Plugins []Plugin

	// Prepended to yazi's default keymap in keymap.toml.
	Keybinds []Keybind

	// Written as yazi.toml.
	Settings Settings

	// Written as init.lua.
	InitLua string
}

type Keybind struct {
	// Keymap section, e.g. "mgr" or "input"; "" means "mgr".
	Mode string

	// Keys sequence, e.g. {"p", "p"}.
	Keys []string

	Command string
	Desc    string
}

type Settings map[string]any

//go:embed *.go *.toml
var src embed.FS

var (
	binBuilder    = dotman.NewBuilder("yazi-bin", src, buildBin)
	configBuilder = dotman.NewBuilder("yazi-config", src, buildConfig)
)

func NewYaziPackage() *Package {
	return &Package{}
}

func (p *Package) Name() string {
	return "yazi"
}

// Derive returns yazi's wrappers, which run the binaries from yazi-bin with
// YAZI_CONFIG_HOME pointing at yazi-config, and the shell aliases.
func (p *Package) Derive(ev *dotman.Eval) (*dotman.Derivation, error) {
	bin, err := p.deriveBin(ev.System)
	if err != nil {
		return nil, err
	}

	config, err := p.deriveConfig(ev)
	if err != nil {
		return nil, err
	}

	var wraps []lib.WrapSpec
	for _, name := range binaries {
		wraps = append(wraps, lib.WrapSpec{
			Name: name,
			Exec: "@bin@/bin/" + name,
			Env:  map[string]string{"YAZI_CONFIG_HOME": "@config@"},
		})
	}

	aliases := map[string]string{}
	for _, alias := range p.ShellAliases {
		aliases[alias] = "yazi"
	}

	return lib.Wrapper(lib.WrapperSpec{
		Name:     "yazi",
		Inputs:   map[string]*dotman.Derivation{"bin": bin, "config": config},
		Wraps:    wraps,
		Aliases:  aliases,
		HostDeps: p.HostDeps,
	}), nil
}

type configAttrs struct {
	Settings Settings
	Theme    *dotman.Theme
	Keymap   map[string]map[string][]keymapEntry
	InitLua  string
	Plugins  []pluginAttrs
}

// deriveConfig returns the derivation of yazi's config directory, with the
// plugins' sources as its inputs.
func (p *Package) deriveConfig(ev *dotman.Eval) (*dotman.Derivation, error) {
	t, err := theme.Resolve(p.Theme, ev.Config)
	if err != nil {
		return nil, err
	}

	attrs := configAttrs{
		Settings: p.Settings,
		Theme:    t,
		InitLua:  p.InitLua,
	}
	if len(p.Keybinds) > 0 {
		attrs.Keymap = p.keymap()
	}

	inputs := map[string]*dotman.Derivation{}
	for _, plugin := range p.Plugins {
		drv, pa, err := plugin.derive()
		if err != nil {
			return nil, err
		}

		if _, dup := inputs[pa.Input]; dup {
			return nil, fmt.Errorf("yazi: two plugins named %s", plugin.Name())
		}

		inputs[pa.Input] = drv
		attrs.Plugins = append(attrs.Plugins, pa)
	}

	return &dotman.Derivation{
		Name:    "yazi-config",
		Builder: configBuilder,
		Attrs:   attrs,
		Inputs:  inputs,
	}, nil
}

// buildConfig writes yazi's config directory: yazi.toml, theme.toml,
// keymap.toml, init.lua and plugins.
func buildConfig(b *dotman.Build) error {
	var attrs configAttrs
	if err := b.Decode(&attrs); err != nil {
		return err
	}

	if err := os.MkdirAll(b.Out, dotman.DirPerm); err != nil {
		return err
	}

	if len(attrs.Settings) > 0 {
		if err := writeToml(filepath.Join(b.Out, "yazi.toml"), attrs.Settings); err != nil {
			return err
		}
	}

	if attrs.Theme != nil {
		b.Log.Debug("Rendering theme.", "theme", attrs.Theme.Name)

		themeToml, err := colorsToYaziThemeToml(attrs.Theme.Colors)
		if err != nil {
			return fmt.Errorf("rendering theme: %w", err)
		}

		if err := os.WriteFile(filepath.Join(b.Out, "theme.toml"), []byte(themeToml), dotman.FilePerm); err != nil {
			return err
		}
	}

	if len(attrs.Keymap) > 0 {
		if err := writeToml(filepath.Join(b.Out, "keymap.toml"), attrs.Keymap); err != nil {
			return err
		}
	}

	if attrs.InitLua != "" {
		if err := os.WriteFile(filepath.Join(b.Out, "init.lua"), []byte(attrs.InitLua), dotman.FilePerm); err != nil {
			return err
		}
	}

	for _, plugin := range attrs.Plugins {
		src := filepath.Join(b.Input(plugin.Input), filepath.FromSlash(plugin.Dir))
		dst := filepath.Join(b.Out, "plugins", plugin.Name+".yazi")

		b.Log.Debug("Installing plugin.", "plugin", plugin.Name)
		if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
			return fmt.Errorf("copying yazi plugin %s: %w", plugin.Name, err)
		}
	}

	return nil
}

type keymapEntry struct {
	On   []string `toml:"on" json:"on"`
	Run  string   `toml:"run" json:"run"`
	Desc string   `toml:"desc,omitempty" json:"desc,omitempty"`
}

// keymap groups the keybinds by mode into keymap.toml's layout.
func (p *Package) keymap() map[string]map[string][]keymapEntry {
	keymap := map[string]map[string][]keymapEntry{}
	for _, k := range p.Keybinds {
		mode := k.Mode
		if mode == "" {
			mode = "mgr"
		}
		if keymap[mode] == nil {
			keymap[mode] = map[string][]keymapEntry{}
		}
		keymap[mode]["prepend_keymap"] = append(keymap[mode]["prepend_keymap"], keymapEntry{
			On:   k.Keys,
			Run:  k.Command,
			Desc: k.Desc,
		})
	}

	return keymap
}

func writeToml(path string, v any) error {
	b, err := toml.Marshal(v)
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, dotman.FilePerm)
}

// Based on tinted-theming/tinted-yazi's base16 template.
//
//go:embed base16-theme-template.toml
var yaziThemeTemplateSource string

var yaziThemeTemplate = template.Must(template.New("theme.toml").Parse(yaziThemeTemplateSource))

// colorsToYaziThemeToml renders a yazi theme.toml from base16 colors.
func colorsToYaziThemeToml(colors dotman.Base16Colors) (string, error) {
	var b strings.Builder
	if err := yaziThemeTemplate.Execute(&b, colors); err != nil {
		return "", err
	}

	return b.String(), nil
}
