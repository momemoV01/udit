package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const repoAPI = "https://api.github.com/repos/momemoV01/udit/releases/latest"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func updateCmd(args []string) error {
	flags := parseSubFlags(args)
	_, checkOnly := flags["check"]

	fmt.Println("Checking for updates...")

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latest := release.TagName
	current := Version

	if current == latest {
		fmt.Printf("Already up to date (%s)\n", current)
		return nil
	}

	fmt.Printf("Update available: %s → %s\n", current, latest)

	if checkOnly {
		return nil
	}

	asset := findAsset(release.Assets)
	if asset == nil {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate current binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("cannot resolve binary path: %w", err)
	}

	fmt.Printf("Downloading %s...\n", asset.Name)

	tmpFile, err := download(asset.BrowserDownloadURL, filepath.Dir(exe))
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile) }()

	// Verify downloaded binary against SHA256SUMS.txt from the release.
	// If the checksum file is missing (older releases), warn and continue.
	if err := verifyChecksum(tmpFile, asset.Name, release.Assets); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := os.Chmod(tmpFile, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Rename dance: backup → replace → cleanup, with restore on failure.
	backup := exe + ".bak"
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	if err := os.Rename(tmpFile, exe); err != nil {
		if restoreErr := os.Rename(backup, exe); restoreErr != nil {
			return fmt.Errorf("replace failed: %w (restore also failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("replace failed: %w", err)
	}

	_ = os.Remove(backup)

	fmt.Printf("Updated to %s\n", latest)
	return nil
}

func fetchLatestRelease() (*ghRelease, error) {
	resp, err := http.Get(repoAPI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// findAsset finds the release asset matching the current OS and architecture.
func findAsset(assets []ghAsset) *ghAsset {
	suffix := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	for i, a := range assets {
		if strings.Contains(a.Name, suffix) {
			return &assets[i]
		}
	}
	return nil
}

// verifyChecksum downloads SHA256SUMS.txt from the release, computes the
// hash of the local file, and returns an error on mismatch. If the checksum
// file is missing from the release (pre-D4 releases), it prints a warning
// and returns nil so the update can proceed.
func verifyChecksum(localPath, assetName string, assets []ghAsset) error {
	// Find SHA256SUMS.txt in release assets.
	var sumsURL string
	for _, a := range assets {
		if a.Name == "SHA256SUMS.txt" {
			sumsURL = a.BrowserDownloadURL
			break
		}
	}
	if sumsURL == "" {
		fmt.Fprintln(os.Stderr, "Warning: SHA256SUMS.txt not found in release (skipping verification).")
		return nil
	}

	// Download the checksum file.
	resp, err := http.Get(sumsURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not download checksums: %v (skipping verification).\n", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Warning: checksum download returned %d (skipping verification).\n", resp.StatusCode)
		return nil
	}

	// Parse: each line is "hex_hash  filename"
	var expected string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == assetName {
			expected = strings.ToLower(parts[0])
			break
		}
	}
	if expected == "" {
		fmt.Fprintf(os.Stderr, "Warning: no checksum entry for %s (skipping verification).\n", assetName)
		return nil
	}

	// Compute actual hash of the downloaded file.
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open downloaded file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash computation failed: %w", err)
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))

	if expected != actual {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, expected, actual)
	}

	fmt.Println("Checksum verified.")
	return nil
}

func download(url string, targetDir string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(targetDir, "udit-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}

	return tmp.Name(), nil
}
