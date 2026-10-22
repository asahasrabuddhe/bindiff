package selfupdate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go.ajitem.com/bindiff"
)

var (
	oldBinary = bytes.Repeat([]byte("old program build "), 2000)
	newBinary = bytes.Replace(oldBinary, []byte("old"), []byte("new"), 50)
)

func makePatch(t testing.TB, old, updated []byte) []byte {
	t.Helper()

	var p bytes.Buffer
	if err := bindiff.Diff(bytes.NewReader(old), bytes.NewReader(updated), &p); err != nil {
		t.Fatal(err)
	}
	return p.Bytes()
}

// release is everything a release publishes for one patch.
type release struct {
	manifest []byte
	assets   map[string][]byte
}

func makeRelease(t testing.TB, binary, goos, arch string, old, updated []byte) release {
	t.Helper()

	p := makePatch(t, old, updated)
	asset := AssetName(binary, "v1.0.0", "v1.1.0", goos, arch)
	m := &Manifest{
		Schema:  SchemaVersion,
		Version: "v1.1.0",
		Patches: []Entry{{
			Binary:      binary,
			OS:          goos,
			Arch:        arch,
			FromVersion: "v1.0.0",
			OldSHA256:   Sum(old),
			NewSHA256:   Sum(updated),
			Asset:       asset,
			PatchSHA256: Sum(p),
			PatchSize:   int64(len(p)),
			NewSize:     int64(len(updated)),
		}},
	}

	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return release{manifest: b, assets: map[string][]byte{asset: p}}
}

func writeFile(t testing.TB, path string, content []byte) {
	t.Helper()

	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeExe writes an executable file into a new temporary directory.
func writeExe(t testing.TB, name string, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	writeFile(t, path, content)
	return path
}

func assertFile(t testing.TB, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s has %d bytes with sha256 %s, want sha256 %s", path, len(got), Sum(got), Sum(want))
	}
}

// assertOnlyFiles fails if dir holds anything but the named files, such as
// a leftover temporary file.
func assertOnlyFiles(t testing.TB, dir string, names ...string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if len(got) != len(names) {
		t.Fatalf("%s holds %v, want %v", dir, got, names)
	}
	for i := range got {
		if got[i] != names[i] {
			t.Fatalf("%s holds %v, want %v", dir, got, names)
		}
	}
}
