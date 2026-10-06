package zellij

import (
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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

	// SHA-256 of the release tar.gz for each target triple, e.g.
	// "x86_64-unknown-linux-musl": "sha256:40bc...". The release's .sha256sum
	// files hash the binary inside, not the tar.gz, so they can't be used
	// here. Installing on a target without a hash fails and reports the
	// downloaded archive's hash.
	Hashes map[string]string

	// Overrides the global config's theme; "" uses it.
	Theme string

	// Written as config.kdl. It shouldn't set theme, which is set from Theme.
	ConfigKdl string

	// Other names to run zellij by, e.g. "z". Each is written as a command
	// into bin, next to zellij, rather than as an alias in a shell's config.
	ShellAliases []string
}

func (z *Zellij) Name() string {
	return "zellij"
}

// Install downloads zellij's binary into storePath/libexec, writes its
// config (settings and theme) into storePath/config, and writes a wrapper
// and the shell aliases into storePath/bin that point zellij at that config.
func (z *Zellij) Install(log *slog.Logger, cfg dotman.Config, store *dotman.Store, storePath string) error {
	log.Info("Downloading zellij...", "version", z.Version)
	unwrappedPath := filepath.Join(storePath, "libexec", "zellij")
	err := z.DownloadZellij(store, z.Version, runtime.GOOS, runtime.GOARCH, unwrappedPath)

	if err != nil {
		log.Error("Failed to download zellij", "version", z.Version, "error", err)
		return err
	}

	configDir := filepath.Join(storePath, "config")
	if err := os.MkdirAll(configDir, dotman.DirPerm); err != nil {
		log.Error("Failed to create config directory.", "err", err)
		return err
	}

	configKdl := z.ConfigKdl

	themeName := z.Theme
	if themeName == "" {
		themeName = cfg.Theme
	}

	if themeName != "" {
		log.Debug("Rendering theme.", "theme", themeName)

		t, err := theme.Get(themeName)
		if err != nil {
			log.Error("Failed to get theme.", "err", err)
			return err
		}

		// zellij loads every theme in config/themes and uses the one
		// config.kdl names.
		if err := writeThemeKdl(t, filepath.Join(configDir, "themes", t.Name+".kdl")); err != nil {
			log.Error("Failed to write theme.", "err", err)
			return err
		}

		if configKdl != "" && !strings.HasSuffix(configKdl, "\n") {
			configKdl += "\n"
		}
		configKdl += "theme " + strconv.Quote(t.Name) + "\n"
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.kdl"), []byte(configKdl), dotman.FilePerm); err != nil {
		log.Error("Failed to write config.", "err", err)
		return err
	}

	binPath := filepath.Join(storePath, "bin")
	if err := os.MkdirAll(binPath, dotman.DirPerm); err != nil {
		log.Error("Failed to create bin directory.", "err", err)
		return err
	}

	zellijPath := filepath.Join(binPath, "zellij")
	wrap := lib.Wrap{
		Path: zellijPath,
		Exec: unwrappedPath,
		Env:  map[string]string{"ZELLIJ_CONFIG_DIR": configDir},
	}

	log.Debug("Writing wrapper.", "path", wrap.Path, "exec", wrap.Exec)
	if err := lib.CreateWrap(wrap); err != nil {
		log.Error("Failed to write wrapper.", "err", err)
		return err
	}

	if err := lib.CreateShellAliases(binPath, zellijPath, z.ShellAliases); err != nil {
		log.Error("Failed to write shell aliases.", "err", err)
		return err
	}

	return nil
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

func (z *Zellij) DownloadZellij(s *dotman.Store, version string, platform string, arch string, dest string) error {

	var target string
	switch platform + "/" + arch {
	case "darwin/amd64":
		target = "x86_64-apple-darwin"
	case "darwin/arm64":
		target = "aarch64-apple-darwin"
	case "linux/amd64":
		target = "x86_64-unknown-linux-musl"
	case "linux/arm64":
		target = "aarch64-unknown-linux-musl"
	default:
		return fmt.Errorf("No zellij release for %s/%s", platform, arch)
	}

	// Without a pin, FakeHash makes the download fail with the hash to pin.
	hash, pinned := z.Hashes[target]
	if !pinned {
		hash = lib.FakeHash
	}

	archivePath, err := lib.DownloadGithubRelease(s, lib.GithubRelease{
		Repo:  "zellij-org/zellij",
		Tag:   "v" + version,
		Asset: "zellij-" + target + ".tar.gz",
		Hash:  hash,
	})
	if err != nil {
		if mismatch, ok := errors.AsType[*lib.HashMismatchError](err); ok && !pinned {
			return fmt.Errorf("zellij %s: no hash pinned for %s; got %s", version, target, mismatch.Got)
		}

		return fmt.Errorf("zellij %s: %w", version, err)
	}

	// The release archive holds just the zellij binary.
	if err := lib.ExtractTarGzFile(archivePath, "zellij", dest); err != nil {
		return fmt.Errorf("extracting zellij from %s: %w", archivePath, err)
	}

	return nil
}
