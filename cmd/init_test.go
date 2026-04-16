package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momemoV01/udit/internal/client"
	"github.com/momemoV01/udit/internal/watch"
	"gopkg.in/yaml.v3"
)

// isolateInstances points client.DiscoverInstance at an empty heartbeat
// directory so tests don't accidentally connect to a real running Unity.
// Returns the fake HOME so callers can also drop in instance fixtures
// for "instance reachable" scenarios.
func isolateInstances(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Also reset the global flags root.go parses so stale state from
	// earlier tests doesn't bleed into DiscoverInstance.
	prevPort, prevProject := flagPort, flagProject
	flagPort, flagProject = 0, ""
	t.Cleanup(func() { flagPort, flagProject = prevPort, prevProject })
	return home
}

// writeHeartbeat drops a single instance file so DiscoverInstance sees
// a live Unity. PID is the test process — guaranteed to be alive
// during the test, so isProcessDead returns false.
func writeHeartbeat(t *testing.T, home, projectPath string, port int) {
	t.Helper()
	dir := filepath.Join(home, ".udit", "instances")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir instances: %v", err)
	}
	inst := client.Instance{
		State:       "ready",
		ProjectPath: projectPath,
		Port:        port,
		PID:         os.Getpid(),
		Timestamp:   time.Now().Unix(),
	}
	data, err := json.Marshal(inst)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	file := filepath.Join(dir, fmt.Sprintf("%d.json", port))
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
}

func TestInit_CreatesMinimalFile(t *testing.T) {
	isolateInstances(t)
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
	isolateInstances(t)
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
	isolateInstances(t)
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
	isolateInstances(t)
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
	// No Unity instance, no Assets/ or ProjectSettings/ → detection
	// fails at every layer → cwd fallback.
	isolateInstances(t)
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
	// No instance connected + Unity project layout in cwd →
	// filesystem walk-up is the layer that wins.
	isolateInstances(t)
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

func TestResolveInitTarget_UsesConnectedUnityInstance(t *testing.T) {
	// A live heartbeat at port 8591 pointing at /fake/project — init
	// should target that project root regardless of cwd.
	home := isolateInstances(t)
	unityRoot := filepath.Join(t.TempDir(), "MyGame")
	if err := os.MkdirAll(unityRoot, 0o755); err != nil {
		t.Fatalf("mkdir unity root: %v", err)
	}
	// Use forward-slash form (matches what the connector writes).
	writeHeartbeat(t, home, filepath.ToSlash(unityRoot), 8591)

	// cwd is unrelated — the instance layer should override.
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	unrelated := t.TempDir()
	if err := os.Chdir(unrelated); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	abs, source, err := resolveInitTarget("")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantAbs, _ := filepath.Abs(filepath.Join(unityRoot, ".udit.yaml"))
	if abs != wantAbs {
		t.Errorf("abs = %q, want %q", abs, wantAbs)
	}
	if !strings.Contains(source, "8591") {
		t.Errorf("source should mention port 8591: %q", source)
	}
}

func TestResolveInitTarget_PortOverrideDoesNotUseStubPath(t *testing.T) {
	// --port sets DiscoverInstance to return ProjectPath="override" (no
	// heartbeat read). That stub must NOT become the scaffold target —
	// the resolver has to fall through to filesystem detection.
	isolateInstances(t)
	flagPort = 9999 // force the override branch
	defer func() { flagPort = 0 }()

	// Unity project layout in cwd so filesystem detection succeeds.
	// EvalSymlinks: on macOS /var → /private/var; os.Getwd() resolves
	// the symlink but t.TempDir() returns the unresolved path.
	root, _ := filepath.EvalSymlinks(t.TempDir())
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

	abs, source, err := resolveInitTarget("")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantAbs := filepath.Join(root, ".udit.yaml")
	if abs != wantAbs {
		t.Errorf("abs = %q, want %q (port stub should not leak)", abs, wantAbs)
	}
	if !strings.Contains(source, "filesystem") {
		t.Errorf("source should be filesystem fallback: %q", source)
	}
}

func TestResolveInitTarget_ExplicitOutputWins(t *testing.T) {
	// Even inside a detected Unity project, explicit --output must override.
	isolateInstances(t)
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
