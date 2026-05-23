package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

var ErrWorkflowDefinitionConflict = errors.New("workflow definition conflict")
var ErrWorkflowDefinitionInvalid = errors.New("workflow definition invalid")

type WorkflowDefinitionRecord struct {
	ID        string
	Name      string
	SpecJSON  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type WorkflowDefinition struct {
	ID          string                         `json:"id"`
	Name        string                         `json:"name"`
	Version     string                         `json:"version"`
	Description string                         `json:"description,omitempty"`
	Inputs      map[string]workflow.InputSpec  `json:"inputs"`
	Outputs     map[string]workflow.OutputSpec `json:"outputs"`
	Graph       string                         `json:"graph"`
	Order       []string                       `json:"order"`
	Spec        workflow.WorkflowSpec          `json:"spec"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`
}

type WorkflowDefinitionSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkflowDefinitionService struct {
	store   *SQLiteRunStore
	rootDir string
}

func NewWorkflowDefinitionService(store *SQLiteRunStore, rootDir string) *WorkflowDefinitionService {
	if rootDir == "" {
		rootDir = defaultWorkflowDefinitionRoot()
	}
	return &WorkflowDefinitionService{store: store, rootDir: rootDir}
}

func (s *WorkflowDefinitionService) Load(ctx context.Context, ref string) (*workflow.WorkflowSpec, string, error) {
	record, err := s.lookupRecord(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	def, err := s.definitionFromRecord(record)
	if err != nil {
		return nil, "", err
	}
	if err := s.ensureMirror(def); err != nil {
		return nil, "", err
	}
	return &def.Spec, s.definitionPath(def.ID), nil
}

func (s *WorkflowDefinitionService) List(ctx context.Context) ([]WorkflowDefinitionSummary, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("workflow definition store is not configured")
	}
	records, err := s.store.LoadWorkflowDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]WorkflowDefinitionSummary, 0, len(records))
	for _, record := range records {
		summary, err := s.summaryFromRecord(record)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (s *WorkflowDefinitionService) Get(ctx context.Context, id string) (WorkflowDefinition, error) {
	record, err := s.getRecordByID(ctx, id)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	def, err := s.definitionFromRecord(record)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	if err := s.ensureMirror(def); err != nil {
		return WorkflowDefinition{}, err
	}
	return def, nil
}

func (s *WorkflowDefinitionService) Create(ctx context.Context, spec workflow.WorkflowSpec) (WorkflowDefinition, error) {
	if s == nil || s.store == nil {
		return WorkflowDefinition{}, fmt.Errorf("workflow definition store is not configured")
	}
	prepared, specJSON, err := prepareWorkflowDefinitionSpec(spec)
	if err != nil {
		return WorkflowDefinition{}, fmt.Errorf("%w: %v", ErrWorkflowDefinitionInvalid, err)
	}
	now := time.Now().UTC()
	record := WorkflowDefinitionRecord{
		ID:        uuid.NewString(),
		Name:      prepared.Name,
		SpecJSON:  string(specJSON),
		CreatedAt: now,
		UpdatedAt: now,
	}
	def, err := workflowDefinitionFromSpec(record.ID, record.Name, prepared, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := insertWorkflowDefinitionTx(ctx, tx, record); err != nil {
		return WorkflowDefinition{}, err
	}
	if err := s.ensureMirror(def); err != nil {
		return WorkflowDefinition{}, err
	}
	if err := tx.Commit(); err != nil {
		_ = os.Remove(s.definitionPath(def.ID))
		return WorkflowDefinition{}, translateWorkflowDefinitionDBError(err)
	}
	committed = true
	return def, nil
}

func (s *WorkflowDefinitionService) Update(ctx context.Context, id string, spec workflow.WorkflowSpec) (WorkflowDefinition, error) {
	if s == nil || s.store == nil {
		return WorkflowDefinition{}, fmt.Errorf("workflow definition store is not configured")
	}
	current, err := s.getRecordByID(ctx, id)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	prepared, specJSON, err := prepareWorkflowDefinitionSpec(spec)
	if err != nil {
		return WorkflowDefinition{}, fmt.Errorf("%w: %v", ErrWorkflowDefinitionInvalid, err)
	}
	now := time.Now().UTC()
	updated := WorkflowDefinitionRecord{
		ID:        current.ID,
		Name:      prepared.Name,
		SpecJSON:  string(specJSON),
		CreatedAt: current.CreatedAt,
		UpdatedAt: now,
	}
	def, err := workflowDefinitionFromSpec(updated.ID, updated.Name, prepared, updated.CreatedAt, updated.UpdatedAt)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := updateWorkflowDefinitionTx(ctx, tx, updated); err != nil {
		return WorkflowDefinition{}, err
	}
	if err := s.ensureMirror(def); err != nil {
		return WorkflowDefinition{}, err
	}
	if err := tx.Commit(); err != nil {
		if restoreErr := s.writeMirrorForRecord(current); restoreErr != nil {
			return WorkflowDefinition{}, fmt.Errorf("commit workflow definition update: %w (restore mirror: %v)", err, restoreErr)
		}
		return WorkflowDefinition{}, translateWorkflowDefinitionDBError(err)
	}
	committed = true
	return def, nil
}

func (s *WorkflowDefinitionService) Delete(ctx context.Context, id string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("workflow definition store is not configured")
	}
	current, err := s.getRecordByID(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := deleteWorkflowDefinitionTx(ctx, tx, id); err != nil {
		return err
	}
	if err := os.Remove(s.definitionPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := tx.Commit(); err != nil {
		if restoreErr := s.writeMirrorForRecord(current); restoreErr != nil {
			return fmt.Errorf("commit workflow definition delete: %w (restore mirror: %v)", err, restoreErr)
		}
		return translateWorkflowDefinitionDBError(err)
	}
	committed = true
	return nil
}

func (s *WorkflowDefinitionService) definitionPath(id string) string {
	return filepath.Join(s.rootDir, id+".json")
}

func (s *WorkflowDefinitionService) lookupRecord(ctx context.Context, ref string) (WorkflowDefinitionRecord, error) {
	if s == nil || s.store == nil {
		return WorkflowDefinitionRecord{}, fmt.Errorf("workflow definition store is not configured")
	}
	name := strings.TrimSpace(ref)
	if name == "" {
		return WorkflowDefinitionRecord{}, fmt.Errorf("workflow definition ref is required")
	}
	if record, err := s.store.GetWorkflowDefinition(ctx, name); err == nil {
		return record, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WorkflowDefinitionRecord{}, err
	}
	record, err := s.store.GetWorkflowDefinitionByName(ctx, name)
	if err != nil {
		return WorkflowDefinitionRecord{}, err
	}
	return record, nil
}

func (s *WorkflowDefinitionService) getRecordByID(ctx context.Context, id string) (WorkflowDefinitionRecord, error) {
	if s == nil || s.store == nil {
		return WorkflowDefinitionRecord{}, fmt.Errorf("workflow definition store is not configured")
	}
	record, err := s.store.GetWorkflowDefinition(ctx, strings.TrimSpace(id))
	if err != nil {
		return WorkflowDefinitionRecord{}, err
	}
	return record, nil
}

func (s *WorkflowDefinitionService) definitionFromRecord(record WorkflowDefinitionRecord) (WorkflowDefinition, error) {
	var spec workflow.WorkflowSpec
	if err := json.Unmarshal([]byte(record.SpecJSON), &spec); err != nil {
		return WorkflowDefinition{}, fmt.Errorf("unmarshal workflow definition %q: %w", record.ID, err)
	}
	return workflowDefinitionFromSpec(record.ID, record.Name, spec, record.CreatedAt, record.UpdatedAt)
}

func workflowDefinitionFromSpec(id string, name string, spec workflow.WorkflowSpec, createdAt time.Time, updatedAt time.Time) (WorkflowDefinition, error) {
	plan, err := workflow.BuildPlan(spec)
	if err != nil {
		return WorkflowDefinition{}, fmt.Errorf("build workflow definition graph %q: %w", id, err)
	}
	var graph bytes.Buffer
	if err := workflow.WriteMermaidGraph(&graph, plan); err != nil {
		return WorkflowDefinition{}, fmt.Errorf("render workflow definition graph %q: %w", id, err)
	}
	inputs := spec.Inputs
	if inputs == nil {
		inputs = map[string]workflow.InputSpec{}
	}
	outputs := spec.Outputs
	if outputs == nil {
		outputs = map[string]workflow.OutputSpec{}
	}
	return WorkflowDefinition{
		ID:          id,
		Name:        name,
		Version:     spec.Version,
		Description: spec.Description,
		Inputs:      inputs,
		Outputs:     outputs,
		Graph:       graph.String(),
		Order:       append([]string(nil), plan.Order...),
		Spec:        spec,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func (s *WorkflowDefinitionService) summaryFromRecord(record WorkflowDefinitionRecord) (WorkflowDefinitionSummary, error) {
	var spec workflow.WorkflowSpec
	if err := json.Unmarshal([]byte(record.SpecJSON), &spec); err != nil {
		return WorkflowDefinitionSummary{}, fmt.Errorf("unmarshal workflow definition %q: %w", record.ID, err)
	}
	return WorkflowDefinitionSummary{
		ID:          record.ID,
		Name:        record.Name,
		Version:     spec.Version,
		Description: spec.Description,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}, nil
}

func (s *WorkflowDefinitionService) ensureMirror(def WorkflowDefinition) error {
	if s == nil {
		return fmt.Errorf("workflow definition service is not configured")
	}
	specJSON, err := json.MarshalIndent(def.Spec, "", "  ")
	if err != nil {
		return err
	}
	return s.ensureMirrorBytes(def.ID, specJSON)
}

func (s *WorkflowDefinitionService) writeMirrorForRecord(record WorkflowDefinitionRecord) error {
	var spec workflow.WorkflowSpec
	if err := json.Unmarshal([]byte(record.SpecJSON), &spec); err != nil {
		return err
	}
	specJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return s.ensureMirrorBytes(record.ID, specJSON)
}

func (s *WorkflowDefinitionService) ensureMirrorBytes(id string, data []byte) error {
	path := s.definitionPath(id)
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, append(append([]byte(nil), data...), '\n')) {
			return nil
		}
	}
	return writeJSONAtomicBytes(path, data)
}

func (s *WorkflowDefinitionService) repository() ports.WorkflowRepository {
	return s
}

func prepareWorkflowDefinitionSpec(spec workflow.WorkflowSpec) (workflow.WorkflowSpec, []byte, error) {
	prepared := spec
	workflow.ApplyWorktreeDefaults(&prepared)
	if err := workflow.Validate(&prepared, nil, nil); err != nil {
		return workflow.WorkflowSpec{}, nil, err
	}
	data, err := json.MarshalIndent(prepared, "", "  ")
	if err != nil {
		return workflow.WorkflowSpec{}, nil, err
	}
	return prepared, data, nil
}

func insertWorkflowDefinitionTx(ctx context.Context, tx *sql.Tx, record WorkflowDefinitionRecord) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO workflow_definitions (id, name, spec_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`,
		record.ID,
		record.Name,
		record.SpecJSON,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return translateWorkflowDefinitionDBError(err)
	}
	return nil
}

func updateWorkflowDefinitionTx(ctx context.Context, tx *sql.Tx, record WorkflowDefinitionRecord) error {
	_, err := tx.ExecContext(ctx, `
UPDATE workflow_definitions
SET name = ?, spec_json = ?, updated_at = ?
WHERE id = ?`,
		record.Name,
		record.SpecJSON,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		record.ID,
	)
	if err != nil {
		return translateWorkflowDefinitionDBError(err)
	}
	return nil
}

func deleteWorkflowDefinitionTx(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM workflow_definitions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return nil
}

func translateWorkflowDefinitionDBError(err error) error {
	if err == nil {
		return nil
	}
	if isSQLiteUniqueConstraint(err) {
		return fmt.Errorf("%w: %v", ErrWorkflowDefinitionConflict, err)
	}
	return err
}

func isSQLiteUniqueConstraint(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") || strings.Contains(msg, "constraint failed")
}

type compositeWorkflowRepository struct {
	internal ports.WorkflowRepository
	external ports.WorkflowRepository
}

func newCompositeWorkflowRepository(internal ports.WorkflowRepository, external ports.WorkflowRepository) ports.WorkflowRepository {
	if internal == nil {
		return external
	}
	if external == nil {
		return internal
	}
	return compositeWorkflowRepository{internal: internal, external: external}
}

func (r compositeWorkflowRepository) Load(ctx context.Context, ref string) (*workflow.WorkflowSpec, string, error) {
	if isWorkflowPathRef(ref) {
		return r.external.Load(ctx, ref)
	}
	if r.internal != nil {
		if spec, sourcePath, err := r.internal.Load(ctx, ref); err == nil {
			return spec, sourcePath, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
	}
	return r.external.Load(ctx, ref)
}

func isWorkflowPathRef(ref string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(ref)))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return false
	}
	if strings.Contains(ref, string(filepath.Separator)) || filepath.IsAbs(ref) {
		return true
	}
	if _, err := os.Stat(ref); err == nil {
		return true
	}
	return false
}

func defaultWorkflowDefinitionRoot() string {
	return filepath.Join(defaultAgentFlowRoot(), "workflows", "internal")
}

func writeJSONAtomicBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".atomic-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
