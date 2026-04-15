package cmd

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- targets / cancel ---------------------------------------------------

func TestBuildCmd_Targets(t *testing.T) {
	send, params := mockSend("manage_build", t)
	if _, err := buildCmd([]string{"targets"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "targets" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	// targets accepts no options — params should be only action.
	if len(*params) != 1 {
		t.Errorf("targets should send only 'action', got %v", *params)
	}
}

func TestBuildCmd_Cancel(t *testing.T) {
	send, params := mockSend("manage_build", t)
	if _, err := buildCmd([]string{"cancel"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "cancel" {
		t.Errorf("action: got %v", (*params)["action"])
	}
}

// --- player -------------------------------------------------------------

func TestBuildCmd_PlayerBasic(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{"player", "--target", "win64", "--output", "/tmp/build"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "player" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["target"] != "win64" {
		t.Errorf("target: got %v", (*params)["target"])
	}
	// /tmp/build is already absolute on POSIX. On Windows the CLI's cwd
	// will get prepended; either way the result is absolute.
	out, _ := (*params)["output"].(string)
	if !filepath.IsAbs(out) {
		t.Errorf("output should be absolute, got %q", out)
	}
}

// --output is resolved against the CLI cwd, not Unity's. Same convention
// as `test --output` and `screenshot --output_path` — see cmd/paths.go.
func TestBuildCmd_PlayerOutputAbsolutized(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{"player", "--target", "win64", "--output", "builds/win64"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, _ := (*params)["output"].(string)
	if !filepath.IsAbs(out) {
		t.Errorf("relative output should become absolute, got %q", out)
	}
	if !strings.HasSuffix(filepath.ToSlash(out), "builds/win64") {
		t.Errorf("output should retain original tail, got %q", out)
	}
}

func TestBuildCmd_PlayerWithScenes(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--target", "android", "--output", "/tmp/apk", "--scenes", "Main,Level1,Level2",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scenes, ok := (*params)["scenes"].([]string)
	if !ok {
		t.Fatalf("scenes should be []string, got %T", (*params)["scenes"])
	}
	want := []string{"Main", "Level1", "Level2"}
	if !reflect.DeepEqual(scenes, want) {
		t.Errorf("scenes: got %v, want %v", scenes, want)
	}
}

// Whitespace around commas is friendly for humans; helper trims it and
// drops empty entries.
func TestBuildCmd_PlayerScenesTrim(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--target", "win64", "--output", "/tmp/build", "--scenes", "  Main , , Level1 ",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scenes, _ := (*params)["scenes"].([]string)
	want := []string{"Main", "Level1"}
	if !reflect.DeepEqual(scenes, want) {
		t.Errorf("scenes: got %v, want %v", scenes, want)
	}
}

func TestBuildCmd_PlayerDevelopment(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--target", "win64", "--output", "/tmp/build", "--development",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["development"] != true {
		t.Errorf("development should be true, got %v", (*params)["development"])
	}
}

// --development absent → no development key on the wire (server's default
// kicks in). Keeps payload minimal.
func TestBuildCmd_PlayerNoDevelopmentByDefault(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--target", "win64", "--output", "/tmp/build",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, set := (*params)["development"]; set {
		t.Errorf("development should not be sent without --development, got %v", (*params)["development"])
	}
}

func TestBuildCmd_PlayerMissingTarget(t *testing.T) {
	send, _ := mockSend("manage_build", t)
	if _, err := buildCmd([]string{"player", "--output", "/tmp/build"}, send); err == nil {
		t.Error("expected error for missing --target")
	}
}

func TestBuildCmd_PlayerMissingOutput(t *testing.T) {
	send, _ := mockSend("manage_build", t)
	if _, err := buildCmd([]string{"player", "--target", "win64"}, send); err == nil {
		t.Error("expected error for missing --output")
	}
}

// --- addressables -------------------------------------------------------

func TestBuildCmd_AddressablesNoProfile(t *testing.T) {
	send, params := mockSend("manage_build", t)
	if _, err := buildCmd([]string{"addressables"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "addressables" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if _, set := (*params)["profile"]; set {
		t.Errorf("profile should not be sent without --profile, got %v", (*params)["profile"])
	}
}

func TestBuildCmd_AddressablesWithProfile(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{"addressables", "--profile", "MobileDebug"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["profile"] != "MobileDebug" {
		t.Errorf("profile: got %v", (*params)["profile"])
	}
}

// --- routing ------------------------------------------------------------

func TestBuildCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_build", t)
	if _, err := buildCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestBuildCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_build", t)
	if _, err := buildCmd([]string{"compile"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}

// --- splitTrim ----------------------------------------------------------

func TestSplitTrim_Basic(t *testing.T) {
	got := splitTrim("a,b,c", ",")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitTrim_TrimAndDropEmpty(t *testing.T) {
	got := splitTrim(" a , , b , ", ",")
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitTrim_AllEmpty(t *testing.T) {
	got := splitTrim(" , , ", ",")
	if len(got) != 0 {
		t.Errorf("all-empty input should return empty slice, got %v", got)
	}
}

// --- --il2cpp flag ------------------------------------------------------

func TestBuildCmd_PlayerIL2CPPFlag(t *testing.T) {
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--target", "win64", "--output", "/tmp/build", "--il2cpp",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["il2cpp"] != true {
		t.Errorf("il2cpp: got %v, want true", (*params)["il2cpp"])
	}
}

func TestBuildCmd_PlayerNoIL2CPPFlagOmitsKey(t *testing.T) {
	// Without --il2cpp (and without a preset), we don't send the key —
	// lets Unity use the project's current PlayerSettings.
	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--target", "win64", "--output", "/tmp/build",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, set := (*params)["il2cpp"]; set {
		t.Errorf("il2cpp should be omitted when not requested, got %v", (*params)["il2cpp"])
	}
}

// --- preset resolution --------------------------------------------------

func TestResolveBuildPreset_NoConfig(t *testing.T) {
	prev := loadedConfig
	loadedConfig = nil
	defer func() { loadedConfig = prev }()

	_, err := resolveBuildPreset("production")
	if err == nil || !strings.Contains(err.Error(), "udit init") {
		t.Errorf("expected 'udit init' hint, got %v", err)
	}
}

func TestResolveBuildPreset_NoTargetsSection(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{}
	defer func() { loadedConfig = prev }()

	_, err := resolveBuildPreset("production")
	if err == nil || !strings.Contains(err.Error(), "build.targets") {
		t.Errorf("expected 'build.targets' hint, got %v", err)
	}
}

func TestResolveBuildPreset_UnknownListsAvailable(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Build: BuildCfg{
			Targets: map[string]BuildPreset{
				"production": {Target: "win64"},
				"dev":        {Target: "linux"},
			},
		},
	}
	defer func() { loadedConfig = prev }()

	_, err := resolveBuildPreset("staging")
	if err == nil {
		t.Fatalf("expected error for unknown preset")
	}
	for _, wanted := range []string{"Available:", "production", "dev"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("error should mention %q, got %v", wanted, err)
		}
	}
}

func TestResolveBuildPreset_Found(t *testing.T) {
	trueV := true
	prev := loadedConfig
	loadedConfig = &Config{
		Build: BuildCfg{
			Targets: map[string]BuildPreset{
				"production": {Target: "win64", Output: "Build/p.exe", IL2CPP: &trueV},
			},
		},
	}
	defer func() { loadedConfig = prev }()

	p, err := resolveBuildPreset("production")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Target != "win64" || p.IL2CPP == nil || !*p.IL2CPP {
		t.Errorf("unexpected preset: %+v", p)
	}
}

// --- preset / flag merge ------------------------------------------------

func TestBuildCmd_PresetCarriesFields(t *testing.T) {
	trueV, falseV := true, false
	prev := loadedConfig
	loadedConfig = &Config{
		Build: BuildCfg{
			Targets: map[string]BuildPreset{
				"production": {
					Target:      "win64",
					Output:      "Build/prod/MyGame.exe",
					Scenes:      []string{"Assets/Scenes/Main.unity"},
					IL2CPP:      &trueV,
					Development: &falseV,
				},
			},
		},
	}
	defer func() { loadedConfig = prev }()

	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{"player", "--config", "production"}, send)
	if err != nil {
		t.Fatalf("buildCmd: %v", err)
	}
	if (*params)["target"] != "win64" {
		t.Errorf("target: %v", (*params)["target"])
	}
	if (*params)["il2cpp"] != true {
		t.Errorf("il2cpp: %v", (*params)["il2cpp"])
	}
	if (*params)["development"] != false {
		t.Errorf("development: %v", (*params)["development"])
	}
	scenes, ok := (*params)["scenes"].([]string)
	if !ok || len(scenes) != 1 || scenes[0] != "Assets/Scenes/Main.unity" {
		t.Errorf("scenes: %v", (*params)["scenes"])
	}
}

func TestBuildCmd_FlagOverridesPreset(t *testing.T) {
	trueV, falseV := true, false
	prev := loadedConfig
	loadedConfig = &Config{
		Build: BuildCfg{
			Targets: map[string]BuildPreset{
				"production": {
					Target:      "win64",
					Output:      "Build/prod/MyGame.exe",
					IL2CPP:      &trueV,
					Development: &falseV,
				},
			},
		},
	}
	defer func() { loadedConfig = prev }()

	send, params := mockSend("manage_build", t)
	_, err := buildCmd([]string{
		"player", "--config", "production",
		"--output", "Build/custom/x.exe",
		"--development",
		"--no-il2cpp",
	}, send)
	if err != nil {
		t.Fatalf("buildCmd: %v", err)
	}
	out, _ := (*params)["output"].(string)
	if !strings.HasSuffix(out, "x.exe") {
		t.Errorf("output should be the flag, got %q", out)
	}
	if (*params)["development"] != true {
		t.Errorf("development should be overridden to true, got %v", (*params)["development"])
	}
	if (*params)["il2cpp"] != false {
		t.Errorf("--no-il2cpp should force false, got %v", (*params)["il2cpp"])
	}
}

func TestBuildCmd_UnknownPresetErrors(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Build: BuildCfg{
			Targets: map[string]BuildPreset{"dev": {Target: "linux", Output: "Build/x"}},
		},
	}
	defer func() { loadedConfig = prev }()

	send, _ := mockSend("manage_build", t) // should not actually be hit
	_, err := buildCmd([]string{"player", "--config", "missing"}, send)
	if err == nil {
		t.Fatalf("expected error for missing preset")
	}
}
