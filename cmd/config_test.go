package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	cfg, path := LoadConfig(dir)
	if cfg != nil {
		t.Errorf("expected nil cfg, got %+v", cfg)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestLoadConfig_ParsesAllFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	configPath := filepath.Join(dir, ".udit.yaml")
	if err := os.WriteFile(configPath, []byte(`default_port: 8591
default_timeout_ms: 60000
exec:
  usings:
    - Unity.Entities
    - Unity.Mathematics
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, path := LoadConfig(dir)
	if cfg == nil {
		t.Fatalf("expected cfg, got nil (path=%q)", path)
	}
	if cfg.DefaultPort != 8591 {
		t.Errorf("DefaultPort: got %d, want 8591", cfg.DefaultPort)
	}
	if cfg.DefaultTimeoutMs != 60000 {
		t.Errorf("DefaultTimeoutMs: got %d, want 60000", cfg.DefaultTimeoutMs)
	}
	if len(cfg.Exec.Usings) != 2 || cfg.Exec.Usings[0] != "Unity.Entities" {
		t.Errorf("Usings: got %v", cfg.Exec.Usings)
	}
	if path != configPath {
		t.Errorf("path: got %q, want %q", path, configPath)
	}
}

func TestLoadConfig_WalksUpward(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Dir(root)) // home is one level above project root
	t.Setenv("USERPROFILE", filepath.Dir(root))

	if err := os.WriteFile(filepath.Join(root, ".udit.yaml"), []byte("default_port: 1234\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg, path := LoadConfig(deep)
	if cfg == nil {
		t.Fatalf("expected to find .udit.yaml walking upward from %s", deep)
	}
	if cfg.DefaultPort != 1234 {
		t.Errorf("DefaultPort: got %d, want 1234", cfg.DefaultPort)
	}
	if path != filepath.Join(root, ".udit.yaml") {
		t.Errorf("path: got %q, want %q", path, filepath.Join(root, ".udit.yaml"))
	}
}

func TestLoadConfig_StopsAtHome(t *testing.T) {
	// Put the config in HOME's parent — should NOT be picked up because
	// the walk stops at HOME exclusive.
	homeParent := t.TempDir()
	home := filepath.Join(homeParent, "user")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(homeParent, ".udit.yaml"), []byte("default_port: 9999\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, _ := LoadConfig(home)
	if cfg != nil {
		t.Errorf("config above home should NOT be loaded; got %+v", cfg)
	}
}

func TestLoadConfig_InvalidYamlReturnsNil(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := os.WriteFile(filepath.Join(dir, ".udit.yaml"), []byte("default_port: not-a-number\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, path := LoadConfig(dir)
	if cfg != nil {
		t.Errorf("expected nil cfg on parse error, got %+v", cfg)
	}
	if path == "" {
		t.Errorf("expected path to be returned even on parse failure (got empty)")
	}
}

func TestMergeExecUsings_NilCfg(t *testing.T) {
	in := map[string]interface{}{"code": "return 1;"}
	out := mergeExecUsings(in, nil)
	if _, ok := out["usings"]; ok {
		t.Errorf("nil cfg should not add usings; got %v", out["usings"])
	}
}

func TestMergeExecUsings_AppendsConfigDefaults(t *testing.T) {
	cfg := &Config{Exec: ExecCfg{Usings: []string{"Unity.Entities", "MyGame.Core"}}}
	in := map[string]interface{}{"code": "return 1;"}
	out := mergeExecUsings(in, cfg)
	got, ok := out["usings"].([]string)
	if !ok {
		t.Fatalf("usings not []string: %T", out["usings"])
	}
	want := []string{"Unity.Entities", "MyGame.Core"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeExecUsings_DedupesAgainstCli(t *testing.T) {
	cfg := &Config{Exec: ExecCfg{Usings: []string{"Unity.Entities", "MyGame.Core"}}}
	in := map[string]interface{}{
		"code":   "return 1;",
		"usings": []string{"MyGame.Core", "Unity.Mathematics"}, // first overlaps
	}
	out := mergeExecUsings(in, cfg)
	got := out["usings"].([]string)
	want := []string{"Unity.Entities", "MyGame.Core", "Unity.Mathematics"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("at %d: got %q, want %q", i, got[i], w)
		}
	}
}
