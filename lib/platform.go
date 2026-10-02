package lib

import (
	"runtime"
)

func GetPlatform() string {
	return runtime.GOOS
}

func GetArch() string {
	return runtime.GOARCH
}
