package yazi

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"dotman/core"
	"dotman/lib"
)

type Plugin struct {
	// Either a local directory or a package id, as `ya pkg` names them:
	//   - "plugins/places.yazi": a local directory in an fs.FS, see
	//     NewLocalPlugin
	//   - "yazi-rs/plugins:piper": the piper.yazi directory of the GitHub
	//     repo yazi-rs/plugins
	//   - "dedukun/bookmarks": the GitHub repo dedukun/bookmarks.yazi
	// Plugins are deployed as plugins/<name>.yazi, where name is the
	// directory's or repo's name without its extension.
	Path string

	// Git revision to pin, e.g. a commit. Unused for local plugins.
	Version string

	name    string
	isLocal bool

	// Where a remote plugin lives: its GitHub repo and the directory in it,
	// "" for the repo's root.
	repo string
	dir  string

	// Contents of a local plugin's directory.
	src fs.FS
}

// NewPlugin returns a plugin downloaded from GitHub at version, a git
// revision. Its source's hash is recorded in the lock the first time it is
// downloaded.
func NewPlugin(pluginPath, version string) Plugin {
	p := Plugin{Path: pluginPath, Version: version, isLocal: false}

	if strings.HasPrefix(pluginPath, "./") || filepath.IsAbs(pluginPath) {
		panic(fmt.Sprintf("yazi plugin %s: local plugins must use NewLocalPlugin", pluginPath))
	}
	if version == "" {
		panic(fmt.Sprintf("yazi plugin %s: remote plugins must pin a revision", pluginPath))
	}

	if repo, child, ok := strings.Cut(pluginPath, ":"); ok {
		p.repo = repo
		p.dir = child + ".yazi"
		p.name = child
	} else {
		p.repo = pluginPath + ".yazi"
		p.name = path.Base(pluginPath)
	}

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

// pluginAttrs is how yazi-config finds a plugin: in the directory Dir, ""
// for its root, of its input.
type pluginAttrs struct {
	Name  string
	Input string
	Dir   string
}

// derive returns the derivation holding the plugin's source, local or
// downloaded, and where in it the plugin is.
func (p Plugin) derive() (*dotman.Derivation, pluginAttrs, error) {
	attrs := pluginAttrs{Name: p.name, Input: "plugin-" + p.name}

	if p.IsLocal() {
		drv, err := dotman.LocalSource(p.name+".yazi", p.src)
		return drv, attrs, err
	}

	attrs.Dir = p.dir
	return lib.FetchGithubArchive(p.repo, p.Version), attrs, nil
}
