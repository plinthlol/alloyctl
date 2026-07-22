// Package config handles Alloy's global configuration, on-disk layout,
// and cross-platform path resolution (config / data / cache dirs).
package config

import (
	"github.com/adrg/xdg"
)

// Paths bundles the three directory roles Alloy uses on disk.
//
//   - ConfigDir: small, human-editable settings (config.toml, instances/*/instance.toml)
//   - DataDir:   the actual game content (instance game dirs, shared version cache)
//   - CacheDir:  transient, re-downloadable data (version manifest cache, etc.)
//
// github.com/adrg/xdg already implements the XDG Base Directory spec on Linux
// and the platform-appropriate equivalents on macOS (~/Library/Application
// Support, ~/Library/Caches, ...) and Windows (%APPDATA%, %LOCALAPPDATA%, ...),
// so we don't hand-roll per-OS path logic here.
type Paths struct {
	ConfigDir string
	DataDir   string
	CacheDir  string
}

const appName = "alloy"

// Resolve returns the Paths for the current OS, creating none of the
// directories yet (callers should mkdir -p lazily, on first write).
func Resolve() (Paths, error) {
	configDir, err := xdg.ConfigFile(appName + "/config.toml")
	if err != nil {
		return Paths{}, err
	}
	dataDir, err := xdg.DataFile(appName + "/.keep")
	if err != nil {
		return Paths{}, err
	}
	cacheDir, err := xdg.CacheFile(appName + "/.keep")
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		ConfigDir: dirOf(configDir),
		DataDir:   dirOf(dataDir),
		CacheDir:  dirOf(cacheDir),
	}, nil
}

func dirOf(fullPath string) string {
	// xdg.*File returns a path to a *file*, ensuring the parent dir exists.
	// We just want the parent directory.
	for i := len(fullPath) - 1; i >= 0; i-- {
		if fullPath[i] == '/' || fullPath[i] == '\\' {
			return fullPath[:i]
		}
	}
	return fullPath
}

// InstanceConfigDir returns $CONFIG/instances/<name>
func (p Paths) InstanceConfigDir(name string) string {
	return p.ConfigDir + "/instances/" + name
}

// InstanceDataDir returns $DATA/instances/<name> — the mutable game directory
// (mods/, saves/, config/, resourcepacks/, logs/) for a single instance.
func (p Paths) InstanceDataDir(name string) string {
	return p.DataDir + "/instances/" + name
}

// VersionCacheDir returns $DATA/cache/versions/<key> — the shared, immutable,
// checksum-verified cache of client jars / libraries / loader files for one
// exact version+loader+loader-version combination. Never mutated after
// download; referenced (not copied) by every instance using that combo.
func (p Paths) VersionCacheDir(key string) string {
	return p.DataDir + "/cache/versions/" + key
}

// AssetsDir returns $DATA/cache/assets — shared across all versions/instances,
// keyed internally by asset hash, same as vanilla Minecraft's assets layout.
func (p Paths) AssetsDir() string {
	return p.DataDir + "/cache/assets"
}

// ManifestCacheFile returns $CACHE/version_manifest_v2.json
func (p Paths) ManifestCacheFile() string {
	return p.CacheDir + "/version_manifest_v2.json"
}

// GlobalConfigFile returns $CONFIG/config.toml
func (p Paths) GlobalConfigFile() string {
	return p.ConfigDir + "/config.toml"
}
