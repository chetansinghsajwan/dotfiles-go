package yazi

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

type Plugin struct {
	// Either a local directory or a GitHub package id:
	//   - "/abs/path/places.yazi": a local directory, see NewLocalPlugin
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
	p := Plugin{Path: pluginPath, Version: version, isLocal: false}

	if strings.HasPrefix(pluginPath, "./") || filepath.IsAbs(pluginPath) {
		panic("local plugins must be specified with a relative path")
	}

	p.name = path.Base(filepath.ToSlash(p.Path))

	return p
}

// NewLocalPlugin returns a plugin installed from a local directory. A relative
// pluginPath is resolved against the directory of the source file that calls
// NewLocalPlugin, like ./ in Nix. Call it directly in that file: wrapping it in
// a helper resolves against the helper's file instead.
//
// It relies on the source being on disk where it was compiled, which holds
// for `go run` from the checkout but not for -trimpath builds or binaries
// moved elsewhere.
func NewLocalPlugin(pluginPath string) Plugin {
	if !filepath.IsAbs(pluginPath) {
		_, file, _, ok := runtime.Caller(1)
		if !ok {
			panic("yazi.NewLocalPlugin: can't determine caller")
		}

		pluginPath = filepath.Join(filepath.Dir(file), pluginPath)
	}

	p := Plugin{Path: pluginPath, isLocal: true}

	base := path.Base(filepath.ToSlash(p.Path))
	p.name = strings.TrimSuffix(base, path.Ext(base))

	return p
}

func (p Plugin) Name() string {
	return p.name
}

// IsLocal reports whether the plugin is a local directory rather than a
// GitHub package id. Relative paths count as local so they're rejected with
// a clear error instead of being passed to `ya pkg`.
func (p Plugin) IsLocal() bool {
	return p.isLocal
}

func (p Plugin) Install(installPath string) error {
	if p.IsLocal() {
		return p.installLocal(installPath)
	}

	return p.installRemote(installPath)
}

func (p Plugin) installLocal(installPath string) error {

	// if p.Version != "" {
	// 	log.Warn("Local plugins aren't versioned; ignoring Version.",
	// 		"plugin", p.Path, "version", p.Version)
	// }

	src := os.DirFS(p.Path)
	dst := filepath.Join(installPath, "plugins", p.Name()+".yazi")

	if err := os.CopyFS(dst, src); err != nil {
		return fmt.Errorf("copying yazi plugin %s: %w", p.Path, err)
	}

	return nil
}

func (p Plugin) installRemote(installPath string) error {

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
