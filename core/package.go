package dotman

type PackageDrv interface {
	// Name identifies the package in logs and store paths.
	Name() string

	Build(in DerivationInput, out DerivationOutput) error
}
