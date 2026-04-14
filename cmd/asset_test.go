package cmd

import "testing"

func TestAssetCmd_FindNoFilters(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	if _, err := assetCmd([]string{"find"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "find" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	for _, k := range []string{"type", "label", "name", "folder", "limit", "offset"} {
		if _, set := (*params)[k]; set {
			t.Errorf("%q should be unset when flag omitted, got %v", k, (*params)[k])
		}
	}
}

func TestAssetCmd_FindAllFilters(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{
		"find",
		"--type", "Prefab",
		"--label", "boss",
		"--name", "Enemy*",
		"--folder", "Assets/Prefabs,Assets/Enemies",
		"--limit", "50",
		"--offset", "10",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["type"] != "Prefab" {
		t.Errorf("type: got %v", (*params)["type"])
	}
	if (*params)["label"] != "boss" {
		t.Errorf("label: got %v", (*params)["label"])
	}
	if (*params)["name"] != "Enemy*" {
		t.Errorf("name: got %v", (*params)["name"])
	}
	if (*params)["folder"] != "Assets/Prefabs,Assets/Enemies" {
		t.Errorf("folder: got %v", (*params)["folder"])
	}
	if (*params)["limit"] != 50 {
		t.Errorf("limit: got %v", (*params)["limit"])
	}
	if (*params)["offset"] != 10 {
		t.Errorf("offset: got %v", (*params)["offset"])
	}
}

func TestAssetCmd_FindInvalidLimit(t *testing.T) {
	send, _ := mockSend("manage_asset", t)
	if _, err := assetCmd([]string{"find", "--limit", "lots"}, send); err == nil {
		t.Error("expected error for non-integer --limit")
	}
}

func TestAssetCmd_Inspect(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{"inspect", "Assets/Scenes/Main.unity"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "inspect" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["path"] != "Assets/Scenes/Main.unity" {
		t.Errorf("path: got %v", (*params)["path"])
	}
}

func TestAssetCmd_InspectMissingPath(t *testing.T) {
	send, _ := mockSend("manage_asset", t)
	if _, err := assetCmd([]string{"inspect"}, send); err == nil {
		t.Error("expected error when path is missing")
	}
}

// Packages/ paths are valid too — the AssetDatabase indexes them.
func TestAssetCmd_InspectPackagesPath(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{"inspect", "Packages/com.example.pkg/Runtime/Thing.cs"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["path"] != "Packages/com.example.pkg/Runtime/Thing.cs" {
		t.Errorf("path: got %v", (*params)["path"])
	}
}

func TestAssetCmd_Dependencies(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{"dependencies", "Assets/Scenes/Main.unity"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "dependencies" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["path"] != "Assets/Scenes/Main.unity" {
		t.Errorf("path: got %v", (*params)["path"])
	}
	if _, set := (*params)["recursive"]; set {
		t.Errorf("recursive should be unset when omitted, got %v", (*params)["recursive"])
	}
}

func TestAssetCmd_DependenciesRecursive(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{"dependencies", "Assets/Scenes/Main.unity", "--recursive"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["recursive"] != true {
		t.Errorf("recursive: got %v", (*params)["recursive"])
	}
}

func TestAssetCmd_References(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{
		"references", "Assets/Prefabs/Player.prefab",
		"--limit", "25", "--offset", "5",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["path"] != "Assets/Prefabs/Player.prefab" {
		t.Errorf("path: got %v", (*params)["path"])
	}
	if (*params)["limit"] != 25 {
		t.Errorf("limit: got %v", (*params)["limit"])
	}
	if (*params)["offset"] != 5 {
		t.Errorf("offset: got %v", (*params)["offset"])
	}
}

func TestAssetCmd_ReferencesMissingPath(t *testing.T) {
	send, _ := mockSend("manage_asset", t)
	if _, err := assetCmd([]string{"references"}, send); err == nil {
		t.Error("expected error when path is missing")
	}
}

func TestAssetCmd_Guid(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{"guid", "Assets/Scenes/Main.unity"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "guid" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["path"] != "Assets/Scenes/Main.unity" {
		t.Errorf("path: got %v", (*params)["path"])
	}
}

func TestAssetCmd_Path(t *testing.T) {
	send, params := mockSend("manage_asset", t)
	_, err := assetCmd([]string{"path", "8c9cfa26abfee488c85f1582747f6a02"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "path" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["guid"] != "8c9cfa26abfee488c85f1582747f6a02" {
		t.Errorf("guid: got %v", (*params)["guid"])
	}
}

func TestAssetCmd_PathMissingGuid(t *testing.T) {
	send, _ := mockSend("manage_asset", t)
	if _, err := assetCmd([]string{"path"}, send); err == nil {
		t.Error("expected error when guid is missing")
	}
}

func TestAssetCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_asset", t)
	if _, err := assetCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestAssetCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_asset", t)
	if _, err := assetCmd([]string{"delete"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}
