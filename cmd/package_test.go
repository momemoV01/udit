package cmd

import "testing"

// --- list ---------------------------------------------------------------

func TestPackageCmd_List(t *testing.T) {
	send, params := mockSend("manage_package", t)
	if _, err := packageCmd([]string{"list"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "list" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	// Default list is manifest-only — no --resolved should appear.
	if _, set := (*params)["resolved"]; set {
		t.Errorf("resolved should not be sent without --resolved, got %v", (*params)["resolved"])
	}
}

func TestPackageCmd_ListResolved(t *testing.T) {
	send, params := mockSend("manage_package", t)
	_, err := packageCmd([]string{"list", "--resolved"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["resolved"] != true {
		t.Errorf("resolved: got %v, want true", (*params)["resolved"])
	}
}

// --- resolve ------------------------------------------------------------

func TestPackageCmd_Resolve(t *testing.T) {
	send, params := mockSend("manage_package", t)
	if _, err := packageCmd([]string{"resolve"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "resolve" {
		t.Errorf("action: got %v", (*params)["action"])
	}
}

// --- add ----------------------------------------------------------------

func TestPackageCmd_AddBasic(t *testing.T) {
	send, params := mockSend("manage_package", t)
	_, err := packageCmd([]string{"add", "com.unity.cinemachine"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "add" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["name"] != "com.unity.cinemachine" {
		t.Errorf("name: got %v", (*params)["name"])
	}
}

// `name@version` is not parsed on the Go side — Unity's Client.Add does the
// version parsing. We just forward the raw string.
func TestPackageCmd_AddVersion(t *testing.T) {
	send, params := mockSend("manage_package", t)
	_, err := packageCmd([]string{"add", "com.unity.cinemachine@2.9.7"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["name"] != "com.unity.cinemachine@2.9.7" {
		t.Errorf("name should pass through version suffix unchanged, got %v", (*params)["name"])
	}
}

// Same for git URLs — Client.Add accepts the URL form directly.
func TestPackageCmd_AddGitUrl(t *testing.T) {
	send, params := mockSend("manage_package", t)
	url := "https://github.com/dbrizov/NaughtyAttributes.git"
	_, err := packageCmd([]string{"add", url}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["name"] != url {
		t.Errorf("name should pass through git URL unchanged, got %v", (*params)["name"])
	}
}

func TestPackageCmd_AddMissingName(t *testing.T) {
	send, _ := mockSend("manage_package", t)
	if _, err := packageCmd([]string{"add"}, send); err == nil {
		t.Error("expected error for missing name")
	}
}

// --- remove -------------------------------------------------------------

func TestPackageCmd_Remove(t *testing.T) {
	send, params := mockSend("manage_package", t)
	_, err := packageCmd([]string{"remove", "com.unity.cinemachine"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "remove" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["name"] != "com.unity.cinemachine" {
		t.Errorf("name: got %v", (*params)["name"])
	}
}

func TestPackageCmd_RemoveMissingName(t *testing.T) {
	send, _ := mockSend("manage_package", t)
	if _, err := packageCmd([]string{"remove"}, send); err == nil {
		t.Error("expected error for missing name")
	}
}

// --- info ---------------------------------------------------------------

func TestPackageCmd_Info(t *testing.T) {
	send, params := mockSend("manage_package", t)
	_, err := packageCmd([]string{"info", "com.unity.cinemachine"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "info" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["name"] != "com.unity.cinemachine" {
		t.Errorf("name: got %v", (*params)["name"])
	}
}

// --- search -------------------------------------------------------------

func TestPackageCmd_Search(t *testing.T) {
	send, params := mockSend("manage_package", t)
	_, err := packageCmd([]string{"search", "cinemachine"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "search" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	// search uses `query` (not `name`) on the wire — that's the C# parameter name.
	if (*params)["query"] != "cinemachine" {
		t.Errorf("query: got %v", (*params)["query"])
	}
}

func TestPackageCmd_SearchMissingQuery(t *testing.T) {
	send, _ := mockSend("manage_package", t)
	if _, err := packageCmd([]string{"search"}, send); err == nil {
		t.Error("expected error for missing query")
	}
}

// --- routing ------------------------------------------------------------

func TestPackageCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_package", t)
	if _, err := packageCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestPackageCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_package", t)
	if _, err := packageCmd([]string{"upgrade"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}

// --- positional helper --------------------------------------------------

// firstPositional must skip `--key value` pairs so a flag's value isn't
// mistaken for the package id. parseSubFlags treats next-arg-without-`--`
// as a value; firstPositional mirrors that.
func TestPackageFirstPositional_SkipsFlagValue(t *testing.T) {
	got := packageFirstPositional([]string{"--scope", "preview", "com.unity.cinemachine"})
	if got != "com.unity.cinemachine" {
		t.Errorf("expected to skip --scope's value, got %q", got)
	}
}

func TestPackageFirstPositional_BoolFlagThenPositional(t *testing.T) {
	// `--resolved` with no value, followed by a positional. The boolean
	// switch must not consume the next arg.
	got := packageFirstPositional([]string{"--resolved", "--limit", "10"})
	if got != "" {
		t.Errorf("only flags here, expected empty, got %q", got)
	}
	got2 := packageFirstPositional([]string{"com.unity.cinemachine", "--resolved"})
	if got2 != "com.unity.cinemachine" {
		t.Errorf("positional first, got %q", got2)
	}
}

func TestPackageFirstPositional_Empty(t *testing.T) {
	if got := packageFirstPositional(nil); got != "" {
		t.Errorf("nil args should return empty, got %q", got)
	}
}
