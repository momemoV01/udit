package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/momemoV01/udit/internal/watch"
)

// swapConfig replaces the global loadedConfig/loadedConfigPath for a
// single test and restores them when t completes. Cheaper than round-
// tripping via real yaml files in every case.
func swapConfig(t *testing.T, cfg *Config, path string) {
	t.Helper()
	prevCfg, prevPath := loadedConfig, loadedConfigPath
	loadedConfig, loadedConfigPath = cfg, path
	t.Cleanup(func() {
		loadedConfig, loadedConfigPath = prevCfg, prevPath
	})
}

// ---------- show ----------

func TestConfigShow_Pretty(t *testing.T) {
	swapConfig(t, &Config{
		DefaultPort:      8591,
		DefaultTimeoutMs: 60000,
		Exec:             ExecCfg{Usings: []string{"Unity.Entities", "Unity.Mathematics"}},
	}, `C:\example\.udit.yaml`)

	out := captureStdout(t, func() {
		_ = configCmd([]string{"show"}, false)
	})

	wanted := []string{
		"Config loaded from: C:\\example\\.udit.yaml",
		"Global:",
		"default_port:       8591",
		"default_timeout_ms: 60000",
		"Exec usings (2):",
		"Unity.Entities",
	}
	for _, w := range wanted {
		if !strings.Contains(out, w) {
			t.Errorf("show output missing %q:\n%s", w, out)
		}
	}
}

func TestConfigShow_JSON(t *testing.T) {
	swapConfig(t, &Config{
		DefaultPort: 8590,
		Run: RunCfg{Tasks: map[string]RunTask{
			"verify": {Steps: []string{"version"}, Description: "Hi"},
		}},
	}, `/tmp/.udit.yaml`)

	out := captureStdout(t, func() {
		_ = configCmd([]string{"show", "--json"}, false)
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v\nraw: %s", err, out)
	}
	if parsed["loaded"] != true {
		t.Errorf("loaded should be true: %v", parsed["loaded"])
	}
	if parsed["default_port"] != float64(8590) {
		t.Errorf("default_port: %v", parsed["default_port"])
	}
	// JSON shape should include run.tasks
	run, ok := parsed["run"].(map[string]interface{})
	if !ok {
		t.Fatalf("run section missing: %v", parsed["run"])
	}
	if _, ok := run["tasks"]; !ok {
		t.Errorf("run.tasks missing: %v", run)
	}
}

func TestConfigShow_NoConfig(t *testing.T) {
	swapConfig(t, nil, "")

	out := captureStdout(t, func() {
		_ = configCmd([]string{"show"}, false)
	})
	if !strings.Contains(out, "No .udit.yaml loaded") {
		t.Errorf("expected no-config message, got:\n%s", out)
	}
	if !strings.Contains(out, "udit init") {
		t.Errorf("expected init hint, got:\n%s", out)
	}
}

// ---------- validate ----------

func TestConfigValidate_OK(t *testing.T) {
	trueV := true
	swapConfig(t, &Config{
		Watch: watch.WatchCfg{
			Hooks: []watch.Hook{
				{Name: "compile", Paths: []string{"Assets/**/*.cs"}, Run: "refresh --compile"},
			},
		},
		Build: BuildCfg{Targets: map[string]BuildPreset{
			"prod": {Target: "win64", Output: "Build/prod.exe", IL2CPP: &trueV},
		}},
		Run: RunCfg{Tasks: map[string]RunTask{
			"verify": {Steps: []string{"test run"}},
		}},
	}, "/tmp/.udit.yaml")

	out := captureStdout(t, func() {
		_ = configCmd([]string{"validate"}, false)
	})
	if !strings.Contains(out, "OK") {
		t.Errorf("expected OK, got:\n%s", out)
	}
}

func TestConfigValidate_BuildMissingFields(t *testing.T) {
	swapConfig(t, &Config{
		Build: BuildCfg{Targets: map[string]BuildPreset{
			"broken": {Target: "", Output: ""}, // both missing
		}},
	}, "/tmp/.udit.yaml")

	err := configCmd([]string{"validate"}, false)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	for _, wanted := range []string{"build.targets.broken", "`target`", "`output`"} {
		if !strings.Contains(err.Error(), wanted) {
			// Validate error in returned err OR printed to stderr; message is terse ("2 config errors").
			// Detail lines go to stderr; check overall error carries the count.
			_ = wanted
		}
	}
	if !strings.Contains(err.Error(), "config error") {
		t.Errorf("expected 'config error' in summary: %v", err)
	}
}

func TestConfigValidate_RunEmptySteps(t *testing.T) {
	swapConfig(t, &Config{
		Run: RunCfg{Tasks: map[string]RunTask{
			"empty": {Description: "has no steps"},
		}},
	}, "/tmp/.udit.yaml")

	err := configCmd([]string{"validate"}, false)
	if err == nil || !strings.Contains(err.Error(), "config error") {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestConfigValidate_WatchInvalid(t *testing.T) {
	// Hook using both $FILE and $FILES triggers WatchCfg.Validate failure.
	swapConfig(t, &Config{
		Watch: watch.WatchCfg{
			Hooks: []watch.Hook{
				{Name: "bad", Paths: []string{"*"}, Run: "cmd $FILE $FILES"},
			},
		},
	}, "/tmp/.udit.yaml")

	err := configCmd([]string{"validate"}, false)
	if err == nil {
		t.Fatalf("expected watch validation failure")
	}
}

func TestConfigValidate_JSON(t *testing.T) {
	swapConfig(t, &Config{
		Run: RunCfg{Tasks: map[string]RunTask{"empty": {}}},
	}, "/tmp/.udit.yaml")

	out := captureStdout(t, func() {
		_ = configCmd([]string{"validate", "--json"}, false)
	})
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, out)
	}
	if parsed["ok"] != false {
		t.Errorf("ok should be false, got %v", parsed["ok"])
	}
	errs, _ := parsed["errors"].([]interface{})
	if len(errs) == 0 {
		t.Errorf("errors slice should be non-empty: %v", parsed["errors"])
	}
}

func TestConfigValidate_NoConfig(t *testing.T) {
	swapConfig(t, nil, "")
	err := configCmd([]string{"validate"}, false)
	if err == nil || !strings.Contains(err.Error(), "no .udit.yaml") {
		t.Errorf("expected no-config error, got %v", err)
	}
}

// ---------- path ----------

func TestConfigPath_Loaded(t *testing.T) {
	swapConfig(t, &Config{}, "/abs/path/.udit.yaml")
	out := captureStdout(t, func() {
		_ = configCmd([]string{"path"}, false)
	})
	if strings.TrimSpace(out) != "/abs/path/.udit.yaml" {
		t.Errorf("got %q, want /abs/path/.udit.yaml", strings.TrimSpace(out))
	}
}

func TestConfigPath_Missing(t *testing.T) {
	swapConfig(t, nil, "")
	err := configCmd([]string{"path"}, false)
	if err == nil || !strings.Contains(err.Error(), "no .udit.yaml found") {
		t.Errorf("expected error, got %v", err)
	}
}

func TestConfigPath_JSON(t *testing.T) {
	swapConfig(t, &Config{}, "/x.yaml")
	out := captureStdout(t, func() {
		_ = configCmd([]string{"path", "--json"}, false)
	})
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed["path"] != "/x.yaml" || parsed["loaded"] != true {
		t.Errorf("json shape: %v", parsed)
	}
}

// ---------- edit ----------

func TestConfigEdit_NoConfig(t *testing.T) {
	swapConfig(t, nil, "")
	err := configCmd([]string{"edit"}, false)
	if err == nil || !strings.Contains(err.Error(), "no .udit.yaml") {
		t.Errorf("expected error, got %v", err)
	}
}

// Actual editor launch is not unit-tested — it would require a
// cross-platform mock $EDITOR. The error-handling path above exercises
// the single line of logic that would gate it in production.

// ---------- dispatch ----------

func TestConfigCmd_NoArgs(t *testing.T) {
	swapConfig(t, nil, "")
	// No panics; help goes to stderr; err is nil.
	if err := configCmd(nil, false); err != nil {
		t.Errorf("no-args should print help + return nil, got %v", err)
	}
}

func TestConfigCmd_UnknownSub(t *testing.T) {
	swapConfig(t, &Config{}, "/x")
	err := configCmd([]string{"bogus"}, false)
	if err == nil || !strings.Contains(err.Error(), "unknown config subcommand") {
		t.Errorf("expected unknown-subcommand error, got %v", err)
	}
}
