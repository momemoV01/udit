package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momemoV01/udit/internal/watch"
	"gopkg.in/yaml.v3"
)

func TestInit_CreatesMinimalFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".udit.yaml")
	if err := initCmd([]string{"--output", target}); err != nil {
		t.Fatalf("initCmd: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	// Header comments
	if !strings.Contains(content, "udit project configuration") {
		t.Errorf("header comment missing: %q", content[:200])
	}
	// Globals are commented out (no active default_port etc.)
	if strings.Contains(content, "\ndefault_port:") {
		t.Errorf("default_port should be commented in minimal scaffold")
	}
	// watch: section also commented
	if strings.Contains(content, "\nwatch:") {
		t.Errorf("watch: section should be commented in minimal scaffold")
	}
	// Must parse as YAML (even if empty after stripping comments)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Errorf("scaffold output fails to parse as YAML: %v", err)
	}
}

func TestInit_WithWatch_EmbedsHooks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".udit.yaml")
	if err := initCmd([]string{"--output", target, "--watch"}); err != nil {
		t.Fatalf("initCmd: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("scaffold does not parse: %v", err)
	}
	if len(cfg.Watch.Hooks) != 2 {
		t.Fatalf("expected 2 sample hooks, got %d", len(cfg.Watch.Hooks))
	}
	if cfg.Watch.Hooks[0].Name != "compile_cs" {
		t.Errorf("first hook name: %q", cfg.Watch.Hooks[0].Name)
	}
	if cfg.Watch.Hooks[1].Name != "reserialize_yaml" {
		t.Errorf("second hook name: %q", cfg.Watch.Hooks[1].Name)
	}
	// The scaffold must itself pass watch Validate() — the sample we ship
	// should be usable without edits.
	w := cfg.Watch.Defaults()
	if err := w.Validate(); err != nil {
		t.Errorf("shipped scaffold fails watch.Validate: %v", err)
	}
	// Defaults documented in comments should still resolve correctly.
	if w.Debounce.Milliseconds() != 300 {
		t.Errorf("debounce parsed: %v", w.Debounce)
	}
	if w.OnBusy != "queue" {
		t.Errorf("on_busy: %q", w.OnBusy)
	}
	// $RELFILE token must be present in the second hook — we documented
	// that example and users will cargo-cult from it.
	if !strings.Contains(cfg.Watch.Hooks[1].Run, "$RELFILE") {
		t.Errorf("reserialize hook should use $RELFILE: %q", cfg.Watch.Hooks[1].Run)
	}
	// The compile_cs hook should not contain any file var — it's idempotent
	// and fires once per batch regardless.
	if strings.Contains(cfg.Watch.Hooks[0].Run, "$") {
		t.Errorf("compile hook should not reference variables: %q", cfg.Watch.Hooks[0].Run)
	}

	// Ensure the watch-mode template's validate path compiles against the
	// matcher / Ignorer plumbing — a lightweight acceptance smoke.
	_ = watch.NewMatcher(cfg.Watch.Hooks, false)
}

func TestInit_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".udit.yaml")
	if err := os.WriteFile(target, []byte("existing: true\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := initCmd([]string{"--output", target})
	if err == nil {
		t.Fatalf("expected error when target exists without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force: %v", err)
	}
	// File must be untouched.
	data, _ := os.ReadFile(target)
	if string(data) != "existing: true\n" {
		t.Errorf("existing file was modified: %q", data)
	}
}

func TestInit_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".udit.yaml")
	if err := os.WriteFile(target, []byte("old: true\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := initCmd([]string{"--output", target, "--force", "--watch"}); err != nil {
		t.Fatalf("initCmd --force: %v", err)
	}
	data, _ := os.ReadFile(target)
	if strings.Contains(string(data), "old: true") {
		t.Errorf("--force did not overwrite: %q", data[:50])
	}
	if !strings.Contains(string(data), "compile_cs") {
		t.Errorf("expected --watch scaffold hooks: %q", data[:200])
	}
}

func TestInit_DefaultOutputFallsBackToCwd(t *testing.T) {
	// No Assets/ or ProjectSettings/ → detection fails → cwd fallback.
	dir := t.TempDir()
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := initCmd(nil); err != nil {
		t.Fatalf("initCmd: %v", err)
	}
	expected := filepath.Join(dir, ".udit.yaml")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected file at %s: %v", expected, err)
	}
}

func TestInit_DefaultOutputDetectsUnityRoot(t *testing.T) {
	// Simulate a Unity project: <root>/Assets + <root>/ProjectSettings.
	// Run from a nested subdir so detection must walk up.
	root := t.TempDir()
	for _, d := range []string{"Assets", "ProjectSettings", filepath.Join("Assets", "Scripts")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	nested := filepath.Join(root, "Assets", "Scripts")
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := initCmd([]string{"--watch"}); err != nil {
		t.Fatalf("initCmd: %v", err)
	}
	// Must land at project root, NOT inside Assets/Scripts.
	atRoot := filepath.Join(root, ".udit.yaml")
	if _, err := os.Stat(atRoot); err != nil {
		t.Errorf("expected file at detected root %s: %v", atRoot, err)
	}
	atNested := filepath.Join(nested, ".udit.yaml")
	if _, err := os.Stat(atNested); err == nil {
		t.Errorf("file should NOT be created at nested cwd %s", atNested)
	}
}

func TestResolveInitTarget_ExplicitOutputWins(t *testing.T) {
	// Even inside a detected Unity project, explicit --output must override.
	root := t.TempDir()
	for _, d := range []string{"Assets", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	explicit := filepath.Join(root, "custom", "my.yaml")
	abs, source, err := resolveInitTarget(explicit)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if abs != explicit {
		t.Errorf("abs = %q, want %q", abs, explicit)
	}
	if source != "from --output" {
		t.Errorf("source = %q, want 'from --output'", source)
	}
}
