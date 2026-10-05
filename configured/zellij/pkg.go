package zellij

import "dotman/pkg/zellij"

var Zellij = zellij.Zellij{
	Version: "0.45.1",
	Hashes: map[string]string{
		"x86_64-unknown-linux-musl":  "sha256:40bcc2e03f5d5ae8e054e39f676081fe12ab70871506996ba595834c3718eefc",
		"aarch64-unknown-linux-musl": "sha256:05f0802afadd53f8db9514e7cae53c9ae8432fed1b35b8294aa816ee3044a16b",
		"x86_64-apple-darwin":        "sha256:8e8bea22737d1652278c51fc5c26c7c22c9855d0ebb9634a84b8873823093114",
		"aarch64-apple-darwin":       "sha256:c029ba4fe1927b79ad9f0cdd59155c4dff80777863c85857d4d09b88b56f9891",
	},
	ShellAliases: []string{"z"},
}
