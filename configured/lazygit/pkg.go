package lazygit

import (
	_ "embed"

	lg "dotman/pkg/lazygit"
)

//go:embed config.yml
var configYaml string

// TODO: delta is assumed to already be on PATH; once dotman can build it as
// a package, point this at its store path instead.
var Lazygit = lg.Package{
	Version: "0.65.1",
	Hashes: map[string]string{
		"linux_x86_64":  "sha256:02beacbcda0fa342e50ae3480ba8147307353af3fb28e1d5f790e02329c201a6",
		"linux_arm64":   "sha256:49abecdf6adf4f2dfdb11bf7b9bfada267ea523612ed809d1c6d87f6c04000a7",
		"darwin_x86_64": "sha256:fde13daf583511aa24c42ca154911643231a5af784c7cdd8117264b2fc035b33",
		"darwin_arm64":  "sha256:65a367c6ea9a88efebaaf7998a6835eedb987e04916cef677264ff9b31b1b13e",
	},
	ConfigYaml: configYaml,
}
