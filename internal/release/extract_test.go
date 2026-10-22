package release

import "testing"

func TestExtract(t *testing.T) {
	linux := readFile(t, hello(t, platform{"linux", "amd64"}, "v1.0.0"))
	windows := readFile(t, hello(t, platform{"windows", "amd64"}, "v1.0.0"))
	names := map[string]bool{"hello": true}

	tests := []struct {
		asset    string
		data     []byte
		wantOS   string
		wantArch string
	}{
		{"hello_linux_amd64.tar.gz", tarGz(t, map[string][]byte{"hello_1.0.0/hello": linux, "hello_1.0.0/README.md": []byte("docs")}), "linux", "amd64"},
		{"hello_windows_amd64.zip", zipOf(t, map[string][]byte{"hello.exe": windows, "LICENSE": []byte("MIT")}), "windows", "amd64"},
		{"hello_linux_amd64", linux, "linux", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			got, err := extract(tt.asset, tt.data, names)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Name != "hello" || got[0].OS != tt.wantOS || got[0].Arch != tt.wantArch {
				t.Fatalf("extract = %+v", got)
			}
		})
	}

	got, err := extract("other_linux_amd64.tar.gz", tarGz(t, map[string][]byte{"other": linux}), names)
	if err != nil || len(got) != 0 {
		t.Fatalf("extract of an archive without hello = %+v, %v", got, err)
	}
}
