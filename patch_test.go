package bindiff

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// rawPatch describes a patch field by field so tests can build malformed ones.
type rawPatch struct {
	magic   [8]byte
	newSize int64
	ctrl    [][3]int64
	diff    []byte
	extra   []byte

	// Overrides applied after the blocks are compressed. Nil means use the
	// real value.
	controlLength *int64
	diffLength    *int64
	// truncate, if positive, cuts the encoded patch to this many bytes.
	truncate int
}

func mustCompress(t testing.TB, p []byte) []byte {
	t.Helper()

	b, err := compress(p)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func (r rawPatch) encode(t testing.TB) []byte {
	t.Helper()

	var ctrl bytes.Buffer
	for _, c := range r.ctrl {
		if err := binary.Write(&ctrl, signMagnitudeLittleEndian{}, c); err != nil {
			t.Fatal(err)
		}
	}

	ctrlBlock := mustCompress(t, ctrl.Bytes())
	diffBlock := mustCompress(t, r.diff)
	extraBlock := mustCompress(t, r.extra)

	h := header{
		Magic:         r.magic,
		ControlLength: int64(len(ctrlBlock)),
		DiffLength:    int64(len(diffBlock)),
		NewSize:       r.newSize,
	}
	if r.controlLength != nil {
		h.ControlLength = *r.controlLength
	}
	if r.diffLength != nil {
		h.DiffLength = *r.diffLength
	}

	var out bytes.Buffer
	if err := binary.Write(&out, signMagnitudeLittleEndian{}, &h); err != nil {
		t.Fatal(err)
	}
	out.Write(ctrlBlock)
	out.Write(diffBlock)
	out.Write(extraBlock)

	b := out.Bytes()
	if r.truncate > 0 {
		b = b[:r.truncate]
	}

	return b
}

func TestPatchHandBuilt(t *testing.T) {
	// old "abcd" -> new "abXdEF": add 4 bytes (one changed), copy "EF".
	p := rawPatch{
		magic:   magic,
		newSize: 6,
		ctrl:    [][3]int64{{4, 2, 0}},
		diff:    []byte{0, 0, 'X' - 'c' + 256, 0},
		extra:   []byte("EF"),
	}.encode(t)

	if got := mustPatch(t, []byte("abcd"), p); string(got) != "abXdEF" {
		t.Fatalf("Patch = %q, want %q", got, "abXdEF")
	}
}

func TestPatchCorrupt(t *testing.T) {
	old := []byte("abcd")
	valid := rawPatch{
		magic:   magic,
		newSize: 6,
		ctrl:    [][3]int64{{4, 2, 0}},
		diff:    []byte{0, 0, 'X' - 'c' + 256, 0},
		extra:   []byte("EF"),
	}

	tests := []struct {
		name  string
		patch func() []byte
	}{
		{"empty", func() []byte { return nil }},
		{"short header", func() []byte { return valid.encode(t)[:20] }},
		{"bad magic", func() []byte {
			p := valid
			p.magic = [8]byte{'B', 'S', 'D', 'I', 'F', 'F', '4', '1'}
			return p.encode(t)
		}},
		{"negative new size", func() []byte {
			p := valid
			p.newSize = -1
			return p.encode(t)
		}},
		{"negative control length", func() []byte {
			p := valid
			p.controlLength = new(int64(-1))
			return p.encode(t)
		}},
		{"negative diff length", func() []byte {
			p := valid
			p.diffLength = new(int64(-1))
			return p.encode(t)
		}},
		{"huge control length", func() []byte {
			p := valid
			p.controlLength = new(int64(1 << 62))
			return p.encode(t)
		}},
		{"huge new size", func() []byte {
			p := valid
			p.newSize = 1 << 62
			return p.encode(t)
		}},
		{"control length too short", func() []byte {
			p := valid
			p.controlLength = new(int64(10))
			return p.encode(t)
		}},
		{"truncated extra block", func() []byte {
			p := valid
			b := p.encode(t)
			p.truncate = len(b) - 5
			return p.encode(t)
		}},
		{"negative add", func() []byte {
			p := valid
			p.ctrl = [][3]int64{{-1, 2, 0}}
			return p.encode(t)
		}},
		{"negative copy", func() []byte {
			p := valid
			p.ctrl = [][3]int64{{4, -2, 0}}
			return p.encode(t)
		}},
		{"add past new size", func() []byte {
			p := valid
			p.ctrl = [][3]int64{{7, 0, 0}}
			return p.encode(t)
		}},
		{"copy past new size", func() []byte {
			p := valid
			p.ctrl = [][3]int64{{4, 3, 0}}
			return p.encode(t)
		}},
		{"missing control triple", func() []byte {
			p := valid
			p.ctrl = nil
			return p.encode(t)
		}},
		{"diff block too short", func() []byte {
			p := valid
			p.diff = p.diff[:2]
			return p.encode(t)
		}},
		{"extra block too short", func() []byte {
			p := valid
			p.extra = p.extra[:1]
			return p.encode(t)
		}},
		{"control block not bzip2", func() []byte {
			b := valid.encode(t)
			copy(b[headerSize:], "nope")
			return b
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Patch panicked: %v", r)
				}
			}()

			err := Patch(bytes.NewReader(old), &bytes.Buffer{}, bytes.NewReader(tt.patch()))
			if !errors.Is(err, ErrCorruptPatch) {
				t.Fatalf("Patch error = %v, want ErrCorruptPatch", err)
			}
		})
	}
}

func TestPatchPropagatesIOErrors(t *testing.T) {
	boom := errors.New("boom")
	old := readFile(t, "testdata/golden/tiny/old")
	p := readFile(t, "testdata/golden/tiny/patch.bindiff")

	if err := Patch(errReader{boom}, &bytes.Buffer{}, bytes.NewReader(p)); !errors.Is(err, boom) {
		t.Errorf("Patch with failing old reader = %v, want %v", err, boom)
	}
	if err := Patch(bytes.NewReader(old), errWriter{boom}, bytes.NewReader(p)); !errors.Is(err, boom) {
		t.Errorf("Patch with failing writer = %v, want %v", err, boom)
	}
}
