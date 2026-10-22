package bindiff

import (
	"fmt"
	"testing"
)

var benchSizes = []int{1 << 10, 64 << 10, 1 << 20}

// benchInputs returns an old file and two candidate updated files: one with
// sparse edits, like a updated build of the same program, and one unrelated.
func benchInputs(size int) (old, edited, unrelated []byte) {
	old = randBytes(size, 1)
	return old, mutate(old, 1024, 2), randBytes(size, 3)
}

func BenchmarkQuickSuffixSort(b *testing.B) {
	for _, size := range benchSizes {
		input := randBytes(size, 1)
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				quickSuffixSort(input)
			}
		})
	}
}

func BenchmarkDiff(b *testing.B) {
	for _, size := range benchSizes {
		old, edited, unrelated := benchInputs(size)
		for _, in := range []struct {
			name    string
			updated []byte
		}{{"edited", edited}, {"unrelated", unrelated}} {
			b.Run(fmt.Sprintf("%s/size=%d", in.name, size), func(b *testing.B) {
				b.SetBytes(int64(size))
				b.ReportAllocs()
				for b.Loop() {
					mustDiff(b, old, in.updated)
				}
			})
		}
	}
}

func BenchmarkPatch(b *testing.B) {
	for _, size := range benchSizes {
		old, edited, unrelated := benchInputs(size)
		for _, in := range []struct {
			name    string
			updated []byte
		}{{"edited", edited}, {"unrelated", unrelated}} {
			p := mustDiff(b, old, in.updated)
			b.Run(fmt.Sprintf("%s/size=%d", in.name, size), func(b *testing.B) {
				b.SetBytes(int64(size))
				b.ReportAllocs()
				for b.Loop() {
					mustPatch(b, old, p)
				}
			})
		}
	}
}
