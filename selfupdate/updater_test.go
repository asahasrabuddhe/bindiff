package selfupdate

import (
	"context"
	"crypto/ed25519"
	"errors"
	"maps"
	"path/filepath"
	"testing"
)

func newTestUpdater(t testing.TB, rel release, exe string) *Updater {
	t.Helper()

	files := maps.Clone(rel.assets)
	files[ManifestName] = rel.manifest
	srv, _ := serveFiles(t, files)
	return &Updater{Source: HTTP(srv.URL), Executable: exe, OS: "linux", Arch: "amd64"}
}

func TestUpdater(t *testing.T) {
	ctx := context.Background()
	exe := writeExe(t, "app", oldBinary)
	u := newTestUpdater(t, makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary), exe)

	up, err := u.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if up.Version != "v1.1.0" || up.Entry.FromVersion != "v1.0.0" {
		t.Fatalf("Check = %+v", up)
	}

	if err := u.Apply(ctx, up); err != nil {
		t.Fatal(err)
	}
	assertFile(t, exe, newBinary)

	if _, err := u.Check(ctx); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("Check after update = %v, want ErrUpToDate", err)
	}
}

func TestUpdaterBinaryNameDefaultsToExecutable(t *testing.T) {
	exe := writeExe(t, "app.exe", oldBinary)
	u := newTestUpdater(t, makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary), exe)

	if _, err := u.Check(context.Background()); err != nil {
		t.Fatalf("Check = %v; want app.exe to match binary app", err)
	}
}

func TestUpdaterNoPatch(t *testing.T) {
	exe := writeExe(t, "app", []byte("a local build"))
	u := newTestUpdater(t, makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary), exe)

	if _, err := u.Check(context.Background()); !errors.Is(err, ErrNoPatch) {
		t.Fatalf("Check = %v, want ErrNoPatch", err)
	}
}

func TestUpdaterSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	rel := makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary)
	exe := writeExe(t, "app", oldBinary)

	unsigned := newTestUpdater(t, rel, exe)
	unsigned.PublicKey = pub
	if _, err := unsigned.Check(context.Background()); !errors.Is(err, ErrSignature) {
		t.Fatalf("Check of unsigned manifest = %v, want ErrSignature", err)
	}

	rel.assets[SignatureName] = Sign(rel.manifest, priv)
	signed := newTestUpdater(t, rel, exe)
	signed.PublicKey = pub
	if _, err := signed.Check(context.Background()); err != nil {
		t.Fatalf("Check of signed manifest = %v", err)
	}
}

func TestUpdaterRejectsTamperedPatch(t *testing.T) {
	ctx := context.Background()
	rel := makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary)
	for name, p := range rel.assets {
		tampered := append([]byte(nil), p...)
		tampered[len(tampered)-1] ^= 1
		rel.assets[name] = tampered
	}

	exe := writeExe(t, "app", oldBinary)
	u := newTestUpdater(t, rel, exe)
	up, err := u.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Apply(ctx, up); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Apply = %v, want ErrChecksumMismatch", err)
	}
	assertFile(t, exe, oldBinary)
	assertOnlyFiles(t, filepath.Dir(exe), "app")
}
