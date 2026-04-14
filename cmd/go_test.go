package cmd

import "testing"

func TestGoCmd_FindNoFilters(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"find"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "find" {
		t.Errorf("expected action=find, got %v", (*params)["action"])
	}
	// No filters passed → nothing beyond action. Sending zero-value defaults
	// would lie about what the user asked for.
	for _, k := range []string{"name", "tag", "component", "include_inactive", "limit", "offset"} {
		if _, set := (*params)[k]; set {
			t.Errorf("expected %q unset when flag omitted, got %v", k, (*params)[k])
		}
	}
}

func TestGoCmd_FindAllFilters(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{
		"find",
		"--name", "Enemy*",
		"--tag", "Enemy",
		"--component", "Rigidbody",
		"--active-only",
		"--limit", "50",
		"--offset", "10",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["name"] != "Enemy*" {
		t.Errorf("name: got %v", (*params)["name"])
	}
	if (*params)["tag"] != "Enemy" {
		t.Errorf("tag: got %v", (*params)["tag"])
	}
	if (*params)["component"] != "Rigidbody" {
		t.Errorf("component: got %v", (*params)["component"])
	}
	if (*params)["include_inactive"] != false {
		t.Errorf("include_inactive: got %v", (*params)["include_inactive"])
	}
	if (*params)["limit"] != 50 {
		t.Errorf("limit: got %v (%T)", (*params)["limit"], (*params)["limit"])
	}
	if (*params)["offset"] != 10 {
		t.Errorf("offset: got %v", (*params)["offset"])
	}
}

func TestGoCmd_FindInvalidLimit(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"find", "--limit", "many"}, send); err == nil {
		t.Error("expected error for non-integer --limit")
	}
}

func TestGoCmd_FindInvalidOffset(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"find", "--offset", "start"}, send); err == nil {
		t.Error("expected error for non-integer --offset")
	}
}

func TestGoCmd_Inspect(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"inspect", "go:abcd1234"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "inspect" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v", (*params)["id"])
	}
}

func TestGoCmd_InspectMissingId(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"inspect"}, send); err == nil {
		t.Error("expected error when id is missing")
	}
	// Passing a non-go: positional is also rejected — agents should see
	// a clear error, not a silent send with id="" that the server rejects
	// generically.
	if _, err := goCmd([]string{"inspect", "Player"}, send); err == nil {
		t.Error("expected error when positional is not a stable ID")
	}
}

func TestGoCmd_Path(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"path", "go:9598abb1"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "path" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:9598abb1" {
		t.Errorf("id: got %v", (*params)["id"])
	}
}

func TestGoCmd_PathMissingId(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"path"}, send); err == nil {
		t.Error("expected error when id is missing")
	}
}

// An --extra flag sitting before the positional ID should not accidentally
// consume the ID as its value. firstStableId skips flag values explicitly.
func TestGoCmd_InspectFlagDoesNotEatId(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"inspect", "--something", "foo", "go:abcd1234"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v — flag value was not skipped correctly", (*params)["id"])
	}
}

func TestGoCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestGoCmd_UnknownAction(t *testing.T) {
	// "destroy" used to be a valid stand-in for "any unknown action" in this
	// test, but it is now a real mutation. Use something that is not on the
	// switch so the test keeps validating the default branch.
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"obliterate"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}

// --- Mutation tests -------------------------------------------------------

func TestGoCmd_Create(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"create", "--name", "Boss"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "create" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["name"] != "Boss" {
		t.Errorf("name: got %v", (*params)["name"])
	}
	for _, k := range []string{"parent", "pos", "dry_run"} {
		if _, set := (*params)[k]; set {
			t.Errorf("%q should be unset by default, got %v", k, (*params)[k])
		}
	}
}

func TestGoCmd_CreateMissingName(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"create"}, send); err == nil {
		t.Error("expected error when --name is missing")
	}
}

func TestGoCmd_CreateAllOptions(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{
		"create",
		"--name", "Minion",
		"--parent", "go:abcd1234",
		"--pos", "1,2,3",
		"--dry-run",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["parent"] != "go:abcd1234" {
		t.Errorf("parent: got %v", (*params)["parent"])
	}
	if (*params)["pos"] != "1,2,3" {
		t.Errorf("pos: got %v", (*params)["pos"])
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
}

func TestGoCmd_Destroy(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"destroy", "go:abcd1234"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "destroy" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v", (*params)["id"])
	}
}

func TestGoCmd_DestroyDryRun(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"destroy", "go:abcd1234", "--dry-run"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
}

func TestGoCmd_DestroyMissingId(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"destroy"}, send); err == nil {
		t.Error("expected error when id is missing")
	}
}

func TestGoCmd_MoveToRoot(t *testing.T) {
	// No --parent means "move to scene root" — the C# side treats absent
	// parent as null, so we omit the param entirely.
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"move", "go:abcd1234"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "move" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if _, set := (*params)["parent"]; set {
		t.Errorf("parent should be unset for move-to-root, got %v", (*params)["parent"])
	}
}

func TestGoCmd_MoveWithParent(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"move", "go:abcd1234", "--parent", "go:9598abb1"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["parent"] != "go:9598abb1" {
		t.Errorf("parent: got %v", (*params)["parent"])
	}
}

func TestGoCmd_Rename(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"rename", "go:abcd1234", "Renamed_Boss"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "rename" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["new_name"] != "Renamed_Boss" {
		t.Errorf("new_name: got %v", (*params)["new_name"])
	}
}

func TestGoCmd_RenameMissingNewName(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"rename", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when new name is missing")
	}
}

func TestGoCmd_RenameMissingId(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"rename", "Renamed"}, send); err == nil {
		t.Error("expected error when go: id is missing")
	}
}

func TestGoCmd_SetActiveTrue(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"setactive", "go:abcd1234", "--active", "true"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "setactive" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["active"] != true {
		t.Errorf("active: got %v", (*params)["active"])
	}
}

func TestGoCmd_SetActiveFalse(t *testing.T) {
	send, params := mockSend("manage_game_object", t)
	_, err := goCmd([]string{"setactive", "go:abcd1234", "--active", "false"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["active"] != false {
		t.Errorf("active: got %v", (*params)["active"])
	}
}

// Spelling variants that are obvious enough to accept rather than reject.
func TestGoCmd_SetActiveAcceptsCommonSpellings(t *testing.T) {
	cases := map[string]bool{
		"yes": true, "on": true, "1": true,
		"no": false, "off": false, "0": false,
	}
	for spelling, expected := range cases {
		send, params := mockSend("manage_game_object", t)
		_, err := goCmd([]string{"setactive", "go:abcd1234", "--active", spelling}, send)
		if err != nil {
			t.Errorf("spelling %q: unexpected error: %v", spelling, err)
			continue
		}
		if (*params)["active"] != expected {
			t.Errorf("spelling %q: got active=%v, want %v", spelling, (*params)["active"], expected)
		}
	}
}

func TestGoCmd_SetActiveRejectsNonsense(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"setactive", "go:abcd1234", "--active", "maybe"}, send); err == nil {
		t.Error("expected error for nonsense --active value")
	}
}

func TestGoCmd_SetActiveMissingFlag(t *testing.T) {
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"setactive", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when --active is missing")
	}
}
