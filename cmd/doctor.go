package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/momemoV01/udit/internal/client"
)

type checkResult struct {
	Name    string      `json:"name"`
	Status  string      `json:"status"` // "ok", "warn", "fail"
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// doctorEnv abstracts OS/filesystem access so tests can inject fakes.
type doctorEnv struct {
	executable  func() (string, error)
	lookPath    func(string) (string, error)
	userHomeDir func() (string, error)
	getenv      func(string) string
	readFile    func(string) ([]byte, error)
	readDir     func(string) ([]os.DirEntry, error)
	stat        func(string) (os.FileInfo, error)
	scanInst    func() ([]client.Instance, error)
	send        sendFn
	version     string
	configPath  string
	cwd         string
	shellName   string // override for testing
}

func defaultDoctorEnv() *doctorEnv {
	cwd, _ := os.Getwd()
	return &doctorEnv{
		executable:  os.Executable,
		lookPath:    exec.LookPath,
		userHomeDir: os.UserHomeDir,
		getenv:      os.Getenv,
		readFile:    os.ReadFile,
		readDir:     os.ReadDir,
		stat:        os.Stat,
		scanInst:    client.ScanInstances,
		version:     Version,
		configPath:  loadedConfigPath,
		cwd:         cwd,
	}
}

func doctorCmd(args []string, useJSON bool) error {
	env := defaultDoctorEnv()
	checks := runDoctorChecks(env)

	if useJSON {
		data, _ := json.Marshal(map[string]interface{}{
			"checks": checks,
		})
		resp := &client.CommandResponse{
			Success: true,
			Message: fmt.Sprintf("Ran %d checks", len(checks)),
			Data:    data,
		}
		emitJSONResponse(resp, "doctor", nil)
		return nil
	}

	for _, c := range checks {
		var prefix string
		switch c.Status {
		case "ok":
			prefix = "[ok]  "
		case "warn":
			prefix = "[warn]"
		case "fail":
			prefix = "[fail]"
		}
		fmt.Printf("%s %s\n", prefix, c.Message)
	}
	return nil
}

func runDoctorChecks(env *doctorEnv) []checkResult {
	var checks []checkResult
	checks = append(checks, checkBinary(env))
	checks = append(checks, checkCompletion(env))
	checks = append(checks, checkConfig(env))
	checks = append(checks, checkInstances(env)...)
	checks = append(checks, checkPitfalls(env))
	return checks
}

// checkBinary reports the binary location, version, and PATH registration.
func checkBinary(env *doctorEnv) checkResult {
	exePath, err := env.executable()
	if err != nil {
		return checkResult{
			Name:    "binary",
			Status:  "fail",
			Message: fmt.Sprintf("Cannot locate binary: %v", err),
		}
	}

	// Check if udit is findable on PATH
	which, lookErr := env.lookPath("udit")
	onPath := lookErr == nil

	details := map[string]interface{}{
		"path":    exePath,
		"version": env.version,
		"on_path": onPath,
	}

	if onPath && which != "" {
		// Compare binaries — try resolving symlinks, fall back to cleaned paths
		resolvedExe, err1 := filepath.EvalSymlinks(exePath)
		resolvedWhich, err2 := filepath.EvalSymlinks(which)
		if err1 != nil {
			resolvedExe = filepath.Clean(exePath)
		}
		if err2 != nil {
			resolvedWhich = filepath.Clean(which)
		}
		if resolvedExe != resolvedWhich {
			details["path_binary"] = which
			return checkResult{
				Name:    "binary",
				Status:  "warn",
				Message: fmt.Sprintf("udit %s at %s (PATH points to different binary: %s)", env.version, exePath, which),
				Details: details,
			}
		}
	}

	if !onPath {
		return checkResult{
			Name:    "binary",
			Status:  "warn",
			Message: fmt.Sprintf("udit %s at %s (not found on PATH)", env.version, exePath),
			Details: details,
		}
	}

	return checkResult{
		Name:    "binary",
		Status:  "ok",
		Message: fmt.Sprintf("udit %s at %s", env.version, exePath),
		Details: details,
	}
}

// checkCompletion looks for the udit completion marker in the user's shell rc.
func checkCompletion(env *doctorEnv) checkResult {
	home, err := env.userHomeDir()
	if err != nil {
		return checkResult{
			Name:    "completion",
			Status:  "warn",
			Message: "Cannot determine home directory; skipping completion check",
		}
	}

	shell := env.shellName
	if shell == "" {
		shell = detectShell()
	}

	marker := "# >>> udit completion >>>"
	var rcFile string

	switch shell {
	case "bash":
		rcFile = filepath.Join(home, ".bashrc")
	case "zsh":
		rcFile = filepath.Join(home, ".zshrc")
	case "fish":
		rcFile = filepath.Join(home, ".config", "fish", "conf.d", "udit.fish")
	case "powershell":
		// PowerShell profile path varies; check common locations
		if p := env.getenv("PROFILE"); p != "" {
			rcFile = p
		} else {
			// Best-effort: look for WindowsPowerShell profile
			rcFile = filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
		}
	default:
		return checkResult{
			Name:    "completion",
			Status:  "warn",
			Message: fmt.Sprintf("Cannot detect shell (got %q); skipping completion check", shell),
		}
	}

	data, err := env.readFile(rcFile)
	if err != nil {
		return checkResult{
			Name:    "completion",
			Status:  "warn",
			Message: fmt.Sprintf("Shell rc not found (%s); completion may not be installed", rcFile),
			Details: map[string]string{"shell": shell, "rc_file": rcFile},
		}
	}

	if strings.Contains(string(data), marker) {
		return checkResult{
			Name:    "completion",
			Status:  "ok",
			Message: fmt.Sprintf("Shell completion installed (%s)", shell),
			Details: map[string]string{"shell": shell, "rc_file": rcFile},
		}
	}

	return checkResult{
		Name:    "completion",
		Status:  "warn",
		Message: fmt.Sprintf("Shell completion not found in %s (run: udit completion install)", rcFile),
		Details: map[string]string{"shell": shell, "rc_file": rcFile},
	}
}

// checkConfig reports whether a .udit.yaml was found.
func checkConfig(env *doctorEnv) checkResult {
	if env.configPath != "" {
		return checkResult{
			Name:    "config",
			Status:  "ok",
			Message: fmt.Sprintf("Config loaded: %s", env.configPath),
			Details: map[string]string{"path": env.configPath},
		}
	}

	return checkResult{
		Name:    "config",
		Status:  "ok",
		Message: "No .udit.yaml found (using defaults)",
	}
}

// checkInstances scans instance files and reports their health.
// Returns one result per instance found, or a single "no instances" result.
func checkInstances(env *doctorEnv) []checkResult {
	instances, err := env.scanInst()
	if err != nil || len(instances) == 0 {
		return []checkResult{{
			Name:    "instances",
			Status:  "warn",
			Message: "No Unity instances found (is Unity running with the Connector?)",
		}}
	}

	var results []checkResult
	for _, inst := range instances {
		age := time.Since(time.UnixMilli(inst.Timestamp))
		stale := age > 3*time.Second

		details := map[string]interface{}{
			"port":             inst.Port,
			"state":            inst.State,
			"project":          inst.ProjectPath,
			"unity_version":    inst.UnityVersion,
			"heartbeat_age_ms": age.Milliseconds(),
			"pid":              inst.PID,
		}

		if inst.State == "stopped" {
			results = append(results, checkResult{
				Name:    "instance",
				Status:  "warn",
				Message: fmt.Sprintf("Unity (port %d): stopped", inst.Port),
				Details: details,
			})
			continue
		}

		if stale {
			results = append(results, checkResult{
				Name:    "instance",
				Status:  "warn",
				Message: fmt.Sprintf("Unity (port %d): not responding (heartbeat %s ago)", inst.Port, age.Truncate(time.Second)),
				Details: details,
			})
			continue
		}

		// Instance is alive — try connectivity if send is available
		if env.send != nil {
			resp, sendErr := env.send("list", nil)
			if sendErr != nil {
				details["connectivity"] = "failed"
				details["error"] = sendErr.Error()
				results = append(results, checkResult{
					Name:    "instance",
					Status:  "warn",
					Message: fmt.Sprintf("Unity (port %d): heartbeat ok but HTTP failed: %v", inst.Port, sendErr),
					Details: details,
				})
				continue
			}
			if resp != nil && resp.Success {
				details["connectivity"] = "ok"
			}
		}

		status := "ok"
		msg := fmt.Sprintf("Unity (port %d): %s — %s (Unity %s)", inst.Port, inst.State, inst.ProjectPath, inst.UnityVersion)
		if inst.CompileErrors {
			status = "warn"
			msg += " [compile errors]"
		}

		results = append(results, checkResult{
			Name:    "instance",
			Status:  status,
			Message: msg,
			Details: details,
		})
	}

	return results
}

// checkPitfalls warns about common configuration issues.
func checkPitfalls(env *doctorEnv) checkResult {
	var warnings []string

	// Check for PATH shadowing: multiple udit binaries
	exePath, _ := env.executable()
	which, err := env.lookPath("udit")
	if err == nil && exePath != "" {
		resolvedExe, _ := filepath.EvalSymlinks(exePath)
		resolvedWhich, _ := filepath.EvalSymlinks(which)
		if resolvedExe != resolvedWhich {
			warnings = append(warnings, fmt.Sprintf("PATH shadow: running %s but PATH resolves to %s", exePath, which))
		}
	}

	// Warn about editor throttling on background
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		warnings = append(warnings, "Tip: if Unity is in the background, the Connector may respond slowly (OS throttles background apps)")
	}

	if len(warnings) == 0 {
		return checkResult{
			Name:    "pitfalls",
			Status:  "ok",
			Message: "No common pitfalls detected",
		}
	}

	return checkResult{
		Name:    "pitfalls",
		Status:  "warn",
		Message: strings.Join(warnings, "; "),
		Details: warnings,
	}
}
