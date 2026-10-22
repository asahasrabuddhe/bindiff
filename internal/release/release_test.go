package release

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.ajitem.com/bindiff"
	"go.ajitem.com/bindiff/selfupdate"
)

// fakeGitHub serves the release endpoints Run uses for repo owner/app.
type fakeGitHub struct {
	t        testing.TB
	srv      *httptest.Server
	mu       sync.Mutex
	releases []ghRelease
	files    map[int64][]byte
	nextID   int64
	uploaded map[string][]byte
	deleted  []string
}

func newFakeGitHub(t testing.TB) *fakeGitHub {
	f := &fakeGitHub{t: t, files: map[int64][]byte{}, nextID: 1, uploaded: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// addRelease adds a release. Releases must be added oldest first.
func (f *fakeGitHub) addRelease(tag string, draft bool, assets map[string][]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := f.nextID
	f.nextID++
	rel := ghRelease{
		ID:        id,
		TagName:   tag,
		Draft:     draft,
		UploadURL: fmt.Sprintf("%s/uploads/%d/assets{?name,label}", f.srv.URL, id),
	}
	for name, b := range assets {
		aid := f.nextID
		f.nextID++
		f.files[aid] = b
		rel.Assets = append(rel.Assets, ghAsset{ID: aid, Name: name, URL: fmt.Sprintf("%s/assets/%d", f.srv.URL, aid), Size: int64(len(b))})
	}
	f.releases = append([]ghRelease{rel}, f.releases...) // newest first
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer test-token" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var id int64
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/app/releases":
		_ = json.NewEncoder(w).Encode(f.releases)

	case r.Method == http.MethodGet && sscan(r.URL.Path, "/assets/%d", &id):
		if r.Header.Get("Accept") != "application/octet-stream" {
			http.Error(w, "want octet-stream", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(f.files[id])

	case r.Method == http.MethodPost && sscan(r.URL.Path, "/uploads/%d/assets", &id):
		name := r.URL.Query().Get("name")
		b, _ := io.ReadAll(r.Body)
		f.uploaded[name] = b
		for i := range f.releases {
			if f.releases[i].ID == id {
				f.releases[i].Assets = append(f.releases[i].Assets, ghAsset{ID: f.nextID, Name: name})
				f.nextID++
			}
		}
		w.WriteHeader(http.StatusCreated)

	case r.Method == http.MethodDelete && sscan(r.URL.Path, "/repos/owner/app/releases/assets/%d", &id):
		for i := range f.releases {
			for j, a := range f.releases[i].Assets {
				if a.ID == id {
					f.deleted = append(f.deleted, a.Name)
					f.releases[i].Assets = append(f.releases[i].Assets[:j], f.releases[i].Assets[j+1:]...)
					break
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.NotFound(w, r)
	}
}

func sscan(path, format string, id *int64) bool {
	n, err := fmt.Sscanf(path, format, id)
	return err == nil && n == 1 && fmt.Sprintf(format, *id) == path
}

// v1Assets returns release v1.0.0's assets as GoReleaser would publish them.
func v1Assets(t testing.TB) map[string][]byte {
	assets := map[string][]byte{"checksums.txt": []byte("not read")}
	for _, p := range platforms {
		bin := hello(t, p, "v1.0.0")
		name := filepath.Base(bin)
		if p.goos == "windows" {
			assets["hello_1.0.0_windows_amd64.zip"] = zipOf(t, map[string][]byte{name: readFile(t, bin)})
			continue
		}
		assets[fmt.Sprintf("hello_1.0.0_%s_%s.tar.gz", p.goos, p.arch)] = tarGz(t, map[string][]byte{name: readFile(t, bin), "README.md": []byte("docs")})
	}
	return assets
}

func newBinaries(t testing.TB) []Binary {
	var out []Binary
	for _, p := range platforms {
		out = append(out, Binary{Name: "hello", OS: p.goos, Arch: p.arch, Path: hello(t, p, "v1.1.0")})
	}
	return out
}

func TestRun(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.addRelease("v0.9.0-rc1", false, nil)
	gh.releases[0].Prerelease = true
	gh.addRelease("v1.0.0", false, v1Assets(t))
	gh.addRelease("v1.1.0", true, nil) // GoReleaser draft

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	cfg := Config{
		GitHub:     &GitHub{BaseURL: gh.srv.URL, Token: "test-token"},
		Repo:       "owner/app",
		Tag:        "v1.1.0",
		Previous:   2,
		Binaries:   newBinaries(t),
		OutDir:     t.TempDir(),
		SigningKey: priv,
		Jobs:       2,
		Upload:     true,
		Log:        &log,
	}

	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, log.String())
	}
	if len(res.Manifest.Patches) != len(platforms) {
		t.Fatalf("made %d patches, want %d\n%s", len(res.Manifest.Patches), len(platforms), log.String())
	}

	manifest := gh.uploaded[selfupdate.ManifestName]
	if err := selfupdate.Verify(manifest, gh.uploaded[selfupdate.SignatureName], pub); err != nil {
		t.Fatalf("uploaded manifest signature: %v", err)
	}
	m, err := selfupdate.ParseManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range m.Patches {
		if e.FromVersion != "v1.0.0" || m.Version != "v1.1.0" {
			t.Errorf("entry %s goes from %s to %s", e.Asset, e.FromVersion, m.Version)
		}

		p := platform{e.OS, e.Arch}
		old, updated := readFile(t, hello(t, p, "v1.0.0")), readFile(t, hello(t, p, "v1.1.0"))
		patch := gh.uploaded[e.Asset]
		if selfupdate.Sum(patch) != e.PatchSHA256 || selfupdate.Sum(old) != e.OldSHA256 || selfupdate.Sum(updated) != e.NewSHA256 {
			t.Errorf("%s: hashes in the manifest are wrong", e.Asset)
		}

		var out bytes.Buffer
		if err := bindiff.Patch(bytes.NewReader(old), &out, bytes.NewReader(patch)); err != nil {
			t.Fatalf("%s: %v", e.Asset, err)
		}
		if !bytes.Equal(out.Bytes(), updated) {
			t.Errorf("%s does not produce the new binary", e.Asset)
		}
	}

	// A re-run replaces the assets rather than failing on duplicates.
	if _, err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(gh.deleted) != len(platforms)+2 {
		t.Fatalf("second run deleted %v, want every uploaded asset", gh.deleted)
	}
}

func TestRunWithoutPreviousRelease(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.addRelease("v1.0.0", true, nil)

	var log bytes.Buffer
	res, err := Run(context.Background(), Config{
		GitHub:   &GitHub{BaseURL: gh.srv.URL, Token: "test-token"},
		Repo:     "owner/app",
		Tag:      "v1.0.0",
		Previous: 1,
		OutDir:   t.TempDir(),
		Upload:   true,
		Log:      &log,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ManifestPath != "" || len(gh.uploaded) != 0 {
		t.Fatalf("Run with no earlier release produced %+v and uploaded %d files", res, len(gh.uploaded))
	}
	if !strings.Contains(log.String(), "No earlier release") {
		t.Fatalf("log = %q", log.String())
	}
}

func TestPickReleases(t *testing.T) {
	rels := []ghRelease{
		{TagName: "v3"},
		{TagName: "v2", Draft: true},
		{TagName: "v1.5-rc", Prerelease: true},
		{TagName: "v1"},
		{TagName: "v0"},
	}

	current, prev := pickReleases(rels, "v2", 5)
	if current == nil || current.TagName != "v2" {
		t.Fatalf("current = %+v", current)
	}
	if len(prev) != 2 || prev[0].TagName != "v1" || prev[1].TagName != "v0" {
		t.Fatalf("previous = %+v, want v1 and v0", prev)
	}

	// A tag with no release yet patches from the newest releases.
	current, prev = pickReleases(rels, "v4", 1)
	if current != nil || len(prev) != 1 || prev[0].TagName != "v3" {
		t.Fatalf("pickReleases(v4) = %+v, %+v", current, prev)
	}
}

func TestMakePatchSkipsUnchangedBinary(t *testing.T) {
	bin := hello(t, platform{"linux", "amd64"}, "v1.0.0")
	_, err := makePatch(Config{OutDir: t.TempDir(), Tag: "v1.1.0"}, "v1.0.0",
		found{Binary{Name: "hello", OS: "linux", Arch: "amd64"}, readFile(t, bin)},
		Binary{Name: "hello", OS: "linux", Arch: "amd64", Path: bin})
	if !errors.Is(err, errSame) {
		t.Fatalf("makePatch of identical binaries = %v, want errSame", err)
	}
}
