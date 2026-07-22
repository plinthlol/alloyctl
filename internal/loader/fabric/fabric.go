// Package fabric implements Fabric loader support: listing loader
// versions via the Fabric Meta API and merging the resulting launcher
// profile JSON into a runnable version.Version, exactly the same shape
// vanilla versions use so internal/launcher doesn't need to know Fabric
// exists.
//
// This is the simplest loader to support (pure JSON merge, no installer
// jar), and per the build order it's implemented first among the loaders.
package fabric

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"alloy/internal/version"
)

const metaBase = "https://meta.fabricmc.net/v2"

// LoaderVersion is one entry from GET /v2/versions/loader/<mc_version>.
type LoaderVersion struct {
	Loader struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	} `json:"loader"`
}

// ListLoaderVersions returns available Fabric loader versions for a given
// Minecraft version, newest first (as the API returns them).
func ListLoaderVersions(mcVersion string) ([]LoaderVersion, error) {
	url := fmt.Sprintf("%s/versions/loader/%s", metaBase, mcVersion)
	body, err := getJSON(url)
	if err != nil {
		return nil, err
	}
	var out []LoaderVersion
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parsing fabric loader versions: %w", err)
	}
	return out, nil
}

// LatestStable returns the newest stable loader version, or an error if
// none is marked stable (falls back to the newest overall in that case,
// since some MC versions briefly have only unstable builds).
func LatestStable(versions []LoaderVersion) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no fabric loader versions available for this Minecraft version")
	}
	for _, v := range versions {
		if v.Loader.Stable {
			return v.Loader.Version, nil
		}
	}
	return versions[0].Loader.Version, nil
}

// FetchProfile downloads the Fabric launcher profile JSON for a specific
// mc_version + loader_version pair
// (GET /v2/versions/loader/<mc_version>/<loader_version>/profile/json)
// and merges it onto the given vanilla base version, producing a single
// runnable version.Version. The merge rules mirror what the official
// Fabric installer does:
//   - id becomes "<mcVersion>-fabric-<loaderVersion>"
//   - mainClass, and the fabric-specific libraries are taken from the
//     profile
//   - inheritsFrom is recorded, but since we do the merge ourselves
//     upfront (rather than at launch time), internal/launcher never needs
//     to resolve inheritance chains itself
//   - arguments.game/jvm from the profile are appended to the vanilla
//     base's, since Fabric profiles only add loader-specific JVM args
//     (e.g. -DFabricMcEmu=...) and don't redefine the game args
func FetchProfile(mcVersion, loaderVersion string, base version.Version) (version.Version, error) {
	url := fmt.Sprintf("%s/versions/loader/%s/%s/profile/json", metaBase, mcVersion, loaderVersion)
	body, err := getJSON(url)
	if err != nil {
		return version.Version{}, err
	}

	var profile version.Version
	if err := json.Unmarshal(body, &profile); err != nil {
		return version.Version{}, fmt.Errorf("parsing fabric profile: %w", err)
	}

	merged := base
	merged.ID = fmt.Sprintf("%s-fabric-%s", mcVersion, loaderVersion)
	merged.MainClass = profile.MainClass
	merged.Libraries = append(append([]version.Library{}, profile.Libraries...), base.Libraries...)

	if profile.Arguments != nil {
		if merged.Arguments == nil {
			merged.Arguments = &version.Arguments{}
		}
		merged.Arguments.JVM = append(merged.Arguments.JVM, profile.Arguments.JVM...)
		merged.Arguments.Game = append(merged.Arguments.Game, profile.Arguments.Game...)
	}

	// Fabric doesn't change the required Java version or client jar/assets;
	// those stay inherited from the vanilla base untouched.
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
