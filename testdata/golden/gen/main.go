// Command gen writes the old and new inputs for each golden test case under
// testdata/golden. Run it through ../gen.sh, which also produces the patches.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", "testdata/golden", "directory to write cases into")
	license := flag.String("license", "LICENSE", "text file used for the text-edit case")
	flag.Parse()

	text, err := os.ReadFile(*license)
	if err != nil {
		log.Fatal(err)
	}

	random := randBytes(32<<10, 1)

	cases := map[string][2][]byte{
		"empty-old":           {nil, []byte("hello, world\n")},
		"empty-new":           {[]byte("hello, world\n"), nil},
		"both-empty":          {nil, nil},
		"identical":           {random[:8192], random[:8192]},
		"one-byte":            {{0x01}, {0x02}},
		"append":              {random[:8192], cat(random[:8192], []byte("appended tail"))},
		"prepend":             {random[:8192], cat([]byte("prepended head"), random[:8192])},
		"insert-middle":       {random[:8192], cat(random[:4096], []byte("inserted in the middle"), random[4096:8192])},
		"delete-middle":       {random[:8192], cat(random[:2048], random[6144:8192])},
		"swap-blocks":         {random[:8192], cat(random[4096:8192], random[:4096])},
		"all-zeros":           {make([]byte, 16<<10), make([]byte, 20<<10)},
		"repeated-pattern":    {bytes.Repeat([]byte("abcdefgh"), 2048), cat(bytes.Repeat([]byte("abcdefgh"), 1024), []byte("X"), bytes.Repeat([]byte("abcdefgh"), 1024))},
		"random-sparse-edits": {random, sparseEdits(random, 256, 2)},
		"text-edit":           {text, editText(text)},
		"shifted-code":        shiftedCode(48<<10, 3),
	}

	for name, c := range cases {
		dir := filepath.Join(*root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
		for i, file := range []string{"old", "new"} {
			if err := os.WriteFile(filepath.Join(dir, file), c[i], 0o644); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func randBytes(n int, seed uint64) []byte {
	r := rand.New(rand.NewPCG(seed, seed))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}

// sparseEdits changes about one byte in every stride.
func sparseEdits(b []byte, stride int, seed uint64) []byte {
	r := rand.New(rand.NewPCG(seed, seed))
	out := bytes.Clone(b)
	for i := r.IntN(stride); i < len(out); i += 1 + r.IntN(2*stride) {
		out[i] ^= byte(1 + r.IntN(255))
	}
	return out
}

// editText makes the kind of edits a person makes to a document.
func editText(text []byte) []byte {
	out := bytes.Replace(text, []byte("2019"), []byte("2019-2026"), 1)
	out = bytes.Replace(out, []byte("Permission is hereby granted"), []byte("Permission is, as before, hereby granted"), 1)
	return append(out, "\nThis line was added at the end.\n"...)
}

// shiftedCode mimics two builds of a program where 100 bytes were inserted
// near the start. Every 4-byte address that points past the insertion moves
// by 100, which is the case bsdiff's diff block is designed for.
func shiftedCode(n int, seed uint64) [2][]byte {
	const insertAt, shift = 1024, 100

	r := rand.New(rand.NewPCG(seed, seed))
	old := make([]byte, n)
	var addrs []int
	for i := 0; i+8 <= n; i += 8 {
		if r.IntN(3) == 0 {
			// An "instruction" carrying an absolute address.
			old[i] = 0xe8
			binary.LittleEndian.PutUint32(old[i+1:], uint32(r.IntN(n)))
			addrs = append(addrs, i+1)
			continue
		}
		for j := range 8 {
			old[i+j] = byte(r.IntN(16)) // low-entropy opcodes
		}
	}

	shifted := bytes.Clone(old)
	for _, a := range addrs {
		if v := binary.LittleEndian.Uint32(shifted[a:]); v >= insertAt {
			binary.LittleEndian.PutUint32(shifted[a:], v+shift)
		}
	}

	inserted := make([]byte, shift)
	for i := range inserted {
		inserted[i] = byte(r.IntN(16))
	}

	return [2][]byte{old, cat(shifted[:insertAt], inserted, shifted[insertAt:])}
}
