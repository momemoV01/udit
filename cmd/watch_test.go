package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWatchConfig_UsesConnectedInstance(t *testing.T) {
	// Heartbeat points at a project with a valid .udit.yaml.
	home := isolateInstances(t)
	projectRoot := t.TempDir()
	cfgPath := filepath.Join(projectRoot, ".udit.yaml")
	if err := os.WriteFile(cfgPath, []byte(`watch:
  hooks:
    - name: compile
      paths: [Assets/**/*.cs]
      run: refresh --compile
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	writeHeartbeat(t, home, filepath.ToSlash(projectRoot), 8591)

	// cwd is unrelated — instance layer should still find the config.
	unrelated := t.TempDir()
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(unrelated); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, path, err := loadWatchConfig("")
	if err != nil {
		t.Fatalf("loadWatchConfig: %v", err)
	}
	if path != cfgPath {
		t.Errorf("path: got %q, want %q", path, cfgPath)
	}
	if len(cfg.Hooks) != 1 || cfg.Hooks[0].Name != "compile" {
		t.Errorf("cfg.Hooks: %+v", cfg.Hooks)
	}
}

func TestLoadWatchConfig_FallsBackToWalkUp(t *testing.T) {
	// Instance present, BUT its projectPath has no .udit.yaml.
	// loadWatchConfig must fall through to cwd walk-up.
	home := isolateInstances(t)
	projectRoot := t.TempDir() // no .udit.yaml inside
	writeHeartbeat(t, home, filepath.ToSlash(projectRoot), 8591)

	// Separate cwd with a real config.
	cwdProject := t.TempDir()
	t.Setenv("HOME", home)        // keep instance-dir isolation
	t.Setenv("USERPROFILE", home) // for LoadConfig's $HOME boundary
	cfgPath := filepath.Join(cwdProject, ".udit.yaml")
	// Note the quoting on "**/*.txt" — bare `**` in a YAML flow sequence
	// trips the parser into alias-lookup mode.
	if err := os.WriteFile(cfgPath, []byte(`watch:
  hooks:
    - name: walkup
      paths: ["**/*.txt"]
      run: version
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(cwdProject); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, path, err := loadWatchConfig("")
	if err != nil {
		t.Fatalf("loadWatchConfig: %v", err)
	}
	if path != cfgPath {
		t.Errorf("path: got %q, want %q (walk-up should win when instance has no config)", path, cfgPath)
	}
	if len(cfg.Hooks) != 1 || cfg.Hooks[0].Name != "walkup" {
		t.Errorf("expected walkup hook, got %+v", cfg.Hooks)
	}
}

func TestLoadWatchConfig_ErrorListsBothLayers(t *testing.T) {
	// No connected instance, no walk-up hit → error message must name
	// both places that were checked so users know where to put the file.
	home := isolateInstances(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	empty := t.TempDir()
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(empty); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	_, _, err := loadWatchConfig("")
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "walking up from") {
		t.Errorf("error should mention walk-up: %q", msg)
	}
	if !strings.Contains(msg, "no running Unity detected") {
		t.Errorf("error should mention missing Unity: %q", msg)
	}
}

// ---- Ad-hoc mode ----

func TestAdhocWatchCfg_BothSet(t *testing.T) {
	cfg, ok, err := adhocWatchCfg([]string{"Assets/**/*.cs"}, "refresh --compile")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatalf("expected ad-hoc mode")
	}
	if len(cfg.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(cfg.Hooks))
	}
	h := cfg.Hooks[0]
	if h.Name != "ad-hoc" {
		t.Errorf("hook name: %q", h.Name)
	}
	if len(h.Paths) != 1 || h.Paths[0] != "Assets/**/*.cs" {
		t.Errorf("paths: %v", h.Paths)
	}
	if h.Run != "refresh --compile" {
		t.Errorf("run: %q", h.Run)
	}
}

func TestAdhocWatchCfg_MultiplePaths(t *testing.T) {
	paths := []string{"Assets/**/*.cs", "Packages/**/*.cs"}
	cfg, ok, err := adhocWatchCfg(paths, "refresh --compile")
	if err != nil || !ok {
		t.Fatalf("want (ok, nil), got (%v, %v)", ok, err)
	}
	if len(cfg.Hooks[0].Paths) != 2 {
		t.Errorf("both paths should be preserved: %v", cfg.Hooks[0].Paths)
	}
}

func TestAdhocWatchCfg_Neither(t *testing.T) {
	_, ok, err := adhocWatchCfg(nil, "")
	if err != nil {
		t.Errorf("neither set should not error: %v", err)
	}
	if ok {
		t.Errorf("neither set should return ok=false (fall through to config)")
	}
}

func TestAdhocWatchCfg_PathsOnly(t *testing.T) {
	_, _, err := adhocWatchCfg([]string{"Assets/**"}, "")
	if err == nil {
		t.Fatalf("expected error for --path without --on-change")
	}
	if !strings.Contains(err.Error(), "--on-change") {
		t.Errorf("error should mention --on-change: %v", err)
	}
}

func TestAdhocWatchCfg_OnChangeOnly(t *testing.T) {
	_, _, err := adhocWatchCfg(nil, "refresh --compile")
	if err == nil {
		t.Fatalf("expected error for --on-change without --path")
	}
	if !strings.Contains(err.Error(), "--path") {
		t.Errorf("error should mention --path: %v", err)
	}
}

// Ensure the constructed hook survives WatchCfg.Validate + Defaults —
// i.e. a real `udit watch --path X --on-change Y` invocation wouldn't
// fail at the validation step.
func TestAdhocWatchCfg_ValidatesAndGetsDefaults(t *testing.T) {
	cfg, ok, err := adhocWatchCfg([]string{"Assets/**/*.cs"}, "refresh --compile")
	if err != nil || !ok {
		t.Fatalf("build: ok=%v err=%v", ok, err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("ad-hoc cfg should validate: %v", err)
	}
	d := cfg.Defaults()
	if d.Debounce <= 0 {
		t.Errorf("defaults should fill debounce: %v", d.Debounce)
	}
	if d.OnBusy == "" {
		t.Errorf("defaults should fill on_busy")
	}
}

func TestStringSliceFlag(t *testing.T) {
	var s stringSliceFlag
	_ = s.Set("a")
	_ = s.Set("b")
	_ = s.Set("c")
	if len(s) != 3 {
		t.Errorf("collect: %v", s)
	}
	if s.String() != "a,b,c" {
		t.Errorf("String(): %q", s.String())
	}
}

func TestLoadWatchConfig_ExplicitConfigWins(t *testing.T) {
	// --config PATH should bypass instance + walk-up entirely.
	home := isolateInstances(t)
	// Instance pointing at a "wrong" project with its own config.
	otherProject := t.TempDir()
	_ = os.WriteFile(filepath.Join(otherProject, ".udit.yaml"), []byte(`watch:
  hooks:
    - name: wrong
      paths: ["*"]
      run: version
`), 0o644)
	writeHeartbeat(t, home, filepath.ToSlash(otherProject), 8591)

	// The caller's explicit config.
	dir := t.TempDir()
	explicit := filepath.Join(dir, "my.yaml")
	if err := os.WriteFile(explicit, []byte(`watch:
  hooks:
    - name: right
      paths: ["*"]
      run: version
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, path, err := loadWatchConfig(explicit)
	if err != nil {
		t.Fatalf("loadWatchConfig: %v", err)
	}
	abs, _ := filepath.Abs(explicit)
	if path != abs {
		t.Errorf("path: got %q, want %q", path, abs)
	}
	if len(cfg.Hooks) != 1 || cfg.Hooks[0].Name != "right" {
		t.Errorf("explicit config not used: %+v", cfg.Hooks)
	}
}
