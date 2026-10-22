package bindiff

import (
	"bytes"
	"math/rand/v2"
	"os"
	"testing"
)

// readFile returns the contents of path or fails the test.
func readFile(t testing.TB, path string) []byte {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// randBytes returns n deterministic pseudo-random bytes for the given seed.
func randBytes(n int, seed uint64) []byte {
	r := rand.New(rand.NewPCG(seed, seed))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Uint32())
	}

	return b
}

// mutate returns a copy of b with roughly one byte in every stride changed,
// which looks like a small edit to a real file.
func mutate(b []byte, stride int, seed uint64) []byte {
	r := rand.New(rand.NewPCG(seed, seed))
	out := bytes.Clone(b)
	for i := r.IntN(stride); i < len(out); i += 1 + r.IntN(2*stride) {
		out[i] ^= byte(1 + r.IntN(255))
	}

	return out
}

// mustDiff runs Diff on in-memory buffers.
func mustDiff(t testing.TB, old, updated []byte) []byte {
	t.Helper()

	var patch bytes.Buffer
	if err := Diff(bytes.NewReader(old), bytes.NewReader(updated), &patch); err != nil {
		t.Fatalf("Diff: %v", err)
	}

	return patch.Bytes()
}

// mustPatch runs Patch on in-memory buffers.
func mustPatch(t testing.TB, old, p []byte) []byte {
	t.Helper()

	var out bytes.Buffer
	if err := Patch(bytes.NewReader(old), &out, bytes.NewReader(p)); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	return out.Bytes()
}
