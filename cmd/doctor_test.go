package cmd

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/momemoV01/udit/internal/client"
)

func testEnv() *doctorEnv {
	return &doctorEnv{
		executable:  func() (string, error) { return "/usr/local/bin/udit", nil },
		lookPath:    func(string) (string, error) { return "/usr/local/bin/udit", nil },
		userHomeDir: func() (string, error) { return "/home/test", nil },
		getenv:      func(string) string { return "" },
		readFile:    func(string) ([]byte, error) { return nil, os.ErrNotExist },
		readDir:     func(string) ([]os.DirEntry, error) { return nil, os.ErrNotExist },
		stat:        func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		scanInst:    func() ([]client.Instance, error) { return nil, nil },
		version:     "v0.10.0-test",
		configPath:  "",
		cwd:         "/test/project",
		shellName:   "bash",
	}
}

func TestCheckBinary_OK(t *testing.T) {
	env := testEnv()
	r := checkBinary(env)
	if r.Status != "ok" {
		t.Errorf("checkBinary status = %q, want ok", r.Status)
	}
	if r.Name != "binary" {
		t.Errorf("checkBinary name = %q, want binary", r.Name)
	}
}

func TestCheckBinary_NotOnPath(t *testing.T) {
	env := testEnv()
	env.lookPath = func(string) (string, error) { return "", fmt.Errorf("not found") }
	r := checkBinary(env)
	if r.Status != "warn" {
		t.Errorf("checkBinary status = %q, want warn (not on PATH)", r.Status)
	}
}

func TestCheckBinary_PathShadow(t *testing.T) {
	env := testEnv()
	env.executable = func() (string, error) { return "/opt/udit/bin/udit", nil }
	env.lookPath = func(string) (string, error) { return "/usr/bin/udit", nil }
	r := checkBinary(env)
	if r.Status != "warn" {
		t.Errorf("checkBinary status = %q, want warn (shadow)", r.Status)
	}
}

func TestCheckBinary_ExecFail(t *testing.T) {
	env := testEnv()
	env.executable = func() (string, error) { return "", fmt.Errorf("os error") }
	r := checkBinary(env)
	if r.Status != "fail" {
		t.Errorf("checkBinary status = %q, want fail", r.Status)
	}
}

func TestCheckCompletion_Installed(t *testing.T) {
	env := testEnv()
	env.readFile = func(path string) ([]byte, error) {
		return []byte("some rc\n# >>> udit completion >>>\nstuff\n# <<< udit completion <<<\n"), nil
	}
	r := checkCompletion(env)
	if r.Status != "ok" {
		t.Errorf("checkCompletion status = %q, want ok", r.Status)
	}
}

func TestCheckCompletion_NotInstalled(t *testing.T) {
	env := testEnv()
	env.readFile = func(path string) ([]byte, error) {
		return []byte("# normal bashrc\nexport PATH=...\n"), nil
	}
	r := checkCompletion(env)
	if r.Status != "warn" {
		t.Errorf("checkCompletion status = %q, want warn (marker not found)", r.Status)
	}
}

func TestCheckCompletion_NoRcFile(t *testing.T) {
	env := testEnv()
	// readFile returns ErrNotExist (default in testEnv)
	r := checkCompletion(env)
	if r.Status != "warn" {
		t.Errorf("checkCompletion status = %q, want warn (rc not found)", r.Status)
	}
}

func TestCheckConfig_Found(t *testing.T) {
	env := testEnv()
	env.configPath = "/test/project/.udit.yaml"
	r := checkConfig(env)
	if r.Status != "ok" {
		t.Errorf("checkConfig status = %q, want ok", r.Status)
	}
}

func TestCheckConfig_NotFound(t *testing.T) {
	env := testEnv()
	r := checkConfig(env)
	if r.Status != "ok" {
		t.Errorf("checkConfig status = %q, want ok (defaults)", r.Status)
	}
}

func TestCheckInstances_None(t *testing.T) {
	env := testEnv()
	results := checkInstances(env)
	if len(results) != 1 || results[0].Status != "warn" {
		t.Errorf("checkInstances no instances: got %d results, first status = %q", len(results), results[0].Status)
	}
}

func TestCheckInstances_FreshInstance(t *testing.T) {
	env := testEnv()
	env.scanInst = func() ([]client.Instance, error) {
		return []client.Instance{{
			State:        "ready",
			Port:         8590,
			PID:          1234,
			ProjectPath:  "/projects/game",
			UnityVersion: "6000.4.2f1",
			Timestamp:    time.Now().UnixMilli(),
		}}, nil
	}
	results := checkInstances(env)
	if len(results) != 1 {
		t.Fatalf("checkInstances: got %d results, want 1", len(results))
	}
	if results[0].Status != "ok" {
		t.Errorf("checkInstances status = %q, want ok", results[0].Status)
	}
}

func TestCheckInstances_StaleInstance(t *testing.T) {
	env := testEnv()
	env.scanInst = func() ([]client.Instance, error) {
		return []client.Instance{{
			State:     "ready",
			Port:      8590,
			PID:       1234,
			Timestamp: time.Now().Add(-10 * time.Second).UnixMilli(),
		}}, nil
	}
	results := checkInstances(env)
	if len(results) != 1 {
		t.Fatalf("checkInstances: got %d results, want 1", len(results))
	}
	if results[0].Status != "warn" {
		t.Errorf("checkInstances stale: status = %q, want warn", results[0].Status)
	}
}

func TestCheckInstances_StoppedInstance(t *testing.T) {
	env := testEnv()
	env.scanInst = func() ([]client.Instance, error) {
		return []client.Instance{{
			State:     "stopped",
			Port:      8590,
			PID:       1234,
			Timestamp: time.Now().UnixMilli(),
		}}, nil
	}
	results := checkInstances(env)
	if len(results) != 1 {
		t.Fatalf("checkInstances: got %d results, want 1", len(results))
	}
	if results[0].Status != "warn" {
		t.Errorf("checkInstances stopped: status = %q, want warn", results[0].Status)
	}
}

func TestCheckInstances_CompileErrors(t *testing.T) {
	env := testEnv()
	env.scanInst = func() ([]client.Instance, error) {
		return []client.Instance{{
			State:         "ready",
			Port:          8590,
			PID:           1234,
			ProjectPath:   "/projects/game",
			UnityVersion:  "6000.4.2f1",
			Timestamp:     time.Now().UnixMilli(),
			CompileErrors: true,
		}}, nil
	}
	results := checkInstances(env)
	if len(results) != 1 {
		t.Fatalf("checkInstances: got %d results, want 1", len(results))
	}
	if results[0].Status != "warn" {
		t.Errorf("checkInstances compile errors: status = %q, want warn", results[0].Status)
	}
}

func TestRunDoctorChecks_CountAndOrder(t *testing.T) {
	env := testEnv()
	checks := runDoctorChecks(env)
	// Expected: binary, completion, config, instances (1 for no instances), pitfalls
	if len(checks) < 5 {
		t.Errorf("runDoctorChecks: got %d checks, want >= 5", len(checks))
	}
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Name
	}
	// First three should be binary, completion, config
	if names[0] != "binary" {
		t.Errorf("first check = %q, want binary", names[0])
	}
	if names[1] != "completion" {
		t.Errorf("second check = %q, want completion", names[1])
	}
	if names[2] != "config" {
		t.Errorf("third check = %q, want config", names[2])
	}
}
