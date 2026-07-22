// Package forge implements Forge loader support.
//
// ⚠️ THIS IS THE HARDEST LOADER, AND THIS IS A DELIBERATELY SCOPED-DOWN
// V1. Forge does not publish a clean "merge this JSON onto vanilla" API
// like Fabric/Quilt. Instead it publishes a per-version installer jar
// (from Forge's Maven, promotions_slim.json for version listing) that:
//
//   - contains install_profile.json (and, for modern Forge, a nested
//     version.json) describing extra libraries and a chain of
//     "processors" — post-install steps that run external jars (e.g.
//     binary patchers) to produce the final patched Forge client jar
//   - for older Forge (roughly pre-1.13), does its own client-side
//     binary patching differently again
//
// Replicating the processor pipeline faithfully (arguments, classpath,
// and jar-in-jar tool invocation) is a substantial, version-dependent
// reverse-engineering effort, and Forge's own installer format has
// changed multiple times across MC's lifetime.
//
// V1 SHORTCUT (implemented here): download the official installer jar
// and shell out to it in headless client-install mode:
//
//	java -jar forge-<mc>-<forge>-installer.jar --installClient <target-dir>
//
// This delegates all of the above complexity to Forge's own installer,
// at the cost of requiring a JVM to be available *before* our own
// Java-detection flow even runs the game (in practice this is fine: any
// Java 8+ can run the installer even if the target MC version needs a
// newer Java for actual gameplay).
//
// TODO for a v2: parse install_profile.json / version.json out of the
// installer jar and replicate the processor pipeline ourselves, so
// `install` doesn't need to shell out to a downloaded jar. This needs
// real-world testing against installer jars from several different
// Forge eras (they are NOT all the same format) before it can be trusted
// — flagging explicitly per the project brief.
package forge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"alloy/internal/download"
)

const mavenBase = "https://maven.minecraftforge.net/net/minecraftforge/forge"

// InstallerURL returns the maven URL for a given "<mcVersion>-<forgeVersion>"
// combo, e.g. "1.20.4-49.0.31". Callers get exact combo strings from
// Forge's promotions_slim.json (not implemented here yet — see TODO
// below); this function just knows the URL shape once you have one.
func InstallerURL(mcForgeVersion string) string {
	return fmt.Sprintf("%s/%s/forge-%s-installer.jar", mavenBase, mcForgeVersion, mcForgeVersion)
}

const PromotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"

type Promotions struct {
	Promos map[string]string `json:"promos"`
}

// FetchPromotions downloads and parses Forge's promotions_slim.json.
func FetchPromotions() (Promotions, error) {
	resp, err := http.Get(PromotionsURL)
	if err != nil {
		return Promotions{}, fmt.Errorf("fetching forge promotions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Promotions{}, fmt.Errorf("fetching forge promotions: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Promotions{}, err
	}
	var p Promotions
	if err := json.Unmarshal(body, &p); err != nil {
		return Promotions{}, fmt.Errorf("parsing forge promotions: %w", err)
	}
	return p, nil
}

// GetLatestVersion returns the recommended or latest Forge version string for a given MC version.
func GetLatestVersion(mcVersion string) (string, error) {
	p, err := FetchPromotions()
	if err != nil {
		return "", err
	}
	if v, ok := p.Promos[mcVersion+"-recommended"]; ok {
		return v, nil
	}
	if v, ok := p.Promos[mcVersion+"-latest"]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no Forge loader version found for Minecraft %s", mcVersion)
}

// DownloadInstaller fetches the installer jar to destPath, verifying sha1
// if provided (Forge's maven-metadata.xml / promotions include hashes;
// pass "" to skip verification when unavailable).
func DownloadInstaller(mcForgeVersion, sha1, destPath string) error {
	_, err := download.Run([]download.Task{{
		URL:  InstallerURL(mcForgeVersion),
		Dest: destPath,
		SHA1: sha1,
	}}, download.Options{Workers: 1})
	return err
}

// RunInstallerHeadless shells out to the installer jar's headless client
// install mode. javaPath should be any working Java 8+ binary — the
// installer itself doesn't need the target MC version's required Java,
// only whatever's needed to run the installer's own (typically old)
// bundled libraries.
func RunInstallerHeadless(javaPath, installerJarPath, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(javaPath, "-jar", installerJarPath, "--installClient", targetDir)
	cmd.Dir = filepath.Dir(installerJarPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("forge installer failed (this loader is the least automated — see internal/loader/forge doc comment): %w", err)
	}
	return nil
}
