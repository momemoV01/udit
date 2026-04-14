package cmd

import "testing"

func TestComponentCmd_List(t *testing.T) {
	send, params := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"list", "go:abcd1234"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "list" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v", (*params)["id"])
	}
}

func TestComponentCmd_ListMissingId(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"list"}, send); err == nil {
		t.Error("expected error when id is missing")
	}
	if _, err := componentCmd([]string{"list", "Player"}, send); err == nil {
		t.Error("expected error when positional is not a go: id")
	}
}

func TestComponentCmd_GetWholeComponent(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"get", "go:abcd1234", "Transform"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "get" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v", (*params)["id"])
	}
	if (*params)["type"] != "Transform" {
		t.Errorf("type: got %v", (*params)["type"])
	}
	// No field → omit so the server dumps every property
	if _, set := (*params)["field"]; set {
		t.Errorf("field should be unset when omitted, got %v", (*params)["field"])
	}
}

func TestComponentCmd_GetField(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"get", "go:abcd1234", "Transform", "position"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["field"] != "position" {
		t.Errorf("field: got %v", (*params)["field"])
	}
}

func TestComponentCmd_GetDottedField(t *testing.T) {
	// Dotted paths traverse nested objects on the server — the CLI just
	// passes them through as a string.
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"get", "go:abcd1234", "Transform", "position.x"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["field"] != "position.x" {
		t.Errorf("field: got %v", (*params)["field"])
	}
}

func TestComponentCmd_GetWithIndex(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"get", "go:abcd1234", "BoxCollider", "--index", "2"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["index"] != 2 {
		t.Errorf("index: got %v (%T)", (*params)["index"], (*params)["index"])
	}
	if (*params)["type"] != "BoxCollider" {
		t.Errorf("type: got %v", (*params)["type"])
	}
}

// Index flag in any position must not swallow subsequent positionals.
func TestComponentCmd_GetIndexMidCommand(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"get", "go:abcd1234", "--index", "1", "Transform", "position"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["type"] != "Transform" {
		t.Errorf("type: got %v", (*params)["type"])
	}
	if (*params)["field"] != "position" {
		t.Errorf("field: got %v", (*params)["field"])
	}
	if (*params)["index"] != 1 {
		t.Errorf("index: got %v", (*params)["index"])
	}
}

func TestComponentCmd_GetInvalidIndex(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"get", "go:abcd1234", "Transform", "--index", "many"}, send); err == nil {
		t.Error("expected error for non-integer --index")
	}
}

func TestComponentCmd_GetMissingArgs(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"get"}, send); err == nil {
		t.Error("expected error when id + type are missing")
	}
	if _, err := componentCmd([]string{"get", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when type is missing")
	}
	if _, err := componentCmd([]string{"get", "Transform"}, send); err == nil {
		t.Error("expected error when id (go:) is missing")
	}
}

func TestComponentCmd_Schema(t *testing.T) {
	send, params := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"schema", "Rigidbody"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "schema" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["type"] != "Rigidbody" {
		t.Errorf("type: got %v", (*params)["type"])
	}
	if _, set := (*params)["id"]; set {
		t.Errorf("id should not be set for schema, got %v", (*params)["id"])
	}
}

func TestComponentCmd_SchemaMissingType(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"schema"}, send); err == nil {
		t.Error("expected error when type is missing")
	}
}

// `schema` takes a type name, not a go: id — pure go: positional should fail.
func TestComponentCmd_SchemaRejectsStableId(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"schema", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when only a stable id is supplied to schema")
	}
}

func TestComponentCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestComponentCmd_UnknownAction(t *testing.T) {
	// `delete` was the stand-in for "unknown" here, but does not collide
	// with any current action name. Mutation verbs (add/remove/set/copy)
	// are now real, so keep the canary using a genuinely unused word.
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"obliterate"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}

// --- Mutation tests -------------------------------------------------------

func TestComponentCmd_Add(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"add", "go:abcd1234", "--type", "Rigidbody"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "add" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:abcd1234" {
		t.Errorf("id: got %v", (*params)["id"])
	}
	if (*params)["type"] != "Rigidbody" {
		t.Errorf("type: got %v", (*params)["type"])
	}
}

func TestComponentCmd_AddDryRun(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"add", "go:abcd1234", "--type", "Rigidbody", "--dry-run"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
}

func TestComponentCmd_AddMissingType(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"add", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when --type is missing")
	}
}

func TestComponentCmd_AddMissingId(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"add", "--type", "Rigidbody"}, send); err == nil {
		t.Error("expected error when go: id is missing")
	}
}

func TestComponentCmd_Remove(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"remove", "go:abcd1234", "Rigidbody"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "remove" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["type"] != "Rigidbody" {
		t.Errorf("type: got %v", (*params)["type"])
	}
}

func TestComponentCmd_RemoveWithIndex(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"remove", "go:abcd1234", "BoxCollider", "--index", "2"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["index"] != 2 {
		t.Errorf("index: got %v", (*params)["index"])
	}
}

func TestComponentCmd_RemoveMissingType(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"remove", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when type is missing")
	}
}

func TestComponentCmd_Set(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"set", "go:abcd1234", "Transform", "position", "0,5,0"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "set" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["type"] != "Transform" {
		t.Errorf("type: got %v", (*params)["type"])
	}
	if (*params)["field"] != "position" {
		t.Errorf("field: got %v", (*params)["field"])
	}
	if (*params)["value"] != "0,5,0" {
		t.Errorf("value: got %v", (*params)["value"])
	}
}

func TestComponentCmd_SetWithIndexAndDryRun(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{
		"set", "go:abcd1234", "BoxCollider", "m_Size", "1,1,1",
		"--index", "1", "--dry-run",
	}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["index"] != 1 {
		t.Errorf("index: got %v", (*params)["index"])
	}
	if (*params)["dry_run"] != true {
		t.Errorf("dry_run: got %v", (*params)["dry_run"])
	}
	if (*params)["value"] != "1,1,1" {
		t.Errorf("value: got %v", (*params)["value"])
	}
}

func TestComponentCmd_SetMissingArgs(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	// Missing value
	if _, err := componentCmd([]string{"set", "go:abcd1234", "Transform", "position"}, send); err == nil {
		t.Error("expected error when value is missing")
	}
	// Missing field + value
	if _, err := componentCmd([]string{"set", "go:abcd1234", "Transform"}, send); err == nil {
		t.Error("expected error when field and value are missing")
	}
	// Missing type + field + value
	if _, err := componentCmd([]string{"set", "go:abcd1234"}, send); err == nil {
		t.Error("expected error when type/field/value are all missing")
	}
	// Missing go: id
	if _, err := componentCmd([]string{"set", "Transform", "position", "0,0,0"}, send); err == nil {
		t.Error("expected error when go: id is missing")
	}
}

func TestComponentCmd_Copy(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"copy", "go:src00000", "Rigidbody", "go:dst00000"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "copy" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if (*params)["id"] != "go:src00000" {
		t.Errorf("id (source): got %v", (*params)["id"])
	}
	if (*params)["dst_id"] != "go:dst00000" {
		t.Errorf("dst_id: got %v", (*params)["dst_id"])
	}
	if (*params)["type"] != "Rigidbody" {
		t.Errorf("type: got %v", (*params)["type"])
	}
}

// Positional order must not matter between the two go: IDs and TypeName as
// long as the first go: id is the source. Agents occasionally reorder these.
func TestComponentCmd_CopyTypeInAnyPosition(t *testing.T) {
	send, params := mockSend("manage_component", t)
	_, err := componentCmd([]string{"copy", "Rigidbody", "go:src00000", "go:dst00000"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["id"] != "go:src00000" {
		t.Errorf("id: got %v", (*params)["id"])
	}
	if (*params)["dst_id"] != "go:dst00000" {
		t.Errorf("dst_id: got %v", (*params)["dst_id"])
	}
	if (*params)["type"] != "Rigidbody" {
		t.Errorf("type: got %v", (*params)["type"])
	}
}

func TestComponentCmd_CopyMissingDst(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	// Only one go: id present -> cannot disambiguate src from dst, error.
	if _, err := componentCmd([]string{"copy", "go:src00000", "Rigidbody"}, send); err == nil {
		t.Error("expected error when destination go: id is missing")
	}
}

func TestComponentCmd_CopyMissingType(t *testing.T) {
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"copy", "go:src00000", "go:dst00000"}, send); err == nil {
		t.Error("expected error when type name is missing between the two go: ids")
	}
}
