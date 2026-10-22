package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"go.ajitem.com/bindiff"
)

// ErrChecksumMismatch is returned when a binary or patch does not have the
// SHA-256 the manifest promises. It is the same error bindiff.Patch returns
// when the data does not match the checksums inside a patch.
var ErrChecksumMismatch = bindiff.ErrChecksumMismatch

// oldSuffix marks the previous binary that Windows cannot delete while it
// runs.
const oldSuffix = ".old"

// rename is os.Rename, replaced in tests to simulate failures.
var rename = os.Rename

// Apply patches the executable at exe in place. The executable must hash to
// oldSHA256, and the patched result must hash to newSHA256, or Apply returns
// an error wrapping ErrChecksumMismatch and leaves exe untouched.
//
// The new binary is written to a temporary file in the same directory and
// renamed over exe, so exe is replaced in one step. If the directory is not
// writable, the error wraps fs.ErrPermission.
func Apply(exe string, patch io.Reader, oldSHA256, newSHA256 Digest) error {
	old, err := os.ReadFile(exe)
	if err != nil {
		return fmt.Errorf("selfupdate: read executable: %w", err)
	}

	if got := Sum(old); got != oldSHA256 {
		return fmt.Errorf("%w: %s has sha256 %s, patch expects %s", ErrChecksumMismatch, exe, got, oldSHA256)
	}

	fi, err := os.Stat(exe)
	if err != nil {
		return fmt.Errorf("selfupdate: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), "."+filepath.Base(exe)+".new-*")
	if err != nil {
		return fmt.Errorf("selfupdate: create temporary file next to %s: %w", exe, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	h := sha256.New()
	if err := bindiff.Patch(bytes.NewReader(old), io.MultiWriter(tmp, h), patch); err != nil {
		tmp.Close()
		return fmt.Errorf("selfupdate: apply patch: %w", err)
	}

	if got := Digest(h.Sum(nil)); got != newSHA256 {
		tmp.Close()
		return fmt.Errorf("%w: patched binary has sha256 %s, want %s", ErrChecksumMismatch, got, newSHA256)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("selfupdate: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("selfupdate: %w", err)
	}
	if err := os.Chmod(tmpName, fi.Mode().Perm()); err != nil {
		return fmt.Errorf("selfupdate: %w", err)
	}

	if runtime.GOOS == "windows" {
		return replaceBySwap(tmpName, exe)
	}
	return replaceByRename(tmpName, exe)
}

// replaceByRename renames src over dst. On Unix a running program keeps
// its open inode, so replacing its file is safe.
func replaceByRename(src, dst string) error {
	if err := rename(src, dst); err != nil {
		return fmt.Errorf("selfupdate: replace %s: %w", dst, err)
	}

	// Make the rename durable. Not every platform can sync a directory.
	if d, err := os.Open(filepath.Dir(dst)); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// replaceBySwap moves dst aside before moving src into place, because
// Windows refuses to overwrite a running executable but allows renaming it.
// If the second rename fails, the first is undone.
func replaceBySwap(src, dst string) error {
	old := dst + oldSuffix
	_ = os.Remove(old) // left over from an earlier update

	if err := rename(dst, old); err != nil {
		return fmt.Errorf("selfupdate: move %s aside: %w", dst, err)
	}

	if err := rename(src, dst); err != nil {
		if rerr := rename(old, dst); rerr != nil {
			return fmt.Errorf("selfupdate: replace %s: %w (restoring the old binary also failed: %w)", dst, err, rerr)
		}
		return fmt.Errorf("selfupdate: replace %s: %w", dst, err)
	}

	// This fails while the old binary is still running, which is expected.
	// CleanupOld removes it on the next start.
	_ = os.Remove(old)
	return nil
}

// CleanupOld removes the previous binary that an update on Windows left
// next to exe. Pass "" for the running executable. It is safe to call on
// every start and on every platform.
func CleanupOld(exe string) error {
	if exe == "" {
		var err error
		if exe, err = executable(); err != nil {
			return err
		}
	}

	if err := os.Remove(exe + oldSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("selfupdate: %w", err)
	}
	return nil
}

// executable returns the path of the running program with symlinks
// resolved, so the update replaces the real file rather than a link to it.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("selfupdate: find executable: %w", err)
	}

	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("selfupdate: find executable: %w", err)
	}
	return exe, nil
}
