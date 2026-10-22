package release

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"go.ajitem.com/bindiff"
	"go.ajitem.com/bindiff/selfupdate"
)

// Config describes one run of the release command.
type Config struct {
	GitHub *GitHub
	// Repo is owner/name.
	Repo string
	// Tag is the release the new binaries belong to.
	Tag string
	// Previous is how many earlier releases to make patches from.
	Previous int
	// Binaries are the new builds.
	Binaries []Binary
	// OutDir receives the patches, manifest and signature.
	OutDir string
	// SigningKey signs the manifest when set.
	SigningKey ed25519.PrivateKey
	// Jobs bounds how many diffs run at once. Each needs about 16 bytes of
	// memory per byte of the old binary.
	Jobs int
	// Upload attaches the output files to the release.
	Upload bool
	// Log receives progress messages.
	Log io.Writer
}

// Result describes what Run produced.
type Result struct {
	Manifest *selfupdate.Manifest
	// ManifestPath is empty when there was nothing to patch from.
	ManifestPath string
}

// Run makes patches from the binaries of the previous releases to the new
// binaries, writes them with a manifest to OutDir, and uploads them if
// asked.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	logf := func(format string, args ...any) {
		if cfg.Log != nil {
			fmt.Fprintf(cfg.Log, format+"\n", args...)
		}
	}

	rels, err := cfg.GitHub.releases(ctx, cfg.Repo)
	if err != nil {
		return nil, err
	}

	current, previous := pickReleases(rels, cfg.Tag, cfg.Previous)
	if len(previous) == 0 {
		logf("No earlier release of %s to patch from.", cfg.Repo)
		return &Result{}, nil
	}

	newByKey := map[string]Binary{}
	names := map[string]bool{}
	for _, b := range cfg.Binaries {
		newByKey[b.key()] = b
		names[b.Name] = true
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}

	m := &selfupdate.Manifest{Schema: selfupdate.SchemaVersion, Version: cfg.Tag}
	for _, rel := range previous {
		olds, err := oldBinaries(ctx, cfg.GitHub, rel, names, logf)
		if err != nil {
			return nil, fmt.Errorf("release %s: %w", rel.TagName, err)
		}
		if len(olds) == 0 {
			logf("Release %s has no binaries named %s.", rel.TagName, strings.Join(slices.Sorted(maps.Keys(names)), ", "))
			continue
		}

		entries, err := diffAll(ctx, cfg, rel.TagName, olds, newByKey, logf)
		if err != nil {
			return nil, err
		}
		m.Patches = append(m.Patches, entries...)
	}

	if len(m.Patches) == 0 {
		logf("No patches made.")
		return &Result{Manifest: m}, nil
	}

	raw, err := m.Marshal()
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{selfupdate.ManifestName: raw}
	if cfg.SigningKey != nil {
		files[selfupdate.SignatureName] = selfupdate.Sign(raw, cfg.SigningKey)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(cfg.OutDir, name), b, 0o644); err != nil {
			return nil, err
		}
	}

	res := &Result{Manifest: m, ManifestPath: filepath.Join(cfg.OutDir, selfupdate.ManifestName)}
	if !cfg.Upload {
		return res, nil
	}

	if current == nil {
		return nil, fmt.Errorf("no release %s to upload to", cfg.Tag)
	}
	for _, e := range m.Patches {
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, e.Asset))
		if err != nil {
			return nil, err
		}
		files[e.Asset] = b
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		logf("Uploading %s", name)
		if err := cfg.GitHub.upload(ctx, cfg.Repo, *current, name, files[name]); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// pickReleases finds the release for tag and up to n published releases
// created before it. The API lists releases newest first.
func pickReleases(rels []ghRelease, tag string, n int) (current *ghRelease, previous []ghRelease) {
	start := 0
	for i := range rels {
		if rels[i].TagName == tag {
			current = &rels[i]
			start = i + 1
			break
		}
	}

	for _, r := range rels[start:] {
		if len(previous) == n {
			break
		}
		if r.Draft || r.Prerelease || r.TagName == tag {
			continue
		}
		previous = append(previous, r)
	}
	return current, previous
}

func oldBinaries(ctx context.Context, gh *GitHub, rel ghRelease, names map[string]bool, logf func(string, ...any)) ([]found, error) {
	var out []found
	for _, a := range rel.Assets {
		if skipAsset(a.Name) {
			continue
		}

		data, err := gh.download(ctx, a)
		if err != nil {
			return nil, err
		}
		got, err := extract(a.Name, data, names)
		if err != nil {
			logf("Skipping %s: %v", a.Name, err)
			continue
		}
		out = append(out, got...)
	}
	return out, nil
}

// skipAsset reports assets that can never hold a binary, to avoid
// downloading them.
func skipAsset(name string) bool {
	for _, ext := range []string{".txt", ".json", ".sig", ".bdiff", ".pem", ".sbom", ".deb", ".rpm", ".apk", ".md"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			return true
		}
	}
	return false
}

func diffAll(ctx context.Context, cfg Config, fromTag string, olds []found, newByKey map[string]Binary, logf func(string, ...any)) ([]selfupdate.Entry, error) {
	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, max(cfg.Jobs, 1))
		errs = make([]error, len(olds))
	)

	entries := make([]*selfupdate.Entry, len(olds))
	skipped := map[string]bool{}
	for i, old := range olds {
		nb, ok := newByKey[old.key()]
		if !ok {
			if !skipped[old.key()] {
				logf("No new %s build for %s/%s, skipping.", old.Name, old.OS, old.Arch)
				skipped[old.key()] = true
			}
			continue
		}

		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				errs[i] = ctx.Err()
				return
			}

			e, err := makePatch(cfg, fromTag, old, nb)
			switch {
			case errors.Is(err, errSame):
				logf("%s %s/%s is unchanged since %s, skipping.", old.Name, old.OS, old.Arch, fromTag)
			case err != nil:
				errs[i] = fmt.Errorf("%s %s/%s from %s: %w", old.Name, old.OS, old.Arch, fromTag, err)
			case e == nil:
				logf("%s %s/%s from %s: patch is no smaller than the binary, skipping.", old.Name, old.OS, old.Arch, fromTag)
			default:
				logf("%s %s/%s from %s: %d byte patch for a %d byte binary.", old.Name, old.OS, old.Arch, fromTag, e.PatchSize, e.NewSize)
				entries[i] = e
			}
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	var out []selfupdate.Entry
	for _, e := range entries {
		if e != nil {
			out = append(out, *e)
		}
	}
	return out, nil
}

var errSame = errors.New("old and new binaries are identical")

func makePatch(cfg Config, fromTag string, old found, nb Binary) (*selfupdate.Entry, error) {
	updated, err := os.ReadFile(nb.Path)
	if err != nil {
		return nil, err
	}

	oldSum, newSum := selfupdate.Sum(old.data), selfupdate.Sum(updated)
	if oldSum == newSum {
		return nil, errSame
	}

	var buf bytes.Buffer
	if err := bindiff.Diff(bytes.NewReader(old.data), bytes.NewReader(updated), &buf); err != nil {
		return nil, err
	}
	p := buf.Bytes()
	if len(p) >= len(updated) {
		return nil, nil
	}

	asset := selfupdate.AssetName(nb.Name, fromTag, cfg.Tag, nb.OS, nb.Arch)
	if err := os.WriteFile(filepath.Join(cfg.OutDir, asset), p, 0o644); err != nil {
		return nil, err
	}

	return &selfupdate.Entry{
		Binary:      nb.Name,
		OS:          nb.OS,
		Arch:        nb.Arch,
		FromVersion: fromTag,
		OldSHA256:   oldSum,
		NewSHA256:   newSum,
		Asset:       asset,
		PatchSHA256: selfupdate.Sum(p),
		PatchSize:   int64(len(p)),
		NewSize:     int64(len(updated)),
	}, nil
}
