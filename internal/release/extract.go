package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// maxBinarySize bounds how much of one archive entry is read.
const maxBinarySize = 1 << 30

// extract returns the executables inside a release asset whose base name,
// without .exe, is one of names. A raw executable asset is returned as is
// when its file name starts with one of the names.
func extract(assetName string, data []byte, names map[string]bool) ([]found, error) {
	lower := strings.ToLower(assetName)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(data, names)
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(data, names)
	}

	for name := range names {
		if strings.HasPrefix(assetName, name) {
			if goos, arch, ok := Detect(data); ok {
				return []found{{Binary{Name: name, OS: goos, Arch: arch}, data}}, nil
			}
		}
	}
	return nil, nil
}

type found struct {
	Binary
	data []byte
}

func wanted(entry string, names map[string]bool) (string, bool) {
	base := strings.TrimSuffix(path.Base(entry), ".exe")
	return base, names[base]
}

func extractTarGz(data []byte, names map[string]bool) ([]found, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var out []found
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}

		name, ok := wanted(h.Name, names)
		if !ok {
			continue
		}

		b, err := readLimited(tr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", h.Name, err)
		}
		if goos, arch, ok := Detect(b); ok {
			out = append(out, found{Binary{Name: name, OS: goos, Arch: arch}, b})
		}
	}
}

func extractZip(data []byte, names map[string]bool) ([]found, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	var out []found
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, ok := wanted(f.Name, names)
		if !ok {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := readLimited(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if goos, arch, ok := Detect(b); ok {
			out = append(out, found{Binary{Name: name, OS: goos, Arch: arch}, b})
		}
	}
	return out, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBinarySize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBinarySize {
		return nil, fmt.Errorf("larger than %d bytes", maxBinarySize)
	}
	return b, nil
}
