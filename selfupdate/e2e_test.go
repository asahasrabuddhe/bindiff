package selfupdate

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEndToEnd builds testdata/app at two versions, publishes a signed
// release for the newer one, and has the older binary update itself.
func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries")
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "app")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}

	build := func(version, out string) []byte {
		t.Helper()
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.version="+version, "-o", out, "./testdata/app")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, b)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	oldBin := build("v1.0.0", exe)
	newBin := build("v1.1.0", filepath.Join(t.TempDir(), "app-new"))

	pubText, privText, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ParsePrivateKey(privText)
	if err != nil {
		t.Fatal(err)
	}

	rel := makeRelease(t, "app", runtime.GOOS, runtime.GOARCH, oldBin, newBin)
	files := maps.Clone(rel.assets)
	files[ManifestName] = rel.manifest
	files[SignatureName] = Sign(rel.manifest, priv)
	srv, _ := serveFiles(t, files)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(exe, args...)
		cmd.Env = append(os.Environ(), "APP_UPDATE_URL="+srv.URL, "APP_UPDATE_KEY="+pubText)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("app %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	if got := run("version"); got != "v1.0.0" {
		t.Fatalf("version before update = %q", got)
	}
	if got := run("update"); got != "updated to v1.1.0" {
		t.Fatalf("update printed %q", got)
	}
	if got := run("version"); got != "v1.1.0" {
		t.Fatalf("version after update = %q", got)
	}
	if got := run("update"); got != "up to date" {
		t.Fatalf("second update printed %q", got)
	}

	assertFile(t, exe, newBin)
	assertOnlyFiles(t, dir, filepath.Base(exe))
}
