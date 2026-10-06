package lib

import "embed"

// Sources is this package's source, part of every builder's toolchain hash
// since builders share its helpers. See dotman.ToolchainHash.
//
//go:embed *.go
var Sources embed.FS
