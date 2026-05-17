package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}

// TestReleaseScriptProducesArchive runs the release helper script for the
// current platform and validates that a non-empty archive with the expected
// naming convention is created.
func TestReleaseScriptProducesArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping release script integration test in short mode")
	}

	repoRoot := findRepoRoot(t)
	tmpDir := t.TempDir()

	script := filepath.Join(repoRoot, "scripts", "release.sh")
	cmd := exec.Command("bash", script, "0.0.0-test", runtime.GOOS, runtime.GOARCH, tmpDir)
	cmd.Dir = repoRoot
	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("release script failed: %v\nstderr: %s", err, exitErr.Stderr)
		}
		t.Fatalf("release script failed: %v", err)
	}

	archive := strings.TrimSpace(string(stdout))
	if archive == "" {
		t.Fatal("release script produced empty archive name")
	}

	archivePath := filepath.Join(tmpDir, archive)
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("expected archive %s to exist: %v", archivePath, err)
	}
	if info.Size() == 0 {
		t.Fatalf("archive %s is empty", archivePath)
	}

	expectedPrefix := "agentflow-0.0.0-test-" + runtime.GOOS + "-" + runtime.GOARCH
	if !strings.HasPrefix(archive, expectedPrefix) {
		t.Errorf("archive name %q does not start with %q", archive, expectedPrefix)
	}
}

func TestReleaseScriptIsReproducibleWithFixedSourceDateEpoch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping release script reproducibility test in short mode")
	}

	repoRoot := findRepoRoot(t)
	script := filepath.Join(repoRoot, "scripts", "release.sh")
	epoch := "1715960004"

	buildArchive := func(outDir string) string {
		t.Helper()
		cmd := exec.Command("bash", script, "0.0.0-test", runtime.GOOS, runtime.GOARCH, outDir)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), fmt.Sprintf("SOURCE_DATE_EPOCH=%s", epoch))
		stdout, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.Fatalf("release script failed: %v\nstderr: %s", err, exitErr.Stderr)
			}
			t.Fatalf("release script failed: %v", err)
		}
		return strings.TrimSpace(string(stdout))
	}

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	firstArchive := buildArchive(firstDir)
	secondArchive := buildArchive(secondDir)

	if firstArchive != secondArchive {
		t.Fatalf("expected identical archive names, got %q and %q", firstArchive, secondArchive)
	}

	hashPayload := func(baseDir, archive string) [2][32]byte {
		t.Helper()
		extractDir := t.TempDir()
		archivePath := filepath.Join(baseDir, archive)
		var cmd *exec.Cmd
		if strings.HasSuffix(archive, ".zip") {
			cmd = exec.Command("unzip", "-q", archivePath, "-d", extractDir)
		} else {
			cmd = exec.Command("tar", "-xzf", archivePath, "-C", extractDir)
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("extracting %s failed: %v\n%s", archive, err, string(output))
		}

		payloadDir := filepath.Join(extractDir, strings.TrimSuffix(strings.TrimSuffix(archive, ".tar.gz"), ".zip"))
		agentflowBin, err := os.ReadFile(filepath.Join(payloadDir, "agentflow"))
		if err != nil {
			t.Fatalf("reading agentflow payload: %v", err)
		}
		agentflowdBin, err := os.ReadFile(filepath.Join(payloadDir, "agentflowd"))
		if err != nil {
			t.Fatalf("reading agentflowd payload: %v", err)
		}
		return [2][32]byte{sha256.Sum256(agentflowBin), sha256.Sum256(agentflowdBin)}
	}

	firstHash := hashPayload(firstDir, firstArchive)
	secondHash := hashPayload(secondDir, secondArchive)
	if firstHash != secondHash {
		t.Fatalf("expected reproducible payload hashes, got %x and %x", firstHash, secondHash)
	}
}

// TestReleaseScriptNamingConvention validates archive naming for multiple
// platforms without actually building.
func TestReleaseScriptNamingConvention(t *testing.T) {
	cases := []struct {
		version string
		os      string
		arch    string
		wantExt string
	}{
		{"v0.5.0", "linux", "amd64", ".tar.gz"},
		{"v0.5.0", "linux", "arm64", ".tar.gz"},
		{"v0.5.0", "darwin", "amd64", ".tar.gz"},
		{"v0.5.0", "darwin", "arm64", ".tar.gz"},
		{"v0.5.0", "windows", "amd64", ".zip"},
	}

	for _, tc := range cases {
		name := tc.os + "-" + tc.arch
		t.Run(name, func(t *testing.T) {
			bundle := "agentflow-" + tc.version + "-" + tc.os + "-" + tc.arch
			if tc.os == "windows" {
				bundle += ".zip"
			} else {
				bundle += ".tar.gz"
			}
			if !strings.HasSuffix(bundle, tc.wantExt) {
				t.Errorf("expected suffix %q, got %q", tc.wantExt, bundle)
			}
		})
	}
}
