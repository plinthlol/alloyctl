package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alloy/internal/auth"
	"alloy/internal/cli"
	"alloy/internal/config"
	"alloy/internal/download"
	"alloy/internal/instance"
	"alloy/internal/javafind"
	"alloy/internal/launcher"
	"alloy/internal/version"
)

// launchFlags holds per-launch overrides parsed from CLI flags.
type launchFlags struct {
	memoryMB int
	width    int
	height   int
	jvmArgs  []string
}

func cmdPlay(paths config.Paths, args []string) error {
	if len(args) == 0 {
		return playList(paths)
	}

	// Parse per-launch flag overrides. These never modify instance.toml.
	var flags launchFlags
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--memory", "-m":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &flags.memoryMB)
			}
		case "--width":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &flags.width)
			}
		case "--height":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &flags.height)
			}
		case "--jvm", "-J":
			// Repeatable: --jvm "-Xss4m" --jvm "-Dfoo=bar"
			if i+1 < len(args) {
				i++
				flags.jvmArgs = append(flags.jvmArgs, args[i])
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) == 0 {
		return playList(paths)
	}
	return playLaunch(paths, positional[0], flags)
}

// playList lists every installed instance, colored by the underlying MC
// version's release/snapshot type, oldest at top / newest at the bottom —
// same ordering rule as `install`'s browse mode.
func playList(paths config.Paths) error {
	names, err := instance.List(paths)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		cli.Info("No instances installed yet. Run `alloyctl install <version>` to create one.")
		return nil
	}

	manifest, err := version.LoadOrFetchManifest(paths.ManifestCacheFile())
	if err != nil {
		// Listing shouldn't hard-fail just because the network/manifest
		// cache is unavailable; fall back to uncolored output.
		cli.Errorf("could not load version manifest for coloring (%v); listing without color", err)
	}

	type row struct {
		meta        instance.Meta
		releaseTime string
		vType       string
	}
	rows := make([]row, 0, len(names))
	for _, name := range names {
		m, err := instance.Load(paths, name)
		if err != nil {
			continue
		}
		vType := ""
		releaseTime := ""
		if entry, ok := manifest.Find(m.MCVersion); ok {
			vType = entry.Type
			releaseTime = entry.ReleaseTime.Format("2006-01-02T15:04:05")
		}
		rows = append(rows, row{meta: m, vType: vType, releaseTime: releaseTime})
	}

	// Oldest at top, newest at the bottom, matching install's listing rule.
	sortRows(rows, func(a, b row) bool { return a.releaseTime < b.releaseTime })

	for _, r := range rows {
		label := r.meta.Name
		if r.meta.Loader != "" {
			label += fmt.Sprintf(" (%s %s %s)", r.meta.MCVersion, r.meta.Loader, r.meta.LoaderVersion)
		} else {
			label += fmt.Sprintf(" (%s)", r.meta.MCVersion)
		}
		fmt.Println(cli.ColorForVersionType(r.vType, label))
	}
	return nil
}

// sortRows is a tiny generic insertion sort so this file doesn't need to
// pull in sort.Slice's reflection-based comparator boilerplate for one
// small, rarely-large list.
func sortRows[T any](rows []T, less func(a, b T) bool) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && less(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func playLaunch(paths config.Paths, name string, flags launchFlags) error {
	if !instance.Exists(paths, name) {
		return fmt.Errorf("no instance named %q. Run `alloyctl play` with no args to see what's available", name)
	}
	meta, err := instance.Load(paths, name)
	if err != nil {
		return err
	}

	g, err := config.Load(paths)
	if err != nil {
		return err
	}
	if g.ActiveAccount == "" {
		return fmt.Errorf("no active account. Run `alloyctl auth offline <username>` or `alloyctl auth online` first")
	}
	account, ok := g.FindAccount(g.ActiveAccount)
	if !ok {
		return fmt.Errorf("active account %q not found in config", g.ActiveAccount)
	}

	cacheDir := paths.VersionCacheDir(meta.CacheKey())
	versionJSONPath := filepath.Join(cacheDir, "version.json")
	resolved, err := loadOrRefetchVersion(meta, versionJSONPath)
	if err != nil {
		return err
	}

	// Java selection: per-instance override > global override > detected.
	override := meta.JavaPath
	if override == "" {
		override = g.JavaPath
	}
	candidates := javafind.Candidates(override)
	var verified []javafind.Verified
	for _, c := range candidates {
		if v, err := javafind.Verify(c); err == nil {
			verified = append(verified, v)
		}
	}
	best, ok := javafind.Best(verified, resolved.JavaVersion.MajorVersion)
	if !ok {
		return fmt.Errorf(
			"this version needs Java %d, found %s — install Java %d or set it manually with `alloyctl java set <path>`",
			resolved.JavaVersion.MajorVersion, javafind.DescribeAvailable(verified), resolved.JavaVersion.MajorVersion,
		)
	}

	// Memory: --memory flag > instance.toml > global default > 2048 MB
	memMB := flags.memoryMB
	if memMB == 0 {
		memMB = meta.MemoryMB
	}
	if memMB == 0 {
		memMB = g.DefaultMemoryMB
	}
	if memMB == 0 {
		memMB = 2048
	}

	accessToken := "0"
	userType := "legacy"
	if account.Type == "microsoft" {
		userType = "msa"
		msResp, err := auth.RefreshMicrosoftToken(account.RefreshToken)
		if err != nil {
			return fmt.Errorf("refreshing Microsoft login token: %w", err)
		}
		prof, err := auth.CompleteAuthChain(msResp.AccessToken, msResp.RefreshToken, msResp.ExpiresIn)
		if err != nil {
			return fmt.Errorf("authenticating Microsoft online profile: %w", err)
		}
		accessToken = prof.MinecraftToken

		// If a new refresh token was issued, persist it to config
		if prof.RefreshToken != "" && prof.RefreshToken != account.RefreshToken {
			account.RefreshToken = prof.RefreshToken
			g.UpsertAccount(account)
			_ = config.Save(paths, g)
		}
	}

	// Ensure any missing library JARs are downloaded before launching
	var missingTasks []download.Task
	env := version.CurrentEnv()
	libsDir := filepath.Join(cacheDir, "libraries")
	for _, lib := range version.ResolveLibraries(resolved.Libraries, env) {
		if art, ok := lib.ArtifactInfo(); ok {
			destPath := filepath.Join(libsDir, filepath.FromSlash(art.Path))
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				missingTasks = append(missingTasks, download.Task{
					URL:  art.URL,
					Dest: destPath,
					SHA1: art.SHA1,
					Size: art.Size,
				})
			}
		}
	}
	if len(missingTasks) > 0 {
		cli.Info(fmt.Sprintf("Downloading %d missing library files...", len(missingTasks)))
		if err := runDownloads(missingTasks); err != nil {
			return fmt.Errorf("downloading missing libraries: %w", err)
		}
	}

	// JVM args: instance.toml args first, then per-launch --jvm flags appended
	extraJVM := append(append([]string{}, meta.JVMArgs...), flags.jvmArgs...)

	plan := launcher.Plan{
		Version:      resolved,
		JavaPath:     best.Path,
		GameDir:      paths.InstanceDataDir(name),
		AssetsDir:    paths.AssetsDir(),
		NativesDir:   filepath.Join(os.TempDir(), "alloyctl-natives-"+name),
		LibrariesDir: filepath.Join(cacheDir, "libraries"),
		ClientJar:    filepath.Join(cacheDir, "client.jar"),
		Username:     account.Username,
		UUID:         account.UUID,
		AccessToken:  accessToken,
		UserType:     userType,
		MemoryMB:     memMB,
		ExtraJVM:     extraJVM,
		Width:        flags.width,
		Height:       flags.height,
	}

	cli.Info(fmt.Sprintf("Launching %q with Java %d (%s)...", name, best.MajorVersion, best.Path))
	return plan.Launch(os.Stdout, os.Stderr)
}

// loadOrRefetchVersion loads the merged version.json we should have
// written during install. Older/partial installs fall back gracefully
// with a clear error rather than a confusing missing-file panic further
// down the launch pipeline.
func loadOrRefetchVersion(meta instance.Meta, versionJSONPath string) (version.Version, error) {
	v, err := version.LoadVersionJSON(versionJSONPath)
	if err == nil {
		return v, nil
	}
	return version.Version{}, fmt.Errorf(
		"could not load cached version definition for instance %q (%v) — try reinstalling it with `alloyctl install %s%s --name %s`",
		meta.Name, err, meta.MCVersion, loaderFlagFor(meta.Loader), meta.Name,
	)
}

func loaderFlagFor(loader string) string {
	if loader == "" {
		return ""
	}
	return " --" + strings.ToLower(loader)
}
