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
	send, _ := mockSend("manage_component", t)
	if _, err := componentCmd([]string{"delete"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}
