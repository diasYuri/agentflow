package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

func (e *Executor) saveNodeArtifacts(
	ctx context.Context,
	state *ExecutionState,
	node coreworkflow.NodeSpec,
	result corerun.NodeResult,
) []corerun.ArtifactRef {
	var refs []corerun.ArtifactRef

	artifactID := func(kind string) string {
		id := "nodes/" + node.ID
		if result.InstanceID != "" {
			id += "/" + result.InstanceID
		}
		return id + "/" + kind
	}

	// Index stdout.txt when present or when a process-backed node produced output.
	if result.Stdout != "" || node.Kind == coreworkflow.NodeKindBash || node.Kind == coreworkflow.NodeKindExtension {
		art := corerun.Artifact{
			ID:           artifactID("stdout.txt"),
			NodeID:       node.ID,
			InstanceID:   result.InstanceID,
			Name:         "stdout.txt",
			RelativePath: artifactID("stdout.txt"),
			MediaType:    "text/plain",
			Kind:         corerun.ArtifactKindStdout,
		}
		masked := []byte(state.masker.MaskString(result.Stdout))
		if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, masked); err == nil {
			refs = append(refs, corerun.ArtifactRef{ID: art.ID, Name: art.Name, MediaType: art.MediaType})
			state.recordArtifact(1)
			_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", NodeID: node.ID, InstanceID: result.InstanceID, Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
		}
	}

	// Index stderr.txt when present or when a process-backed node produced output.
	if result.Stderr != "" || node.Kind == coreworkflow.NodeKindBash || node.Kind == coreworkflow.NodeKindExtension {
		art := corerun.Artifact{
			ID:           artifactID("stderr.txt"),
			NodeID:       node.ID,
			InstanceID:   result.InstanceID,
			Name:         "stderr.txt",
			RelativePath: artifactID("stderr.txt"),
			MediaType:    "text/plain",
			Kind:         corerun.ArtifactKindStderr,
		}
		masked := []byte(state.masker.MaskString(result.Stderr))
		if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, masked); err == nil {
			refs = append(refs, corerun.ArtifactRef{ID: art.ID, Name: art.Name, MediaType: art.MediaType})
			state.recordArtifact(1)
			_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", NodeID: node.ID, InstanceID: result.InstanceID, Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
		}
	}

	// For process-backed nodes, copy declared artifacts from working dir.
	if (node.Kind == coreworkflow.NodeKindBash || node.Kind == coreworkflow.NodeKindExtension) && len(node.Artifacts) > 0 {
		workingDir := resolvePath(state.baseWorkingDir, effectiveWorkingDir(state.plan.Workflow, node))
		for _, spec := range node.Artifacts {
			ref := e.copyDeclaredArtifact(ctx, state, node, result, workingDir, spec)
			if ref.ID != "" {
				refs = append(refs, ref)
			}
		}
	}

	// Always index result.json as an artifact, after other refs are collected
	// so the marshaled snapshot includes them without self-reference.
	resultCopy := result
	resultCopy.Artifacts = refs
	if data, err := json.Marshal(resultCopy); err == nil {
		art := corerun.Artifact{
			ID:           artifactID("result.json"),
			NodeID:       node.ID,
			InstanceID:   result.InstanceID,
			Name:         "result.json",
			RelativePath: artifactID("result.json"),
			MediaType:    "application/json",
			Kind:         corerun.ArtifactKindResult,
		}
		masked := []byte(state.masker.MaskString(string(data)))
		if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, masked); err == nil {
			refs = append(refs, corerun.ArtifactRef{ID: art.ID, Name: art.Name, MediaType: art.MediaType})
			state.recordArtifact(1)
			_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", NodeID: node.ID, InstanceID: result.InstanceID, Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
		}
	}

	return refs
}

func (e *Executor) copyDeclaredArtifact(
	ctx context.Context,
	state *ExecutionState,
	node coreworkflow.NodeSpec,
	result corerun.NodeResult,
	workingDir string,
	spec coreworkflow.ArtifactSpec,
) corerun.ArtifactRef {
	srcPath := filepath.Join(workingDir, filepath.Clean(spec.Path))
	info, err := os.Lstat(srcPath)
	if err != nil {
		return corerun.ArtifactRef{}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return corerun.ArtifactRef{}
	}
	// Reject files outside working dir.
	resolved, err := filepath.EvalSymlinks(srcPath)
	if err != nil {
		return corerun.ArtifactRef{}
	}
	resolvedWorkingDir, err := filepath.EvalSymlinks(workingDir)
	if err != nil {
		resolvedWorkingDir = workingDir
	}
	absResolved, _ := filepath.Abs(resolved)
	absWorkingDir, _ := filepath.Abs(resolvedWorkingDir)
	if !strings.HasPrefix(absResolved, absWorkingDir+string(filepath.Separator)) && absResolved != absWorkingDir {
		return corerun.ArtifactRef{}
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return corerun.ArtifactRef{}
	}

	mediaType := spec.MediaType
	if mediaType == "" {
		mediaType = mediaTypeByExt(filepath.Ext(spec.Path))
	}

	if isTextMediaType(mediaType) {
		data = []byte(state.masker.MaskString(string(data)))
	}

	id := "nodes/" + node.ID
	if result.InstanceID != "" {
		id += "/" + result.InstanceID
	}
	id += "/artifacts/" + sanitizeName(spec.Name)

	art := corerun.Artifact{
		ID:           id,
		NodeID:       node.ID,
		InstanceID:   result.InstanceID,
		Name:         spec.Name,
		RelativePath: id,
		MediaType:    mediaType,
		Kind:         corerun.ArtifactKindCustom,
		Description:  spec.Description,
	}
	if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, data); err != nil {
		return corerun.ArtifactRef{}
	}
	state.recordArtifact(1)
	_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", NodeID: node.ID, InstanceID: result.InstanceID, Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
	return corerun.ArtifactRef{ID: art.ID, Name: art.Name, MediaType: art.MediaType}
}

func mediaTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".json":
		return "application/json"
	case ".txt", ".text":
		return "text/plain"
	case ".md", ".markdown":
		return "text/markdown"
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".gz":
		return "application/gzip"
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	default:
		return "application/octet-stream"
	}
}

func isTextMediaType(mt string) bool {
	return strings.HasPrefix(mt, "text/") ||
		mt == "application/json" ||
		mt == "application/x-yaml" ||
		mt == "application/javascript" ||
		mt == "application/xml" ||
		mt == "application/sql"
}
