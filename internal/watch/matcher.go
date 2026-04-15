package watch

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher answers "which hooks should fire for this path?". Stateless;
// safe for concurrent use. Construct once at watch startup, reuse across all
// events.
type Matcher struct {
	hooks           []Hook
	caseInsensitive bool
}

// NewMatcher captures the hook list and case-sensitivity. Does NOT validate
// patterns here — malformed doublestar patterns produce "no match" silently
// at event time. WatchCfg.Validate should catch obvious issues; runtime
// mismatch is preferable to a crash in the watcher hot loop.
func NewMatcher(hooks []Hook, caseInsensitive bool) *Matcher {
	return &Matcher{hooks: hooks, caseInsensitive: caseInsensitive}
}

// Match returns pointers to every hook whose `Paths` patterns accept the
// given path. The returned slice shares hook storage with the matcher, so
// callers must not mutate individual hooks. Empty slice (never nil) when no
// match.
//
// `path` should be project-relative, forward-slash. Match normalizes
// defensively (filepath.ToSlash + lowercase when case-insensitive) so callers
// can pass raw fsnotify paths.
func (m *Matcher) Match(path string) []*Hook {
	if m == nil || len(m.hooks) == 0 {
		return nil
	}
	target := filepath.ToSlash(path)
	if m.caseInsensitive {
		target = strings.ToLower(target)
	}
	out := make([]*Hook, 0, 2)
	for i := range m.hooks {
		h := &m.hooks[i]
		if hookMatches(h, target, m.caseInsensitive) {
			out = append(out, h)
		}
	}
	return out
}

// hookMatches is the inner loop of Match, extracted so tests can target it
// without constructing a full Matcher.
func hookMatches(h *Hook, target string, caseInsensitive bool) bool {
	for _, pat := range h.Paths {
		p := pat
		if caseInsensitive {
			p = strings.ToLower(p)
		}
		if ok, _ := doublestar.Match(p, target); ok {
			return true
		}
	}
	return false
}
