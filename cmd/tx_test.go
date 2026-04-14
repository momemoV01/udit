package cmd

import "testing"

func TestTxCmd_Begin(t *testing.T) {
	send, params := mockSend("manage_transaction", t)
	if _, err := txCmd([]string{"begin"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "begin" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	if _, set := (*params)["name"]; set {
		t.Errorf("name should default (unset) to server fallback, got %v", (*params)["name"])
	}
}

func TestTxCmd_BeginWithName(t *testing.T) {
	send, params := mockSend("manage_transaction", t)
	_, err := txCmd([]string{"begin", "--name", "Spawn boss setup"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["name"] != "Spawn boss setup" {
		t.Errorf("name: got %v", (*params)["name"])
	}
}

func TestTxCmd_Commit(t *testing.T) {
	send, params := mockSend("manage_transaction", t)
	if _, err := txCmd([]string{"commit"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "commit" {
		t.Errorf("action: got %v", (*params)["action"])
	}
}

func TestTxCmd_CommitWithName(t *testing.T) {
	// Commit can override the begin-time name. Useful when the final
	// description only crystallises after the work is done.
	send, params := mockSend("manage_transaction", t)
	_, err := txCmd([]string{"commit", "--name", "Refined: boss & minions"}, send)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["name"] != "Refined: boss & minions" {
		t.Errorf("name: got %v", (*params)["name"])
	}
}

func TestTxCmd_Rollback(t *testing.T) {
	send, params := mockSend("manage_transaction", t)
	if _, err := txCmd([]string{"rollback"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "rollback" {
		t.Errorf("action: got %v", (*params)["action"])
	}
	// rollback takes no --name — if passed it should still not end up in
	// the params because rollback dispatch doesn't look at it.
	if _, set := (*params)["name"]; set {
		t.Errorf("name should not be passed to rollback, got %v", (*params)["name"])
	}
}

func TestTxCmd_Status(t *testing.T) {
	send, params := mockSend("manage_transaction", t)
	if _, err := txCmd([]string{"status"}, send); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*params)["action"] != "status" {
		t.Errorf("action: got %v", (*params)["action"])
	}
}

func TestTxCmd_EmptyArgs(t *testing.T) {
	send, _ := mockSend("manage_transaction", t)
	if _, err := txCmd(nil, send); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestTxCmd_UnknownAction(t *testing.T) {
	send, _ := mockSend("manage_transaction", t)
	if _, err := txCmd([]string{"abort"}, send); err == nil {
		t.Error("expected error for unknown action")
	}
}
