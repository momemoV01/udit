package watch

import (
	"runtime"
	"testing"
)

func TestMatcher_BasicMatches(t *testing.T) {
	hooks := []Hook{
		{Name: "cs", Paths: []string{"Assets/**/*.cs"}, Run: "refresh --compile"},
		{Name: "pr", Paths: []string{"Assets/**/*.prefab", "Assets/**/*.unity"}, Run: "reserialize $FILE"},
	}
	m := NewMatcher(hooks, false)
	cases := []struct {
		path      string
		wantHooks []string
	}{
		{"Assets/Scripts/Foo.cs", []string{"cs"}},
		{"Assets/Deep/Nested/Bar.cs", []string{"cs"}},
		{"Assets/Prefabs/Boss.prefab", []string{"pr"}},
		{"Assets/Scenes/Main.unity", []string{"pr"}},
		{"Assets/Images/foo.png", nil},
		{"Packages/my.pkg/Foo.cs", nil}, // Packages/ not in pattern
		{"Assets/Scripts/Foo.CS", nil},  // case-sensitive mismatch
	}
	for _, c := range cases {
		got := m.Match(c.path)
		if len(got) != len(c.wantHooks) {
			t.Errorf("Match(%q) returned %d hooks, want %d", c.path, len(got), len(c.wantHooks))
			continue
		}
		for i, h := range got {
			if h.Name != c.wantHooks[i] {
				t.Errorf("Match(%q)[%d].Name = %q, want %q", c.path, i, h.Name, c.wantHooks[i])
			}
		}
	}
}

func TestMatcher_CaseInsensitive(t *testing.T) {
	hooks := []Hook{{Name: "cs", Paths: []string{"Assets/**/*.cs"}, Run: "x"}}
	m := NewMatcher(hooks, true)
	if len(m.Match("Assets/Scripts/Foo.CS")) != 1 {
		t.Errorf("case-insensitive should match Foo.CS")
	}
	if len(m.Match("ASSETS/scripts/foo.cs")) != 1 {
		t.Errorf("case-insensitive should match ASSETS/... path")
	}
}

func TestMatcher_MultipleHooksOnOneFile(t *testing.T) {
	// A single .cs file might match both a "compile" hook and a "lint" hook.
	hooks := []Hook{
		{Name: "compile", Paths: []string{"Assets/**/*.cs"}, Run: "refresh --compile"},
		{Name: "lint", Paths: []string{"Assets/**/*.cs", "Packages/**/*.cs"}, Run: "lint $FILE"},
	}
	m := NewMatcher(hooks, false)
	got := m.Match("Assets/Scripts/Foo.cs")
	if len(got) != 2 {
		t.Fatalf("expected both hooks, got %d", len(got))
	}
	if got[0].Name != "compile" || got[1].Name != "lint" {
		t.Errorf("hook order should match config order: %v", []string{got[0].Name, got[1].Name})
	}
}

func TestMatcher_BackslashInput(t *testing.T) {
	// Windows-only: filepath.ToSlash maps backslashes to forward only on
	// Windows. On Unix, backslash is a valid filename character and the
	// matcher must preserve it (don't silently rewrite user data).
	if runtime.GOOS != "windows" {
		t.Skip("windows-only: backslash is a valid Unix filename character")
	}
	hooks := []Hook{{Name: "cs", Paths: []string{"Assets/**/*.cs"}, Run: "x"}}
	m := NewMatcher(hooks, false)
	if len(m.Match(`Assets\Scripts\Foo.cs`)) != 1 {
		t.Errorf("backslash input should match after ToSlash")
	}
}

func TestMatcher_EmptyHooks(t *testing.T) {
	m := NewMatcher(nil, false)
	if m.Match("Assets/Foo.cs") != nil {
		t.Errorf("empty matcher should return nil")
	}
}

func TestMatcher_NilReceiver(t *testing.T) {
	var m *Matcher
	if m.Match("Assets/Foo.cs") != nil {
		t.Errorf("nil matcher should return nil")
	}
}

func TestHookMatches_Direct(t *testing.T) {
	h := Hook{Paths: []string{"Assets/**/*.cs", "Packages/**/*.cs"}}
	cases := []struct {
		path string
		want bool
	}{
		{"Assets/Foo.cs", true},
		{"Packages/x/Foo.cs", true},
		{"Other/Foo.cs", false},
		{"Assets/Foo.txt", false},
	}
	for _, c := range cases {
		if got := hookMatches(&h, c.path, false); got != c.want {
			t.Errorf("hookMatches(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
