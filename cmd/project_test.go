package cmd

import "testing"

func TestProjectCmd_Info(t *testing.T) {
	send, params := mockSend("manage_project", t)
	if _, err := projectCmd([]string{"info"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "info" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	// info accepts no options — params should be only action.
	if _, set := (*params)["assets_only"]; set {
		t.Errorf("assets_only should not be sent for info, got %v", (*params)["assets_only"])
	}
}

func TestProjectCmd_Validate(t *testing.T) {
	send, params := mockSend("manage_project", t)
	if _, err := projectCmd([]string{"validate"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "validate" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	// Default is Assets-only on the server side; we don't explicitly send
	// assets_only=true to avoid lying if the server's default ever changes.
	if _, set := (*params)["assets_only"]; set {
		t.Errorf("assets_only should default unset, got %v", (*params)["assets_only"])
	}
}

func TestProjectCmd_ValidateIncludePackages(t *testing.T) {
	send, params := mockSend("manage_project", t)
	_, err := projectCmd([]string{"validate", "--include-packages"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// CLI maps --include-packages -> assets_only: false. The inverse mapping
	// keeps the CLI surface positive ("include packages") while the server
	// param stays a boolean with a meaningful default (true).
	if (*params)["assets_only"] != false {
		t.Errorf("assets_only: got %v, want false", (*params)["assets_only"])
	}
}

func TestProjectCmd_ValidateLimit(t *testing.T) {
	send, params := mockSend("manage_project", t)
	_, err := projectCmd([]string{"validate", "--limit", "25"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["limit"] != 25 {
		t.Errorf("limit: got %v", (*params)["limit"])
	}
}

func TestProjectCmd_ValidateInvalidLimit(t *testing.T) {
	send, _ := mockSend("manage_project", t)
	if _, err := projectCmd([]string{"validate", "--limit", "many"}, send); err == nil {
		t.Error("expected error for non-integer --limit")
	}
}

func TestProjectCmd_Preflight(t *testing.T) {
	send, params := mockSend("manage_project", t)
	if _, err := projectCmd([]string{"preflight"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "preflight" {
		t.Errorf("action: got %v", (*params)["action"])
	}
}

// Preflight accepts the same --include-packages / --limit flags as validate,
// because it extends the same scan. Confirm pass-through.
func TestProjectCmd_PreflightAllFlags(t *testing.T) {
	send, params := mockSend("manage_project", t)
	_, err := projectCmd([]string{"preflight", "--include-packages", "--limit", "50"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["assets_only"] != false {
		t.Errorf("assets_only: got %v", (*params)["assets_only"])
	}
	if (*params)["limit"] != 50 {
		t.Errorf("limit: got %v", (*params)["limit"])
	}
}

func TestProjectCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_project", t)
	if _, err := projectCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestProjectCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_project", t)
	if _, err := projectCmd([]string{"audit"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}
