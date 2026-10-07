package yazi

import (
	"fmt"
	"io/fs"
	"os"
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

	// Git revision to pin, e.g. a commit. Required for remote plugins, since
	// their download is cached by it; unused for local ones.
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

// Install deploys the plugin into the yazi config directory configPath.
// Remote plugins are downloaded into store first, reusing an earlier download.
func (p Plugin) Install(store *dotman.Store, configPath string) error {
	if p.IsLocal() {
		log.Debug("Installing plugin...", "plugin", p.Path)
		return p.installLocal(configPath)
	}

	log.Debug("Installing plugin...", "plugin", p.Path, "version", p.Version)
	return p.installRemote(store, configPath)
}

func (p Plugin) installLocal(installPath string) error {
	dst := filepath.Join(installPath, "plugins", p.Name()+".yazi")

	if err := os.CopyFS(dst, p.src); err != nil {
		return fmt.Errorf("Copying yazi plugin %s: %w", p.Path, err)
	}

	return nil
}

// installRemote downloads the source archive of p.repo at p.Version and
// deploys its p.dir into installPath/plugins.
func (p Plugin) installRemote(store *dotman.Store, installPath string) error {
	// TODO: pin the archive's hash. GitHub doesn't promise byte-stable source
	// archives, so a pin may need refreshing, but it'd catch tampering.
	url := lib.GetGithubArchiveUrl(p.repo, p.Version)
	archivePath, err := lib.DownloadFile(store, url, "")
	if err != nil {
		return fmt.Errorf("downloading yazi plugin %s: %w", p.Path, err)
	}

	dst := filepath.Join(installPath, "plugins", p.Name()+".yazi")
	if err := lib.ExtractTarGzDir(archivePath, p.dir, dst); err != nil {
		return fmt.Errorf("extracting yazi plugin %s: %w", p.Path, err)
	}

	return nil
}
