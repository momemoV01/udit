package watch

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// defaultUnityIgnores is the built-in ignore list for Unity projects. These
// dirs/files generate enormous write churn that drowns the fsnotify buffer
// and never contain assets the agent would want to react to.
var defaultUnityIgnores = []string{
	// Auto-generated Unity dirs
	"Library/**",
	"Temp/**",
	"Logs/**",
	"MemoryCaptures/**",
	"UserSettings/**",
	"Build/**",
	"Builds/**",
	"obj/**",
	// IDE / VCS artifacts
	".git/**",
	".vs/**",
	".idea/**",
	".vscode/**",
	// Project files regenerated on every import
	"*.csproj",
	"*.sln",
	// Backup / lock files
	"*~",
	"*.tmp",
	".#*",
}

// Ignorer evaluates whether a path should be filtered from watch events.
// Build via NewIgnorer; match via Match.
type Ignorer struct {
	patterns        []string
	caseInsensitive bool
}

// NewIgnorer composes built-in Unity defaults (when defaults=true) with
// user-provided patterns. Patterns use doublestar (`**`) syntax and are
// matched against forward-slash paths. Empty/whitespace-only user patterns
// are skipped silently.
func NewIgnorer(userPatterns []string, defaults bool, caseInsensitive bool) *Ignorer {
	var ps []string
	if defaults {
		ps = append(ps, defaultUnityIgnores...)
	}
	for _, p := range userPatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Convenience: if the user writes "Foo/" they probably mean
		// "everything under Foo". Expand to "Foo/**" so they don't have
		// to remember the doublestar suffix.
		if strings.HasSuffix(p, "/") {
			p += "**"
		}
		ps = append(ps, p)
	}
	return &Ignorer{patterns: ps, caseInsensitive: caseInsensitive}
}

// Match reports whether path is covered by any ignore pattern. `path` should
// already be project-relative and forward-slash normalized; Match normalizes
// defensively anyway so callers don't silently miss matches.
//
// Match is case-insensitive when the Ignorer was built with caseInsensitive.
// On Windows this prevents "why doesn't my hook fire for Foo.CS?" confusion.
func (ig *Ignorer) Match(path string) bool {
	if ig == nil || len(ig.patterns) == 0 {
		return false
	}
	p := filepath.ToSlash(path)
	if ig.caseInsensitive {
		p = strings.ToLower(p)
	}
	for _, pat := range ig.patterns {
		target := pat
		if ig.caseInsensitive {
			target = strings.ToLower(target)
		}
		// doublestar.Match returns (matched, err). Err is only non-nil for
		// malformed patterns — we surface those at config-validate time,
		// so treat here as "no match" rather than aborting a hot loop.
		if ok, _ := doublestar.Match(target, p); ok {
			return true
		}
	}
	return false
}

// Patterns exposes the composed pattern list for logging / debugging.
func (ig *Ignorer) Patterns() []string {
	out := make([]string, len(ig.patterns))
	copy(out, ig.patterns)
	return out
}
