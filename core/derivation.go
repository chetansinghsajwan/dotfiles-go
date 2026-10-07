package dotman

type Derivation interface {
	Name() string
	Inputs() map[string]Derivation
	Build(in *DerivationInput, out *DerivationOutput) error
}
