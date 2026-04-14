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
