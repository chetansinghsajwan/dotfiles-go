package lib

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	dotman "dotman/core"
)

// extractFile writes the file name in zr to dst as an executable.
func ExtractFile(zr *zip.Reader, name, dst string) error {
	src, err := zr.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, dotman.ExecPerm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

// ExtractTarGzDir writes the directory dir of the tar.gz at archivePath to
// dst, ignoring the archive's single top-level directory, as GitHub's source
// archives have; dir "" extracts everything under it. Files keep their
// executable bit; links and other special files are skipped.
func ExtractTarGzDir(archivePath, dir, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	found := false
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Drop the top-level directory, then keep only what's under dir.
		_, name, _ := strings.Cut(strings.TrimSuffix(hdr.Name, "/"), "/")
		if dir != "" {
			if name != dir && !strings.HasPrefix(name, dir+"/") {
				continue
			}
			name = strings.TrimPrefix(strings.TrimPrefix(name, dir), "/")
		}
		found = true

		if name != "" && !filepath.IsLocal(name) {
			return fmt.Errorf("%s: unsafe path %q in archive", archivePath, hdr.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, dotman.DirPerm); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), dotman.DirPerm); err != nil {
				return err
			}

			perm := dotman.FilePerm
			if hdr.Mode&0o111 != 0 {
				perm = dotman.ExecPerm
			}

			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
			if err != nil {
				return err
			}

			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}

			if err := out.Close(); err != nil {
				return err
			}
		}
	}

	if !found {
		return fmt.Errorf("%s: %q not found in archive", archivePath, dir)
	}

	return nil
}

// ExtractTarGzFile writes the regular file called name, at any depth, of the
// tar.gz at archivePath to dst as an executable, creating dst's directory.
// It is for release archives that hold a binary, maybe next to a README or
// LICENSE.
func ExtractTarGzFile(archivePath, name, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s: %q not found in archive", archivePath, name)
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag != tar.TypeReg || path.Base(hdr.Name) != name {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), dotman.DirPerm); err != nil {
			return err
		}

		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, dotman.ExecPerm)
		if err != nil {
			return err
		}

		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}

		return out.Close()
	}
}

type ExtractDrv struct {
	name, filePath, outFilePath string
}

func NewExtractDrv(name, filePath, outFilePath string) ExtractDrv {
	return ExtractDrv{
		name:        name,
		filePath:    filePath,
		outFilePath: outFilePath,
	}
}

func (e *ExtractDrv) Name() string {
	return e.name
}

func (e *ExtractDrv) Inputs() map[string]dotman.Derivation {
	return nil
}

func (e *ExtractDrv) Build(in dotman.DerivationInput, out dotman.DerivationOutput) error {
	return nil
}
