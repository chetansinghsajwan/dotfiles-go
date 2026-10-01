package yazi

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pelletier/go-toml/v2"

	"dotman/config"
	"dotman/pkg"
	"dotman/store"
	"dotman/theme"
)

type Package struct {
	Aliases []string
	Version string

	// List of dependencies
	Depends []string

	// Overrides the global config's theme; "" uses it.
	Theme string

	// Upstream plugins are installed with `ya pkg`; local ones are copied.
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

func NewYaziPackage() *Package {
	return &Package{}
}

func (p *Package) Name() string {
	return "yazi"
}

// Outputs builds yazi's config directory in storePath and returns where it
// should be linked. log should already be tagged with the package's name.
func (p *Package) Outputs(log *slog.Logger, cfg config.Config, storePath string) (pkg.PackageOutputs, error) {
	if len(p.Settings) > 0 {
		if err := writeToml(log, filepath.Join(storePath, "yazi.toml"), p.Settings); err != nil {
			return nil, err
		}
	}

	themeName := p.Theme
	if themeName == "" {
		themeName = cfg.Theme
	}

	if themeName != "" {
		log.Debug("Rendering theme.", "theme", themeName)

		t, err := theme.Get(themeName)
		if err != nil {
			log.Error("Failed to get theme.", "err", err)
			return nil, err
		}

		themeToml, err := colorsToYaziThemeToml(t.Colors)
		if err != nil {
			log.Error("Failed to convert colors to theme.", "err", err)
			return nil, err
		}

		if err := writeFile(log, filepath.Join(storePath, "theme.toml"), []byte(themeToml)); err != nil {
			log.Error("Failed to write theme.", "err", err)
			return nil, err
		}
	}

	if len(p.Keybinds) > 0 {
		if err := writeToml(log, filepath.Join(storePath, "keymap.toml"), p.keymap()); err != nil {
			return nil, err
		}
	}

	if p.InitLua != "" {
		if err := writeFile(log, filepath.Join(storePath, "init.lua"), []byte(p.InitLua)); err != nil {
			return nil, err
		}
	}

	if err := p.installPlugins(log, storePath); err != nil {
		return nil, err
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	return pkg.PackageOutputs{
		filepath.Join(configDir, "yazi"): storePath,
	}, nil
}

type keymapEntry struct {
	On   []string `toml:"on"`
	Run  string   `toml:"run"`
	Desc string   `toml:"desc,omitempty"`
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

type packageToml struct {
	Plugin struct {
		Deps []packageDep `toml:"deps"`
	} `toml:"plugin"`
	Flavor struct {
		Deps []packageDep `toml:"deps"`
	} `toml:"flavor"`
}

type packageDep struct {
	Use string `toml:"use"`
	Rev string `toml:"rev,omitempty"`
}

func writeToml(log *slog.Logger, path string, v any) error {
	b, err := toml.Marshal(v)
	if err != nil {
		return err
	}

	return writeFile(log, path, b)
}

func writeFile(log *slog.Logger, path string, b []byte) error {
	log.Debug("Writing config file.", "path", path, "bytes", len(b))
	return os.WriteFile(path, b, store.FilePerm)
}

// Based on tinted-theming/tinted-yazi's base16 template.
//
//go:embed base16-theme-template.toml
var yaziThemeTemplateSource string

var yaziThemeTemplate = template.Must(template.New("theme.toml").Parse(yaziThemeTemplateSource))

// colorsToYaziThemeToml renders a yazi theme.toml from base16 colors.
func colorsToYaziThemeToml(colors theme.Base16Colors) (string, error) {
	var b strings.Builder
	if err := yaziThemeTemplate.Execute(&b, colors); err != nil {
		return "", err
	}

	return b.String(), nil
}
