package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// upsertMarkerBlock — block-mode rc files (bash / zsh / powershell)
// ----------------------------------------------------------------------

func TestUpsert_FreshFile(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	changed, action, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !changed || action != "installed" {
		t.Errorf("changed=%v action=%q, want true/installed", changed, action)
	}
	got, _ := os.ReadFile(rc)
	if !strings.Contains(string(got), completionMarkerStart) {
		t.Errorf("missing start marker in:\n%s", got)
	}
	if !strings.Contains(string(got), "source <(udit completion bash)") {
		t.Errorf("missing body line in:\n%s", got)
	}
}

func TestUpsert_PreservesPriorContent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	prior := "# user-managed lines\nexport FOO=bar\nalias ll='ls -al'\n"
	if err := os.WriteFile(rc, []byte(prior), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, _, err := upsertMarkerBlock(rc, "source <(udit completion zsh)", false); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, _ := os.ReadFile(rc)
	for _, want := range []string{"export FOO=bar", "alias ll='ls -al'", "# user-managed lines"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("prior content %q lost:\n%s", want, got)
		}
	}

	// And a .bak with the original.
	bak, err := os.ReadFile(rc + ".bak")
	if err != nil {
		t.Fatalf("missing .bak: %v", err)
	}
	if string(bak) != prior {
		t.Errorf(".bak = %q, want original %q", bak, prior)
	}
}

func TestUpsert_IdempotentOnReinstall(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	if _, _, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false); err != nil {
		t.Fatalf("install: %v", err)
	}
	first, _ := os.ReadFile(rc)

	// Second call with the same body must report "no change".
	changed, _, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false)
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if changed {
		t.Errorf("expected no-change on identical re-install")
	}
	second, _ := os.ReadFile(rc)
	if string(first) != string(second) {
		t.Errorf("file changed despite no-op:\nbefore=%q\nafter=%q", first, second)
	}
}

func TestUpsert_ReplacesExistingBlockNotDuplicates(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	mixed := "# top\nexport A=1\n\n" +
		completionMarkerStart + "\n" +
		"source <(udit completion bash)  # OLD\n" +
		completionMarkerEnd + "\n" +
		"\n# bottom\nexport B=2\n"
	if err := os.WriteFile(rc, []byte(mixed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	changed, action, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !changed || action != "updated" {
		t.Errorf("changed=%v action=%q, want true/updated", changed, action)
	}

	got, _ := os.ReadFile(rc)
	// Old comment "# OLD" must be gone.
	if strings.Contains(string(got), "# OLD") {
		t.Errorf("old block content survived:\n%s", got)
	}
	// Surrounding lines must still be there in their original positions.
	for _, want := range []string{"# top", "export A=1", "# bottom", "export B=2"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("surrounding content %q lost:\n%s", want, got)
		}
	}
	// Exactly one start marker (no duplication).
	if c := strings.Count(string(got), completionMarkerStart); c != 1 {
		t.Errorf("got %d start markers, want 1:\n%s", c, got)
	}
}

func TestUpsert_HalfOpenMarkerIsRefused(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	bad := "export A=1\n" + completionMarkerStart + "\nsome stuff\n# (no closing marker)\n"
	if err := os.WriteFile(rc, []byte(bad), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, _, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false)
	if err == nil {
		t.Fatal("expected error for half-open marker, got nil")
	}
	if !strings.Contains(err.Error(), "matching") {
		t.Errorf("error should mention the missing closing marker; got %v", err)
	}
}

func TestUpsert_ForceRewritesIdenticalBlock(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	if _, _, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false); err != nil {
		t.Fatalf("install: %v", err)
	}

	// force=true → "updated" even though content matches.
	changed, action, err := upsertMarkerBlock(rc, "source <(udit completion bash)", true)
	if err != nil {
		t.Fatalf("force re-install: %v", err)
	}
	if !changed || action != "updated" {
		t.Errorf("changed=%v action=%q, want true/updated under --force", changed, action)
	}
}

// ----------------------------------------------------------------------
// removeMarkerBlock
// ----------------------------------------------------------------------

func TestRemove_ExistingBlock(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	if _, _, err := upsertMarkerBlock(rc, "source <(udit completion bash)", false); err != nil {
		t.Fatalf("install: %v", err)
	}

	removed, err := removeMarkerBlock(rc)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	got, _ := os.ReadFile(rc)
	if strings.Contains(string(got), completionMarkerStart) {
		t.Errorf("start marker survived:\n%s", got)
	}
}

func TestRemove_AbsentMarkerIsNoOp(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	original := "export A=1\nalias x='y'\n"
	if err := os.WriteFile(rc, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	removed, err := removeMarkerBlock(rc)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed {
		t.Error("expected removed=false on file without markers")
	}
	got, _ := os.ReadFile(rc)
	if string(got) != original {
		t.Errorf("file changed despite no-op:\n%s", got)
	}
}

func TestRemove_MissingFileIsNoOp(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "never-existed")
	removed, err := removeMarkerBlock(rc)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed {
		t.Error("expected removed=false on missing file")
	}
}

// ----------------------------------------------------------------------
// resolveCompletionTarget — shell → rc path mapping
// ----------------------------------------------------------------------

func TestResolveTarget_ExplicitShells(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PROFILE", "")

	cases := map[string]struct {
		wantSuffix string
		wantMode   string
	}{
		"zsh":        {".zshrc", "block"},
		"fish":       {filepath.Join(".config", "fish", "completions", "udit.fish"), "file"},
		"powershell": {"Microsoft.PowerShell_profile.ps1", "block"},
		"pwsh":       {"Microsoft.PowerShell_profile.ps1", "block"}, // alias
	}
	for shell, want := range cases {
		t.Run(shell, func(t *testing.T) {
			tgt, err := resolveCompletionTarget(shell)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if !strings.HasSuffix(tgt.rcPath, want.wantSuffix) {
				t.Errorf("rcPath=%q, want suffix %q", tgt.rcPath, want.wantSuffix)
			}
			if tgt.mode != want.wantMode {
				t.Errorf("mode=%q, want %q", tgt.mode, want.wantMode)
			}
		})
	}
}

func TestResolveTarget_BashIsOSDependent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	tgt, err := resolveCompletionTarget("bash")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if runtime.GOOS == "darwin" {
		if !strings.HasSuffix(tgt.rcPath, ".bash_profile") {
			t.Errorf("on darwin, want .bash_profile, got %q", tgt.rcPath)
		}
	} else {
		if !strings.HasSuffix(tgt.rcPath, ".bashrc") {
			t.Errorf("on %s, want .bashrc, got %q", runtime.GOOS, tgt.rcPath)
		}
	}
}

func TestResolveTarget_PowerShellHonorsProfileEnv(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "MyProfile.ps1")
	t.Setenv("PROFILE", custom)
	tgt, err := resolveCompletionTarget("powershell")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tgt.rcPath != custom {
		t.Errorf("rcPath=%q, want %q ($PROFILE)", tgt.rcPath, custom)
	}
}

func TestResolveTarget_UnknownShellErrors(t *testing.T) {
	if _, err := resolveCompletionTarget("ksh"); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

// ----------------------------------------------------------------------
// detectShell + parseCompletionFlags
// ----------------------------------------------------------------------

func TestDetectShell_FromEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	if got := detectShell(); got != "zsh" {
		t.Errorf("detectShell()=%q, want zsh", got)
	}
	t.Setenv("SHELL", "/bin/bash")
	if got := detectShell(); got != "bash" {
		t.Errorf("detectShell()=%q, want bash", got)
	}
}

func TestParseCompletionFlags(t *testing.T) {
	cases := []struct {
		args      []string
		wantShell string
		wantForce bool
	}{
		{[]string{}, "", false},
		{[]string{"--shell", "zsh"}, "zsh", false},
		{[]string{"--force"}, "", true},
		{[]string{"--shell", "fish", "--force"}, "fish", true},
		{[]string{"-s", "bash", "-f"}, "bash", true},
		{[]string{"--shell"}, "", false},        // missing value — silent ignore
		{[]string{"--unknown", "x"}, "", false}, // unknown flag — silent ignore
	}
	for _, c := range cases {
		gotShell, gotForce := parseCompletionFlags(c.args)
		if gotShell != c.wantShell || gotForce != c.wantForce {
			t.Errorf("parse(%v) = (%q,%v), want (%q,%v)",
				c.args, gotShell, gotForce, c.wantShell, c.wantForce)
		}
	}
}

// ----------------------------------------------------------------------
// Fish (file mode) — content equals the printed script
// ----------------------------------------------------------------------

func TestCompletionFileContents_Fish(t *testing.T) {
	got := completionFileContents("fish")
	if got != fishScript {
		t.Errorf("fish file contents != fishScript")
	}
}

func TestWriteFileWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "udit.fish")

	// Initial write — no .bak yet.
	if err := writeFileWithBackup(path, []byte("v1")); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf(".bak shouldn't exist on first write")
	}

	// Second write — old content should land in .bak.
	if err := writeFileWithBackup(path, []byte("v2")); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "v2" {
		t.Errorf("contents = %q, want v2", got)
	}
	if got, _ := os.ReadFile(path + ".bak"); string(got) != "v1" {
		t.Errorf(".bak = %q, want v1", got)
	}
}
