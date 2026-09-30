package yazi

import (
	_ "embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pelletier/go-toml/v2"

	"dotman/pkg"
	"dotman/store"
	"dotman/theme"
)

type Package struct {
	Aliases []string
	Version string

	// List of dependencies
	Depends []string
	Theme   string

	// Upstream plugins are installed with `ya pkg`; local ones are copied.
	Plugins []Plugin

	// Prepended to yazi's default keymap in keymap.toml.
	Keybinds []Keybind

	// Written as yazi.toml.
	Settings Settings

	// Written as init.lua.
	InitLua string
}

type Plugin struct {
	// Either a local directory or a GitHub package id:
	//   - "/abs/path/places.yazi": a local directory; use paths.Rel for
	//     paths relative to the defining file
	//   - "yazi-rs/plugins:piper" or "dedukun/bookmarks": installed with
	//     `ya pkg`
	// Local plugins are deployed as plugins/<name>.yazi, where name is the
	// directory's name without its extension.
	Path string

	// Git revision to pin; "" installs the latest. Ignored, with a warning,
	// for local plugins.
	Version string
}

// IsLocal reports whether the plugin is a local directory rather than a
// GitHub package id. Relative paths count as local so they're rejected with
// a clear error instead of being passed to `ya pkg`.
func (y Plugin) IsLocal() bool {
	return strings.HasPrefix(y.Path, "./") || filepath.IsAbs(y.Path)
}

// localName returns the name a local plugin is deployed under.
func (y Plugin) localName() string {
	base := path.Base(filepath.ToSlash(y.Path))
	return strings.TrimSuffix(base, path.Ext(base))
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

const DefaultTheme = "ayu-dark"

func NewYaziPackage() *Package {
	return &Package{
		Theme: DefaultTheme,
	}
}

func (p *Package) Name() string {
	return "yazi"
}

// Outputs builds yazi's config directory in storePath and returns where it
// should be linked. log should already be tagged with the package's name.
func (p *Package) Outputs(log *slog.Logger, storePath string) (pkg.PackageOutputs, error) {
	if len(p.Settings) > 0 {
		if err := writeToml(log, filepath.Join(storePath, "yazi.toml"), p.Settings); err != nil {
			return nil, err
		}
	}

	if p.Theme != "" {
		log.Debug("Rendering theme.", "theme", p.Theme)

		t, err := theme.Get(p.Theme)
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

	if err := p.deployPlugins(log, storePath); err != nil {
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

// deployPlugins copies local plugins into storePath/plugins and installs
// upstream ones with `ya pkg`.
func (p *Package) deployPlugins(log *slog.Logger, storePath string) error {
	var upstream []Plugin
	for _, plugin := range p.Plugins {
		if !plugin.IsLocal() {
			upstream = append(upstream, plugin)
			continue
		}

		if plugin.Version != "" {
			log.Warn("Local plugins aren't versioned; ignoring Version.",
				"plugin", plugin.Path, "version", plugin.Version)
		}

		src, err := localSource(plugin.Path)
		if err != nil {
			return err
		}

		dst := filepath.Join(storePath, "plugins", plugin.localName()+".yazi")
		log.Debug("Copying local plugin.", "plugin", plugin.Path, "dst", dst)
		if err := os.CopyFS(dst, src); err != nil {
			return fmt.Errorf("copying yazi plugin %s: %w", plugin.Path, err)
		}
	}

	if len(upstream) == 0 {
		return nil
	}

	return installPlugins(log, storePath, upstream)
}

// localSource opens a local plugin directory.
func localSource(pluginPath string) (fs.FS, error) {
	if !filepath.IsAbs(pluginPath) {
		return nil, fmt.Errorf("yazi plugin %s: local paths must be absolute; use paths.Rel", pluginPath)
	}

	return os.DirFS(pluginPath), nil
}

// installPlugins writes package.toml and lets `ya pkg install` fetch and
// deploy the plugins into storePath/plugins.
func installPlugins(log *slog.Logger, storePath string, plugins []Plugin) error {
	var pkgs packageToml
	pkgs.Flavor.Deps = []packageDep{}
	for _, plugin := range plugins {
		pkgs.Plugin.Deps = append(pkgs.Plugin.Deps, packageDep{Use: plugin.Path, Rev: plugin.Version})
	}

	if err := writeToml(log, filepath.Join(storePath, "package.toml"), pkgs); err != nil {
		return err
	}

	log.Info("Installing plugins.", "count", len(plugins))
	for _, plugin := range plugins {
		log.Debug("Plugin to install.", "plugin", plugin.Path, "version", plugin.Version)
	}

	cmd := exec.Command("ya", "pkg", "install")
	cmd.Env = append(os.Environ(), "YAZI_CONFIG_HOME="+storePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ya pkg install: %w\n%s", err, out)
	}

	log.Debug("Installed plugins.", "output", string(out))
	return nil
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
