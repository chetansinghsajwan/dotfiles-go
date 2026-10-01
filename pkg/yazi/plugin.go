package yazi

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Plugin struct {
	// Either a local directory or a GitHub package id:
	//   - "plugins/places.yazi": a local directory in an fs.FS, see
	//     NewLocalPlugin
	//   - "yazi-rs/plugins:piper" or "dedukun/bookmarks": installed with
	//     `ya pkg`
	// Local plugins are deployed as plugins/<name>.yazi, where name is the
	// directory's name without its extension.
	Path string

	// Git revision to pin; "" installs the latest. Unused for local plugins.
	Version string

	name    string
	isLocal bool

	// Contents of a local plugin's directory.
	src fs.FS
}

func NewPlugin(pluginPath, version string) Plugin {
	p := Plugin{Path: pluginPath, Version: version, isLocal: false}

	if strings.HasPrefix(pluginPath, "./") || filepath.IsAbs(pluginPath) {
		panic(fmt.Sprintf("yazi plugin %s: local plugins must use NewLocalPlugin", pluginPath))
	}

	p.name = path.Base(filepath.ToSlash(p.Path))

	return p
}

// NewLocalPlugin returns a plugin installed from the directory pluginPath in
// fsys, usually an embed.FS so the plugin ships inside the binary. It panics
// if pluginPath isn't a directory in fsys.
func NewLocalPlugin(fsys fs.FS, pluginPath string) Plugin {
	info, err := fs.Stat(fsys, pluginPath)
	if err != nil {
		panic(fmt.Sprintf("yazi plugin %s: %v", pluginPath, err))
	}
	if !info.IsDir() {
		panic(fmt.Sprintf("yazi plugin %s: not a directory", pluginPath))
	}

	src, err := fs.Sub(fsys, pluginPath)
	if err != nil {
		panic(fmt.Sprintf("yazi plugin %s: %v", pluginPath, err))
	}

	p := Plugin{Path: pluginPath, isLocal: true, src: src}

	base := path.Base(p.Path)
	p.name = strings.TrimSuffix(base, path.Ext(base))

	return p
}

func (p Plugin) Name() string {
	return p.name
}

// IsLocal reports whether the plugin is a local directory rather than a
// GitHub package id.
func (p Plugin) IsLocal() bool {
	return p.isLocal
}

// Install deploys the plugin into installPath/plugins.
func (p Plugin) Install(log *slog.Logger, installPath string) error {
	log.Debug("Installing plugin...", "plugin", p.Path, "version", p.Version)

	if p.IsLocal() {
		return p.installLocal(installPath)
	}

	return p.installRemote(log, installPath)
}

func (p Plugin) installLocal(installPath string) error {
	dst := filepath.Join(installPath, "plugins", p.Name()+".yazi")

	if err := os.CopyFS(dst, p.src); err != nil {
		return fmt.Errorf("copying yazi plugin %s: %w", p.Path, err)
	}

	return nil
}

// installRemote adds the plugin to installPath/package.toml and lets
// `ya pkg install` fetch and deploy it into installPath/plugins. `ya pkg add`
// can't pin a revision, hence editing package.toml directly.
func (p Plugin) installRemote(log *slog.Logger, installPath string) error {
	pkgPath := filepath.Join(installPath, "package.toml")

	pkgs, err := readPackageToml(pkgPath)
	if err != nil {
		return err
	}

	dep := packageDep{Use: p.Path, Rev: p.Version}
	replaced := false
	for i, d := range pkgs.Plugin.Deps {
		if d.Use != p.Path {
			continue
		}

		// ya records a hash of what it deployed; keep it only if the
		// revision is unchanged.
		if d.Rev == p.Version {
			dep.Hash = d.Hash
		}
		pkgs.Plugin.Deps[i] = dep
		replaced = true
	}
	if !replaced {
		pkgs.Plugin.Deps = append(pkgs.Plugin.Deps, dep)
	}

	if err := writeToml(log, pkgPath, pkgs); err != nil {
		return err
	}

	cmd := exec.Command("ya", "pkg", "install")
	cmd.Env = append(os.Environ(), "YAZI_CONFIG_HOME="+installPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ya pkg install %s: %w\n%s", p.Path, err, out)
	}

	log.Debug("Installed plugin.", "plugin", p.Path, "output", string(out))
	return nil
}

// readPackageToml reads ya's package.toml, returning an empty one if it
// doesn't exist yet.
func readPackageToml(path string) (packageToml, error) {
	var pkgs packageToml

	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return pkgs, err
	}

	if err == nil {
		if err := toml.Unmarshal(b, &pkgs); err != nil {
			return pkgs, fmt.Errorf("parsing %s: %w", path, err)
		}
	}

	if pkgs.Flavor.Deps == nil {
		pkgs.Flavor.Deps = []packageDep{}
	}

	return pkgs, nil
}
