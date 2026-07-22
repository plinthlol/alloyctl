// Package quilt implements Quilt loader support. Quilt's meta API and
// profile JSON shape are intentionally near-identical to Fabric's (Quilt
// is a Fabric fork that maintains API compatibility), so this mirrors
// internal/loader/fabric almost exactly, just pointed at the Quilt Meta
// API and using Quilt's id naming.
package quilt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"alloy/internal/version"
)

const metaBase = "https://meta.quiltmc.org/v3"

// LoaderVersion is one entry from GET /v3/versions/loader/<mc_version>.
type LoaderVersion struct {
	Loader struct {
		Version string `json:"version"`
	} `json:"loader"`
}

// ListLoaderVersions returns available Quilt loader versions for a given
// Minecraft version, newest first.
func ListLoaderVersions(mcVersion string) ([]LoaderVersion, error) {
	url := fmt.Sprintf("%s/versions/loader/%s", metaBase, mcVersion)
	body, err := getJSON(url)
	if err != nil {
		return nil, err
	}
	var out []LoaderVersion
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parsing quilt loader versions: %w", err)
	}
	return out, nil
}

// Latest returns the newest loader version. Unlike Fabric's Meta API,
// Quilt's doesn't mark builds "stable" in the same field, so we just take
// the first (newest) entry the API returns.
func Latest(versions []LoaderVersion) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no quilt loader versions available for this Minecraft version")
	}
	return versions[0].Loader.Version, nil
}

// FetchProfile downloads the Quilt launcher profile JSON and merges it
// onto a vanilla base version, same merge strategy as Fabric.
func FetchProfile(mcVersion, loaderVersion string, base version.Version) (version.Version, error) {
	url := fmt.Sprintf("%s/versions/loader/%s/%s/profile/json", metaBase, mcVersion, loaderVersion)
	body, err := getJSON(url)
	if err != nil {
		return version.Version{}, err
	}

	var profile version.Version
	if err := json.Unmarshal(body, &profile); err != nil {
		return version.Version{}, fmt.Errorf("parsing quilt profile: %w", err)
	}

	merged := base
	merged.ID = fmt.Sprintf("%s-quilt-%s", mcVersion, loaderVersion)
	merged.MainClass = profile.MainClass
	merged.Libraries = append(append([]version.Library{}, profile.Libraries...), base.Libraries...)

	if profile.Arguments != nil {
		if merged.Arguments == nil {
			merged.Arguments = &version.Arguments{}
		}
		merged.Arguments.JVM = append(merged.Arguments.JVM, profile.Arguments.JVM...)
		merged.Arguments.Game = append(merged.Arguments.Game, profile.Arguments.Game...)
	}

	return merged, nil
}

func getJSON(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
