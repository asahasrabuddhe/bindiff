package selfupdate

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.ajitem.com/bindiff"
)

func TestApply(t *testing.T) {
	exe := writeExe(t, "app", oldBinary)
	p := makePatch(t, oldBinary, newBinary)

	if err := Apply(exe, bytes.NewReader(p), Sum(oldBinary), Sum(newBinary)); err != nil {
		t.Fatal(err)
	}

	assertFile(t, exe, newBinary)
	assertOnlyFiles(t, filepath.Dir(exe), "app")

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(exe)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Fatalf("mode = %v, want 0755", fi.Mode().Perm())
		}
	}
}

func TestApplyFailuresLeaveExecutableUntouched(t *testing.T) {
	p := makePatch(t, oldBinary, newBinary)

	tests := []struct {
		name     string
		patch    []byte
		old, new Digest
		want     error
	}{
		{"wrong old hash", p, Sum([]byte("other")), Sum(newBinary), ErrChecksumMismatch},
		{"wrong new hash", p, Sum(oldBinary), Sum([]byte("other")), ErrChecksumMismatch},
		{"corrupt patch", p[:len(p)/2], Sum(oldBinary), Sum(newBinary), bindiff.ErrCorruptPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exe := writeExe(t, "app", oldBinary)
			if err := Apply(exe, bytes.NewReader(tt.patch), tt.old, tt.new); !errors.Is(err, tt.want) {
				t.Fatalf("Apply = %v, want %v", err, tt.want)
			}

			assertFile(t, exe, oldBinary)
			assertOnlyFiles(t, filepath.Dir(exe), "app")
		})
	}
}

func TestApplyReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a Unix directory permission the current user cannot bypass")
	}

	exe := writeExe(t, "app", oldBinary)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := Apply(exe, bytes.NewReader(makePatch(t, oldBinary, newBinary)), Sum(oldBinary), Sum(newBinary))
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Apply = %v, want fs.ErrPermission", err)
	}
	assertFile(t, exe, oldBinary)
}

func TestReplaceBySwap(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "app.exe")
	src := filepath.Join(dir, "new")
	writeFile(t, dst, oldBinary)
	writeFile(t, src, newBinary)
	writeFile(t, dst+oldSuffix, []byte("leftover"))

	if err := replaceBySwap(src, dst); err != nil {
		t.Fatal(err)
	}

	assertFile(t, dst, newBinary)
	assertOnlyFiles(t, dir, "app.exe")
}

func TestReplaceBySwapRollsBack(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "app.exe")
	src := filepath.Join(dir, "new")
	writeFile(t, dst, oldBinary)
	writeFile(t, src, newBinary)

	boom := errors.New("boom")
	calls := 0
	rename = func(from, to string) error {
		calls++
		if calls == 2 { // moving the new binary into place
			return boom
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { rename = os.Rename })

	if err := replaceBySwap(src, dst); !errors.Is(err, boom) {
		t.Fatalf("replaceBySwap = %v, want %v", err, boom)
	}

	assertFile(t, dst, oldBinary)
	assertOnlyFiles(t, dir, "app.exe", "new")
}

func TestCleanupOld(t *testing.T) {
	exe := writeExe(t, "app", newBinary)
	if err := CleanupOld(exe); err != nil {
		t.Fatalf("CleanupOld with nothing to remove: %v", err)
	}

	writeFile(t, exe+oldSuffix, oldBinary)
	if err := CleanupOld(exe); err != nil {
		t.Fatal(err)
	}
	assertOnlyFiles(t, filepath.Dir(exe), "app")
}
