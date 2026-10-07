package lazygit

import (
	_ "embed"
	"path/filepath"
	"strings"
	"text/template"

	dotman "dotman/core"
	"dotman/theme"
)

func (p *Package) GetThemeYaml() (string, error) {
	log.Debug("Rendering theme.", "theme", themeName)

	themeName := p.Theme
	if themeName == "" {
		themeName = cfg.Theme
	}

	t, err := theme.Get(themeName)
	if err != nil {
		log.Error("Failed to get theme.", "err", err)
		return err
	}

	path := filepath.Join(configDir, "theme.yml")
	if err := colorsToThemeYaml(t.Colors, path); err != nil {
		log.Error("Failed to write theme.", "err", err)
		return err
	}
}

// Based on tinted-theming's base16 lazygit template.
//
//go:embed base16-theme-template.yml
var lazygitThemeTemplateSource string

var lazygitThemeTemplate = template.Must(template.New("theme.yml").Parse(lazygitThemeTemplateSource))

// colorsToThemeYaml renders a lazygit theme.yml from base16 colors to path.
func colorsToThemeYaml(colors dotman.Base16Colors, path string) (string, error) {
	var b strings.Builder
	if err := lazygitThemeTemplate.Execute(&b, colors); err != nil {
		return "", err
	}

	return b.String(), nil
}
