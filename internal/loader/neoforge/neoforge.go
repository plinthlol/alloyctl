// Package neoforge implements NeoForge loader support. NeoForge forked
// from Forge's installer system, so this reuses the exact same v1
// shortcut strategy as internal/loader/forge (download + invoke the
// official installer jar in headless mode) — see the doc comment on that
// package for the full rationale and the v2 TODO. The same caveat about
// needing real-world testing against real installer jars applies here.
package neoforge

import (
	"fmt"

	"alloy/internal/download"
	"alloy/internal/loader/forge"
)

const mavenBase = "https://maven.neoforged.net/releases/net/neoforged/neoforge"

// InstallerURL returns the maven URL for a given NeoForge version string
// (NeoForge's own versioning, decoupled from the MC version number as of
// the 1.20.2+ NeoForge era — e.g. "20.4.190").
func InstallerURL(neoForgeVersion string) string {
	return fmt.Sprintf("%s/%s/neoforge-%s-installer.jar", mavenBase, neoForgeVersion, neoForgeVersion)
}

// DownloadInstaller and RunInstallerHeadless are identical in spirit to
// Forge's; re-exported thin wrappers so callers don't need to import both
// packages just to get the shared installer-invocation logic.
func DownloadInstaller(neoForgeVersion, sha1, destPath string) error {
	_, err := download.Run([]download.Task{{
		URL:  InstallerURL(neoForgeVersion),
		Dest: destPath,
		SHA1: sha1,
	}}, download.Options{Workers: 1})
	return err
}

func RunInstallerHeadless(javaPath, installerJarPath, targetDir string) error {
	return forge.RunInstallerHeadless(javaPath, installerJarPath, targetDir)
}
