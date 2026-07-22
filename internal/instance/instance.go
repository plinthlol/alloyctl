// Package instance manages named Minecraft instances: their metadata
// (instance.toml), and the split between the shared immutable version
// cache and each instance's own mutable game directory.
package instance

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"alloy/internal/config"
)

// Meta is the content of instances/<name>/instance.toml.
type Meta struct {
	Name          string   `toml:"name"`
	MCVersion     string   `toml:"mc_version"`
	Loader        string   `toml:"loader,omitempty"` // "", "fabric", "quilt", "forge", "neoforge"
	LoaderVersion string   `toml:"loader_version,omitempty"`
	JavaPath      string   `toml:"java_path,omitempty"` // per-instance override, wins over global
	MemoryMB      int      `toml:"memory_mb,omitempty"` // 0 = use global default
	JVMArgs       []string `toml:"jvm_args,omitempty"`
}

// CacheKey is the shared version-cache directory key for this instance's
// exact version+loader+loader-version combination, e.g.
// "1.20.4-fabric-0.15.11" or "1.20.4-vanilla".
func (m Meta) CacheKey() string {
	if m.Loader == "" {
		return m.MCVersion + "-vanilla"
	}
	return fmt.Sprintf("%s-%s-%s", m.MCVersion, m.Loader, m.LoaderVersion)
}

// DefaultName implements the naming rule: "<version>" for vanilla,
// "<version>-<loader>" for a loader, always lowercase.
func DefaultName(mcVersion, loader string) string {
	if loader == "" {
		return strings.ToLower(mcVersion)
	}
	return strings.ToLower(mcVersion + "-" + loader)
}

// Exists reports whether an instance with this name is already installed
// (i.e. has a config dir with instance.toml).
func Exists(p config.Paths, name string) bool {
	_, err := os.Stat(p.InstanceConfigDir(name) + "/instance.toml")
	return err == nil
}

// Load reads an instance's metadata.
func Load(p config.Paths, name string) (Meta, error) {
	data, err := os.ReadFile(p.InstanceConfigDir(name) + "/instance.toml")
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := toml.Unmarshal(data, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

// Save writes an instance's metadata, creating its config dir if needed.
// It does NOT touch the instance's data dir (mods/saves/etc) — callers
// create that separately via EnsureDataDir.
func Save(p config.Paths, m Meta) error {
	dir := p.InstanceConfigDir(m.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(dir+"/instance.toml", data, 0o644)
}

// EnsureDataDir creates the instance's mutable game directory tree:
// mods/, saves/, resourcepacks/, config/, logs/.
func EnsureDataDir(p config.Paths, name string) error {
	base := p.InstanceDataDir(name)
	for _, sub := range []string{"mods", "saves", "resourcepacks", "config", "logs"} {
		if err := os.MkdirAll(base+"/"+sub, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// List returns all installed instance names, sorted by their underlying
// MC version's manifest order via the caller (this just enumerates names;
// callers needing release/snapshot coloring should Load() each and cross
// reference against the version manifest).
func List(p config.Paths) ([]string, error) {
	entries, err := os.ReadDir(p.ConfigDir + "/instances")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Rename moves an instance's config and data directories and updates the
// `name` field in its metadata. Refuses if newName already exists.
func Rename(p config.Paths, oldName, newName string) error {
	if Exists(p, newName) {
		return fmt.Errorf("instance %q already exists", newName)
	}
	if !Exists(p, oldName) {
		return fmt.Errorf("instance %q does not exist", oldName)
	}

	m, err := Load(p, oldName)
	if err != nil {
		return err
	}
	m.Name = newName

	if err := os.Rename(p.InstanceConfigDir(oldName), p.InstanceConfigDir(newName)); err != nil {
		return err
	}
	// The data dir may not exist yet if install failed partway; that's OK.
	if _, err := os.Stat(p.InstanceDataDir(oldName)); err == nil {
		if err := os.Rename(p.InstanceDataDir(oldName), p.InstanceDataDir(newName)); err != nil {
			return err
		}
	}

	return Save(p, m)
}

// Remove deletes an instance's per-instance folders (mods/saves/config/
// everything mutable, plus its instance.toml). It never touches the
// shared version/library cache, since other instances may still
// reference those same downloaded files.
func Remove(p config.Paths, name string) error {
	if err := os.RemoveAll(p.InstanceConfigDir(name)); err != nil {
		return err
	}
	return os.RemoveAll(p.InstanceDataDir(name))
}
