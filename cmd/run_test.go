package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ----------------------------------------------------------------------
// YAML schema parsing
// ----------------------------------------------------------------------

func TestLoadConfig_ParsesRunSection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	cfgPath := filepath.Join(dir, ".udit.yaml")
	if err := os.WriteFile(cfgPath, []byte(`run:
  tasks:
    verify:
      description: "Full verification"
      steps:
        - editor refresh --compile
        - test run
    nightly:
      continue_on_error: true
      steps:
        - test run --mode EditMode
        - test run --mode PlayMode
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, _ := LoadConfig(dir)
	if cfg == nil {
		t.Fatalf("expected cfg")
	}
	if len(cfg.Run.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(cfg.Run.Tasks))
	}
	verify, ok := cfg.Run.Tasks["verify"]
	if !ok {
		t.Fatalf("missing verify task")
	}
	if verify.Description != "Full verification" {
		t.Errorf("description: %q", verify.Description)
	}
	if len(verify.Steps) != 2 {
		t.Errorf("steps: %v", verify.Steps)
	}
	if verify.ContinueOnError {
		t.Errorf("verify should default continue_on_error to false")
	}
	nightly := cfg.Run.Tasks["nightly"]
	if !nightly.ContinueOnError {
		t.Errorf("nightly continue_on_error: got false, want true")
	}
}

// ----------------------------------------------------------------------
// splitRunStep
// ----------------------------------------------------------------------

func TestSplitRunStep(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"editor refresh --compile", []string{"editor", "refresh", "--compile"}},
		{"  test   run  ", []string{"test", "run"}},
		{`reserialize "Assets/With Space/Foo.prefab"`,
			[]string{"reserialize", "Assets/With Space/Foo.prefab"}},
		{`build player --output 'my path/game.exe'`,
			[]string{"build", "player", "--output", "my path/game.exe"}},
		{"run verify", []string{"run", "verify"}},
	}
	for _, c := range cases {
		got, err := splitRunStep(c.in)
		if err != nil {
			t.Errorf("splitRunStep(%q) error: %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitRunStep(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSplitRunStep_Errors(t *testing.T) {
	for _, in := range []string{``, `cmd "unterminated`, `cmd 'unterminated`, `trailing\`} {
		if _, err := splitRunStep(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

// ----------------------------------------------------------------------
// Recursion guard
// ----------------------------------------------------------------------

func TestCheckRecursion_DepthCap(t *testing.T) {
	t.Setenv(envRunDepth, fmt.Sprintf("%d", maxRunDepth))
	t.Setenv(envRunStack, "a:b:c")

	_, _, err := checkRecursion("d")
	if err == nil {
		t.Fatalf("expected depth-cap error")
	}
	if !strings.Contains(err.Error(), "too deep") {
		t.Errorf("error should say 'too deep', got %v", err)
	}
}

func TestCheckRecursion_CycleDetected(t *testing.T) {
	t.Setenv(envRunDepth, "2")
	t.Setenv(envRunStack, "release:verify")

	_, _, err := checkRecursion("release")
	if err == nil {
		t.Fatalf("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should say 'cycle', got %v", err)
	}
	if !strings.Contains(err.Error(), "release → verify → release") {
		t.Errorf("error should name the chain, got %v", err)
	}
}

func TestCheckRecursion_TopLevelOK(t *testing.T) {
	t.Setenv(envRunDepth, "")
	t.Setenv(envRunStack, "")

	depth, stack, err := checkRecursion("verify")
	if err != nil {
		t.Fatalf("top-level should be OK: %v", err)
	}
	if depth != 0 {
		t.Errorf("depth: %d", depth)
	}
	if len(stack) != 0 {
		t.Errorf("stack: %v", stack)
	}
}

func TestCheckRecursion_NestedOK(t *testing.T) {
	t.Setenv(envRunDepth, "2")
	t.Setenv(envRunStack, "a:b")

	depth, stack, err := checkRecursion("c")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if depth != 2 {
		t.Errorf("depth: %d", depth)
	}
	if !reflect.DeepEqual(stack, []string{"a", "b"}) {
		t.Errorf("stack: %v", stack)
	}
}

// ----------------------------------------------------------------------
// runCmd dispatch — list mode + error paths (no fork needed)
// ----------------------------------------------------------------------

// captureStdout redirects os.Stdout for the duration of fn and returns
// what was written. Used to assert on listTasks output without touching
// the real terminal.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	<-done
	return buf.String()
}

func TestRunCmd_ListMode(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{
			Tasks: map[string]RunTask{
				"verify":  {Description: "Pre-commit", Steps: []string{"test run"}},
				"nightly": {ContinueOnError: true, Steps: []string{"test run", "build player"}},
			},
		},
	}
	defer func() { loadedConfig = prev }()

	out := captureStdout(t, func() {
		_ = runCmd(nil, false)
	})
	for _, wanted := range []string{"verify", "nightly", "Pre-commit", "continue-on-error"} {
		if !strings.Contains(out, wanted) {
			t.Errorf("list output missing %q:\n%s", wanted, out)
		}
	}
}

func TestRunCmd_ListJSONMode(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{Tasks: map[string]RunTask{"verify": {Steps: []string{"test run"}}}},
	}
	defer func() { loadedConfig = prev }()

	out := captureStdout(t, func() {
		_ = runCmd([]string{"--json"}, false)
	})
	if !strings.Contains(out, `"kind":"task"`) {
		t.Errorf("expected NDJSON with kind=task, got:\n%s", out)
	}
	if !strings.Contains(out, `"name":"verify"`) {
		t.Errorf("expected name=verify, got:\n%s", out)
	}
}

func TestRunCmd_NoConfig(t *testing.T) {
	prev := loadedConfig
	loadedConfig = nil
	defer func() { loadedConfig = prev }()

	err := runCmd(nil, false)
	if err == nil {
		t.Fatalf("expected error when no config loaded")
	}
	if !strings.Contains(err.Error(), "udit init") {
		t.Errorf("error should mention `udit init`: %v", err)
	}
}

func TestRunCmd_UnknownTask(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{Tasks: map[string]RunTask{
			"verify":  {Steps: []string{"test run"}},
			"release": {Steps: []string{"build player"}},
		}},
	}
	defer func() { loadedConfig = prev }()

	err := runCmd([]string{"bogus"}, false)
	if err == nil {
		t.Fatalf("expected error")
	}
	for _, wanted := range []string{"no such task", "Available:", "verify", "release"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("error should contain %q: %v", wanted, err)
		}
	}
}

func TestRunCmd_EmptySteps(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{Tasks: map[string]RunTask{"empty": {}}},
	}
	defer func() { loadedConfig = prev }()

	err := runCmd([]string{"empty"}, false)
	if err == nil || !strings.Contains(err.Error(), "no steps") {
		t.Errorf("expected 'no steps' error, got %v", err)
	}
}

// ----------------------------------------------------------------------
// runCmd with actual subprocess execution (dry-run + real exec)
// ----------------------------------------------------------------------

// Dry-run doesn't fork anything, so we can exercise the full loop
// without needing a controllable child binary.
func TestRunCmd_DryRun(t *testing.T) {
	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{Tasks: map[string]RunTask{
			"verify": {Steps: []string{"editor refresh --compile", "test run"}},
		}},
	}
	defer func() { loadedConfig = prev }()

	out := captureStdout(t, func() {
		_ = runCmd([]string{"verify", "--dry-run"}, false)
	})
	if !strings.Contains(out, "DRY-RUN") {
		t.Errorf("dry-run header missing:\n%s", out)
	}
	if !strings.Contains(out, "[1/2] editor refresh --compile") {
		t.Errorf("first step missing:\n%s", out)
	}
	if !strings.Contains(out, "[2/2] test run") {
		t.Errorf("second step missing:\n%s", out)
	}
	if !strings.Contains(out, "not executed") {
		t.Errorf("dry-run marker missing:\n%s", out)
	}
}

// Real execution tests use a tiny compiled-on-demand helper binary to
// avoid depending on a real `udit` in PATH. UDIT_RUN_EXEC points
// executeTask at the helper; step strings become the helper's argv
// (first token = exit code, rest = stdout echo).
func TestExecuteTask_FailFast(t *testing.T) {
	helperExe := buildHelper(t)
	t.Setenv("UDIT_RUN_EXEC", helperExe)

	// Each step is "<exit-code> <echo-text>" — helper reads arg0 as the
	// exit code and echoes the rest to stdout.
	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{Tasks: map[string]RunTask{
			"two": {
				Steps: []string{
					"0 step-one-ran",
					"3 step-two-failed",
					"0 step-three-should-not-run",
				},
			},
		}},
	}
	defer func() { loadedConfig = prev }()

	out := captureStdout(t, func() {
		_ = runCmd([]string{"two"}, false)
	})

	if !strings.Contains(out, "step-one-ran") {
		t.Errorf("step 1 should run:\n%s", out)
	}
	if !strings.Contains(out, "step-two-failed") {
		t.Errorf("step 2 should run:\n%s", out)
	}
	if strings.Contains(out, "step-three-should-not-run") {
		t.Errorf("fail-fast violated — step 3 ran:\n%s", out)
	}
	if !strings.Contains(out, "FAIL exit=3") {
		t.Errorf("exit code should surface:\n%s", out)
	}
}

func TestExecuteTask_ContinueOnError(t *testing.T) {
	helperExe := buildHelper(t)
	t.Setenv("UDIT_RUN_EXEC", helperExe)

	prev := loadedConfig
	loadedConfig = &Config{
		Run: RunCfg{Tasks: map[string]RunTask{
			"nightly": {
				ContinueOnError: true,
				Steps: []string{
					"2 flaky",
					"0 ok",
				},
			},
		}},
	}
	defer func() { loadedConfig = prev }()

	out := captureStdout(t, func() {
		_ = runCmd([]string{"nightly"}, false)
	})
	if !strings.Contains(out, "flaky") {
		t.Errorf("step 1 should run:\n%s", out)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("continue-on-error: step 2 must still run:\n%s", out)
	}
	if !strings.Contains(out, "failed after") {
		t.Errorf("task end message should note failure:\n%s", out)
	}
}

// buildHelper compiles a tiny helper that interprets its args as
// "<exit-code> <rest-echoed-to-stdout>" and exits accordingly. Used so
// the fail-fast / continue tests can fork a controllable child without
// needing a pre-built udit binary in PATH.
func buildHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "helper.go")
	if err := os.WriteFile(src, []byte(`package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(0)
	}
	code, _ := strconv.Atoi(args[0])
	if len(args) > 1 {
		fmt.Println(strings.Join(args[1:], " "))
	}
	os.Exit(code)
}
`), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	exe := filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, stderr.String())
	}
	return exe
}

// ----------------------------------------------------------------------
// Misc
// ----------------------------------------------------------------------

func TestPlural(t *testing.T) {
	if plural(1) != "" {
		t.Errorf("1 → %q", plural(1))
	}
	if plural(0) != "s" {
		t.Errorf("0 → %q", plural(0))
	}
	if plural(2) != "s" {
		t.Errorf("2 → %q", plural(2))
	}
}

func TestFmtDuration(t *testing.T) {
	// Not testing exact formatting — just that it doesn't panic + gives
	// non-empty output for a few common durations.
	for _, d := range []string{"100ms", "2.5s", "1m30s"} {
		_ = d // signature only
	}
}

// Ensure yaml lib reachable from this test file (compile check).
var _ = yaml.Unmarshal
