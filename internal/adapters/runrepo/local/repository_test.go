package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/core/run"
)

func TestRepository_SaveArtifact_CreatesIndex(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	art := run.Artifact{
		ID:           "hello.txt",
		Name:         "hello.txt",
		RelativePath: "hello.txt",
		MediaType:    "text/plain",
		Kind:         run.ArtifactKindFile,
	}
	if err := r.SaveArtifact(ctx, "run-1", art, []byte("world")); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	indexPath := filepath.Join(tmp, "run-1", "artifacts", "index.json")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index.json not created: %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(data), "hello.txt") {
		t.Fatalf("expected index to contain hello.txt, got %s", string(data))
	}
}

func TestRepository_SaveArtifact_NestedUpdatesIndex(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	arts := []run.Artifact{
		{ID: "a.txt", Name: "a.txt", RelativePath: "a.txt", Kind: run.ArtifactKindFile},
		{ID: "sub/b.txt", Name: "b.txt", RelativePath: "sub/b.txt", Kind: run.ArtifactKindFile},
	}
	for _, art := range arts {
		if err := r.SaveArtifact(ctx, "run-1", art, []byte("data")); err != nil {
			t.Fatalf("save artifact %s: %v", art.ID, err)
		}
	}

	list, err := r.ListArtifacts(ctx, "run-1")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(list))
	}

	// nested file should exist on disk
	nestedPath := filepath.Join(tmp, "run-1", "artifacts", "sub", "b.txt")
	if _, err := os.Stat(nestedPath); err != nil {
		t.Fatalf("nested artifact not on disk: %v", err)
	}
}

func TestRepository_SaveArtifact_RejectsPathTraversal(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	badPaths := []string{
		"../secret.txt",
		"foo/../../secret.txt",
		"/etc/passwd",
	}
	for _, p := range badPaths {
		art := run.Artifact{
			ID:           p,
			Name:         p,
			RelativePath: p,
			Kind:         run.ArtifactKindFile,
		}
		if err := r.SaveArtifact(ctx, "run-1", art, []byte("x")); err == nil {
			t.Errorf("expected error for path %q", p)
		}
	}
}

func TestRepository_ListArtifacts_StableSort(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	arts := []run.Artifact{
		{ID: "z", Name: "z", RelativePath: "z", Kind: run.ArtifactKindFile, CreatedAt: base.Add(2 * time.Second)},
		{ID: "a", Name: "a", RelativePath: "a", Kind: run.ArtifactKindCustom, CreatedAt: base.Add(1 * time.Second)},
		{ID: "m", Name: "m", RelativePath: "m", Kind: run.ArtifactKindFile, CreatedAt: base.Add(1 * time.Second), NodeID: "n1"},
	}
	for _, art := range arts {
		if err := r.SaveArtifact(ctx, "run-1", art, []byte("data")); err != nil {
			t.Fatalf("save artifact %s: %v", art.ID, err)
		}
	}

	list, err := r.ListArtifacts(ctx, "run-1")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(list))
	}

	// expected order: a (1s, empty node), m (1s, n1), z (2s)
	if list[0].ID != "a" {
		t.Errorf("expected first id a, got %s", list[0].ID)
	}
	if list[1].ID != "m" {
		t.Errorf("expected second id m, got %s", list[1].ID)
	}
	if list[2].ID != "z" {
		t.Errorf("expected third id z, got %s", list[2].ID)
	}
}

func TestRepository_ListArtifacts_MissingIndex(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	list, err := r.ListArtifacts(ctx, "run-1")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 artifacts, got %d", len(list))
	}
}

func TestRepository_ReadArtifact(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	art := run.Artifact{
		ID:           "file.txt",
		Name:         "file.txt",
		RelativePath: "file.txt",
		MediaType:    "text/plain",
		Kind:         run.ArtifactKindFile,
	}
	if err := r.SaveArtifact(ctx, "run-1", art, []byte("hello")); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	data, meta, err := r.ReadArtifact(ctx, "run-1", "file.txt")
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected data hello, got %s", string(data))
	}
	if meta.ID != "file.txt" {
		t.Errorf("expected id file.txt, got %s", meta.ID)
	}
	if meta.SizeBytes != 5 {
		t.Errorf("expected size 5, got %d", meta.SizeBytes)
	}
}

func TestRepository_SaveArtifact_RejectsSymlink(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	artifactDir := filepath.Join(tmp, "run-1", "artifacts")
	target := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	linkPath := filepath.Join(artifactDir, "link.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skip("skipping symlink test: " + err.Error())
	}

	art := run.Artifact{
		ID:           "link.txt",
		Name:         "link.txt",
		RelativePath: "link.txt",
		Kind:         run.ArtifactKindFile,
	}
	if err := r.SaveArtifact(ctx, "run-1", art, []byte("data")); err == nil {
		t.Fatal("expected error for symlink path")
	}
}

func TestRepository_SaveArtifact_RejectsSymlinkAncestor(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	artifactDir := filepath.Join(tmp, "run-1", "artifacts")
	targetDir := filepath.Join(tmp, "escape")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create escape dir: %v", err)
	}
	linkPath := filepath.Join(artifactDir, "link")
	if err := os.Symlink(targetDir, linkPath); err != nil {
		t.Skip("skipping symlink test: " + err.Error())
	}

	art := run.Artifact{
		ID:           "link/foo.txt",
		Name:         "foo.txt",
		RelativePath: "link/foo.txt",
		Kind:         run.ArtifactKindFile,
	}
	if err := r.SaveArtifact(ctx, "run-1", art, []byte("data")); err == nil {
		t.Fatal("expected error for symlink ancestor")
	}
}

func TestRepository_ReadArtifact_RejectsSymlink(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	art := run.Artifact{
		ID:           "file.txt",
		Name:         "file.txt",
		RelativePath: "file.txt",
		Kind:         run.ArtifactKindFile,
	}
	if err := r.SaveArtifact(ctx, "run-1", art, []byte("hello")); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	artifactPath := filepath.Join(tmp, "run-1", "artifacts", "file.txt")
	if err := os.Remove(artifactPath); err != nil {
		t.Fatalf("remove artifact: %v", err)
	}
	target := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, artifactPath); err != nil {
		t.Skip("skipping symlink test: " + err.Error())
	}

	_, _, err = r.ReadArtifact(ctx, "run-1", "file.txt")
	if err == nil {
		t.Fatal("expected error for symlink path")
	}
}

func TestRepository_ReadArtifact_RejectsSymlinkAncestor(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	artifactDir := filepath.Join(tmp, "run-1", "artifacts")
	targetDir := filepath.Join(tmp, "escape")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create escape dir: %v", err)
	}
	linkPath := filepath.Join(artifactDir, "link")
	if err := os.Symlink(targetDir, linkPath); err != nil {
		t.Skip("skipping symlink test: " + err.Error())
	}

	index := map[string]run.Artifact{
		"link/foo.txt": {
			ID:           "link/foo.txt",
			RunID:        "run-1",
			Name:         "foo.txt",
			RelativePath: "link/foo.txt",
			Kind:         run.ArtifactKindFile,
		},
	}
	if err := writeJSON(filepath.Join(artifactDir, "index.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "foo.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write escaped file: %v", err)
	}

	_, _, err = r.ReadArtifact(ctx, "run-1", "link/foo.txt")
	if err == nil {
		t.Fatal("expected error for symlink ancestor")
	}
}

func TestRepository_ReadArtifact_NotFound(t *testing.T) {
	tmp := t.TempDir()
	r := New(tmp)
	ctx := context.Background()

	_, err := r.CreateRun(ctx, run.RunMetadata{RunID: "run-1", Workflow: "wf"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	_, _, err = r.ReadArtifact(ctx, "run-1", "missing.txt")
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
}

func TestValidateRelativePath(t *testing.T) {
	cases := []struct {
		path string
		ok   bool
	}{
		{"file.txt", true},
		{"sub/file.txt", true},
		{"", false},
		{"../secret.txt", false},
		{"foo/../../secret.txt", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		err := validateRelativePath(c.path)
		if c.ok && err != nil {
			t.Errorf("expected %q to be valid, got %v", c.path, err)
		}
		if !c.ok && err == nil {
			t.Errorf("expected %q to be invalid", c.path)
		}
	}
}
