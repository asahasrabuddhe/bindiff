package bindiff

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"
)

func TestPatchRejectsWrongOldData(t *testing.T) {
	old := randBytes(8192, 1)
	updated := mutate(old, 256, 2)
	p := mustDiff(t, old, updated)

	wrong := mutate(old, 4096, 3)
	var out bytes.Buffer
	err := Patch(bytes.NewReader(wrong), &out, bytes.NewReader(p))
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Patch with the wrong old data = %v, want ErrChecksumMismatch", err)
	}
	if out.Len() != 0 {
		t.Fatalf("Patch wrote %d bytes before rejecting the old data", out.Len())
	}
}

func TestPatchChecksNewData(t *testing.T) {
	old := randBytes(8192, 1)
	p := mustDiff(t, old, mutate(old, 256, 2))

	// Flip a bit in the new-data checksum.
	p[len(p)-len(trailerMagic)-1] ^= 1

	err := Patch(bytes.NewReader(old), &bytes.Buffer{}, bytes.NewReader(p))
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Patch = %v, want ErrChecksumMismatch", err)
	}
}

func TestPatchWithoutTrailer(t *testing.T) {
	old := randBytes(8192, 1)
	updated := mutate(old, 256, 2)
	p := mustDiff(t, old, updated)
	legacy := p[:len(p)-trailerSize]

	if got := mustPatch(t, old, legacy); !bytes.Equal(got, updated) {
		t.Fatal("Patch without a trailer gave the wrong output")
	}

	// Without checksums, the wrong old data goes undetected, as in bsdiff.
	if err := Patch(bytes.NewReader(mutate(old, 4096, 3)), &bytes.Buffer{}, bytes.NewReader(legacy)); err != nil {
		t.Fatalf("Patch without a trailer = %v", err)
	}
}

func TestDiffWritesTrailer(t *testing.T) {
	old, updated := []byte("old data"), []byte("new data")
	_, sums := splitTrailer(mustDiff(t, old, updated))
	if sums == nil {
		t.Fatal("Diff wrote no trailer")
	}
	if sums.old != sha256.Sum256(old) || sums.new != sha256.Sum256(updated) {
		t.Fatal("trailer checksums are wrong")
	}
}
