// Package manifest implements the signed binary catalog used to distribute
// every precompiled artifact Lair installs.
//
// Trust comes exclusively from the Ed25519 signature over the raw manifest
// bytes, never from the host that served it. That makes base-URL fallback safe.
package manifest

import (
	"encoding/json"
	"fmt"
	"time"
)

// Manifest is the top-level signed document.
type Manifest struct {
	Schema    int                                       `json:"schema"`
	Channel   string                                    `json:"channel"`
	Serial    int64                                     `json:"serial"`
	Generated time.Time                                 `json:"generated"`
	Expires   time.Time                                 `json:"expires"`
	Packages  map[string]map[string]map[string]Artifact `json:"packages"`
}

// Artifact is one build of one package version for one platform.
type Artifact struct {
	Build      string            `json:"build"`
	URL        string            `json:"url"`
	SHA256     string            `json:"sha256"`
	Size       int64             `json:"size"`
	Published  time.Time         `json:"published"`
	Prerelease bool              `json:"prerelease"`
	LTS        string            `json:"lts,omitempty"` // e.g. Node's "Lithium"; empty for non-LTS builds
	Yanked     map[string]string `json:"yanked,omitempty"`
	// Files, when set, selects archive members and places them at exact
	// relative destinations. It is used by builds whose layout differs from
	// Lair's (for example static PHP, which ships "php" and "php-fpm" at the
	// archive root instead of bin/ and sbin/). When empty the archive is
	// installed with its original layout.
	Files []FileMap `json:"files,omitempty"`
	// Raw installs a verified single-file release (no archive) at this path.
	// Meilisearch publishes its binaries directly, without a tarball.
	Raw string `json:"raw,omitempty"`
	// Components are additional verified archives merged into the same install
	// directory. PHP ships CLI and FPM as separate tarballs.
	Components []Component `json:"components,omitempty"`
}

// FileMap moves one regular file from an archive member to a destination path.
type FileMap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Component is an extra archive that belongs to the same installed version.
type Component struct {
	URL    string    `json:"url"`
	SHA256 string    `json:"sha256"`
	Size   int64     `json:"size"`
	Files  []FileMap `json:"files,omitempty"`
}

// IsYanked reports whether this artifact's build was yanked and why.
func (a Artifact) IsYanked() (string, bool) {
	if a.Yanked == nil {
		return "", false
	}
	reason, ok := a.Yanked[a.Build]
	return reason, ok
}

// Get returns an artifact for package/version/platform.
func (m *Manifest) Get(pkg, version, platform string) (Artifact, bool) {
	versions, ok := m.Packages[pkg]
	if !ok {
		return Artifact{}, false
	}
	platforms, ok := versions[version]
	if !ok {
		return Artifact{}, false
	}
	a, ok := platforms[platform]
	return a, ok
}

// LTSOf returns the long-term-support codename declared for one build, or ""
// when the version is not an LTS release (Node) or does not use LTS lines.
func (m *Manifest) LTSOf(pkg, version, platform string) string {
	a, ok := m.Get(pkg, version, platform)
	if !ok {
		return ""
	}
	return a.LTS
}

// Versions returns the available non-yanked versions for a package/platform.
func (m *Manifest) Versions(pkg, platform string) []string {
	out := []string{}
	for version, platforms := range m.Packages[pkg] {
		a, ok := platforms[platform]
		if !ok || a.SHA256 == "" {
			continue
		}
		if _, yanked := a.IsYanked(); yanked {
			continue
		}
		out = append(out, version)
	}
	return out
}

// Parse parses manifest bytes without verifying them. Use Verify for untrusted
// input; Parse is exported for tooling that already verified the signature.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Packages == nil {
		m.Packages = map[string]map[string]map[string]Artifact{}
	}
	return &m, nil
}
