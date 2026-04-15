package watch

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Event is the minimal shape of a file-system change that the expander needs.
// Matches what the watcher produces after debounce collapse.
type Event struct {
	// Path is the absolute, forward-slash path of the changed file.
	Path string

	// Kind is one of "create", "write", "remove", "rename".
	Kind string
}

// Batch is the debounced unit the runner dispatches on: one hook, one or
// more files, collapsed from N raw fsnotify events.
type Batch struct {
	// HookName is the matched hook's Name; fed into $HOOK.
	HookName string

	// Files is the deduped set of events in dispatch order (sorted by Path
	// for determinism — matters because $FILE per-file mode fans out in
	// this order, so reproducibility helps diagnostic comparisons).
	Files []Event

	// ProjectRoot is the Unity project root (absolute) used to compute
	// $RELFILE / $RELFILES. If empty, $RELFILE falls back to $FILE.
	ProjectRoot string
}

// hasPerFileVar reports whether `run` contains a per-file variable token —
// either $FILE or $RELFILE. Their presence triggers per-file fan-out (one
// invocation per event).
func hasPerFileVar(run string) bool {
	return containsToken(run, "$FILE") || containsToken(run, "$RELFILE")
}

// hasBatchVar reports whether `run` contains a batch variable — $FILES or
// $RELFILES. Their presence triggers single invocation + env-var injection.
func hasBatchVar(run string) bool {
	return containsToken(run, "$FILES") || containsToken(run, "$RELFILES")
}

// containsToken matches token as a whole variable token — i.e. "$FILE" in
// "reserialize $FILE" but not in "$FILES". The token ends at a non-word
// character (anything not [A-Za-z0-9_]) or end-of-string.
func containsToken(s, token string) bool {
	for i := 0; i <= len(s)-len(token); i++ {
		if s[i:i+len(token)] != token {
			continue
		}
		end := i + len(token)
		if end == len(s) {
			return true
		}
		next := rune(s[end])
		if !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_' {
			return true
		}
	}
	return false
}

// ExpandSpec is the result of expanding a hook's `run` string against a
// Batch. `Commands` is the list of (argv, env) pairs to execute; one entry
// for single-dispatch hooks, N entries for $FILE per-file fan-out.
type ExpandSpec struct {
	Commands []Command
}

// Command is a single exec target: argv (already split) + env vars to
// inject (beyond the parent process's env).
type Command struct {
	// Argv is the split argument list to pass to the udit binary. Does NOT
	// include the binary name itself — runner prepends that.
	Argv []string

	// Env is the extra env var injection (e.g. UDIT_CHANGED_FILES). Each
	// entry is "KEY=VALUE". Empty when no env injection needed.
	Env []string
}

// Expand produces the full list of commands to execute for a given batch.
// Handles the per-token policy:
//   - $FILE present  ⇒ one Command per file, each with $FILE bound to that file
//   - $FILES present ⇒ one Command, with UDIT_CHANGED_FILES env + literal "$FILES" left in argv
//   - neither        ⇒ one Command, no path-related expansion
//
// $EVENT / $HOOK are always substituted. Path vars produce forward-slash
// paths for consistency across OSes.
func Expand(runStr string, b Batch) (ExpandSpec, error) {
	if strings.TrimSpace(runStr) == "" {
		return ExpandSpec{}, fmt.Errorf("empty run string")
	}

	// Per-token decision uses the pre-expansion run string so we see the
	// literal $FILE/$RELFILE/$FILES/$RELFILES markers.
	perFile := hasPerFileVar(runStr)
	batch := hasBatchVar(runStr)

	// Dominant event across the batch: create > write > remove > rename.
	// Create wins because it's the most actionable signal; write second
	// because it's the common case.
	event := dominantEvent(b.Files)

	baseSubs := map[string]string{
		"$HOOK":  b.HookName,
		"$EVENT": event,
	}

	out := ExpandSpec{}

	if perFile {
		// One command per file. $FILE / $RELFILE bind to this specific
		// event's path; $EVENT binds to this file's individual event kind
		// (not batch-dominant) so per-file hooks see precise signal.
		for _, f := range b.Files {
			subs := cloneMap(baseSubs)
			subs["$FILE"] = filepath.ToSlash(f.Path)
			subs["$RELFILE"] = relPath(f.Path, b.ProjectRoot)
			subs["$EVENT"] = f.Kind
			argv, err := substituteAndSplit(runStr, subs)
			if err != nil {
				return ExpandSpec{}, err
			}
			out.Commands = append(out.Commands, Command{Argv: argv})
		}
		return out, nil
	}

	if batch {
		// One command, env var carrying the list. $FILES / $RELFILES stay
		// literal in argv — users reference the env vars
		// UDIT_CHANGED_FILES / UDIT_CHANGED_RELFILES directly. No $FILE
		// convenience substitution here: that would cause "$FILES" to be
		// rewritten as "<path>S" when $FILE is replaced first.
		subs := cloneMap(baseSubs)
		argv, err := substituteAndSplit(runStr, subs)
		if err != nil {
			return ExpandSpec{}, err
		}
		abs := make([]string, 0, len(b.Files))
		rel := make([]string, 0, len(b.Files))
		for _, f := range b.Files {
			abs = append(abs, filepath.ToSlash(f.Path))
			rel = append(rel, relPath(f.Path, b.ProjectRoot))
		}
		out.Commands = append(out.Commands, Command{
			Argv: argv,
			Env: []string{
				"UDIT_CHANGED_FILES=" + strings.Join(abs, "\n"),
				"UDIT_CHANGED_RELFILES=" + strings.Join(rel, "\n"),
			},
		})
		return out, nil
	}

	// Neither $FILE nor $FILES: a once-per-batch command with no path
	// information. Common for "run my test suite on any change" style
	// hooks.
	argv, err := substituteAndSplit(runStr, baseSubs)
	if err != nil {
		return ExpandSpec{}, err
	}
	out.Commands = append(out.Commands, Command{Argv: argv})
	return out, nil
}

// substituteAndSplit replaces $-prefixed tokens in s using subs, then splits
// the result into shell-like tokens. Substitution happens before splitting so
// a value with embedded spaces (unlikely for paths but possible) can be
// wrapped in quotes by the user.
func substituteAndSplit(s string, subs map[string]string) ([]string, error) {
	expanded := s
	// Replace longer tokens first so $RELFILE is replaced before $REL.
	// stable-ish iteration — sort keys by descending length.
	keys := keysByLengthDesc(subs)
	for _, k := range keys {
		expanded = strings.ReplaceAll(expanded, k, subs[k])
	}
	return splitArgs(expanded)
}

// splitArgs splits a command string into argv using POSIX-like rules:
// whitespace separates tokens; single or double quotes group; backslash
// escapes the next character (outside quotes).
//
// We do not use "github.com/google/shlex" to avoid adding a dep for ~40
// lines of code. This is permissive enough for hook strings like
//
//	reserialize "$FILE"   or   refresh --compile
//
// but does not attempt to reproduce every corner of sh(1).
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	emitted := false // did we start a token?
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			cur.WriteByte(c)
			escape = false
			emitted = true
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			emitted = true
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			emitted = true
			continue
		}
		if (c == ' ' || c == '\t') && !inSingle && !inDouble {
			if emitted {
				out = append(out, cur.String())
				cur.Reset()
				emitted = false
			}
			continue
		}
		cur.WriteByte(c)
		emitted = true
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in run string")
	}
	if escape {
		return nil, fmt.Errorf("trailing backslash in run string")
	}
	if emitted {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty run string after expansion")
	}
	return out, nil
}

// dominantEvent picks the "winning" event type when the batch contains a
// mix. Priority: create > write > remove > rename. Empty batch ⇒ "write"
// as a safe fallback (most common case).
func dominantEvent(files []Event) string {
	prio := map[string]int{"create": 4, "write": 3, "remove": 2, "rename": 1}
	best := "write"
	bestP := 0
	for _, f := range files {
		if p := prio[f.Kind]; p > bestP {
			best = f.Kind
			bestP = p
		}
	}
	return best
}

// relPath computes a project-root-relative, forward-slash path. If
// projectRoot is empty or path is not under it, falls back to the absolute
// forward-slash path. Used for $RELFILE substitution.
func relPath(absPath, projectRoot string) string {
	if projectRoot == "" {
		return filepath.ToSlash(absPath)
	}
	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	// filepath.Rel will happily return "../Something/Foo.cs" if the path is
	// outside the project. Agents don't want that — fall back to absolute.
	if strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func keysByLengthDesc(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// insertion sort: expected input is tiny (≤ 6 keys) so O(n²) is fine.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && len(out[j]) > len(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
