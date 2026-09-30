package main

import (
	"log/slog"

	dm "dotman/dotman/dotman"
	_ "dotman/pkgs"
)

func main() {

	slog.Info("Searching for packages")
	packages := dm.FindPackages()
	slog.Info("Found packages", "packages", packages)
}
