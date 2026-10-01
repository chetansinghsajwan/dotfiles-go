package yazi

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

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

	name    string
	isLocal bool
}

func NewPlugin(pluginPath, version string) Plugin {
	p := Plugin{Path: pluginPath, Version: version}
	p.isLocal = strings.HasPrefix(pluginPath, "./") || filepath.IsAbs(pluginPath)

	if p.isLocal {
		base := path.Base(filepath.ToSlash(p.Path))
		p.name = strings.TrimSuffix(base, path.Ext(base))
	} else {
		p.name = path.Base(filepath.ToSlash(p.Path))
	}

	return p
}

func (y Plugin) Name() string {
	return y.name
}

// IsLocal reports whether the plugin is a local directory rather than a
// GitHub package id. Relative paths count as local so they're rejected with
// a clear error instead of being passed to `ya pkg`.
func (y Plugin) IsLocal() bool {
	return y.isLocal
}

func (y Plugin) Install(path string) error {

}

func (p *Package) installPlugins(log *slog.Logger, storePath string) error {
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

		dst := filepath.Join(storePath, "plugins", plugin.Name()+".yazi")

		log.Debug("Installing plugin...", "plugin", plugin.Path)
		if err := os.CopyFS(dst, src); err != nil {
			return fmt.Errorf("copying yazi plugin %s: %w", plugin.Path, err)
		}
	}

	if len(upstream) == 0 {
		return nil
	}

	return installRemotePlugins(log, storePath, upstream)
}

// localSource opens a local plugin directory.
func localSource(pluginPath string) (fs.FS, error) {
	if !filepath.IsAbs(pluginPath) {
		return nil, fmt.Errorf("yazi plugin %s: local paths must be absolute; use paths.Rel", pluginPath)
	}

	return os.DirFS(pluginPath), nil
}

// installRemotePlugins writes package.toml and lets `ya pkg install` fetch and
// deploy the plugins into storePath/plugins.
func installRemotePlugins(log *slog.Logger, storePath string, plugins []Plugin) error {
	var pkgs packageToml
	pkgs.Flavor.Deps = []packageDep{}
	for _, plugin := range plugins {
		pkgs.Plugin.Deps = append(pkgs.Plugin.Deps, packageDep{Use: plugin.Path, Rev: plugin.Version})
	}

	if err := writeToml(log, filepath.Join(storePath, "package.toml"), pkgs); err != nil {
		return err
	}

	for _, plugin := range plugins {
		log.Debug("Installing plugin...", "plugin", plugin.Path, "version", plugin.Version)
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
