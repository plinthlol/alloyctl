package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"alloy/internal/cli"
	"alloy/internal/config"
	"alloy/internal/download"
	"alloy/internal/instance"
	"alloy/internal/javafind"
	"alloy/internal/loader/fabric"
	"alloy/internal/loader/forge"
	"alloy/internal/loader/neoforge"
	"alloy/internal/loader/quilt"
	"alloy/internal/version"
)

func cmdInstall(paths config.Paths, args []string) error {
	// Parse flags manually to allow them anywhere in the argument list
	var fabricFlag, quiltFlag, forgeFlag, neoforgeFlag bool
	var nameFlag string
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--fabric":
			fabricFlag = true
		case "--quilt":
			quiltFlag = true
		case "--forge":
			forgeFlag = true
		case "--neoforge":
			neoforgeFlag = true
		case "--name":
			if i+1 < len(args) {
				i++
				nameFlag = args[i]
			}
		default:
			positional = append(positional, args[i])
		}
	}

	loaderCount := boolCount(fabricFlag, quiltFlag, forgeFlag, neoforgeFlag)
	if loaderCount > 1 {
		return fmt.Errorf("only one of --fabric/--quilt/--forge/--neoforge may be given")
	}
	loaderName := ""
	switch {
	case fabricFlag:
		loaderName = "fabric"
	case quiltFlag:
		loaderName = "quilt"
	case forgeFlag:
		loaderName = "forge"
	case neoforgeFlag:
		loaderName = "neoforge"
	}

	manifest, err := version.LoadOrFetchManifest(paths.ManifestCacheFile())
	if err != nil {
		return fmt.Errorf("loading version manifest: %w", err)
	}

	var mcVersion string
	if len(positional) == 0 {
		mcVersion, err = browseAndPickVersion(manifest)
		if err != nil {
			return err
		}
	} else {
		mcVersion = positional[0]
	}

	entry, ok := manifest.Find(mcVersion)
	if !ok {
		return fmt.Errorf("no such Minecraft version %q (run `alloyctl install` with no args to browse)", mcVersion)
	}

	instName := nameFlag
	if instName == "" {
		instName = instance.DefaultName(mcVersion, loaderName)
	}
	if instance.Exists(paths, instName) {
		return fmt.Errorf(
			"instance %q already exists. Rename it first with `alloyctl rename %s <new-name>`, or install this one under a different name with --name",
			instName, instName,
		)
	}

	cli.Info(fmt.Sprintf("Installing %s%s as instance %q...", mcVersion, loaderSuffix(loaderName), instName))

	base, _, err := version.FetchVersionJSON(entry.URL)
	if err != nil {
		return fmt.Errorf("fetching version json: %w", err)
	}

	resolved := base
	loaderVersion := ""
	switch loaderName {
	case "fabric":
		versions, err := fabric.ListLoaderVersions(mcVersion)
		if err != nil {
			return err
		}
		loaderVersion, err = fabric.LatestStable(versions)
		if err != nil {
			return err
		}
		resolved, err = fabric.FetchProfile(mcVersion, loaderVersion, base)
		if err != nil {
			return err
		}
	case "quilt":
		versions, err := quilt.ListLoaderVersions(mcVersion)
		if err != nil {
			return err
		}
		loaderVersion, err = quilt.Latest(versions)
		if err != nil {
			return err
		}
		resolved, err = quilt.FetchProfile(mcVersion, loaderVersion, base)
		if err != nil {
			return err
		}
	case "forge":
		forgeVer, err := forge.GetLatestVersion(mcVersion)
		if err != nil {
			return err
		}
		loaderVersion = forgeVer
		mcForgeVersion := mcVersion + "-" + forgeVer
		cli.Info(fmt.Sprintf("Downloading Forge installer (%s)...", mcForgeVersion))
		installerPath := filepath.Join(os.TempDir(), fmt.Sprintf("forge-%s-installer.jar", mcForgeVersion))
		if err := forge.DownloadInstaller(mcForgeVersion, "", installerPath); err != nil {
			return fmt.Errorf("downloading forge installer: %w", err)
		}
		defer os.Remove(installerPath)

		g, _ := config.Load(paths)
		candidates := javafind.Candidates(g.JavaPath)
		var verified []javafind.Verified
		for _, c := range candidates {
			if v, err := javafind.Verify(c); err == nil {
				verified = append(verified, v)
			}
		}
		bestJava, ok := javafind.Best(verified, 8)
		if !ok {
			return fmt.Errorf("Forge installer requires Java 8+, but no Java runtime was found")
		}

		cacheKey := cacheKeyFor(mcVersion, loaderName, loaderVersion)
		cacheDir := paths.VersionCacheDir(cacheKey)

		cli.Info(fmt.Sprintf("Running Forge installer in headless mode..."))
		if err := forge.RunInstallerHeadless(bestJava.Path, installerPath, cacheDir); err != nil {
			return err
		}
	case "neoforge":
		loaderVersion = mcVersion
		cli.Info(fmt.Sprintf("Downloading NeoForge installer (%s)...", mcVersion))
		installerPath := filepath.Join(os.TempDir(), fmt.Sprintf("neoforge-%s-installer.jar", mcVersion))
		if err := neoforge.DownloadInstaller(mcVersion, "", installerPath); err != nil {
			return fmt.Errorf("downloading neoforge installer: %w", err)
		}
		defer os.Remove(installerPath)

		g, _ := config.Load(paths)
		candidates := javafind.Candidates(g.JavaPath)
		var verified []javafind.Verified
		for _, c := range candidates {
			if v, err := javafind.Verify(c); err == nil {
				verified = append(verified, v)
			}
		}
		bestJava, ok := javafind.Best(verified, 17)
		if !ok {
			return fmt.Errorf("NeoForge installer requires Java 17+, but no Java runtime was found")
		}

		cacheKey := cacheKeyFor(mcVersion, loaderName, loaderVersion)
		cacheDir := paths.VersionCacheDir(cacheKey)

		cli.Info(fmt.Sprintf("Running NeoForge installer in headless mode..."))
		if err := neoforge.RunInstallerHeadless(bestJava.Path, installerPath, cacheDir); err != nil {
			return err
		}
	}

	cacheKey := cacheKeyFor(mcVersion, loaderName, loaderVersion)
	cacheDir := paths.VersionCacheDir(cacheKey)

	// Phase 1: client jar, libraries, and the asset *index* itself (we
	// need the index on disk before we can know what asset objects exist).
	phase1 := buildDownloadTasks(resolved, cacheDir, paths.AssetsDir())
	cli.Info(fmt.Sprintf("Downloading %d files (client jar + libraries)...", len(phase1)))
	if err := runDownloads(phase1); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Phase 2: expand the now-downloaded asset index into per-object
	// download tasks and fetch those too, so `install` leaves everything
	// needed to launch on disk, not just the index describing it.
	if resolved.AssetIndex.URL != "" {
		indexPath := filepath.Join(paths.AssetsDir(), "indexes", resolved.AssetIndex.ID+".json")
		assetTasks, err := assetObjectTasks(indexPath, paths.AssetsDir())
		if err != nil {
			return fmt.Errorf("expanding asset index: %w", err)
		}
		cli.Info(fmt.Sprintf("Downloading %d asset objects...", len(assetTasks)))
		if err := runDownloads(assetTasks); err != nil {
			return fmt.Errorf("asset download failed: %w", err)
		}
	}

	if err := saveResolvedVersionJSON(resolved, cacheDir); err != nil {
		return fmt.Errorf("caching resolved version definition: %w", err)
	}

	if err := instance.EnsureDataDir(paths, instName); err != nil {
		return err
	}
	meta := instance.Meta{
		Name:          instName,
		MCVersion:     mcVersion,
		Loader:        loaderName,
		LoaderVersion: loaderVersion,
	}
	if err := instance.Save(paths, meta); err != nil {
		return err
	}

	cli.Info(fmt.Sprintf("Instance %q installed. Run `alloyctl play %s` to launch it.", instName, instName))
	return nil
}

func boolCount(bs ...bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}

func loaderSuffix(loader string) string {
	if loader == "" {
		return " (vanilla)"
	}
	return " + " + loader
}

func cacheKeyFor(mcVersion, loader, loaderVersion string) string {
	if loader == "" {
		return mcVersion + "-vanilla"
	}
	return mcVersion + "-" + loader + "-" + loaderVersion
}

// browseAndPickVersion implements `alloyctl install` with no version arg:
// print every version (oldest at top, latest at the bottom), colored by
// type, then prompt which to install.
func browseAndPickVersion(m version.Manifest) (string, error) {
	ordered := m.OldestFirst()
	labels := make([]string, len(ordered))
	for i, v := range ordered {
		label := fmt.Sprintf("%s [%s]", v.ID, v.Type)
		labels[i] = cli.ColorForVersionType(v.Type, label)
	}
	idx, err := cli.PromptSelectIndex("Which version would you like to install?", labels)
	if err != nil {
		return "", err
	}
	return ordered[idx].ID, nil
}

// buildDownloadTasks turns a resolved version into the flat list of files
// to fetch: client jar, resolved libraries (respecting OS/arch rules),
// and asset objects from the assets index.
func buildDownloadTasks(v version.Version, cacheDir, assetsDir string) []download.Task {
	var tasks []download.Task

	tasks = append(tasks, download.Task{
		URL:  v.Downloads.Client.URL,
		Dest: filepath.Join(cacheDir, "client.jar"),
		SHA1: v.Downloads.Client.SHA1,
		Size: v.Downloads.Client.Size,
	})

	env := version.CurrentEnv()
	for _, lib := range version.ResolveLibraries(v.Libraries, env) {
		if art, ok := lib.ArtifactInfo(); ok {
			tasks = append(tasks, download.Task{
				URL:  art.URL,
				Dest: filepath.Join(cacheDir, "libraries", filepath.FromSlash(art.Path)),
				SHA1: art.SHA1,
				Size: art.Size,
			})
		}
		if classifier, ok := version.NativesClassifier(lib, env); ok {
			if art, ok := lib.Downloads.Classifiers[classifier]; ok {
				tasks = append(tasks, download.Task{
					URL:  art.URL,
					Dest: filepath.Join(cacheDir, "natives", filepath.Base(art.Path)),
					SHA1: art.SHA1,
					Size: art.Size,
				})
			}
		}
	}

	// The asset index itself (a small JSON manifest of {path -> hash,size}
	// for every asset object). The individual asset *objects* it
	// references can only be enumerated once this index is on disk, so
	// those are fetched as a second phase — see assetObjectTasks below,
	// called from cmdInstall after this first batch completes.
	if v.AssetIndex.URL != "" {
		tasks = append(tasks, download.Task{
			URL:  v.AssetIndex.URL,
			Dest: filepath.Join(assetsDir, "indexes", v.AssetIndex.ID+".json"),
			SHA1: v.AssetIndex.SHA1,
			Size: v.AssetIndex.Size,
		})
	}

	return tasks
}

// assetObjectTasks parses a downloaded asset index and returns a download
// Task for every object it references, laid out the same way vanilla does
// — assets/objects/<hash[:2]>/<hash> — so this shared assetsDir can be
// reused unmodified across every instance/version, exactly like Mojang's
// own launcher does (assets are content-addressed, not per-version).
func assetObjectTasks(indexPath, assetsDir string) ([]download.Task, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("reading asset index %s: %w", indexPath, err)
	}

	var idx struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
			Size int64  `json:"size"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing asset index %s: %w", indexPath, err)
	}

	tasks := make([]download.Task, 0, len(idx.Objects))
	for _, obj := range idx.Objects {
		if len(obj.Hash) < 2 {
			continue
		}
		prefix := obj.Hash[:2]
		tasks = append(tasks, download.Task{
			URL:  "https://resources.download.minecraft.net/" + prefix + "/" + obj.Hash,
			Dest: filepath.Join(assetsDir, "objects", prefix, obj.Hash),
			SHA1: obj.Hash, // asset object hashes are themselves sha1
			Size: obj.Size,
		})
	}
	return tasks, nil
}

// saveResolvedVersionJSON writes the final, already-merged version
// definition (vanilla, or vanilla+loader after FetchProfile) to the
// shared version cache dir as version.json, so `play` never needs to
// re-resolve loader profiles at launch time — install is the only place
// that talks to Mojang/Fabric/Quilt metadata APIs.
func saveResolvedVersionJSON(v version.Version, cacheDir string) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cacheDir, "version.json"), data, 0o644)
}

// shared progress-line renderer, used by both download phases in
// cmdInstall.
func runDownloads(tasks []download.Task) error {
	_, err := download.Run(tasks, download.Options{
		Workers: 16,
		Progress: func(done, total int, r download.Result) {
			label := filepath.Base(r.Task.Dest)
			cli.ProgressLine(done, total, label, r.Skipped, r.Err != nil)
		},
	})
	return err
}
