package cmd

import "testing"

func TestPrefabCmd_Instantiate(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	_, err := prefabCmd([]string{"instantiate", "Assets/Prefabs/Enemy.prefab"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "instantiate" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["path"] != "Assets/Prefabs/Enemy.prefab" {
		t.Errorf("path: got %v", (*params)["path"])
	}
	for _, k := range []string{"parent", "pos", "dry_run"} {
		if _, set := (*params)[k]; set {
			t.Errorf("%q should be unset by default, got %v", k, (*params)[k])
		}
	}
}

func TestPrefabCmd_InstantiateAllOptions(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	_, err := prefabCmd([]string{
		"instantiate", "Assets/Prefabs/Enemy.prefab",
		"--parent", "go:abcd1234",
		"--pos", "5,0,0",
		"--dry-run",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["parent"] != "go:abcd1234" {
		t.Errorf("parent: got %v", (*params)["parent"])
	}
	if (*params)["pos"] != "5,0,0" {
		t.Errorf("pos: got %v", (*params)["pos"])
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
}

func TestPrefabCmd_InstantiateMissingPath(t *testing.T) {
	send, _ := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"instantiate"}, send); err == nil {
		t.Error("expected error when path is missing")
	}
}

func TestPrefabCmd_InstantiatePackagesPath(t *testing.T) {
	// Packages/ paths are as valid as Assets/ ones.
	send, params := mockSend("manage_prefab", t)
	_, err := prefabCmd([]string{"instantiate", "Packages/com.example.pkg/Prefabs/Sample.prefab"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["path"] != "Packages/com.example.pkg/Prefabs/Sample.prefab" {
		t.Errorf("path: got %v", (*params)["path"])
	}
}

func TestPrefabCmd_Unpack(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"unpack", "go:abcd1234"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "unpack" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v", (*params)["id"])
	}
	if _, set := (*params)["mode"]; set {
		t.Errorf("mode should default (unset) to server's 'root', got %v", (*params)["mode"])
	}
}

func TestPrefabCmd_UnpackCompletely(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	_, err := prefabCmd([]string{"unpack", "go:abcd1234", "--mode", "completely", "--dry-run"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["mode"] != "completely" {
		t.Errorf("mode: got %v", (*params)["mode"])
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
}

func TestPrefabCmd_UnpackMissingId(t *testing.T) {
	send, _ := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"unpack"}, send); err == nil {
		t.Error("expected error when id is missing")
	}
}

func TestPrefabCmd_Apply(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"apply", "go:5678abcd"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "apply" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:5678abcd" {
		t.Errorf("id: got %v", (*params)["id"])
	}
}

func TestPrefabCmd_ApplyDryRun(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	_, err := prefabCmd([]string{"apply", "go:5678abcd", "--dry-run"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
}

func TestPrefabCmd_ApplyMissingId(t *testing.T) {
	send, _ := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"apply"}, send); err == nil {
		t.Error("expected error when id is missing")
	}
}

func TestPrefabCmd_FindInstances(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	_, err := prefabCmd([]string{"find-instances", "Assets/Prefabs/Enemy.prefab"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// CLI accepts the dashed form but dispatches to the server's
	// underscored action key for consistency with the rest of the JSON
	// protocol (underscored snake_case).
	if (*params)["action"] != "find_instances" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["path"] != "Assets/Prefabs/Enemy.prefab" {
		t.Errorf("path: got %v", (*params)["path"])
	}
}

func TestPrefabCmd_FindInstancesUnderscoreSpelling(t *testing.T) {
	send, params := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"find_instances", "Assets/Prefabs/Enemy.prefab"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "find_instances" {
		t.Errorf("action: got %v", (*params)["action"])
	}
}

func TestPrefabCmd_FindInstancesMissingPath(t *testing.T) {
	send, _ := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"find-instances"}, send); err == nil {
		t.Error("expected error when path is missing")
	}
}

func TestPrefabCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_prefab", t)
	if _, err := prefabCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestPrefabCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_prefab", t)
	if _, err := prefabCmd([]string{"detonate"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}
