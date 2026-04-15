package watch

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitArgs_Simple(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"refresh --compile", []string{"refresh", "--compile"}},
		{"  refresh   --compile  ", []string{"refresh", "--compile"}},
		{`reserialize "Assets/With Space/Foo.prefab"`, []string{"reserialize", "Assets/With Space/Foo.prefab"}},
		{`reserialize 'Assets/q.prefab'`, []string{"reserialize", "Assets/q.prefab"}},
		{`echo hello\ world`, []string{"echo", "hello world"}},
		{`cmd "a b" 'c d' e`, []string{"cmd", "a b", "c d", "e"}},
	}
	for _, c := range cases {
		got, err := splitArgs(c.in)
		if err != nil {
			t.Errorf("splitArgs(%q) error: %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitArgs(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSplitArgs_UnterminatedQuote(t *testing.T) {
	_, err := splitArgs(`cmd "unterminated`)
	if err == nil {
		t.Errorf("expected error for unterminated quote")
	}
}

func TestSplitArgs_EmptyString(t *testing.T) {
	_, err := splitArgs(``)
	if err == nil {
		t.Errorf("expected error for empty string")
	}
}

func TestHasPerFileHasBatch(t *testing.T) {
	// Per-file == $FILE or $RELFILE; batch == $FILES or $RELFILES.
	cases := []struct {
		s           string
		wantPerFile bool
		wantBatch   bool
	}{
		{"refresh --compile", false, false},
		{"reserialize $FILE", true, false},
		{"reserialize $RELFILE", true, false},
		{"cmd $FILES", false, true},
		{"cmd $RELFILES", false, true},
		{"cmd $FILE $FILES", true, true}, // conflict case — detected as both
		{"cmd $FILE_BACKUP", false, false},
		{"$FILE", true, false},
	}
	for _, c := range cases {
		if got := hasPerFileVar(c.s); got != c.wantPerFile {
			t.Errorf("hasPerFileVar(%q) = %v, want %v", c.s, got, c.wantPerFile)
		}
		if got := hasBatchVar(c.s); got != c.wantBatch {
			t.Errorf("hasBatchVar(%q) = %v, want %v", c.s, got, c.wantBatch)
		}
	}
}

func TestExpand_NoVars(t *testing.T) {
	b := Batch{HookName: "h", Files: []Event{{Path: "/abs/Assets/Foo.cs", Kind: "write"}}}
	spec, err := Expand("refresh --compile", b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(spec.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(spec.Commands))
	}
	if !reflect.DeepEqual(spec.Commands[0].Argv, []string{"refresh", "--compile"}) {
		t.Errorf("argv: %v", spec.Commands[0].Argv)
	}
	if spec.Commands[0].Env != nil {
		t.Errorf("env should be nil, got %v", spec.Commands[0].Env)
	}
}

func TestExpand_FILE_PerFile(t *testing.T) {
	b := Batch{
		HookName:    "reserialize",
		ProjectRoot: `/project`,
		Files: []Event{
			{Path: "/project/Assets/A.prefab", Kind: "write"},
			{Path: "/project/Assets/B.prefab", Kind: "create"},
		},
	}
	spec, err := Expand("reserialize $RELFILE", b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(spec.Commands) != 2 {
		t.Fatalf("expected 2 commands (per-file), got %d", len(spec.Commands))
	}
	if spec.Commands[0].Argv[1] != "Assets/A.prefab" {
		t.Errorf("cmd0 RELFILE: %v", spec.Commands[0].Argv)
	}
	if spec.Commands[1].Argv[1] != "Assets/B.prefab" {
		t.Errorf("cmd1 RELFILE: %v", spec.Commands[1].Argv)
	}
}

func TestExpand_FILES_Once(t *testing.T) {
	b := Batch{
		HookName:    "dolint",
		ProjectRoot: `/project`,
		Files: []Event{
			{Path: "/project/Assets/A.cs", Kind: "write"},
			{Path: "/project/Assets/B.cs", Kind: "write"},
		},
	}
	spec, err := Expand("lint --files $FILES", b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(spec.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(spec.Commands))
	}
	c := spec.Commands[0]
	if c.Argv[len(c.Argv)-1] != "$FILES" {
		t.Errorf("$FILES should remain literal in argv, got %v", c.Argv)
	}
	foundAbs := false
	foundRel := false
	for _, e := range c.Env {
		if strings.HasPrefix(e, "UDIT_CHANGED_FILES=") {
			foundAbs = true
			if !strings.Contains(e, "/project/Assets/A.cs") || !strings.Contains(e, "/project/Assets/B.cs") {
				t.Errorf("UDIT_CHANGED_FILES missing paths: %q", e)
			}
		}
		if strings.HasPrefix(e, "UDIT_CHANGED_RELFILES=") {
			foundRel = true
			if !strings.Contains(e, "Assets/A.cs") || !strings.Contains(e, "Assets/B.cs") {
				t.Errorf("UDIT_CHANGED_RELFILES missing paths: %q", e)
			}
		}
	}
	if !foundAbs {
		t.Errorf("UDIT_CHANGED_FILES env var missing")
	}
	if !foundRel {
		t.Errorf("UDIT_CHANGED_RELFILES env var missing")
	}
}

func TestExpand_HOOK_EVENT(t *testing.T) {
	b := Batch{
		HookName: "hello",
		Files: []Event{
			{Path: "/a/b.cs", Kind: "write"},
			{Path: "/a/c.cs", Kind: "create"}, // create wins dominance
		},
	}
	spec, err := Expand("echo $HOOK $EVENT", b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if spec.Commands[0].Argv[1] != "hello" {
		t.Errorf("$HOOK: %v", spec.Commands[0].Argv)
	}
	if spec.Commands[0].Argv[2] != "create" {
		t.Errorf("$EVENT: %v (expected create as dominant)", spec.Commands[0].Argv)
	}
}

func TestExpand_FILE_WithOtherVars(t *testing.T) {
	b := Batch{
		HookName:    "h",
		ProjectRoot: `/project`,
		Files: []Event{
			{Path: "/project/Assets/X.cs", Kind: "write"},
		},
	}
	spec, err := Expand("udit go inspect $RELFILE --event=$EVENT --hook=$HOOK", b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(spec.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(spec.Commands))
	}
	got := spec.Commands[0].Argv
	want := []string{"udit", "go", "inspect", "Assets/X.cs", "--event=write", "--hook=h"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv: %v, want %v", got, want)
	}
}

func TestExpand_EmptyRun(t *testing.T) {
	_, err := Expand("   ", Batch{})
	if err == nil {
		t.Errorf("expected error for empty run")
	}
}

func TestExpand_UnterminatedQuote(t *testing.T) {
	_, err := Expand(`cmd "unterminated`, Batch{Files: []Event{{Path: "/x", Kind: "write"}}})
	if err == nil {
		t.Errorf("expected error for unterminated quote")
	}
}

func TestRelPath(t *testing.T) {
	cases := []struct {
		abs, root, want string
	}{
		{"/project/Assets/Foo.cs", "/project", "Assets/Foo.cs"},
		{`C:\project\Assets\Foo.cs`, `C:\project`, "Assets/Foo.cs"},
		{"/project/Assets/Foo.cs", "", "/project/Assets/Foo.cs"},
		{"/outside/Foo.cs", "/project", "/outside/Foo.cs"}, // walks up → fallback to abs
	}
	for _, c := range cases {
		if got := relPath(c.abs, c.root); got != c.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", c.abs, c.root, got, c.want)
		}
	}
}

func TestDominantEvent(t *testing.T) {
	cases := []struct {
		in   []Event
		want string
	}{
		{[]Event{{Kind: "write"}, {Kind: "write"}}, "write"},
		{[]Event{{Kind: "write"}, {Kind: "create"}}, "create"},
		{[]Event{{Kind: "remove"}, {Kind: "write"}}, "write"},
		{[]Event{{Kind: "rename"}}, "rename"},
		{nil, "write"},
	}
	for _, c := range cases {
		if got := dominantEvent(c.in); got != c.want {
			t.Errorf("dominantEvent(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}
