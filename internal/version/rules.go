package version

import (
	"regexp"
	"runtime"
	"strings"
)

// Env is the set of external facts argument/library rules can be
// conditioned on. Passing it explicitly (instead of reading runtime.GOOS
// directly inside the resolver) keeps the resolver pure and unit-testable
// for every OS from a single test binary.
type Env struct {
	OSName              string // "windows" | "osx" | "linux"
	OSVersion           string // e.g. "10.0", matched as a regex against Rule.OS.Version
	Arch                string // "x86" | "arm64" | ...
	IsDemoUser          bool
	HasCustomResolution bool
}

// CurrentEnv builds an Env for the machine actually running alloyctl.
func CurrentEnv() Env {
	return Env{
		OSName: mojangOSName(runtime.GOOS),
		Arch:   mojangArch(runtime.GOARCH),
	}
}

func mojangOSName(goos string) string {
	switch goos {
	case "darwin":
		return "osx"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

func mojangArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86" // Mojang's manifest uses "x86" for the arch field even on 64-bit; arch rules in practice key off word size via other means, but we pass this through for completeness
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return goarch
	}
}

// RuleApplies evaluates whether a single Rule's conditions match env. It
// does NOT consider Action — callers combine RuleApplies across a full
// rule list via RulesAllow.
func RuleApplies(r Rule, env Env) bool {
	if r.OS != nil {
		if r.OS.Name != "" && r.OS.Name != env.OSName {
			return false
		}
		if r.OS.Arch != "" && r.OS.Arch != env.Arch {
			return false
		}
		if r.OS.Version != "" {
			matched, err := regexp.MatchString(r.OS.Version, env.OSVersion)
			if err != nil || !matched {
				return false
			}
		}
	}
	for feature, want := range r.Features {
		var have bool
		switch feature {
		case "is_demo_user":
			have = env.IsDemoUser
		case "has_custom_resolution":
			have = env.HasCustomResolution
		default:
			// Unknown feature flags (newer Mojang additions we don't model
			// yet) are treated as "not present", matching vanilla launcher
			// behavior of not enabling features it doesn't understand.
			have = false
		}
		if have != want {
			return false
		}
	}
	return true
}

// RulesAllow implements Mojang's documented semantics: an empty rule list
// means "always allow". Otherwise, evaluate rules in order; the LAST rule
// whose conditions match determines the outcome (allow/disallow). If no
// rule matches, the entry is disallowed.
//
// This is the exact algorithm the vanilla launcher uses, and it's also
// what makes rule lists like:
//
//	[{"action":"allow"}, {"action":"disallow","os":{"name":"osx"}}]
//
// mean "allow everywhere except macOS" rather than something else — a
// naive "any matching disallow wins" or "any matching allow wins"
// implementation gets multi-rule lists like this wrong.
func RulesAllow(rules []Rule, env Env) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		if RuleApplies(r, env) {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}

// ResolveArguments expands an ArgumentEntry list into flat argument
// strings, dropping entries whose rules don't allow env and substituting
// ${placeholder} tokens using subs.
func ResolveArguments(entries []ArgumentEntry, env Env, subs map[string]string) []string {
	var out []string
	for _, e := range entries {
		if !RulesAllow(e.Rules, env) {
			continue
		}
		for _, v := range e.Value {
			out = append(out, substitute(v, subs))
		}
	}
	return out
}

func substitute(s string, subs map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	for k, v := range subs {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

// ResolveLibraries filters a library list down to those whose rules allow
// env — this is how OS-specific libraries (e.g. LWJGL natives for a
// platform you're not on) get excluded from the classpath/download plan.
func ResolveLibraries(libs []Library, env Env) []Library {
	out := make([]Library, 0, len(libs))
	for _, l := range libs {
		if RulesAllow(l.Rules, env) {
			out = append(out, l)
		}
	}
	return out
}

// NativesClassifier returns the classifier key (e.g. "natives-linux") to
// use for this library's natives on env, and whether this library has any
// natives at all under the old-style `natives` map format.
func NativesClassifier(l Library, env Env) (string, bool) {
	if l.Natives == nil {
		return "", false
	}
	classifier, ok := l.Natives[env.OSName]
	if !ok {
		return "", false
	}
	// Old-style natives classifiers sometimes contain an "${arch}" token
	// (historically "32"/"64").
	arch := "64"
	if env.Arch == "x86" {
		arch = "32"
	}
	classifier = strings.ReplaceAll(classifier, "${arch}", arch)
	return classifier, true
}

// LegacyArguments splits pre-1.13 `minecraftArguments` (a single
// space-separated string with ${placeholder} tokens, no rules) into
// substituted argument strings. There's no equivalent legacy JVM argument
// string — callers must synthesize standard JVM args themselves for old
// versions (see internal/launcher).
func LegacyArguments(raw string, subs map[string]string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Fields(raw)
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = substitute(p, subs)
	}
	return out
}
