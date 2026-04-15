package cmd

import "path/filepath"

// absolutizePath converts a relative filesystem path to absolute against the
// CLI's current working directory. Returns the original value unchanged if it
// is already absolute, empty, or if resolution fails.
//
// Rationale: Unity runs in its own process with its own CWD, so a bare
// relative path sent across the HTTP boundary would be interpreted against
// Unity's project root. Users typing `udit <cmd> --output foo.xml` expect
// the file next to them — matching POSIX CLI convention. Resolving on the
// CLI side makes the wire value unambiguous.
func absolutizePath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// absolutizePathParam applies absolutizePath to params[key] in place, iff
// the value is a non-empty string. Silent no-op otherwise (missing key,
// nil, empty string, or non-string value).
func absolutizePathParam(params map[string]interface{}, key string) {
	if s, ok := params[key].(string); ok && s != "" {
		params[key] = absolutizePath(s)
	}
}
