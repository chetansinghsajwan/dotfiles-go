package lib

import (
	"archive/zip"
	"dotman/store"
	"io"
	"os"
)

// extractFile writes the file name in zr to dst as an executable.
func ExtractFile(zr *zip.Reader, name, dst string) error {
	src, err := zr.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, store.ExecPerm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}
