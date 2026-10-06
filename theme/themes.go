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
