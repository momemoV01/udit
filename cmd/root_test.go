package cmd

import (
	"testing"

	"github.com/momemoV01/udit/internal/client"
)

func mockSend(wantCmd string, t *testing.T) (sendFn, *map[string]interface{}) {
	t.Helper()
	captured := map[string]interface{}{}
	fn := func(cmd string, params interface{}) (*client.CommandResponse, error) {
		if cmd != wantCmd {
			t.Errorf("send called with command %q, want %q", cmd, wantCmd)
		}
		if p, ok := params.(map[string]interface{}); ok {
			for k, v := range p {
				captured[k] = v
			}
		}
		return &client.CommandResponse{Success: true}, nil
	}
	return fn, &captured
}

func TestParseSubFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]string
	}{
		{"empty", nil, map[string]string{}},
		{"key value pair", []string{"--port", "8080"}, map[string]string{"port": "8080"}},
		{"boolean flag", []string{"--wait"}, map[string]string{"wait": "true"}},
		{"mixed", []string{"--port", "8080", "--wait", "--filter", "error"}, map[string]string{"port": "8080", "wait": "true", "filter": "error"}},
		{"consecutive boolean flags", []string{"--wait", "--clear"}, map[string]string{"wait": "true", "clear": "true"}},
		{"non-flag args ignored", []string{"play", "--wait"}, map[string]string{"wait": "true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubFlags(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("parseSubFlags(%v) = %v, want %v", tt.args, got, tt.want)
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseSubFlags(%v)[%q] = %q, want %q", tt.args, k, got[k], v)
				}
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantFlags    []string
		wantCommands []string
	}{
		{"empty", nil, nil, nil},
		{"commands only", []string{"editor", "play"}, nil, []string{"editor", "play"}},
		{"port flag", []string{"--port", "8080", "editor", "play"}, []string{"--port", "8080"}, []string{"editor", "play"}},
		{"project flag", []string{"--project", "myproj", "status"}, []string{"--project", "myproj"}, []string{"status"}},
		{"timeout flag", []string{"exec", "--timeout", "5000", "Time.time"}, []string{"--timeout", "5000"}, []string{"exec", "Time.time"}},
		{"multiple global flags", []string{"--port", "8080", "--timeout", "3000", "exec", "code"}, []string{"--port", "8080", "--timeout", "3000"}, []string{"exec", "code"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, commands := splitArgs(tt.args)
			if !sliceEqual(flags, tt.wantFlags) {
				t.Errorf("splitArgs(%v) flags = %v, want %v", tt.args, flags, tt.wantFlags)
			}
			if !sliceEqual(commands, tt.wantCommands) {
				t.Errorf("splitArgs(%v) commands = %v, want %v", tt.args, commands, tt.wantCommands)
			}
		})
	}
}

func TestBuildParams_IntParsing(t *testing.T) {
	p, err := buildParams([]string{"--lines", "50"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p["lines"] != 50 {
		t.Errorf("expected lines=50, got %v", p["lines"])
	}
}

func TestBuildParams_BoolParsing(t *testing.T) {
	p, err := buildParams([]string{"--clear"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p["clear"] != true {
		t.Errorf("expected clear=true, got %v", p["clear"])
	}
}

func TestBuildParams_StringParsing(t *testing.T) {
	p, err := buildParams([]string{"--filter", "error"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p["filter"] != "error" {
		t.Errorf("expected filter=error, got %v", p["filter"])
	}
}

func TestBuildParams_BaseParams(t *testing.T) {
	p, err := buildParams([]string{"--depth", "5"}, map[string]interface{}{"action": "hierarchy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p["action"] != "hierarchy" {
		t.Errorf("expected action=hierarchy, got %v", p["action"])
	}
	if p["depth"] != 5 {
		t.Errorf("expected depth=5, got %v", p["depth"])
	}
}

// Regression: previously `--filter true` wrongly became bool true because the
// value happened to match the literal "true". After the switch/value-flag
// split, a string value stays a string regardless of its content.
func TestBuildParams_ValueLiteralTrueStaysString(t *testing.T) {
	p, err := buildParams([]string{"--filter", "true"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := p["filter"].(string)
	if !ok {
		t.Fatalf("expected filter to be string, got %T (%v)", p["filter"], p["filter"])
	}
	if got != "true" {
		t.Errorf("expected filter=\"true\", got %q", got)
	}
}

// Regression: `--filter false` likewise must stay a string.
func TestBuildParams_ValueLiteralFalseStaysString(t *testing.T) {
	p, err := buildParams([]string{"--filter", "false"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := p["filter"].(string)
	if !ok {
		t.Fatalf("expected filter to be string, got %T (%v)", p["filter"], p["filter"])
	}
	if got != "false" {
		t.Errorf("expected filter=\"false\", got %q", got)
	}
}

// A switch flag with no value still produces bool true (existing behavior).
func TestBuildParams_SwitchFlagIsBool(t *testing.T) {
	p, err := buildParams([]string{"--wait"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := p["wait"].(bool)
	if !ok || !got {
		t.Errorf("expected wait=true (bool), got %v (%T)", p["wait"], p["wait"])
	}
}

// Switch followed by another switch — both should be bool true.
func TestBuildParams_TwoSwitches(t *testing.T) {
	p, err := buildParams([]string{"--wait", "--clear"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p["wait"] != true {
		t.Errorf("expected wait=true, got %v", p["wait"])
	}
	if p["clear"] != true {
		t.Errorf("expected clear=true, got %v", p["clear"])
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
