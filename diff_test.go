package bindiff

import (
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"os"
	"testing"
)

func TestDiff(t *testing.T) {
	var tests = []struct {
		old   *os.File
		new   *os.File
		patch *os.File
	}{
		{
			old:   mustOpen("sample/old.bin"),
			new:   mustOpen("sample/new.bin"),
			patch: mustOpen("sample/patch.bin"),
		},
	}

	for _, tt := range tests {
		patch, err := ioutil.TempFile("/tmp", "patch.")
		if err != nil {
			t.Error(err)
		}

		err = Diff(tt.old, tt.new, patch)
		if err != nil {
			t.Error(err)
		}
		_, err = patch.Seek(0, 0)
		if err != nil {
			t.Error(err)
		}

		expectedBytes := mustReadAll(tt.patch)

		gotBytes := mustReadAll(patch)

		if !cmp.Equal(gotBytes, expectedBytes) {
			t.Errorf(cmp.Diff(gotBytes, expectedBytes))
		}
	}
}
