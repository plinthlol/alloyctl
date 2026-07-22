// Package version handles the Mojang version manifest, per-version JSON
// resolution, argument-rule evaluation, and download planning for vanilla
// Minecraft. Mod loaders (internal/loader/...) build on top of the Version
// type this package produces.
package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const manifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"

// ManifestEntry is one line of the top-level manifest: a version id, its
// type (release/snapshot/old_beta/old_alpha), and where to fetch its full
// version JSON.
type ManifestEntry struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	ReleaseTime time.Time `json:"releaseTime"`
	SHA1        string    `json:"sha1"`
}

// Manifest is the full version_manifest_v2.json.
type Manifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []ManifestEntry `json:"versions"`
}

// FetchManifest downloads the manifest fresh from Mojang and writes it to
// cachePath for offline reuse. Callers wanting cache-first behavior should
// use LoadOrFetchManifest instead.
func FetchManifest(cachePath string) (Manifest, error) {
	resp, err := http.Get(manifestURL)
	if err != nil {
		return Manifest{}, fmt.Errorf("fetching version manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("fetching version manifest: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Manifest{}, err
	}

	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing version manifest: %w", err)
	}

	if cachePath != "" {
		_ = os.MkdirAll(dirOf(cachePath), 0o755)
		_ = os.WriteFile(cachePath, body, 0o644)
	}
	return m, nil
}

// LoadOrFetchManifest tries the network first (the manifest is small and
// version lists change often); if that fails, it falls back to the last
// cached copy so `alloyctl install` still works offline for versions
// already seen before.
func LoadOrFetchManifest(cachePath string) (Manifest, error) {
	m, err := FetchManifest(cachePath)
	if err == nil {
		return m, nil
	}
	if cachePath != "" {
		if data, readErr := os.ReadFile(cachePath); readErr == nil {
			var cached Manifest
			if jsonErr := json.Unmarshal(data, &cached); jsonErr == nil {
				return cached, nil
			}
		}
	}
	return Manifest{}, err
}

// Find looks up a version by exact id.
func (m Manifest) Find(id string) (ManifestEntry, bool) {
	for _, v := range m.Versions {
		if v.ID == id {
			return v, true
		}
	}
	return ManifestEntry{}, false
}

// Oldest-first order matching the CLI's "oldest at top, latest at the very
// bottom" listing rule. The manifest itself is newest-first, so this just
// reverses a copy.
func (m Manifest) OldestFirst() []ManifestEntry {
	out := make([]ManifestEntry, len(m.Versions))
	for i, v := range m.Versions {
		out[len(m.Versions)-1-i] = v
	}
	return out
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}
