package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Version is a fully-resolved, launchable version definition — this is
// what a vanilla version JSON deserializes into, and it's also the shape
// mod loaders (Fabric/Quilt/Forge/NeoForge) produce after merging their
// own loader JSON on top of a vanilla base, so internal/launcher only
// needs to know about this one type regardless of loader.
type Version struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	MainClass     string          `json:"mainClass"`
	InheritsFrom  string          `json:"inheritsFrom,omitempty"`
	AssetIndex    AssetIndexRef   `json:"assetIndex"`
	Assets        string          `json:"assets"`
	Downloads     Downloads       `json:"downloads"`
	Libraries     []Library       `json:"libraries"`
	JavaVersion   JavaVersionSpec `json:"javaVersion"`
	Arguments     *Arguments      `json:"arguments,omitempty"`          // 1.13+
	MinecraftArgs string          `json:"minecraftArguments,omitempty"` // pre-1.13
}

// JavaVersionSpec is the source of truth for which Java major version a
// given Minecraft version needs. We read this directly rather than
// guessing from the MC version number (see internal/javafind).
type JavaVersionSpec struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

type AssetIndexRef struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	TotalSize int64  `json:"totalSize"`
}

type Downloads struct {
	Client DownloadArtifact `json:"client"`
	Server DownloadArtifact `json:"server,omitempty"`
}

type DownloadArtifact struct {
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	Path string `json:"path,omitempty"` // only present on library artifacts
}

// Library describes one Java library dependency, optionally restricted to
// specific OSes/architectures via Rules, and optionally carrying
// platform-specific native artifacts (pre-1.19 style).
type Library struct {
	Name      string          `json:"name"` // maven coordinate: group:artifact:version[:classifier]
	URL       string          `json:"url,omitempty"`
	Downloads LibraryDownload `json:"downloads"`
	Rules     []Rule          `json:"rules,omitempty"`

	// Natives maps OS name -> classifier, e.g. {"linux": "natives-linux"}.
	// Present on older-style native library entries.
	Natives map[string]string `json:"natives,omitempty"`

	// Extract lists paths to exclude when unpacking a natives jar
	// (e.g. META-INF). Only meaningful when Natives is set.
	Extract *struct {
		Exclude []string `json:"exclude,omitempty"`
	} `json:"extract,omitempty"`
}

type LibraryDownload struct {
	Artifact    *DownloadArtifact           `json:"artifact,omitempty"`
	Classifiers map[string]DownloadArtifact `json:"classifiers,omitempty"`
}

// LibraryArtifactInfo contains download and location details for a library jar.
type LibraryArtifactInfo struct {
	URL  string
	Path string
	SHA1 string
	Size int64
}

// ArtifactInfo resolves the download URL, relative path, SHA1, and Size for a library.
// If Downloads.Artifact is provided with a Path (Mojang style), it is used directly.
// Otherwise, the relative path and download URL are computed from Name and URL (Fabric/Quilt style).
func (l Library) ArtifactInfo() (LibraryArtifactInfo, bool) {
	if l.Downloads.Artifact != nil && l.Downloads.Artifact.Path != "" {
		return LibraryArtifactInfo{
			URL:  l.Downloads.Artifact.URL,
			Path: l.Downloads.Artifact.Path,
			SHA1: l.Downloads.Artifact.SHA1,
			Size: l.Downloads.Artifact.Size,
		}, true
	}

	if l.Name == "" {
		return LibraryArtifactInfo{}, false
	}

	relPath, err := MavenPath(l.Name)
	if err != nil {
		return LibraryArtifactInfo{}, false
	}

	baseURL := l.URL
	if baseURL == "" {
		if strings.HasPrefix(l.Name, "net.fabricmc") {
			baseURL = "https://maven.fabricmc.net/"
		} else if strings.HasPrefix(l.Name, "org.quiltmc") {
			baseURL = "https://maven.quiltmc.org/repository/release/"
		} else {
			baseURL = "https://repo1.maven.org/maven2/"
		}
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	return LibraryArtifactInfo{
		URL:  baseURL + relPath,
		Path: relPath,
		SHA1: "",
		Size: 0,
	}, true
}

// MavenPath parses a Maven coordinate (e.g. "net.fabricmc:fabric-loader:0.15.11")
// into a relative path ("net/fabricmc/fabric-loader/0.15.11/fabric-loader-0.15.11.jar").
func MavenPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty maven coordinate")
	}

	ext := "jar"
	if idx := strings.Index(name, "@"); idx != -1 {
		ext = name[idx+1:]
		name = name[:idx]
	}

	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid maven coordinate %q", name)
	}

	group := strings.ReplaceAll(parts[0], ".", "/")
	artifact := parts[1]
	version := parts[2]
	classifier := ""
	if len(parts) >= 4 {
		classifier = parts[3]
	}

	filename := fmt.Sprintf("%s-%s", artifact, version)
	if classifier != "" {
		filename += "-" + classifier
	}
	filename += "." + ext

	return fmt.Sprintf("%s/%s/%s/%s", group, artifact, version, filename), nil
}

// Arguments holds the 1.13+ rules-based game/jvm argument lists. Each
// element is either a plain string or a conditional object; see
// ArgumentEntry.
type Arguments struct {
	Game []ArgumentEntry `json:"game,omitempty"`
	JVM  []ArgumentEntry `json:"jvm,omitempty"`
}

// ArgumentEntry represents one entry in a 1.13+ arguments.game/jvm array.
// It's polymorphic in the source JSON: either a bare string, or an object
// {rules: [...], value: string|[]string}. UnmarshalJSON below normalizes
// both shapes into this struct.
type ArgumentEntry struct {
	Rules []Rule
	Value []string // always normalized to a slice, even for single-string values
}

func (a *ArgumentEntry) UnmarshalJSON(data []byte) error {
	// Case 1: bare string.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		a.Value = []string{s}
		a.Rules = nil
		return nil
	}

	// Case 2: conditional object.
	var obj struct {
		Rules []Rule          `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("argument entry is neither a string nor a rule object: %w", err)
	}
	a.Rules = obj.Rules

	// value can itself be a string or an array of strings.
	var single string
	if err := json.Unmarshal(obj.Value, &single); err == nil {
		a.Value = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(obj.Value, &multi); err == nil {
		a.Value = multi
		return nil
	}
	return fmt.Errorf("argument entry value is neither string nor []string")
}

// Rule is a single "rules" entry, used both by Arguments and by Library,
// e.g. {"action": "allow", "os": {"name": "osx"}} or
// {"action": "disallow", "features": {"is_demo_user": true}}.
type Rule struct {
	Action   string          `json:"action"` // "allow" or "disallow"
	OS       *OSRule         `json:"os,omitempty"`
	Features map[string]bool `json:"features,omitempty"`
}

type OSRule struct {
	Name    string `json:"name,omitempty"`    // "windows" | "osx" | "linux"
	Version string `json:"version,omitempty"` // regex matched against os.Version
	Arch    string `json:"arch,omitempty"`    // "x86" | "arm64" | ...
}

// FetchVersionJSON downloads and parses the per-version JSON referenced by
// a manifest entry (or by a loader's own version URL).
func FetchVersionJSON(url string) (Version, []byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Version{}, nil, fmt.Errorf("fetching version json: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Version{}, nil, fmt.Errorf("fetching version json: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Version{}, nil, err
	}
	var v Version
	if err := json.Unmarshal(body, &v); err != nil {
		return Version{}, nil, fmt.Errorf("parsing version json: %w", err)
	}
	return v, body, nil
}

// LoadVersionJSON reads a previously-cached version JSON from disk.
func LoadVersionJSON(path string) (Version, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Version{}, err
	}
	var v Version
	if err := json.Unmarshal(data, &v); err != nil {
		return Version{}, err
	}
	return v, nil
}
