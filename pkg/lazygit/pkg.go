package lazygit

import (
	_ "embed"
	"path/filepath"
	"strings"

	dotman "dotman/core"
	"dotman/lib"
	"dotman/logging"
)

var log = logging.Get("pkg.lazygit")

type Config map[string]any

type Package struct {
	Version string

	// SHA-256 of the release tar.gz for each <platform>_<arch>, as the
	// release's checksums.txt names them, e.g. "linux_x86_64":
	// "sha256:02be...". Installing on a target without a hash fails and
	// reports the downloaded archive's hash.
	Hashes map[string]string

	// Overrides the global config's theme; "" uses it.
	Theme string

	ConfigYamlPath string

	// Written verbatim as config.yml; takes precedence over Settings.
	ConfigYaml string

	// Written as config.yml.
	Config Config
}

func (p *Package) Name() string {
	return "lazygit"
}

func (p *Package) Inputs() map[string]any {

	return map[string]any{
		"lazygit": NewFetchLazygitDrv(p.Version),
	}
}

// Install downloads lazygit's binary into storePath/bin and writes its
// config (settings and theme) into storePath/config, then wraps the binary
// to point LG_CONFIG_FILE at that config.
// func (p *Package) Install(cfg dotman.Config, store *dotman.Store, storePath string) error {
func (p *Package) Build(in *dotman.DerivationInput, out *dotman.DerivationOutput) error {
	log.Info("Installing lazygit...", "version", p.Version)

	lazygitBinPath := filepath.Join(in.Get("lazygit"), "bin", "lazygit")

	if err := out.CreateSymlink(lazygitBinPath, "bin/lazygit-unwrapped"); err != nil {
		return err
	}

	// if err := out.WriteFile("config/config.yml", p.ConfigYaml, dotman.FilePerm); err != nil {
	if err := out.WriteFile("config/config.yml", p.ConfigYaml); err != nil {
		return err
	}

	var configFiles []string

	cfgYaml, err := p.GetConfigYaml()
	if err != nil {
		return err
	}

	if cfgYaml != "" {
		out.WriteFile("config/config.yml", cfgYaml)
		configFiles = append(configFiles, "config/config.yml")
	}

	themeYaml, err := p.GetThemeYaml()
	if err != nil {
		return err
	}

	if themeYaml != "" {
		out.WriteFile("config/theme.yml", themeYaml)
		configFiles = append(configFiles, "config/theme.yml")
	}

	wrap := lib.Wrap{
		Path: out.PathFor("bin/lazygit-unwrapped"),
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

func NewFetchLazygitDrv(version string) *lib.FetchDrv {
	return NewFetchLazygitForDrv(version, lib.GetArch(), lib.GetPlatform())
}

func NewFetchLazygitForDrv(version string, arch string, platform string) *lib.FetchDrv {

	if arch == "amd64" {
		arch = "x86_64"
	}

	asset := "lazygit_" + version + "_" + platform + "_" + arch + ".tar.gz"

	return lib.NewFetchGithubReleaseDrv(lib.GithubRelease{
		Repo:  "jesseduffield/lazygit",
		Tag:   "v" + version,
		Asset: asset,
	})
}
