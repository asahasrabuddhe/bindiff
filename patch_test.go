package bindiff

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPatch(t *testing.T) {
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
		{
			old: mustWriteRandomFile("old.", 1e3, 1),
			new: mustWriteRandomFile("new.", 1e3, 2),
		}, {
			old: mustWriteRandomFile("old.", 1e5, 1),
			new: mustWriteRandomFile("new.", 1e5, 2),
		}, {
			old: mustWriteRandomFile("old.", 1e7, 1),
			new: mustWriteRandomFile("new.", 1e7, 2),
		},
	}

	for _, tt := range tests {
		var err error
		if tt.patch == nil {
			tt.patch, err = ioutil.TempFile("/tmp", "patch.")
			if err != nil {
				t.Error(err)
			}

			err = Diff(tt.old, tt.new, tt.patch)
			if err != nil {
				t.Error(err)
			}

			_, err = tt.new.Seek(0, 0)
			if err != nil {
				t.Error(err)
			}

			_, err = tt.patch.Seek(0, 0)
			if err != nil {
				t.Error(err)
			}
		}

		patchedFile, err := ioutil.TempFile("/tmp", "patched.")
		if err != nil {
			t.Error(err)
		}

		err = Patch(tt.old, patchedFile, tt.patch)
		if err != nil {
			t.Error(err)
		}

		_, err = patchedFile.Seek(0, 0)
		if err != nil {
			t.Error(err)
		}

		expectedBytes := mustReadAll(tt.new)

		gotBytes := mustReadAll(patchedFile)

		if !cmp.Equal(gotBytes, expectedBytes) {
			t.Errorf(cmp.Diff(gotBytes, expectedBytes))
		}
	}
}
