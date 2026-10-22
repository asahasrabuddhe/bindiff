package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ErrNotFound is returned by a Source when a file does not exist.
var ErrNotFound = errors.New("selfupdate: not found")

// maxManifestSize bounds how much of a manifest or signature is read.
const maxManifestSize = 4 << 20

// Source fetches the files a release publishes. Implement it to update
// from a private store. GitHub and HTTP cover the common cases.
type Source interface {
	// Manifest returns the latest manifest and its signature. The
	// signature is nil if the release has none.
	Manifest(ctx context.Context) (manifest, signature []byte, err error)
	// Asset opens a file from the same release as the last Manifest call.
	Asset(ctx context.Context, name string) (io.ReadCloser, error)
}

// Option configures GitHub and HTTP sources.
type Option func(*client)

// WithHTTPClient sets the client used for requests. The default is
// http.DefaultClient.
func WithHTTPClient(c *http.Client) Option { return func(cl *client) { cl.http = c } }

// WithHeader adds a header to every request, for example for
// authentication with an HTTP source.
func WithHeader(key, value string) Option {
	return func(cl *client) { cl.header.Add(key, value) }
}

// WithToken authenticates GitHub requests, which private repositories need.
func WithToken(token string) Option {
	return WithHeader("Authorization", "Bearer "+token)
}

// WithBaseURL sets the GitHub API URL, for GitHub Enterprise Server. The
// default is https://api.github.com.
func WithBaseURL(u string) Option { return func(cl *client) { cl.base = strings.TrimSuffix(u, "/") } }

type client struct {
	http   *http.Client
	header http.Header
	base   string
}

func newClient(base string, opts []Option) *client {
	c := &client{http: http.DefaultClient, header: http.Header{}, base: base}
	for _, o := range opts {
		o(c)
	}
	return c
}

// get returns the body of a successful GET. The caller closes it.
func (c *client) get(ctx context.Context, u, accept string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header = c.header.Clone()
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s", ErrNotFound, u)
	case resp.StatusCode != http.StatusOK:
		resp.Body.Close()
		return nil, fmt.Errorf("selfupdate: GET %s: %s", u, resp.Status)
	}
	return resp.Body, nil
}

func (c *client) getSmall(ctx context.Context, u, accept string) ([]byte, error) {
	body, err := c.get(ctx, u, accept)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	b, err := io.ReadAll(io.LimitReader(body, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("selfupdate: %w", err)
	}
	if len(b) > maxManifestSize {
		return nil, fmt.Errorf("selfupdate: %s is larger than %d bytes", u, maxManifestSize)
	}
	return b, nil
}

// HTTP returns a Source that reads a release from static hosting. The
// manifest is at baseURL/bindiff-manifest.json and each asset at
// baseURL/<name>. Point baseURL at the directory of the latest release.
func HTTP(baseURL string, opts ...Option) Source {
	return &httpSource{c: newClient(strings.TrimSuffix(baseURL, "/"), opts)}
}

type httpSource struct{ c *client }

func (s *httpSource) Manifest(ctx context.Context) ([]byte, []byte, error) {
	m, err := s.c.getSmall(ctx, s.c.base+"/"+ManifestName, "")
	if err != nil {
		return nil, nil, err
	}

	sig, err := s.c.getSmall(ctx, s.c.base+"/"+SignatureName, "")
	if errors.Is(err, ErrNotFound) {
		return m, nil, nil
	}
	return m, sig, err
}

func (s *httpSource) Asset(ctx context.Context, name string) (io.ReadCloser, error) {
	return s.c.get(ctx, s.c.base+"/"+url.PathEscape(name), "")
}

// GitHub returns a Source that reads the latest release of a GitHub
// repository. Drafts and prereleases are skipped, as GitHub's "latest"
// release does.
func GitHub(owner, repo string, opts ...Option) Source {
	return &githubSource{
		c:     newClient("https://api.github.com", opts),
		owner: owner,
		repo:  repo,
	}
}

type githubSource struct {
	c           *client
	owner, repo string

	mu     sync.Mutex
	assets map[string]string // name -> API URL
}

func (s *githubSource) release(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.assets != nil {
		return s.assets, nil
	}

	u := fmt.Sprintf("%s/repos/%s/%s/releases/latest", s.c.base, url.PathEscape(s.owner), url.PathEscape(s.repo))
	b, err := s.c.getSmall(ctx, u, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}

	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(b, &rel); err != nil {
		return nil, fmt.Errorf("selfupdate: parse GitHub release: %w", err)
	}

	s.assets = make(map[string]string, len(rel.Assets))
	for _, a := range rel.Assets {
		s.assets[a.Name] = a.URL
	}
	return s.assets, nil
}

func (s *githubSource) Manifest(ctx context.Context) ([]byte, []byte, error) {
	assets, err := s.release(ctx)
	if err != nil {
		return nil, nil, err
	}

	mu, ok := assets[ManifestName]
	if !ok {
		return nil, nil, fmt.Errorf("%w: latest release of %s/%s has no %s", ErrNotFound, s.owner, s.repo, ManifestName)
	}
	m, err := s.c.getSmall(ctx, mu, "application/octet-stream")
	if err != nil {
		return nil, nil, err
	}

	su, ok := assets[SignatureName]
	if !ok {
		return m, nil, nil
	}
	sig, err := s.c.getSmall(ctx, su, "application/octet-stream")
	return m, sig, err
}

func (s *githubSource) Asset(ctx context.Context, name string) (io.ReadCloser, error) {
	assets, err := s.release(ctx)
	if err != nil {
		return nil, err
	}

	u, ok := assets[name]
	if !ok {
		return nil, fmt.Errorf("%w: asset %s", ErrNotFound, name)
	}
	// The API URL with this Accept header redirects to the file, and works
	// for private repositories with a token.
	return s.c.get(ctx, u, "application/octet-stream")
}
