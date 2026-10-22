package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

type platform struct{ goos, arch string }

var platforms = []platform{{"linux", "amd64"}, {"windows", "amd64"}, {"darwin", "arm64"}}

var (
	buildOnce sync.Once
	buildDir  string
	buildErr  error
)

// hello returns the path of testdata/hello built for p at version, building
// every platform and version once per test run.
func hello(t testing.TB, p platform, version string) string {
	t.Helper()

	if testing.Short() {
		t.Skip("cross-compiles test binaries")
	}

	buildOnce.Do(func() {
		buildDir, buildErr = os.MkdirTemp("", "bindiff-release-test-")
		if buildErr != nil {
			return
		}
		for _, v := range []string{"v1.0.0", "v1.1.0"} {
			for _, p := range platforms {
				cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+v, "-o", helloPath(p, v), "./testdata/hello")
				cmd.Env = append(os.Environ(), "GOOS="+p.goos, "GOARCH="+p.arch, "CGO_ENABLED=0")
				if out, err := cmd.CombinedOutput(); err != nil {
					buildErr = &buildError{err, out}
					return
				}
			}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return helloPath(p, version)
}

type buildError struct {
	err error
	out []byte
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + string(e.out) }

func helloPath(p platform, version string) string {
	name := "hello"
	if p.goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(buildDir, version, p.goos+"_"+p.arch, name)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

func readFile(t testing.TB, path string) []byte {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tarGz builds a .tar.gz holding the given files, the way GoReleaser
// archives a release.
func tarGz(t testing.TB, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, b := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipOf(t testing.TB, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
