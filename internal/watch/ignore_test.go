package watch

import "testing"

func TestIgnorer_UnityDefaults(t *testing.T) {
	ig := NewIgnorer(nil, true, false)
	cases := []struct {
		path string
		want bool
	}{
		{"Library/ScriptAssemblies/Foo.dll", true},
		{"Temp/cache.bin", true},
		{"Logs/shader.log", true},
		{"MyProject.csproj", true},
		{"MyProject.sln", true},
		{"Foo.cs~", true},
		{".#Foo.cs", true},
		{".git/HEAD", true},
		{"Assets/Scripts/Foo.cs", false},
		{"Packages/com.unity.render-pipelines.core/package.json", false},
	}
	for _, c := range cases {
		if got := ig.Match(c.path); got != c.want {
			t.Errorf("Match(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIgnorer_UserPatternsAppended(t *testing.T) {
	ig := NewIgnorer([]string{"**/*.generated.cs", "MyDir/"}, true, false)
	cases := []struct {
		path string
		want bool
	}{
		{"Assets/Scripts/Foo.generated.cs", true},
		{"Assets/Scripts/Foo.cs", false},
		{"MyDir/anything.cs", true},
		{"MyDir/", true},
		{"Library/Foo.dll", true}, // defaults still active
	}
	for _, c := range cases {
		if got := ig.Match(c.path); got != c.want {
			t.Errorf("Match(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIgnorer_DefaultsDisabled(t *testing.T) {
	ig := NewIgnorer([]string{"MyDir/"}, false, false)
	// Library would normally match; with defaults off it shouldn't.
	if ig.Match("Library/foo.dll") {
		t.Errorf("Library matched even with defaults off")
	}
	if !ig.Match("MyDir/x.cs") {
		t.Errorf("user pattern MyDir/ not matched")
	}
}

func TestIgnorer_CaseInsensitive(t *testing.T) {
	ig := NewIgnorer([]string{"Foo/**"}, false, true)
	if !ig.Match("foo/bar.cs") {
		t.Errorf("case-insensitive mismatch on lowercase path")
	}
	if !ig.Match("FOO/BAR.CS") {
		t.Errorf("case-insensitive mismatch on uppercase path")
	}
}

func TestIgnorer_CaseSensitive(t *testing.T) {
	ig := NewIgnorer([]string{"Foo/**"}, false, false)
	if ig.Match("foo/bar.cs") {
		t.Errorf("case-sensitive unexpected match on lowercase path")
	}
	if !ig.Match("Foo/bar.cs") {
		t.Errorf("case-sensitive should still match exact case")
	}
}

func TestIgnorer_BackslashPathNormalized(t *testing.T) {
	// fsnotify on Windows returns backslash paths. Match normalizes.
	ig := NewIgnorer(nil, true, false)
	if !ig.Match(`Library\ScriptAssemblies\Foo.dll`) {
		t.Errorf("backslash Library path not matched")
	}
}

func TestIgnorer_EmptyPatternSkipped(t *testing.T) {
	ig := NewIgnorer([]string{"", "  ", "Foo/**"}, false, false)
	patterns := ig.Patterns()
	if len(patterns) != 1 || patterns[0] != "Foo/**" {
		t.Errorf("empty patterns not skipped: %v", patterns)
	}
}

func TestIgnorer_NilNeverMatches(t *testing.T) {
	var ig *Ignorer
	if ig.Match("anything") {
		t.Errorf("nil Ignorer should not match")
	}
}
