package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	err := run(context.Background(), append([]string{"bindiff"}, args...), &out, &out)
	return out.String(), err
}

func TestDiffThenPatch(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	patchPath := filepath.Join(dir, "patch")
	outPath := filepath.Join(dir, "out")

	old := bytes.Repeat([]byte("the quick brown fox "), 500)
	updated := bytes.Replace(old, []byte("brown"), []byte("red"), 7)
	if err := os.WriteFile(oldPath, old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "diff", "-o", oldPath, "-n", newPath, "-p", patchPath); err != nil {
		t.Fatalf("diff: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "patch", "--oldfile", oldPath, "--newfile", outPath, "--patchfile", patchPath); err != nil {
		t.Fatalf("patch: %v\n%s", err, out)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, updated) {
		t.Fatal("patched file differs from new")
	}
}

func TestPatchLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	patchPath := filepath.Join(dir, "patch")
	outPath := filepath.Join(dir, "out")

	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patchPath, []byte("not a patch"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "patch", "-o", oldPath, "-n", outPath, "-p", patchPath); err == nil {
		t.Fatal("patch with a corrupt patch file succeeded")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory has %v, want only old and patch", names)
	}
}

func TestErrors(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing flag", []string{"diff", "-o", "a", "-n", "b"}, "patchfile"},
		{"missing old file", []string{"diff", "-o", filepath.Join(dir, "nope"), "-n", "b", "-p", "c"}, "nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runCLI(t, tt.args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error()+out, tt.want) {
				t.Fatalf("error %q (output %q) does not mention %q", err, out, tt.want)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	out, err := runCLI(t, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bindiff version") {
		t.Fatalf("--version printed %q", out)
	}
}
