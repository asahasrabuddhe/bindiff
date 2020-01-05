package bindiff

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

var sortT = [][]byte{
	mustRandBytes(1000),
	mustReadAll(mustOpen("sample/old.bin")),
	[]byte("abcdefabcdef"),
}

func TestQsufsort(t *testing.T) {
	for _, s := range sortT {
		I := quickSuffixSort(s)
		for i := 1; i < len(I); i++ {
			if cmp.Equal(s[I[i-1]:], s[I[i]:]) {
				t.Fatalf("unsorted at %d", i)
			}
		}
	}
}
