package release

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GitHub is a minimal client for the release endpoints of the GitHub API.
type GitHub struct {
	// BaseURL is the API root, https://api.github.com by default.
	BaseURL string
	// Token authenticates requests. Uploads need contents: write.
	Token string
	// HTTP is the client to use, http.DefaultClient by default.
	HTTP *http.Client
}

type ghRelease struct {
	ID         int64     `json:"id"`
	TagName    string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	UploadURL  string    `json:"upload_url"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

func (g *GitHub) do(ctx context.Context, method, u, accept string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	c := g.HTTP
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %s: %s", method, u, resp.Status, bytes.TrimSpace(b))
	}
	return resp, nil
}

func (g *GitHub) api(path string) string {
	base := g.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	return strings.TrimSuffix(base, "/") + path
}

// releases returns the newest releases of repo, drafts included when the
// token can see them, newest first.
func (g *GitHub) releases(ctx context.Context, repo string) ([]ghRelease, error) {
	resp, err := g.do(ctx, http.MethodGet, g.api("/repos/"+repo+"/releases?per_page=100"), "application/vnd.github+json", http.NoBody, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rels []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, fmt.Errorf("parse releases: %w", err)
	}
	return rels, nil
}

func (g *GitHub) download(ctx context.Context, a ghAsset) ([]byte, error) {
	resp, err := g.do(ctx, http.MethodGet, a.URL, "application/octet-stream", http.NoBody, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readLimited(resp.Body)
}

// upload attaches data to rel as name, replacing an asset of the same name
// so a release job can be re-run.
func (g *GitHub) upload(ctx context.Context, repo string, rel ghRelease, name string, data []byte) error {
	for _, a := range rel.Assets {
		if a.Name == name {
			resp, err := g.do(ctx, http.MethodDelete, g.api(fmt.Sprintf("/repos/%s/releases/assets/%d", repo, a.ID)), "", http.NoBody, "")
			if err != nil {
				return err
			}
			resp.Body.Close()
		}
	}

	// upload_url is a URI template such as .../assets{?name,label}.
	u, _, _ := strings.Cut(rel.UploadURL, "{")
	u += "?name=" + url.QueryEscape(name)

	resp, err := g.do(ctx, http.MethodPost, u, "application/vnd.github+json", bytes.NewReader(data), "application/octet-stream")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
