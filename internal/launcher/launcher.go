// Package launcher builds the final `java` command line (classpath, JVM
// args, main class, game args) from a resolved version.Version plus
// instance/account/Java details, and runs the game process, streaming its
// stdout/stderr back to the launcher's own terminal.
package launcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"alloy/internal/version"
)

// Plan is everything needed to build and run a launch command.
type Plan struct {
	Version      version.Version
	JavaPath     string
	GameDir      string // instance's mutable game directory
	AssetsDir    string // shared assets root
	NativesDir   string // per-launch temp dir with extracted native libs
	LibrariesDir string // shared library cache root (jars laid out by maven path)
	ClientJar    string // path to the downloaded client jar

	// Account/session details.
	Username    string
	UUID        string
	AccessToken string // "0" for offline accounts — vanilla accepts this
	UserType    string // "legacy" (offline) or "msa" (online)

	MemoryMB int
	ExtraJVM []string

	Width, Height int // 0 = don't pass a fixed resolution
}

// Env returns the version.Env for the current machine — the same
// resolver used for both libraries and arguments so a version's declared
// rules only need to be evaluated once, consistently.
func (p Plan) env() version.Env {
	return version.CurrentEnv()
}

// substitutions builds the ${...} placeholder map used by both the
// modern arguments.* format and the legacy minecraftArguments string.
func (p Plan) substitutions(classpath string) map[string]string {
	width := "854"
	height := "480"
	if p.Width > 0 {
		width = fmt.Sprint(p.Width)
	}
	if p.Height > 0 {
		height = fmt.Sprint(p.Height)
	}
	return map[string]string{
		"auth_player_name":  p.Username,
		"version_name":      p.Version.ID,
		"game_directory":    p.GameDir,
		"assets_root":       p.AssetsDir,
		"assets_index_name": p.Version.AssetIndex.ID,
		"auth_uuid":         p.UUID,
		"auth_access_token": p.AccessToken,
		"user_type":         p.UserType,
		"version_type":      p.Version.Type,
		"natives_directory": p.NativesDir,
		"launcher_name":     "alloy",
		"launcher_version":  "0.1.0",
		"classpath":         classpath,
		"resolution_width":  width,
		"resolution_height": height,
		"clientid":          "",
		"auth_xuid":         "",
	}
}

// BuildCommand assembles the full argv for the java process (JVM args +
// main class + game args), handling both the modern rules-based
// arguments.* format and the pre-1.13 minecraftArguments string.
func (p Plan) BuildCommand() ([]string, error) {
	env := p.env()
	classpath := p.Classpath()
	subs := p.substitutions(classpath)

	var argv []string

	if p.Version.Arguments != nil {
		// Modern format (1.13+): JVM args come from the version JSON too.
		jvm := version.ResolveArguments(p.Version.Arguments.JVM, env, subs)
		argv = append(argv, jvm...)
	} else {
		// Legacy format: synthesize the standard JVM args ourselves, since
		// old version JSONs don't declare any.
		argv = append(argv, p.legacyJVMArgs(classpath)...)
	}

	argv = append(argv, p.ExtraJVM...)
	argv = append(argv, fmt.Sprintf("-Xms%dM", p.MemoryMB), fmt.Sprintf("-Xmx%dM", p.MemoryMB))
	argv = append(argv, p.Version.MainClass)

	if p.Version.Arguments != nil {
		game := version.ResolveArguments(p.Version.Arguments.Game, env, subs)
		argv = append(argv, game...)
	} else {
		game := version.LegacyArguments(p.Version.MinecraftArgs, subs)
		argv = append(argv, game...)
	}

	return argv, nil
}

func (p Plan) legacyJVMArgs(classpath string) []string {
	args := []string{
		"-Djava.library.path=" + p.NativesDir,
		"-cp", classpath,
	}
	if osIsMac() {
		args = append([]string{"-XstartOnFirstThread"}, args...)
	}
	return args
}

func osIsMac() bool {
	return version.CurrentEnv().OSName == "osx"
}

// Classpath builds the OS path-list-separated classpath string: every
// resolved (non-natives) library jar, plus the client jar last.
func (p Plan) Classpath() string {
	env := p.env()
	libs := version.ResolveLibraries(p.Version.Libraries, env)

	sep := ":"
	if env.OSName == "windows" {
		sep = ";"
	}

	var parts []string
	for _, lib := range libs {
		if art, ok := lib.ArtifactInfo(); ok {
			parts = append(parts, filepath.Join(p.LibrariesDir, filepath.FromSlash(art.Path)))
		}
	}
	parts = append(parts, p.ClientJar)
	return strings.Join(parts, sep)
}

// Launch runs the built command, streaming stdout/stderr to the current
// process's own streams, and blocks until the game exits.
func (p Plan) Launch(stdout, stderr io.Writer) error {
	argv, err := p.BuildCommand()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.GameDir, 0o755); err != nil {
		return err
	}

	cmd := exec.Command(p.JavaPath, argv...)
	cmd.Dir = p.GameDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin

	// Isolate Java into its own process group so closing the terminal (SIGHUP) doesn't kill it
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting game process: %w", err)
	}

	// Ignore SIGHUP so closing terminal window doesn't crash alloyctl/Java
	ignoreSIGHUP()

	// Catch Ctrl+C (SIGINT) and SIGTERM so we can forward them to terminate Java cleanly
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	doneChan := make(chan error, 1)
	go func() {
		doneChan <- cmd.Wait()
	}()

	select {
	case err := <-doneChan:
		return err
	case sig := <-sigChan:
		// Forward Ctrl+C (SIGINT / SIGTERM) to the Java child process
		if cmd.Process != nil {
			_ = cmd.Process.Signal(sig)
		}
		// Wait for Java process to terminate cleanly
		return <-doneChan
	}
}
