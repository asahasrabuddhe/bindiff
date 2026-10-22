// Package release builds the patches and manifest that the selfupdate
// package consumes, from a new release's binaries and the binaries of
// earlier releases on GitHub.
package release

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Binary is one build of a program for one platform.
type Binary struct {
	Name string // program name, without .exe
	OS   string
	Arch string
	Path string
}

func (b Binary) key() string { return b.Name + "/" + b.OS + "/" + b.Arch }

// FromGoReleaser reads the binaries GoReleaser built from dist/artifacts.json.
func FromGoReleaser(dist string) ([]Binary, error) {
	raw, err := os.ReadFile(filepath.Join(dist, "artifacts.json"))
	if err != nil {
		return nil, err
	}

	var artifacts []struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Goos    string `json:"goos"`
		Goarch  string `json:"goarch"`
		Goamd64 string `json:"goamd64"`
		Type    string `json:"type"`
		Extra   struct {
			Binary string `json:"Binary"`
		} `json:"extra"`
	}
	if err := json.Unmarshal(raw, &artifacts); err != nil {
		return nil, fmt.Errorf("parse artifacts.json: %w", err)
	}

	var out []Binary
	seen := map[string]bool{}
	for _, a := range artifacts {
		if a.Type != "Binary" {
			continue
		}
		// Only the baseline amd64 build gets patches. Users of other
		// microarchitecture levels fall back to a full download.
		if a.Goamd64 != "" && a.Goamd64 != "v1" {
			continue
		}

		name := a.Extra.Binary
		if name == "" {
			name = strings.TrimSuffix(a.Name, ".exe")
		}

		b := Binary{Name: name, OS: a.Goos, Arch: a.Goarch, Path: a.Path}
		if seen[b.key()] {
			return nil, fmt.Errorf("artifacts.json has more than one %s binary for %s/%s", b.Name, b.OS, b.Arch)
		}
		seen[b.key()] = true
		out = append(out, b)
	}

	if len(out) == 0 {
		return nil, errors.New("artifacts.json lists no binaries")
	}
	return out, nil
}

// FromGlobs finds binaries matching the glob patterns and detects each
// one's platform from its executable header. Files that are not
// executables are skipped.
func FromGlobs(patterns []string) ([]Binary, error) {
	var out []Binary
	seen := map[string]string{}
	for _, p := range patterns {
		paths, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}

		for _, path := range paths {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}

			goos, arch, ok := Detect(b)
			if !ok {
				continue
			}

			bin := Binary{Name: strings.TrimSuffix(filepath.Base(path), ".exe"), OS: goos, Arch: arch, Path: path}
			if prev, dup := seen[bin.key()]; dup {
				return nil, fmt.Errorf("%s and %s are both %s for %s/%s", prev, path, bin.Name, goos, arch)
			}
			seen[bin.key()] = path
			out = append(out, bin)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no executables match %v", patterns)
	}
	return out, nil
}

// Detect reports the GOOS and GOARCH an executable was built for. A macOS
// universal binary reports arch "all", as GoReleaser names it.
func Detect(b []byte) (goos, arch string, ok bool) {
	r := bytes.NewReader(b)

	if f, err := elf.NewFile(r); err == nil {
		goos = "linux"
		switch f.OSABI {
		case elf.ELFOSABI_FREEBSD:
			goos = "freebsd"
		case elf.ELFOSABI_NETBSD:
			goos = "netbsd"
		case elf.ELFOSABI_OPENBSD:
			goos = "openbsd"
		}

		switch f.Machine {
		case elf.EM_X86_64:
			arch = "amd64"
		case elf.EM_AARCH64:
			arch = "arm64"
		case elf.EM_386:
			arch = "386"
		case elf.EM_ARM:
			arch = "arm"
		case elf.EM_RISCV:
			arch = "riscv64"
		case elf.EM_PPC64:
			arch = "ppc64"
			if f.ByteOrder.String() == "LittleEndian" {
				arch = "ppc64le"
			}
		case elf.EM_S390:
			arch = "s390x"
		case elf.EM_LOONGARCH:
			arch = "loong64"
		default:
			return "", "", false
		}
		return goos, arch, true
	}

	if f, err := macho.NewFile(r); err == nil {
		switch f.Cpu {
		case macho.CpuAmd64:
			return "darwin", "amd64", true
		case macho.CpuArm64:
			return "darwin", "arm64", true
		}
		return "", "", false
	}

	if _, err := macho.NewFatFile(r); err == nil {
		return "darwin", "all", true
	}

	if f, err := pe.NewFile(r); err == nil {
		switch f.Machine {
		case pe.IMAGE_FILE_MACHINE_AMD64:
			return "windows", "amd64", true
		case pe.IMAGE_FILE_MACHINE_ARM64:
			return "windows", "arm64", true
		case pe.IMAGE_FILE_MACHINE_I386:
			return "windows", "386", true
		}
		return "", "", false
	}

	return "", "", false
}
