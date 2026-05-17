package adapter

import "os"

// WorkflowSummary resume um workflow disponivel para listagem.
type WorkflowSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// LoadedWorkflow representa um workflow carregado para edicao/inspecao.
type LoadedWorkflow struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Version     string                   `json:"version"`
	Inputs      map[string]InputDocument `json:"inputs"`
	Nodes       []NodeSummary            `json:"nodes"`
	SourcePath  string                   `json:"sourcePath"`
	RawYAML     string                   `json:"rawYaml,omitempty"`
}

// InputDocument descreve uma entrada de workflow.
type InputDocument struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// NodeSummary resume um no do workflow.
type NodeSummary struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

// ValidationResult retorna o resultado da validacao de um workflow.
type ValidationResult struct {
	Valid     bool           `json:"valid"`
	Name      string         `json:"name"`
	NodeCount int            `json:"nodeCount"`
	Errors    []DesktopError `json:"errors,omitempty"`
}

// GraphResult retorna o grafo Mermaid de um workflow.
type GraphResult struct {
	Mermaid string         `json:"mermaid"`
	Valid   bool           `json:"valid"`
	Errors  []DesktopError `json:"errors,omitempty"`
}

// DryRunResult retorna o plano de execucao sem executar.
type DryRunResult struct {
	Workflow string              `json:"workflow"`
	Inputs   map[string]any      `json:"inputs"`
	Order    []string            `json:"order"`
	Nodes    map[string]NodePlan `json:"nodes"`
	Valid    bool                `json:"valid"`
	Errors   []DesktopError      `json:"errors,omitempty"`
}

// NodePlan resume um no dentro de um plano de execucao.
type NodePlan struct {
	ID           string   `json:"id"`
	Dependencies []string `json:"dependencies"`
	Dependents   []string `json:"dependents"`
	Kind         string   `json:"kind"`
}

// AppSettings persistem preferencias locais do desktop.
type AppSettings struct {
	WorkspacePath string   `json:"workspacePath"`
	RecentFiles   []string `json:"recentFiles"`
	Theme         string   `json:"theme"`
	CodexPath     string   `json:"codexPath"`
	ClaudePath    string   `json:"claudePath"`
	PiPath        string   `json:"piPath"`
	LogFormat     string   `json:"logFormat"`
}

// ProjectSummary describes a configured project.
type ProjectSummary struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FileSystem abstrai operacoes de arquivo para testabilidade.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(name string) ([]os.DirEntry, error)
}

type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (osFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }
