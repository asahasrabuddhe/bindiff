package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveFiles serves files by name at the root and records request headers.
func serveFiles(t testing.TB, files map[string][]byte) (*httptest.Server, *[]http.Header) {
	t.Helper()

	var headers []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Clone())
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, &headers
}

func readAsset(t testing.TB, s Source, name string) ([]byte, error) {
	t.Helper()

	rc, err := s.Asset(context.Background(), name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func TestHTTPSource(t *testing.T) {
	srv, headers := serveFiles(t, map[string][]byte{
		ManifestName:  []byte("manifest"),
		SignatureName: []byte("sig"),
		"a b.bdiff":   []byte("patch"),
	})

	s := HTTP(srv.URL+"/", WithHeader("X-Token", "secret"))
	m, sig, err := s.Manifest(context.Background())
	if err != nil || string(m) != "manifest" || string(sig) != "sig" {
		t.Fatalf("Manifest = %q, %q, %v", m, sig, err)
	}

	if b, err := readAsset(t, s, "a b.bdiff"); err != nil || string(b) != "patch" {
		t.Fatalf("Asset = %q, %v", b, err)
	}
	if _, err := readAsset(t, s, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Asset(missing) = %v, want ErrNotFound", err)
	}

	for _, h := range *headers {
		if h.Get("X-Token") != "secret" {
			t.Fatal("WithHeader was not sent")
		}
	}
}

func TestHTTPSourceWithoutSignature(t *testing.T) {
	srv, _ := serveFiles(t, map[string][]byte{ManifestName: []byte("manifest")})

	m, sig, err := HTTP(srv.URL).Manifest(context.Background())
	if err != nil || string(m) != "manifest" || sig != nil {
		t.Fatalf("Manifest = %q, %q, %v", m, sig, err)
	}
}

func TestHTTPSourceErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if _, _, err := HTTP(srv.URL).Manifest(context.Background()); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("Manifest = %v, want a 503 error", err)
	}

	empty, _ := serveFiles(t, nil)
	if _, _, err := HTTP(empty.URL).Manifest(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Manifest = %v, want ErrNotFound", err)
	}
}

// fakeGitHub serves the latest-release API for owner/repo with the given
// assets, the way api.github.com does.
func fakeGitHub(t testing.TB, assets map[string][]byte) (*httptest.Server, *[]http.Header) {
	t.Helper()

	var headers []http.Header
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Clone())

		switch {
		case r.URL.Path == "/repos/owner/repo/releases/latest":
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			}
			var list []asset
			for name := range assets {
				list = append(list, asset{name, srv.URL + "/assets/" + name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.1.0", "assets": list})

		case strings.HasPrefix(r.URL.Path, "/assets/"):
			if r.Header.Get("Accept") != "application/octet-stream" {
				http.Error(w, "asset API needs Accept: application/octet-stream", http.StatusBadRequest)
				return
			}
			b, ok := assets[strings.TrimPrefix(r.URL.Path, "/assets/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &headers
}

func TestGitHubSource(t *testing.T) {
	srv, headers := fakeGitHub(t, map[string][]byte{
		ManifestName:  []byte("manifest"),
		SignatureName: []byte("sig"),
		"x.bdiff":     []byte("patch"),
	})

	s := GitHub("owner", "repo", WithBaseURL(srv.URL+"/"), WithToken("t0ken"))
	m, sig, err := s.Manifest(context.Background())
	if err != nil || string(m) != "manifest" || string(sig) != "sig" {
		t.Fatalf("Manifest = %q, %q, %v", m, sig, err)
	}

	if b, err := readAsset(t, s, "x.bdiff"); err != nil || string(b) != "patch" {
		t.Fatalf("Asset = %q, %v", b, err)
	}
	if _, err := readAsset(t, s, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Asset(missing) = %v, want ErrNotFound", err)
	}

	releaseCalls := 0
	for _, h := range *headers {
		if h.Get("Authorization") != "Bearer t0ken" {
			t.Fatal("WithToken was not sent")
		}
		if h.Get("Accept") == "application/vnd.github+json" {
			releaseCalls++
		}
	}
	if releaseCalls != 1 {
		t.Fatalf("fetched the release %d times, want once", releaseCalls)
	}
}

func TestGitHubSourceWithoutManifest(t *testing.T) {
	srv, _ := fakeGitHub(t, map[string][]byte{"app.tar.gz": nil})

	_, _, err := GitHub("owner", "repo", WithBaseURL(srv.URL)).Manifest(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Manifest = %v, want ErrNotFound", err)
	}

	_, _, err = GitHub("owner", "missing", WithBaseURL(srv.URL)).Manifest(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Manifest for a missing repo = %v, want ErrNotFound", err)
	}
}
