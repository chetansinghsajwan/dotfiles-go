package theme

import "fmt"

var Themes = []Theme{
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

var themesByName = func() map[string]Theme {
	m := make(map[string]Theme, len(Themes))
	for _, t := range Themes {
		m[t.Name] = t
	}
	return m
}()

func Get(name string) (Theme, error) {
	t, ok := themesByName[name]

	if !ok {
		return Theme{}, fmt.Errorf("theme not found: %s", name)
	}

	return t, nil
}
