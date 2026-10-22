package release

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	for _, p := range platforms {
		goos, arch, ok := Detect(readFile(t, hello(t, p, "v1.0.0")))
		if !ok || goos != p.goos || arch != p.arch {
			t.Errorf("Detect(%s/%s build) = %s, %s, %v", p.goos, p.arch, goos, arch, ok)
		}
	}

	for _, junk := range [][]byte{nil, []byte("#!/bin/sh\necho hi\n"), make([]byte, 4096)} {
		if _, _, ok := Detect(junk); ok {
			t.Errorf("Detect(%q) reported an executable", junk[:min(len(junk), 10)])
		}
	}
}

func TestFromGoReleaser(t *testing.T) {
	dist := t.TempDir()
	artifacts := `[
	  {"name": "app", "path": "dist/app_linux_amd64_v1/app", "goos": "linux", "goarch": "amd64", "goamd64": "v1", "type": "Binary", "extra": {"Binary": "app"}},
	  {"name": "app", "path": "dist/app_linux_amd64_v3/app", "goos": "linux", "goarch": "amd64", "goamd64": "v3", "type": "Binary", "extra": {"Binary": "app"}},
	  {"name": "app.exe", "path": "dist/app_windows_arm64/app.exe", "goos": "windows", "goarch": "arm64", "type": "Binary", "extra": {"Binary": "app"}},
	  {"name": "app_linux_amd64.tar.gz", "path": "dist/app_linux_amd64.tar.gz", "goos": "linux", "goarch": "amd64", "type": "Archive"}
	]`
	if err := os.WriteFile(filepath.Join(dist, "artifacts.json"), []byte(artifacts), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FromGoReleaser(dist)
	if err != nil {
		t.Fatal(err)
	}
	want := []Binary{
		{"app", "linux", "amd64", "dist/app_linux_amd64_v1/app"},
		{"app", "windows", "arm64", "dist/app_windows_arm64/app.exe"},
	}
	if len(got) != len(want) {
		t.Fatalf("FromGoReleaser = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FromGoReleaser = %+v, want %+v", got, want)
		}
	}
}

func TestFromGlobs(t *testing.T) {
	dir := t.TempDir()
	for _, p := range platforms {
		src := hello(t, p, "v1.1.0")
		dst := filepath.Join(dir, p.goos+"_"+p.arch, filepath.Base(src))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, readFile(t, src), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "linux_amd64", "README"), []byte("not a binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FromGlobs([]string{filepath.Join(dir, "*", "*")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(platforms) {
		t.Fatalf("FromGlobs found %+v", got)
	}
	for _, b := range got {
		if b.Name != "hello" {
			t.Errorf("binary name = %q, want hello", b.Name)
		}
	}
}
