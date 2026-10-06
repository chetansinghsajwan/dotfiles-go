package theme

import (
	"fmt"

	"dotman/core"
)

var Themes = []dotman.Theme{
	AyuDark,
	AyuMirage,
	AyuLight,
	CatppuccinMocha,
	CatppuccinMacchiato,
	CatppuccinFrappe,
	CatppuccinLatte,
	Dracula,
	Everforest,
	EverforestLightMedium,
	GruvboxDarkHard,
	GruvboxDarkMedium,
	GruvboxDarkSoft,
	GruvboxLightMedium,
	Kanagawa,
	Nord,
	OneDark,
	OneLight,
	RosePine,
	RosePineMoon,
	RosePineDawn,
	SolarizedDark,
	SolarizedLight,
	TokyoNightDark,
	TokyoNightStorm,
	TokyoNightLight,
}

var themesByName = func() map[string]dotman.Theme {
	m := make(map[string]dotman.Theme, len(Themes))
	for _, t := range Themes {
		m[t.Name] = t
	}
	return m
}()

func Get(name string) (dotman.Theme, error) {
	t, ok := themesByName[name]

	if !ok {
		return dotman.Theme{}, fmt.Errorf("theme not found: %s", name)
	}

	return t, nil
}

// Resolve returns the theme a package uses: override if it is set, or else
// cfg's theme, or nil if neither is. Packages resolve it while deriving, so
// the theme's colors, not just its name, are part of their derivation.
func Resolve(override string, cfg dotman.Config) (*dotman.Theme, error) {
	name := override
	if name == "" {
		name = cfg.Theme
	}
	if name == "" {
		return nil, nil
	}

	t, err := Get(name)
	if err != nil {
		return nil, err
	}

	return &t, nil
}
