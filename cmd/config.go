package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape of `.udit.yaml`. All fields are optional —
// missing values fall through to the built-in defaults set in flag.*Var().
//
// Search rule: starting at cwd, walk up to filesystem root and use the
// first file named `.udit.yaml`. Stops at the user's home directory so a
// stray file in the home root doesn't leak across unrelated projects.
type Config struct {
	DefaultPort      int     `yaml:"default_port"`
	DefaultTimeoutMs int     `yaml:"default_timeout_ms"`
	Exec             ExecCfg `yaml:"exec"`
}

type ExecCfg struct {
	// Usings is the list of additional `using` directives prepended to every
	// `udit exec` invocation. Useful for project-wide namespaces like
	// Unity.Entities or your own gameplay assemblies. Merged with the per-call
	// --usings flag (config first, then CLI; duplicates kept — csc tolerates them).
	Usings []string `yaml:"usings"`
}

// LoadConfig walks from `start` upward looking for `.udit.yaml`. Returns
// (nil, "") if none found. Returns (nil, path) with a stderr warning if a
// file was found but failed to parse — the caller continues with defaults.
func LoadConfig(start string) (*Config, string) {
	path := findConfigFile(start)
	if path == "" {
		return nil, ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v (using defaults)\n", path, err)
		return nil, path
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid YAML in %s: %v (using defaults)\n", path, err)
		return nil, path
	}
	return &cfg, path
}

// mergeExecUsings prepends config.exec.usings to params["usings"] without
// duplicates (case-sensitive). Returns params unchanged when no config or no
// configured usings. The CLI value (if any) is kept as-is, just appended to
// after the config defaults.
func mergeExecUsings(params map[string]interface{}, cfg *Config) map[string]interface{} {
	if cfg == nil || len(cfg.Exec.Usings) == 0 {
		return params
	}
	if params == nil {
		params = map[string]interface{}{}
	}
	// Collect existing CLI-provided usings (could be []string, []interface{},
	// or string from `--usings A,B,C`).
	var existing []string
	switch v := params["usings"].(type) {
	case []string:
		existing = v
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				existing = append(existing, s)
			}
		}
	case string:
		// "A,B,C" form — leave it alone; the C# side splits commas itself.
		// Build a fresh combined list anyway.
		existing = append(existing, splitCommaList(v)...)
	}
	seen := map[string]bool{}
	merged := make([]string, 0, len(cfg.Exec.Usings)+len(existing))
	for _, u := range cfg.Exec.Usings {
		if !seen[u] {
			seen[u] = true
			merged = append(merged, u)
		}
	}
	for _, u := range existing {
		if !seen[u] {
			seen[u] = true
			merged = append(merged, u)
		}
	}
	params["usings"] = merged
	return params
}

func splitCommaList(s string) []string {
	out := []string{}
	curr := ""
	for _, ch := range s {
		if ch == ',' {
			if curr != "" {
				out = append(out, curr)
			}
			curr = ""
			continue
		}
		curr += string(ch)
	}
	if curr != "" {
		out = append(out, curr)
	}
	return out
}

// findConfigFile walks from start upward. Stops at the user's home dir
// (exclusive) or filesystem root, whichever comes first. Returns "" on miss.
func findConfigFile(start string) string {
	home, _ := os.UserHomeDir()
	dir := start
	for {
		candidate := filepath.Join(dir, ".udit.yaml")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		// Stop at home or root
		if home != "" && dir == home {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // hit filesystem root
		}
		dir = parent
	}
}
