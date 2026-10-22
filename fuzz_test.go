package bindiff

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// maxFuzzInput keeps each fuzz iteration fast. Diff is quadratic in the
// worst case.
const maxFuzzInput = 4 << 10

func FuzzRoundTrip(f *testing.F) {
	for _, c := range goldenCases(f) {
		if len(c.old) <= maxFuzzInput && len(c.updated) <= maxFuzzInput {
			f.Add(c.old, c.updated)
		}
	}

	f.Fuzz(func(t *testing.T, old, updated []byte) {
		if len(old) > maxFuzzInput || len(updated) > maxFuzzInput {
			t.Skip()
		}

		if got := mustPatch(t, old, mustDiff(t, old, updated)); !bytes.Equal(got, updated) {
			t.Fatalf("Patch(Diff(%q, %q)) = %q", old, updated, got)
		}
	})
}

func FuzzPatch(f *testing.F) {
	for _, c := range goldenCases(f) {
		for _, name := range []string{"patch.bindiff", "patch.bsdiff"} {
			if p, err := os.ReadFile(filepath.Join(c.dir, name)); err == nil && len(c.old) <= maxFuzzInput {
				f.Add(c.old, p)
			}
		}
	}

	f.Fuzz(func(t *testing.T, old, patch []byte) {
		if len(old) > maxFuzzInput {
			t.Skip()
		}

		var out bytes.Buffer
		err := Patch(bytes.NewReader(old), &out, bytes.NewReader(patch))
		if err != nil && !errors.Is(err, ErrCorruptPatch) {
			t.Fatalf("Patch error %v does not wrap ErrCorruptPatch", err)
		}
	})
}
