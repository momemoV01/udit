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
