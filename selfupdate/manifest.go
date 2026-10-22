package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// ManifestName is the file name of a release manifest.
	ManifestName = "bindiff-manifest.json"
	// SignatureName is the file name of a manifest's signature.
	SignatureName = ManifestName + ".sig"
	// SchemaVersion is the manifest schema this package reads and writes.
	SchemaVersion = 1
)

// Digest is a SHA-256 hash. It encodes to JSON as a hex string.
type Digest [sha256.Size]byte

// Sum returns the SHA-256 of b.
func Sum(b []byte) Digest { return sha256.Sum256(b) }

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

// MarshalText implements encoding.TextMarshaler.
func (d Digest) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Digest) UnmarshalText(text []byte) error {
	if hex.DecodedLen(len(text)) != len(d) {
		return fmt.Errorf("selfupdate: digest %q is not %d hex bytes", text, len(d))
	}
	_, err := hex.Decode(d[:], text)
	return err
}

// Manifest lists the patches published with one release.
type Manifest struct {
	Schema  int     `json:"schema"`
	Version string  `json:"version"`
	Patches []Entry `json:"patches"`
}

// Entry describes one patch from an older build of a binary to the build in
// this release.
type Entry struct {
	Binary      string `json:"binary"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	FromVersion string `json:"from_version"`
	OldSHA256   Digest `json:"old_sha256"`
	NewSHA256   Digest `json:"new_sha256"`
	Asset       string `json:"asset"`
	PatchSHA256 Digest `json:"patch_sha256"`
	PatchSize   int64  `json:"patch_size"`
	NewSize     int64  `json:"new_size"`
}

// ParseManifest decodes and validates a manifest.
func ParseManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("selfupdate: parse manifest: %w", err)
	}

	if m.Schema != SchemaVersion {
		return nil, fmt.Errorf("selfupdate: manifest schema %d, want %d", m.Schema, SchemaVersion)
	}

	for i, e := range m.Patches {
		if err := e.validate(); err != nil {
			return nil, fmt.Errorf("selfupdate: manifest entry %d: %w", i, err)
		}
	}

	return &m, nil
}

func (e Entry) validate() error {
	switch {
	case e.Binary == "" || e.OS == "" || e.Arch == "":
		return errors.New("binary, os and arch are required")
	case e.Asset == "" || strings.ContainsAny(e.Asset, `/\`) || e.Asset == "." || e.Asset == "..":
		return fmt.Errorf("invalid asset name %q", e.Asset)
	case e.PatchSize <= 0 || e.NewSize < 0:
		return errors.New("invalid size")
	}
	return nil
}

// Marshal encodes the manifest as indented JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// find returns the entry that patches the binary with hash old, or nil.
func (m *Manifest) find(binary, goos, arch string, old Digest) *Entry {
	for i, e := range m.Patches {
		if e.Binary == binary && e.OS == goos && e.Arch == arch && e.OldSHA256 == old {
			return &m.Patches[i]
		}
	}
	return nil
}

// isLatest reports whether old is the hash of this release's build.
func (m *Manifest) isLatest(binary, goos, arch string, old Digest) bool {
	for _, e := range m.Patches {
		if e.Binary == binary && e.OS == goos && e.Arch == arch && e.NewSHA256 == old {
			return true
		}
	}
	return false
}

// AssetName returns the conventional file name for a patch asset.
func AssetName(binary, fromVersion, toVersion, goos, arch string) string {
	return fmt.Sprintf("%s_%s_%s_%s_%s.bdiff", binary, fromVersion, toVersion, goos, arch)
}
