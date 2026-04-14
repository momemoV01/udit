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
	send, _ := mockSend("manage_game_object", t)
	if _, err := goCmd([]string{"destroy"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}
