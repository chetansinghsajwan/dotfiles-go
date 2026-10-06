package dotman

import "embed"

// Sources is this package's source, part of every builder's toolchain hash
// since every builder runs through it. See ToolchainHash.
//
//go:embed *.go
var Sources embed.FS
