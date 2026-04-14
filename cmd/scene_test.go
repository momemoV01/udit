package cmd

import "testing"

func TestSceneCmd_List(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"list"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "list" {
		t.Errorf("expected action=list, got %v", (*params)["action"])
	}
}

func TestSceneCmd_Active(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"active"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "active" {
		t.Errorf("expected action=active, got %v", (*params)["action"])
	}
}

func TestSceneCmd_Open(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	_, err := sceneCmd([]string{"open", "Assets/Scenes/Main.unity"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "open" {
		t.Errorf("expected action=open, got %v", (*params)["action"])
	}
	if (*params)["path"] != "Assets/Scenes/Main.unity" {
		t.Errorf("expected path passthrough, got %v", (*params)["path"])
	}
	if (*params)["force"] != false {
		t.Errorf("expected force=false by default, got %v", (*params)["force"])
	}
}

func TestSceneCmd_OpenWithForce(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	_, err := sceneCmd([]string{"open", "Assets/Scenes/Menu.unity", "--force"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["force"] != true {
		t.Errorf("expected force=true, got %v", (*params)["force"])
	}
	if (*params)["path"] != "Assets/Scenes/Menu.unity" {
		t.Errorf("expected path passthrough, got %v", (*params)["path"])
	}
}

// Flag order must not eat the positional: `scene open --force Assets/...` is
// a common keystroke pattern and should be accepted the same as the reverse.
func TestSceneCmd_OpenForceBeforePath(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	_, err := sceneCmd([]string{"open", "--force", "Assets/Scenes/Menu.unity"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["path"] != "Assets/Scenes/Menu.unity" {
		t.Errorf("expected path passthrough with flag-first order, got %v", (*params)["path"])
	}
	if (*params)["force"] != true {
		t.Errorf("expected force=true, got %v", (*params)["force"])
	}
}

func TestSceneCmd_OpenMissingPath(t *testing.T) {
	send, _ := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"open"}, send); err == nil {
		t.Error("expected error when path is missing")
	}
	if _, err := sceneCmd([]string{"open", "--force"}, send); err == nil {
		t.Error("expected error when only flags (no path) are supplied")
	}
}

func TestSceneCmd_Save(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"save"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "save" {
		t.Errorf("expected action=save, got %v", (*params)["action"])
	}
}

func TestSceneCmd_Reload(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"reload"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "reload" {
		t.Errorf("expected action=reload, got %v", (*params)["action"])
	}
	if (*params)["force"] != false {
		t.Errorf("expected force=false by default, got %v", (*params)["force"])
	}
}

func TestSceneCmd_ReloadWithForce(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	_, err := sceneCmd([]string{"reload", "--force"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["force"] != true {
		t.Errorf("expected force=true, got %v", (*params)["force"])
	}
}

func TestSceneCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_scene", t)
	if _, err := sceneCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestSceneCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"merge"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestSceneCmd_Tree(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"tree"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "tree" {
		t.Errorf("expected action=tree, got %v", (*params)["action"])
	}
	// No depth or active-only → omit both so the server applies its defaults
	// (unlimited depth, include inactive). Sending explicit defaults would be
	// a lie if the server default ever changes.
	if _, set := (*params)["depth"]; set {
		t.Errorf("depth should not be set when flag omitted, got %v", (*params)["depth"])
	}
	if _, set := (*params)["include_inactive"]; set {
		t.Errorf("include_inactive should not be set when flag omitted, got %v", (*params)["include_inactive"])
	}
}

func TestSceneCmd_TreeWithDepth(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"tree", "--depth", "3"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["depth"] != 3 {
		t.Errorf("expected depth=3 (int), got %v (%T)", (*params)["depth"], (*params)["depth"])
	}
}

func TestSceneCmd_TreeWithDepthZero(t *testing.T) {
	// depth=0 is a meaningful value (roots only), so it must survive the
	// flag parser's "0 is falsy" temptation. This guards against a regression.
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"tree", "--depth", "0"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["depth"] != 0 {
		t.Errorf("expected depth=0, got %v", (*params)["depth"])
	}
}

func TestSceneCmd_TreeWithNegativeDepth(t *testing.T) {
	// Negative = unlimited on the C# side. parseSubFlags treats "-1" as a
	// value (not a flag name) only if preceded by --depth; verify the round-trip.
	send, params := mockSend("manage_scene", t)
	_, err := sceneCmd([]string{"tree", "--depth", "-1"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["depth"] != -1 {
		t.Errorf("expected depth=-1, got %v", (*params)["depth"])
	}
}

func TestSceneCmd_TreeInvalidDepth(t *testing.T) {
	send, _ := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"tree", "--depth", "deep"}, send); err == nil {
		t.Error("expected error for non-integer --depth")
	}
}

func TestSceneCmd_TreeActiveOnly(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	if _, err := sceneCmd([]string{"tree", "--active-only"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["include_inactive"] != false {
		t.Errorf("expected include_inactive=false, got %v", (*params)["include_inactive"])
	}
}

func TestSceneCmd_TreeAllOptions(t *testing.T) {
	send, params := mockSend("manage_scene", t)
	_, err := sceneCmd([]string{"tree", "--depth", "2", "--active-only"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "tree" {
		t.Errorf("expected action=tree, got %v", (*params)["action"])
	}
	if (*params)["depth"] != 2 {
		t.Errorf("expected depth=2, got %v", (*params)["depth"])
	}
	if (*params)["include_inactive"] != false {
		t.Errorf("expected include_inactive=false, got %v", (*params)["include_inactive"])
	}
}
