package bindiff

import (
	"bytes"
	"errors"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	random := randBytes(64<<10, 1)

	tests := []struct {
		name         string
		old, updated []byte
	}{
		{"both empty", nil, nil},
		{"empty old", nil, []byte("hello")},
		{"empty new", []byte("hello"), nil},
		{"identical", random, random},
		{"one byte", []byte{1}, []byte{2}},
		{"append", random[:4096], append(bytes.Clone(random[:4096]), "tail"...)},
		{"prepend", random[:4096], append([]byte("head"), random[:4096]...)},
		{"insert middle", random[:4096], bytes.Join([][]byte{random[:2048], []byte("inserted"), random[2048:4096]}, nil)},
		{"delete middle", random[:4096], append(bytes.Clone(random[:1024]), random[3072:4096]...)},
		{"swap blocks", random[:4096], append(bytes.Clone(random[2048:4096]), random[:2048]...)},
		{"all zeros", make([]byte, 4096), make([]byte, 5000)},
		{"sparse edits", random, mutate(random, 512, 2)},
		{"unrelated random", randBytes(4096, 3), randBytes(4096, 4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustDiff(t, tt.old, tt.updated)
			if got := mustPatch(t, tt.old, p); !bytes.Equal(got, tt.updated) {
				t.Fatalf("Patch(Diff(old, new)) differs from new: got %d bytes, want %d", len(got), len(tt.updated))
			}
		})
	}
}

func TestDiffIsSmallForSmallEdits(t *testing.T) {
	old := randBytes(256<<10, 5)
	updated := mutate(old, 4096, 6)

	p := mustDiff(t, old, updated)
	if len(p) > len(updated)/10 {
		t.Fatalf("patch is %d bytes for a %d byte input with sparse edits, want under 10%%", len(p), len(updated))
	}
}

func TestRoundTripLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 8 MiB round trip in short mode")
	}

	old := randBytes(8<<20, 7)
	updated := mutate(old, 1<<16, 8)
	if got := mustPatch(t, old, mustDiff(t, old, updated)); !bytes.Equal(got, updated) {
		t.Fatal("Patch(Diff(old, new)) differs from new")
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func TestDiffPropagatesIOErrors(t *testing.T) {
	boom := errors.New("boom")

	if err := Diff(errReader{boom}, bytes.NewReader(nil), &bytes.Buffer{}); !errors.Is(err, boom) {
		t.Errorf("Diff with failing old reader = %v, want %v", err, boom)
	}
	if err := Diff(bytes.NewReader(nil), errReader{boom}, &bytes.Buffer{}); !errors.Is(err, boom) {
		t.Errorf("Diff with failing new reader = %v, want %v", err, boom)
	}
	if err := Diff(bytes.NewReader(nil), bytes.NewReader(nil), errWriter{boom}); !errors.Is(err, boom) {
		t.Errorf("Diff with failing writer = %v, want %v", err, boom)
	}
}
