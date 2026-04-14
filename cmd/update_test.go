package cmd

import "testing"

func TestFindAsset(t *testing.T) {
	assets := []ghAsset{
		{Name: "udit-linux-amd64"},
		{Name: "udit-darwin-arm64"},
		{Name: "udit-windows-amd64.exe"},
	}

	// findAsset uses runtime.GOOS/GOARCH, so we just verify it returns something on the current platform
	got := findAsset(assets)
	if got == nil {
		t.Error("findAsset: should find asset for current platform")
	}

	empty := findAsset(nil)
	if empty != nil {
		t.Error("findAsset: should return nil for empty list")
	}

	noMatch := []ghAsset{{Name: "udit-plan9-mips"}}
	got = findAsset(noMatch)
	if got != nil {
		t.Error("findAsset: should return nil when no platform match")
	}
}
