package cmd

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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

func TestVerifyChecksum_Match(t *testing.T) {
	// Create a temp "binary" file.
	content := []byte("hello udit binary content")
	tmp := filepath.Join(t.TempDir(), "udit-test")
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute its hash.
	h := sha256.Sum256(content)
	hash := fmt.Sprintf("%x", h[:])

	// Serve SHA256SUMS.txt via httptest.
	sums := hash + "  udit-test\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, sums)
	}))
	defer srv.Close()

	assets := []ghAsset{
		{Name: "SHA256SUMS.txt", BrowserDownloadURL: srv.URL},
	}

	err := verifyChecksum(tmp, "udit-test", assets)
	if err != nil {
		t.Errorf("verifyChecksum should pass for matching hash: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	content := []byte("real binary")
	tmp := filepath.Join(t.TempDir(), "udit-test")
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Serve wrong hash.
	sums := "0000000000000000000000000000000000000000000000000000000000000000  udit-test\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, sums)
	}))
	defer srv.Close()

	assets := []ghAsset{
		{Name: "SHA256SUMS.txt", BrowserDownloadURL: srv.URL},
	}

	err := verifyChecksum(tmp, "udit-test", assets)
	if err == nil {
		t.Error("verifyChecksum should fail for mismatched hash")
	}
}

func TestVerifyChecksum_NoSumsFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "udit-test")
	if err := os.WriteFile(tmp, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// No SHA256SUMS.txt in assets — should warn and return nil.
	assets := []ghAsset{
		{Name: "udit-linux-amd64", BrowserDownloadURL: "https://example.com/binary"},
	}

	err := verifyChecksum(tmp, "udit-test", assets)
	if err != nil {
		t.Errorf("verifyChecksum should return nil when no checksum file: %v", err)
	}
}
