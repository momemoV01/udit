package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// --- Backward-compatibility: bare `udit test` routes to `run_tests` ---

func TestTestCmd_BareDefaultsToRun(t *testing.T) {
	send, params := mockSend("run_tests", t)
	if _, err := testCmd(nil, send, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["mode"] != "EditMode" {
		t.Errorf("default mode should be EditMode, got %v", (*params)["mode"])
	}
}

func TestTestCmd_BareWithFlags(t *testing.T) {
	// `udit test --mode PlayMode --filter X` must still work — predates the
	// run/list subcommand split and agents in the wild have scripts using it.
	send, params := mockSend("run_tests", t)
	_, err := testCmd([]string{"--mode", "PlayMode", "--filter", "MyNs"}, send, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["mode"] != "PlayMode" {
		t.Errorf("mode: got %v", (*params)["mode"])
	}
	if (*params)["filter"] != "MyNs" {
		t.Errorf("filter: got %v", (*params)["filter"])
	}
}

// --- Explicit `run` subcommand -----------------------------------------

func TestTestCmd_RunExplicit(t *testing.T) {
	send, params := mockSend("run_tests", t)
	if _, err := testCmd([]string{"run"}, send, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["mode"] != "EditMode" {
		t.Errorf("default mode: got %v", (*params)["mode"])
	}
}

func TestTestCmd_RunAllFlags(t *testing.T) {
	send, params := mockSend("run_tests", t)
	_, err := testCmd([]string{
		"run",
		"--mode", "PlayMode",
		"--filter", "Integration.Level1",
		"--output", "test-results/playmode.xml",
	}, send, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["mode"] != "PlayMode" {
		t.Errorf("mode: got %v", (*params)["mode"])
	}
	if (*params)["filter"] != "Integration.Level1" {
		t.Errorf("filter: got %v", (*params)["filter"])
	}
	// --output is resolved to an absolute path against the CLI's CWD so the
	// file lands where the caller typed the command, not in Unity's CWD.
	out, _ := (*params)["output"].(string)
	if !filepath.IsAbs(out) {
		t.Errorf("output should be absolute, got %q", out)
	}
	if !strings.HasSuffix(filepath.ToSlash(out), "test-results/playmode.xml") {
		t.Errorf("output should end with the given relative path, got %q", out)
	}
}

// TestTestCmd_RunOutputAbsolute verifies an already-absolute --output is
// forwarded unchanged (no double-joining against CWD).
func TestTestCmd_RunOutputAbsolute(t *testing.T) {
	send, params := mockSend("run_tests", t)
	abs, err := filepath.Abs(filepath.Join(t.TempDir(), "report.xml"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := testCmd([]string{"run", "--output", abs}, send, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*params)["output"]; got != abs {
		t.Errorf("absolute output should pass through unchanged: got %v want %v", got, abs)
	}
}

func TestTestCmd_RunInvalidMode(t *testing.T) {
	send, _ := mockSend("run_tests", t)
	if _, err := testCmd([]string{"run", "--mode", "Invalid"}, send, 0); err == nil {
		t.Error("expected error for invalid --mode")
	}
}

// --- `list` subcommand --------------------------------------------------

func TestTestCmd_ListDefault(t *testing.T) {
	send, params := mockSend("list_tests", t)
	if _, err := testCmd([]string{"list"}, send, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mode unset: server defaults to EditMode. We don't send it explicitly
	// so the default on the server side is the source of truth.
	if _, set := (*params)["mode"]; set {
		t.Errorf("mode should default (unset), got %v", (*params)["mode"])
	}
}

func TestTestCmd_ListPlayMode(t *testing.T) {
	send, params := mockSend("list_tests", t)
	_, err := testCmd([]string{"list", "--mode", "PlayMode"}, send, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["mode"] != "PlayMode" {
		t.Errorf("mode: got %v", (*params)["mode"])
	}
}

func TestTestCmd_ListInvalidMode(t *testing.T) {
	send, _ := mockSend("list_tests", t)
	if _, err := testCmd([]string{"list", "--mode", "Invalid"}, send, 0); err == nil {
		t.Error("expected error for invalid --mode")
	}
}

// --- Routing -------------------------------------------------------------

func TestTestCmd_UnknownSubcommand(t *testing.T) {
	send, _ := mockSend("run_tests", t)
	if _, err := testCmd([]string{"cover"}, send, 0); err == nil {
		t.Error("expected error for unknown subcommand")
	}
}
