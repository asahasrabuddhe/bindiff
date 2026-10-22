package bindiff

import (
	"bytes"
	"testing"
)

func TestQuickSuffixSort(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", nil},
		{"one byte", []byte{42}},
		{"all same", bytes.Repeat([]byte{7}, 300)},
		{"repeated pattern", []byte("abcdefabcdefabcdef")},
		{"banana", []byte("banana")},
		{"tiny sample", readFile(t, "testdata/golden/tiny/old")},
		{"random", randBytes(4096, 1)},
		{"low entropy", bytes.Repeat(randBytes(13, 2), 500)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			I := quickSuffixSort(tt.input)
			if len(I) != len(tt.input)+1 {
				t.Fatalf("len(I) = %d, want %d", len(I), len(tt.input)+1)
			}

			seen := make([]bool, len(I))
			for _, p := range I {
				if p < 0 || p > len(tt.input) || seen[p] {
					t.Fatalf("I is not a permutation of 0..%d: %v", len(tt.input), I)
				}
				seen[p] = true
			}

			for i := 1; i < len(I); i++ {
				if bytes.Compare(tt.input[I[i-1]:], tt.input[I[i]:]) >= 0 {
					t.Fatalf("suffixes out of order at %d: %q >= %q", i, tt.input[I[i-1]:], tt.input[I[i]:])
				}
			}
		})
	}
}
