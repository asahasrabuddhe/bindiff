package selfupdate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDigestJSON(t *testing.T) {
	d := Sum([]byte("hello"))

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"`; string(b) != want {
		t.Fatalf("Marshal = %s, want %s", b, want)
	}

	var back Digest
	if err := json.Unmarshal(b, &back); err != nil || back != d {
		t.Fatalf("Unmarshal = %v, %v", back, err)
	}

	for _, bad := range []string{`"abc"`, `"zz` + strings.Repeat("0", 62) + `"`} {
		if err := json.Unmarshal([]byte(bad), &back); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", bad)
		}
	}
}

func TestParseManifest(t *testing.T) {
	valid := makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary).manifest
	m, err := ParseManifest(valid)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "v1.1.0" || len(m.Patches) != 1 {
		t.Fatalf("ParseManifest = %+v", m)
	}

	replace := func(from, to string) []byte { return []byte(strings.Replace(string(valid), from, to, 1)) }
	tests := map[string][]byte{
		"not json":         []byte("nope"),
		"wrong schema":     replace(`"schema": 1`, `"schema": 2`),
		"asset with slash": replace(`"asset": "app_`, `"asset": "../app_`),
		"empty asset":      []byte(`{"schema":1,"patches":[{"binary":"a","os":"b","arch":"c","asset":"","patch_size":1}]}`),
		"no binary":        replace(`"binary": "app"`, `"binary": ""`),
		"zero patch size":  []byte(`{"schema":1,"patches":[{"binary":"a","os":"b","arch":"c","asset":"x","patch_size":0}]}`),
		"bad digest":       replace(`"old_sha256": "`, `"old_sha256": "00`),
	}
	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(b); err == nil {
				t.Fatal("ParseManifest succeeded")
			}
		})
	}
}

func TestManifestMatching(t *testing.T) {
	m, err := ParseManifest(makeRelease(t, "app", "linux", "amd64", oldBinary, newBinary).manifest)
	if err != nil {
		t.Fatal(err)
	}

	old, updated := Sum(oldBinary), Sum(newBinary)
	if m.find("app", "linux", "amd64", old) == nil {
		t.Error("find missed the matching entry")
	}
	for _, c := range []struct{ binary, goos, arch string }{
		{"other", "linux", "amd64"},
		{"app", "darwin", "amd64"},
		{"app", "linux", "arm64"},
	} {
		if m.find(c.binary, c.goos, c.arch, old) != nil {
			t.Errorf("find matched %v", c)
		}
	}
	if m.find("app", "linux", "amd64", updated) != nil {
		t.Error("find matched the new hash")
	}
	if !m.isLatest("app", "linux", "amd64", updated) || m.isLatest("app", "linux", "amd64", old) {
		t.Error("isLatest is wrong")
	}
}
