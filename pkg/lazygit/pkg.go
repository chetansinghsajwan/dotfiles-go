package lazygit

import (
	"archive/tar"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"dotman"
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

func (p *Package) Name() string {
	return "lazygit"
}

// Install downloads lazygit's binary into storePath/bin and writes its
// config (settings and theme) into storePath/config, then wraps the binary
// to point LG_CONFIG_FILE at that config.
func (p *Package) Install(log *slog.Logger, cfg dotman.Config, storePath string) error {
	log.Info("Installing lazygit...", "version", p.Version)

	binPath := filepath.Join(storePath, "bin")
	if err := os.MkdirAll(binPath, dotman.DirPerm); err != nil {
		log.Error("Failed to create bin directory.", "err", err)
		return err
	}

	lazygitUnwrappedPath := filepath.Join(binPath, "lazygit-unwrapped")
	if err := DownloadLazygit(p.Version, lazygitUnwrappedPath); err != nil {
		log.Error("Failed to download lazygit.", "err", err)
		return err
	}

	configDir := filepath.Join(storePath, "config")
	if err := os.MkdirAll(configDir, dotman.DirPerm); err != nil {
		log.Error("Failed to create config directory.", "err", err)
		return err
	}

	var configFiles []string

	if p.ConfigYaml != "" {
		path := filepath.Join(configDir, "config.yml")
		if err := os.WriteFile(path, []byte(p.ConfigYaml), dotman.FilePerm); err != nil {
			log.Error("Failed to write config.", "err", err)
			return err
		}
		configFiles = append(configFiles, path)
	} else if len(p.Settings) > 0 {
		path := filepath.Join(configDir, "config.yml")
		if err := writeSettingsYaml(path, p.Settings); err != nil {
			log.Error("Failed to write config.", "err", err)
			return err
		}
		configFiles = append(configFiles, path)
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
			return err
		}

		path := filepath.Join(configDir, "theme.yml")
		if err := writeThemeYaml(t.Colors, path); err != nil {
			log.Error("Failed to write theme.", "err", err)
			return err
		}
		configFiles = append(configFiles, path)
	}

	wrap := lib.Wrap{
		Path: filepath.Join(binPath, "lazygit"),
		Exec: lazygitUnwrappedPath,
	}
	if len(configFiles) > 0 {
		wrap.Env = map[string]string{
			"LG_CONFIG_FILE": strings.Join(configFiles, ","),
		}
	}

	log.Debug("Writing wrapper.", "path", wrap.Path, "exec", wrap.Exec)
	return lib.CreateWrap(wrap)
}

func writeSettingsYaml(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, dotman.FilePerm)
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

// DownloadLazygit maps GOARCH to the arch name lazygit's release assets use
// (e.g. "amd64" -> "x86_64") before downloading.
func DownloadLazygit(version string, dest string) error {
	arch := lib.GetArch()
	if arch == "amd64" {
		arch = "x86_64"
	}

	return DownloadLazygitFor(version, arch, lib.GetPlatform(), dest)
}

// DownloadLazygitFor downloads the lazygit release asset for arch/platform
// and extracts the lazygit binary out of its tar.gz into dest.
func DownloadLazygitFor(version string, arch string, platform string, dest string) error {
	asset := "lazygit_" + version + "_" + platform + "_" + arch + ".tar.gz"

	tmp, err := os.CreateTemp("", "lazygit-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	url := lib.GetGithubUrl("jesseduffield/lazygit", "v"+version, asset)
	if _, err := lib.Download(url, tmp); err != nil {
		return err
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dest), dotman.DirPerm); err != nil {
		return err
	}

	if err := extractLazygitBinary(tmp, dest); err != nil {
		return fmt.Errorf("extracting lazygit from %s: %w", url, err)
	}

	return nil
}

// extractLazygitBinary reads the "lazygit" entry out of the tar.gz in r and
// writes it to dest. The release archive holds the binary at its root
// alongside README.md/LICENSE.
func extractLazygitBinary(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf(`"lazygit" not found in archive`)
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "lazygit" {
			continue
		}

		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, dotman.ExecPerm)
		if err != nil {
			return err
		}

		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}

		return f.Close()
	}
}
