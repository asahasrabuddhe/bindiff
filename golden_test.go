package bindiff

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/golden/*/patch.bindiff from the current Diff output")

// goldenCase is one directory under testdata/golden. Each holds the inputs
// old and new, patch.bindiff produced by this package, and patch.bsdiff
// produced by the reference bsdiff 4.3 (see testdata/golden/gen.sh).
type goldenCase struct {
	name, dir    string
	old, updated []byte
}

func goldenCases(t testing.TB) []goldenCase {
	t.Helper()

	dirs, err := filepath.Glob("testdata/golden/*")
	if err != nil {
		t.Fatal(err)
	}

	var cases []goldenCase
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "old")); err != nil {
			continue // not a case, such as the gen directory
		}

		cases = append(cases, goldenCase{
			name:    filepath.Base(dir),
			dir:     dir,
			old:     readFile(t, filepath.Join(dir, "old")),
			updated: readFile(t, filepath.Join(dir, "new")),
		})
	}

	if len(cases) == 0 {
		t.Fatal("no golden cases found")
	}

	return cases
}

// TestGoldenDiff pins the exact bytes Diff produces, so a change to the
// algorithm or the compressor shows up as a test failure.
func TestGoldenDiff(t *testing.T) {
	for _, c := range goldenCases(t) {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(c.dir, "patch.bindiff")
			got := mustDiff(t, c.old, c.updated)

			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			if want := readFile(t, path); !bytes.Equal(got, want) {
				t.Fatalf("Diff output (%d bytes) differs from %s (%d bytes); run go test -run TestGoldenDiff -update if the change is intended", len(got), path, len(want))
			}
		})
	}
}

// TestGoldenPatch applies both our patch and the reference bsdiff patch.
func TestGoldenPatch(t *testing.T) {
	for _, c := range goldenCases(t) {
		for _, name := range []string{"patch.bindiff", "patch.bsdiff"} {
			t.Run(c.name+"/"+name, func(t *testing.T) {
				path := filepath.Join(c.dir, name)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					// bsdiff 4.3 cannot mmap an empty file, so empty cases
					// have no reference patch.
					t.Skipf("%s not generated", path)
				}

				if got := mustPatch(t, c.old, readFile(t, path)); !bytes.Equal(got, c.updated) {
					t.Fatalf("Patch(old, %s) differs from new", name)
				}
			})
		}
	}
}

// TestInteropBspatch checks that the reference bspatch applies our patches.
func TestInteropBspatch(t *testing.T) {
	bspatch, err := exec.LookPath("bspatch")
	if err != nil {
		t.Skip("bspatch not on PATH")
	}

	for _, c := range goldenCases(t) {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "new")
			cmd := exec.Command(bspatch, filepath.Join(c.dir, "old"), out, filepath.Join(c.dir, "patch.bindiff"))
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bspatch: %v\n%s", err, b)
			}

			if got := readFile(t, out); !bytes.Equal(got, c.updated) {
				t.Fatal("bspatch output differs from new")
			}
		})
	}
}
