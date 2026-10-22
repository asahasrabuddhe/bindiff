package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// ErrUpToDate is returned by Check when the running binary is the one
	// the latest release built.
	ErrUpToDate = errors.New("selfupdate: already up to date")
	// ErrNoPatch is returned by Check when the latest release has no patch
	// from the running binary, for example a build several releases old or
	// a local build. Fall back to a full download.
	ErrNoPatch = errors.New("selfupdate: no patch for this build")
)

// Updater checks for and applies updates to the running program.
type Updater struct {
	// Source fetches release files. Required.
	Source Source

	// Binary is the name the manifest uses for this program. It defaults
	// to the executable's file name without a .exe suffix.
	Binary string

	// PublicKey, if set, makes Check reject a manifest without a valid
	// signature from the matching private key.
	PublicKey ed25519.PublicKey

	// Executable is the file to update. It defaults to the running
	// program, with symlinks resolved.
	Executable string

	// OS and Arch default to runtime.GOOS and runtime.GOARCH.
	OS, Arch string
}

// Update is an available update found by Check.
type Update struct {
	// Version is the release the update moves to.
	Version string
	// Entry is the manifest entry for the patch.
	Entry Entry

	exe string
}

func (u *Updater) defaults() (exe, binary, goos, arch string, err error) {
	exe, binary, goos, arch = u.Executable, u.Binary, u.OS, u.Arch
	if exe == "" {
		if exe, err = executable(); err != nil {
			return
		}
	}
	if binary == "" {
		binary = strings.TrimSuffix(filepath.Base(exe), ".exe")
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return
}

// Check fetches the latest manifest and looks for a patch from the running
// binary. It returns ErrUpToDate or ErrNoPatch when there is nothing to
// apply.
func (u *Updater) Check(ctx context.Context) (*Update, error) {
	exe, binary, goos, arch, err := u.defaults()
	if err != nil {
		return nil, err
	}

	current, err := os.ReadFile(exe)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: read executable: %w", err)
	}
	sum := Sum(current)

	raw, sig, err := u.Source.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	if u.PublicKey != nil {
		if err := Verify(raw, sig, u.PublicKey); err != nil {
			return nil, err
		}
	}

	m, err := ParseManifest(raw)
	if err != nil {
		return nil, err
	}

	if m.isLatest(binary, goos, arch, sum) {
		return nil, ErrUpToDate
	}

	e := m.find(binary, goos, arch, sum)
	if e == nil {
		return nil, fmt.Errorf("%w: %s %s/%s with sha256 %s in release %s", ErrNoPatch, binary, goos, arch, sum, m.Version)
	}

	return &Update{Version: m.Version, Entry: *e, exe: exe}, nil
}

// Apply downloads the update's patch, checks its hash, and applies it to
// the executable in place. See the package Apply for details.
func (u *Updater) Apply(ctx context.Context, up *Update) error {
	body, err := u.Source.Asset(ctx, up.Entry.Asset)
	if err != nil {
		return err
	}
	defer body.Close()

	patch, err := io.ReadAll(io.LimitReader(body, up.Entry.PatchSize+1))
	if err != nil {
		return fmt.Errorf("selfupdate: download %s: %w", up.Entry.Asset, err)
	}
	if int64(len(patch)) != up.Entry.PatchSize {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrChecksumMismatch, up.Entry.Asset, len(patch), up.Entry.PatchSize)
	}
	if got := Sum(patch); got != up.Entry.PatchSHA256 {
		return fmt.Errorf("%w: %s has sha256 %s, want %s", ErrChecksumMismatch, up.Entry.Asset, got, up.Entry.PatchSHA256)
	}

	return Apply(up.exe, bytes.NewReader(patch), up.Entry.OldSHA256, up.Entry.NewSHA256)
}
