package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowRoots(t *testing.T) {
	roots := workflowRoots()
	if len(roots) < 2 {
		t.Fatalf("expected at least 2 roots, got %d", len(roots))
	}
	if !strings.Contains(roots[0], ".agentflow") {
		t.Fatalf("expected local root to contain .agentflow, got %s", roots[0])
	}
}

func TestDecodeWorkflowForList(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.yaml")
	data := []byte("name: hello-world\nversion: '1'\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := decodeWorkflowForList(path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "hello-world" {
		t.Fatalf("expected name hello-world, got %s", spec.Name)
	}
}

func TestDecodeWorkflowForListInvalid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte("!@#"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := decodeWorkflowForList(path)
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}
