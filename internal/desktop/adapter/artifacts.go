package adapter

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/desktop/runtime"
)

const MaxArtifactInline = 128 * 1024

// ArtifactInfo resume um artefato.
type ArtifactInfo struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Size         int64                `json:"size"`
	ContentType  string               `json:"content_type,omitempty"`
	ModifiedAt   time.Time            `json:"modified_at,omitempty"`
	NodeID       string               `json:"node_id,omitempty"`
	InstanceID   string               `json:"instance_id,omitempty"`
	RelativePath string               `json:"relative_path,omitempty"`
	MediaType    string               `json:"media_type,omitempty"`
	SizeBytes    int64                `json:"size_bytes,omitempty"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`
	Kind         corerun.ArtifactKind `json:"kind,omitempty"`
	Description  string               `json:"description,omitempty"`
}

// ArtifactsResponse retorna artefatos de uma run.
type ArtifactsResponse struct {
	RunID     string         `json:"run_id"`
	Artifacts []ArtifactInfo `json:"artifacts"`
}

// ArtifactResponse retorna conteudo de um artefato.
type ArtifactResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Size         int64                `json:"size"`
	ContentType  string               `json:"content_type,omitempty"`
	NodeID       string               `json:"node_id,omitempty"`
	InstanceID   string               `json:"instance_id,omitempty"`
	RelativePath string               `json:"relative_path,omitempty"`
	MediaType    string               `json:"media_type,omitempty"`
	SizeBytes    int64                `json:"size_bytes,omitempty"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`
	Kind         corerun.ArtifactKind `json:"kind,omitempty"`
	Description  string               `json:"description,omitempty"`
	Encoding     string               `json:"encoding,omitempty"`
	Content      string               `json:"content,omitempty"`
	TextContent  string               `json:"text_content,omitempty"`
	Truncated    bool                 `json:"truncated,omitempty"`
	IsText       bool                 `json:"is_text,omitempty"`
}

// NodeResultDTO representa resultado de um no.
type NodeResultDTO struct {
	NodeID     string   `json:"node_id"`
	InstanceID string   `json:"instance_id,omitempty"`
	Path       []string `json:"path,omitempty"`
	Index      *int     `json:"index,omitempty"`
	Status     string   `json:"status"`
	Output     any      `json:"output,omitempty"`
	Outputs    []any    `json:"outputs,omitempty"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	Error      string   `json:"error,omitempty"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	Duration   int64    `json:"duration_ms,omitempty"`
	Attempts   int      `json:"attempts,omitempty"`
}

// NodesResponse retorna resultados de nos.
type NodesResponse struct {
	RunID string          `json:"run_id"`
	Nodes []NodeResultDTO `json:"nodes"`
}

// NodeResponse retorna instancias de um no.
type NodeResponse struct {
	RunID     string          `json:"run_id"`
	NodeID    string          `json:"node_id"`
	Instances []NodeResultDTO `json:"instances,omitempty"`
}

// PlanResponse retorna o plano da run.
type PlanResponse struct {
	RunID      string         `json:"run_id"`
	Workflow   string         `json:"workflow,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Normalized map[string]any `json:"normalized,omitempty"`
	Plan       map[string]any `json:"plan,omitempty"`
}

// LogsResponse retorna linhas de log (events.jsonl).
type LogsResponse struct {
	RunID string   `json:"run_id"`
	Lines []string `json:"lines"`
}

// EventsResponse retorna eventos paginados.
type EventsResponse struct {
	RunID      string             `json:"run_id"`
	Events     []runtime.RunEvent `json:"events"`
	NextCursor int                `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// GetRunArtifacts lista artefatos de uma run.
func (a *Adapter) GetRunArtifacts(runID string) (ArtifactsResponse, error) {
	if a.runtime == nil {
		return ArtifactsResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return ArtifactsResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	resp := ArtifactsResponse{RunID: runID}
	index, err := a.loadArtifactIndex(runDir)
	if err != nil {
		return ArtifactsResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	if len(index) > 0 {
		for _, art := range index {
			resp.Artifacts = append(resp.Artifacts, artifactInfoToDTO(art))
		}
		sort.Slice(resp.Artifacts, func(i, j int) bool {
			return resp.Artifacts[i].ID < resp.Artifacts[j].ID
		})
		return resp, nil
	}

	// Fallback para runs antigas sem indice.
	artifacts, err := a.fallbackScanArtifacts(runDir)
	if err != nil {
		return ArtifactsResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	resp.Artifacts = artifacts
	return resp, nil
}

// GetRunArtifact retorna conteudo de um artefato.
func (a *Adapter) GetRunArtifact(runID, artifactID string) (ArtifactResponse, error) {
	if a.runtime == nil {
		return ArtifactResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return ArtifactResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	index, err := a.loadArtifactIndex(runDir)
	if err != nil {
		return ArtifactResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}

	artifactsDir := filepath.Join(runDir, "artifacts")
	var artifact corerun.Artifact
	var path string

	if len(index) > 0 {
		var found bool
		artifact, found = index[artifactID]
		if !found {
			return ArtifactResponse{}, DesktopError{Message: "artifact not found", Code: ErrCodeWorkflowNotFound}
		}
		path, err = safeJoin(artifactsDir, artifact.RelativePath)
		if err != nil {
			return ArtifactResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeInvalidPath}
		}
	} else {
		// Fallback para runs antigas sem indice.
		path, err = safeJoin(artifactsDir, artifactID)
		if err != nil {
			return ArtifactResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeInvalidPath}
		}
	}

	if isSymlink(path) {
		return ArtifactResponse{}, DesktopError{Message: "symlink not allowed", Code: ErrCodeInvalidPath}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ArtifactResponse{}, DesktopError{Message: "artifact not found", Code: ErrCodeWorkflowNotFound}
		}
		return ArtifactResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}

	info, err := os.Stat(path)
	if err != nil {
		return ArtifactResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}

	mt := artifact.MediaType
	if mt == "" {
		mt = http.DetectContentType(data)
	}
	text := isTextMediaType(mt)

	resp := ArtifactResponse{
		ID:           artifactID,
		Name:         filepath.Base(artifactID),
		Path:         artifactID,
		Size:         info.Size(),
		ContentType:  mt,
		NodeID:       artifact.NodeID,
		InstanceID:   artifact.InstanceID,
		RelativePath: artifact.RelativePath,
		MediaType:    mt,
		SizeBytes:    info.Size(),
		CreatedAt:    artifact.CreatedAt,
		Kind:         artifact.Kind,
		Description:  artifact.Description,
		IsText:       text,
	}

	if text {
		resp.Encoding = "text"
		if len(data) > MaxArtifactInline {
			resp.Truncated = true
			resp.TextContent = string(data[:MaxArtifactInline])
		} else {
			resp.TextContent = string(data)
		}
	} else {
		resp.Encoding = "base64"
		resp.Truncated = true
	}

	return resp, nil
}

// GetRunArtifactPath resolve o path absoluto de um artefato indexado para open/export controlado.
func (a *Adapter) GetRunArtifactPath(runID, artifactID string) (string, error) {
	if a.runtime == nil {
		return "", DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return "", DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	index, err := a.loadArtifactIndex(runDir)
	if err != nil {
		return "", DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	if len(index) == 0 {
		return "", DesktopError{Message: "artifact index not available", Code: ErrCodeWorkflowNotFound}
	}

	artifact, found := index[artifactID]
	if !found {
		return "", DesktopError{Message: "artifact not found", Code: ErrCodeWorkflowNotFound}
	}

	artifactsDir := filepath.Join(runDir, "artifacts")
	path, err := safeJoin(artifactsDir, artifact.RelativePath)
	if err != nil {
		return "", DesktopError{Message: err.Error(), Code: ErrCodeInvalidPath}
	}
	if isSymlink(path) {
		return "", DesktopError{Message: "symlink not allowed", Code: ErrCodeInvalidPath}
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", DesktopError{Message: "artifact not found", Code: ErrCodeWorkflowNotFound}
		}
		return "", DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	return path, nil
}

// GetRunNodes lista resultados de nos.
func (a *Adapter) GetRunNodes(runID string) (NodesResponse, error) {
	if a.runtime == nil {
		return NodesResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return NodesResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	resp := NodesResponse{RunID: runID}
	nodesDir := filepath.Join(runDir, "nodes")

	if err := filepath.WalkDir(nodesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		if isSymlink(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var result corerun.NodeResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}
		resp.Nodes = append(resp.Nodes, nodeResultToDTO(result))
		return nil
	}); err != nil {
		return NodesResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}

	sort.Slice(resp.Nodes, func(i, j int) bool {
		if resp.Nodes[i].NodeID != resp.Nodes[j].NodeID {
			return resp.Nodes[i].NodeID < resp.Nodes[j].NodeID
		}
		if resp.Nodes[i].InstanceID != resp.Nodes[j].InstanceID {
			return resp.Nodes[i].InstanceID < resp.Nodes[j].InstanceID
		}
		ii, ji := -1, -1
		if resp.Nodes[i].Index != nil {
			ii = *resp.Nodes[i].Index
		}
		if resp.Nodes[j].Index != nil {
			ji = *resp.Nodes[j].Index
		}
		return ii < ji
	})

	return resp, nil
}

// GetRunNode retorna instancias de um no especifico.
func (a *Adapter) GetRunNode(runID, nodeID string) (NodeResponse, error) {
	if a.runtime == nil {
		return NodeResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return NodeResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	resp := NodeResponse{RunID: runID, NodeID: nodeID}
	nodesDir := filepath.Join(runDir, "nodes")
	found := false

	if err := filepath.WalkDir(nodesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		if isSymlink(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var result corerun.NodeResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}
		if result.NodeID != nodeID {
			return nil
		}
		found = true
		resp.Instances = append(resp.Instances, nodeResultToDTO(result))
		return nil
	}); err != nil {
		return NodeResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}

	if !found {
		return NodeResponse{}, DesktopError{Message: "node not found", Code: ErrCodeWorkflowNotFound}
	}

	sort.Slice(resp.Instances, func(i, j int) bool {
		if resp.Instances[i].InstanceID != resp.Instances[j].InstanceID {
			return resp.Instances[i].InstanceID < resp.Instances[j].InstanceID
		}
		ii, ji := -1, -1
		if resp.Instances[i].Index != nil {
			ii = *resp.Instances[i].Index
		}
		if resp.Instances[j].Index != nil {
			ji = *resp.Instances[j].Index
		}
		return ii < ji
	})

	return resp, nil
}

// GetRunPlan retorna metadados, workflow, normalized e plan.
func (a *Adapter) GetRunPlan(runID string) (PlanResponse, error) {
	if a.runtime == nil {
		return PlanResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return PlanResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	resp := PlanResponse{RunID: runID}

	if data, err := os.ReadFile(filepath.Join(runDir, "run.json")); err == nil {
		var meta map[string]any
		if err := json.Unmarshal(data, &meta); err == nil {
			resp.Metadata = meta
		}
	}
	if data, err := os.ReadFile(filepath.Join(runDir, "workflow.yaml")); err == nil {
		resp.Workflow = string(data)
	}
	if data, err := os.ReadFile(filepath.Join(runDir, "normalized.json")); err == nil {
		var norm map[string]any
		if err := json.Unmarshal(data, &norm); err == nil {
			resp.Normalized = norm
		}
	}
	if data, err := os.ReadFile(filepath.Join(runDir, "plan.json")); err == nil {
		var plan map[string]any
		if err := json.Unmarshal(data, &plan); err == nil {
			resp.Plan = plan
		}
	}
	return resp, nil
}

// GetRunLogs retorna linhas brutas do events.jsonl.
func (a *Adapter) GetRunLogs(runID string) (LogsResponse, error) {
	if a.runtime == nil {
		return LogsResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return LogsResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	file, err := os.Open(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LogsResponse{RunID: runID, Lines: []string{}}, nil
		}
		return LogsResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return LogsResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	return LogsResponse{RunID: runID, Lines: lines}, nil
}

// GetRunEvents retorna eventos paginados.
func (a *Adapter) GetRunEvents(runID string, cursor, limit int) (EventsResponse, error) {
	if a.runtime == nil {
		return EventsResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return EventsResponse{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}

	raw, err := runtime.GetRunEvents(runDir, cursor, limit)
	if err != nil {
		return EventsResponse{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem}
	}
	resp := EventsResponse{
		RunID:      runID,
		NextCursor: raw.NextCursor,
		HasMore:    raw.HasMore,
		Events:     make([]runtime.RunEvent, len(raw.Events)),
	}
	copy(resp.Events, raw.Events)
	return resp, nil
}

func nodeResultToDTO(result corerun.NodeResult) NodeResultDTO {
	dto := NodeResultDTO{
		NodeID:     result.NodeID,
		InstanceID: result.InstanceID,
		Path:       result.Path,
		Index:      result.Index,
		Status:     string(result.Status),
		Output:     result.Output,
		Outputs:    result.Outputs,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		Error:      result.Error,
		ExitCode:   result.ExitCode,
		Attempts:   result.Attempts,
	}
	if result.Duration > 0 {
		dto.Duration = result.Duration.Milliseconds()
	}
	return dto
}

func safeJoin(base, rel string) (string, error) {
	if err := validateArtifactRelativePath(rel); err != nil {
		return "", err
	}
	if err := validateArtifactAncestors(base, rel); err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(rel)), nil
}

func (a *Adapter) loadArtifactIndex(runDir string) (map[string]corerun.Artifact, error) {
	indexPath := filepath.Join(runDir, "artifacts", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var index map[string]corerun.Artifact
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index == nil {
		return map[string]corerun.Artifact{}, nil
	}
	return index, nil
}

func artifactInfoToDTO(a corerun.Artifact) ArtifactInfo {
	return ArtifactInfo{
		ID:           a.ID,
		Name:         a.Name,
		Path:         a.ID,
		Size:         a.SizeBytes,
		ContentType:  a.MediaType,
		NodeID:       a.NodeID,
		InstanceID:   a.InstanceID,
		RelativePath: a.RelativePath,
		MediaType:    a.MediaType,
		SizeBytes:    a.SizeBytes,
		CreatedAt:    a.CreatedAt,
		Kind:         a.Kind,
		Description:  a.Description,
	}
}

func (a *Adapter) fallbackScanArtifacts(runDir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := filepath.WalkDir(artifactsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isSymlink(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(artifactsDir, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, ArtifactInfo{
			ID:          rel,
			Name:        filepath.Base(rel),
			Path:        rel,
			Size:        info.Size(),
			ContentType: detectFileContentType(path),
			ModifiedAt:  info.ModTime(),
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func isTextMediaType(mt string) bool {
	return strings.HasPrefix(mt, "text/") ||
		mt == "application/json" ||
		mt == "application/x-yaml" ||
		mt == "application/javascript" ||
		mt == "application/xml" ||
		mt == "application/sql"
}

func validateArtifactRelativePath(rel string) error {
	if rel == "" {
		return fmt.Errorf("path is empty")
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		return fmt.Errorf("path is absolute")
	}
	if strings.Contains(clean, "..") {
		return fmt.Errorf("path contains ..")
	}
	if strings.HasPrefix(filepath.ToSlash(clean), "..") {
		return fmt.Errorf("path escapes base directory")
	}
	return nil
}

func validateArtifactAncestors(baseDir, rel string) error {
	current := baseDir
	parts := strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path is a symlink")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("path is not a directory")
		}
	}
	return nil
}

func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func detectFileContentType(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	var sample [512]byte
	n, err := file.Read(sample[:])
	if err != nil && n == 0 {
		return ""
	}
	return http.DetectContentType(sample[:n])
}
