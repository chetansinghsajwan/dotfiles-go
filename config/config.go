// Package config holds settings shared by all packages.
package config

type Config struct {
	Theme string
}

var Default = Config{
	Theme: "ayu-dark",
}
