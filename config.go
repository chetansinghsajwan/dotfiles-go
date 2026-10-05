// Package config holds settings shared by all packages.
package dotman

type Config struct {
	Theme string
}

var DefaultConfig = Config{
	Theme: "ayu-dark",
}
