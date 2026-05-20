// Package api wires HTTP handlers on top of the persistence,
// session, events, and diagnostics packages. The Service type is the
// composition root: callers build it once and register its handlers
// onto a mux.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/web/diagnostics"
	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
	"github.com/diasYuri/agentflow/internal/web/session"
)

// Service exposes session, message, tool-call, approval, diagnostic,
// SSE, and debug bundle endpoints under /api/v1.
type Service struct {
	Sessions            *session.Sessions
	Diagnostics         *diagnostics.Recorder
	Broker              *events.Broker
	Projects            session.ProjectResolver
	FolderPicker        FolderPicker
	WorkflowDefinitions WorkflowDefinitionClient
	DB                  *persistence.DB
	Bundler             *diagnostics.BundleExporter
}

// Options bundles dependencies for NewService.
type Options struct {
	DB                  *persistence.DB
	Projects            session.ProjectResolver
	Broker              *events.Broker
	Policy              diagnostics.RedactionPolicy
	FolderPicker        FolderPicker
	WorkflowDefinitions WorkflowDefinitionClient
}

type projectAdder interface {
	Add(name, path string) error
}

type FolderPicker interface {
	PickFolder(r *http.Request) (string, error)
}

type WorkflowDefinitionClient interface {
	ListWorkflowDefinitions(ctx context.Context) (daemon.WorkflowDefinitionsResponse, error)
	CreateWorkflowDefinition(ctx context.Context, spec workflow.WorkflowSpec) (daemon.WorkflowDefinitionResponse, error)
	WorkflowDefinition(ctx context.Context, id string) (daemon.WorkflowDefinitionResponse, error)
	UpdateWorkflowDefinition(ctx context.Context, id string, spec workflow.WorkflowSpec) (daemon.WorkflowDefinitionResponse, error)
	DeleteWorkflowDefinition(ctx context.Context, id string) error
}

// NewService wires the service, building any optional dependencies
// that were not provided by the caller.
func NewService(opts Options) (*Service, error) {
	if opts.DB == nil {
		return nil, errors.New("api: DB is required")
	}
	if opts.Projects == nil {
		return nil, errors.New("api: Projects resolver is required")
	}
	if opts.Broker == nil {
		opts.Broker = events.NewBroker(64)
	}
	if opts.FolderPicker == nil {
		opts.FolderPicker = NativeFolderPicker{}
	}
	policy := opts.Policy
	if policy.MaxValueBytes == 0 && len(policy.SecretKeySubstrings) == 0 && len(policy.SecretValuePatterns) == 0 {
		policy = diagnostics.DefaultPolicy()
	}
	sessions, err := session.NewSessions(session.Options{DB: opts.DB, Projects: opts.Projects})
	if err != nil {
		return nil, err
	}
	rec, err := diagnostics.NewRecorder(diagnostics.Options{
		DB:     opts.DB,
		Policy: policy,
		Publish: func(diag persistence.Diagnostic) {
			opts.Broker.Publish(diag.SessionID, events.KindDiagnostic, diag, diag.CorrelationID)
		},
	})
	if err != nil {
		return nil, err
	}
	bundler := diagnostics.NewBundleExporter(diagnostics.BundleSources{
		Sessions:    persistence.NewSessionRepository(opts.DB),
		Messages:    persistence.NewMessageRepository(opts.DB),
		Tools:       persistence.NewToolCallRepository(opts.DB),
		Approvals:   persistence.NewApprovalRepository(opts.DB),
		Diagnostics: persistence.NewDiagnosticRepository(opts.DB),
		Events:      persistence.NewFrontendEventRepository(opts.DB),
		Payloads:    persistence.NewPayloadStore(opts.DB),
	}, policy)
	return &Service{
		Sessions:            sessions,
		Diagnostics:         rec,
		Broker:              opts.Broker,
		Projects:            opts.Projects,
		FolderPicker:        opts.FolderPicker,
		WorkflowDefinitions: opts.WorkflowDefinitions,
		DB:                  opts.DB,
		Bundler:             bundler,
	}, nil
}

// Register attaches every API route onto mux under /api/v1.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/projects", s.handleProjects)
	mux.HandleFunc("/api/v1/projects/pick-folder", s.handlePickProjectFolder)
	mux.HandleFunc("/api/v1/projects/", s.handleProjectChild)
	mux.HandleFunc("/api/v1/workflow-definitions", s.handleWorkflowDefinitions)
	mux.HandleFunc("/api/v1/workflow-definitions/", s.handleWorkflowDefinition)
	mux.HandleFunc("/api/v1/sessions", s.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionChild)
	mux.HandleFunc("/api/v1/approvals/", s.handleApprovalChild)
	mux.HandleFunc("/api/v1/tool-calls/", s.handleToolCallChild)
	mux.HandleFunc("/api/v1/diagnostics", s.handleRecentDiagnostics)
	mux.HandleFunc("/api/v1/stream", s.handleGlobalStream)
}

// Close releases dependencies that the service owns.
func (s *Service) Close() {
	if s.Broker != nil {
		s.Broker.Close()
	}
}

// helpers ----

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, out any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func trimPrefixPath(path, prefix string) (string, string) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return "", ""
	}
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return rest, ""
	}
	return rest[:idx], rest[idx+1:]
}

// emptyProjects is a thin adapter that supports tests that build a
// Service without an explicit project store. Production callers
// always pass an app.ProjectRegistry.
type emptyProjects struct{}

func (emptyProjects) Resolve(_ string) (app.Project, error) {
	return app.Project{}, errors.New("project resolver not configured")
}

func (emptyProjects) List() ([]app.Project, error) { return nil, nil }
