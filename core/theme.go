package dotman

type Variant string

const (
	VariantDark  Variant = "dark"
	VariantLight Variant = "light"
)

type Theme struct {
	Name    string
	Variant Variant
	Colors  Base16Colors
}

type Base16Colors struct {
	Base00 string
	Base01 string
	Base02 string
	Base03 string
	Base04 string
	Base05 string
	Base06 string
	Base07 string
	Base08 string
	Base09 string
	Base0A string
	Base0B string
	Base0C string
	Base0D string
	Base0E string
	Base0F string
}
