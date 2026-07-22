// Package javafind locates, verifies, and selects Java installations.
//
// Design follows a 4-step pipeline, matching the project's requirements
// exactly so each step is independently testable:
//
//  1. RequiredMajor  — read from the version JSON's javaVersion.majorVersion
//     (see internal/version), not guessed from the MC version number.
//  2. Candidates      — enumerate possible java binaries in priority order.
//  3. Verify          — actually run each candidate and parse its real
//     version from `-XshowSettings:properties -version` output; never
//     trust folder/file names.
//  4. Best            — pick the best verified candidate for a required
//     major version.
package javafind

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Candidate is a possible java executable, before verification.
type Candidate struct {
	Path   string
	Source string // where we found it: "override", "JAVA_HOME", "PATH", "well-known:<dir>"
}

// Verified is a Candidate we've actually run and confirmed.
type Verified struct {
	Candidate
	MajorVersion int
	RawVersion   string // e.g. "21.0.3"
}

// Candidates returns possible java binaries in priority order:
// explicit override > JAVA_HOME > PATH > OS well-known install locations.
// Later steps de-duplicate by resolved absolute path.
func Candidates(overridePath string) []Candidate {
	var out []Candidate
	seen := map[string]bool{}

	add := func(path, source string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, Candidate{Path: path, Source: source})
	}

	if overridePath != "" {
		add(overridePath, "override")
	}
	if home := os.Getenv("JAVA_HOME"); home != "" {
		add(javaBinIn(home), "JAVA_HOME")
	}
	if p, err := exec.LookPath(javaExeName()); err == nil {
		add(p, "PATH")
	}
	for _, dir := range wellKnownJDKRoots() {
		for _, home := range expandJDKHomes(dir) {
			add(javaBinIn(home), "well-known:"+dir)
		}
	}
	for _, home := range macOSJavaHomes() {
		add(javaBinIn(home), "well-known:/usr/libexec/java_home")
	}

	return out
}

func javaExeName() string {
	if runtime.GOOS == "windows" {
		return "java.exe"
	}
	return "java"
}

func javaBinIn(jdkHome string) string {
	return filepath.Join(jdkHome, "bin", javaExeName())
}

// wellKnownJDKRoots returns directories that, when globbed one level deep,
// contain JDK install directories (i.e. each child is a JDK home).
func wellKnownJDKRoots() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		// NOTE: the spec also calls for checking
		// HKLM\SOFTWARE\JavaSoft\JDK / JRE in the registry. That requires
		// golang.org/x/sys/windows/registry (a Windows-only build), which
		// this dev sandbox can't fetch to test against (no Windows host
		// available anyway). The folder scan below covers the common
		// installer layouts; registry scanning is a documented follow-up
		// for real Windows testing — see README "Known Gaps".
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		return []string{
			filepath.Join(pf, "Java"),
			filepath.Join(pf, "Eclipse Adoptium"),
			filepath.Join(pf, "Zulu"),
			filepath.Join(pf, "Amazon Corretto"),
		}
	case "darwin":
		return []string{
			"/Library/Java/JavaVirtualMachines", // children need /Contents/Home appended
		}
	default: // linux and other unix-likes
		roots := []string{"/usr/lib/jvm", "/opt/java"}
		if home != "" {
			roots = append(roots,
				filepath.Join(home, ".sdkman/candidates/java"),
				filepath.Join(home, ".jabba/jdk"),
			)
		}
		return roots
	}
}

// expandJDKHomes globs one level under root, returning each child as a
// JDK home path. On macOS, /Contents/Home is appended per Apple's bundle
// layout.
func expandJDKHomes(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(root, e.Name())
		if runtime.GOOS == "darwin" {
			child = filepath.Join(child, "Contents", "Home")
		}
		out = append(out, child)
	}
	return out
}

// macOSJavaHomes runs `/usr/libexec/java_home -V` and parses its stderr
// output, which is the canonical way macOS enumerates installed JDKs.
// Example line:
//
//	21.0.3 (arm64) "Eclipse Temurin 21" /Library/Java/JavaVirtualMachines/temurin-21.jdk/Contents/Home
func macOSJavaHomes() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	cmd := exec.Command("/usr/libexec/java_home", "-V")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	_ = cmd.Run() // this command exits non-zero when it also prints the list; ignore the error

	var out []string
	scanner := bufio.NewScanner(strings.NewReader(stderr.String()))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Matching Java Virtual Machines") {
			continue
		}
		// Last whitespace-separated field on the line is the path.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		last := fields[len(fields)-1]
		if strings.HasPrefix(last, "/") {
			out = append(out, last)
		}
	}
	return out
}

// Verify actually runs the candidate's java binary and parses its real
// version, so we never trust folder names. Returns an error if the binary
// doesn't exist or doesn't run.
func Verify(c Candidate) (Verified, error) {
	cmd := exec.Command(c.Path, "-XshowSettings:properties", "-version")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out // -XshowSettings prints to stderr on most JVMs
	if err := cmd.Run(); err != nil {
		return Verified{}, fmt.Errorf("running %s: %w", c.Path, err)
	}

	specVersion := parseProperty(out.String(), "java.specification.version")
	fullVersion := parseProperty(out.String(), "java.version")
	if specVersion == "" && fullVersion == "" {
		return Verified{}, fmt.Errorf("could not parse java version output from %s", c.Path)
	}

	major, err := majorFromVersionString(firstNonEmpty(specVersion, fullVersion))
	if err != nil {
		return Verified{}, err
	}

	return Verified{Candidate: c, MajorVersion: major, RawVersion: firstNonEmpty(fullVersion, specVersion)}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// parseProperty extracts `key = value` from -XshowSettings:properties
// output, e.g. "    java.version = 21.0.3".
func parseProperty(output, key string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, key) {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != key {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// majorFromVersionString handles both the old "1.8.0_392" scheme (major
// version is the second component) and the modern "17", "21.0.3" scheme
// (major version is the first component).
func majorFromVersionString(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("empty version string")
	}
	parts := strings.Split(v, ".")
	if parts[0] == "1" && len(parts) > 1 {
		// Legacy scheme: 1.8 -> Java 8.
		n, err := strconv.Atoi(strings.SplitN(parts[1], "_", 2)[0])
		if err != nil {
			return 0, fmt.Errorf("parsing legacy version %q: %w", v, err)
		}
		return n, nil
	}
	n, err := strconv.Atoi(strings.SplitN(parts[0], "_", 2)[0])
	if err != nil {
		return 0, fmt.Errorf("parsing version %q: %w", v, err)
	}
	return n, nil
}

// Best picks the closest verified candidate for a required major version:
// exact match preferred; otherwise the closest version ABOVE the
// requirement (never below — running a Java-21-required version on an
// older JVM hard-fails, whereas newer JVMs are generally backward
// compatible for older versions). Returns ok=false if nothing suitable
// exists.
func Best(candidates []Verified, required int) (Verified, bool) {
	var exact *Verified
	var aboveBest *Verified

	sorted := make([]Verified, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MajorVersion < sorted[j].MajorVersion })

	for i := range sorted {
		v := sorted[i]
		switch {
		case v.MajorVersion == required:
			exact = &v
		case v.MajorVersion > required:
			if aboveBest == nil || v.MajorVersion < aboveBest.MajorVersion {
				aboveBest = &v
			}
		}
	}

	if exact != nil {
		return *exact, true
	}
	if aboveBest != nil {
		return *aboveBest, true
	}
	return Verified{}, false
}

// DescribeAvailable renders a short human list of what majors were found,
// for error messages like "found Java 8 and Java 17".
func DescribeAvailable(candidates []Verified) string {
	if len(candidates) == 0 {
		return "no Java installations found"
	}
	majors := map[int]bool{}
	for _, c := range candidates {
		majors[c.MajorVersion] = true
	}
	list := make([]int, 0, len(majors))
	for m := range majors {
		list = append(list, m)
	}
	sort.Ints(list)
	parts := make([]string, len(list))
	for i, m := range list {
		parts[i] = fmt.Sprintf("Java %d", m)
	}
	return strings.Join(parts, ", ")
}
