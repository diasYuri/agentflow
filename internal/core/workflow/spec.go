package workflow

type WorkflowSpec struct {
	Version     string                `json:"version" yaml:"version"`
	Name        string                `json:"name" yaml:"name"`
	Description string                `json:"description,omitempty" yaml:"description"`
	Inputs      map[string]InputSpec  `json:"inputs,omitempty" yaml:"inputs"`
	Vars        map[string]any        `json:"vars,omitempty" yaml:"vars"`
	Secrets     map[string]SecretSpec `json:"secrets,omitempty" yaml:"secrets"`
	Defaults    DefaultsSpec          `json:"defaults,omitempty" yaml:"defaults"`
	Execution   ExecutionSpec         `json:"execution,omitempty" yaml:"execution"`
	Nodes       []NodeSpec            `json:"nodes" yaml:"nodes"`
	Worktree    WorktreeSpec          `json:"worktree,omitempty" yaml:"worktree"`
}

type WorktreeSpec struct {
	Enabled  bool                `json:"enabled,omitempty" yaml:"enabled"`
	Provider string              `json:"provider,omitempty" yaml:"provider"`
	Base     string              `json:"base,omitempty" yaml:"base"`
	Merge    WorktreeMergeSpec   `json:"merge,omitempty" yaml:"merge"`
	Cleanup  WorktreeCleanupSpec `json:"cleanup,omitempty" yaml:"cleanup"`
}

type WorktreeMergeSpec struct {
	Strategy   string `json:"strategy,omitempty" yaml:"strategy"`
	OnConflict string `json:"on_conflict,omitempty" yaml:"on_conflict"`
}

type WorktreeCleanupSpec struct {
	OnSuccess *bool  `json:"on_success,omitempty" yaml:"on_success"`
	OnFailure string `json:"on_failure,omitempty" yaml:"on_failure"`
}

type InputSpec struct {
	Type     string `json:"type" yaml:"type"`
	Required bool   `json:"required,omitempty" yaml:"required"`
	Default  any    `json:"default,omitempty" yaml:"default"`
}

type SecretSpec struct {
	Env      string `json:"env" yaml:"env"`
	Required bool   `json:"required,omitempty" yaml:"required"`
}

type DefaultsSpec struct {
	Timeout    int    `json:"timeout,omitempty" yaml:"timeout"`
	Retries    int    `json:"retries,omitempty" yaml:"retries"`
	WorkingDir string `json:"working_dir,omitempty" yaml:"working_dir"`
}

type ExecutionSpec struct {
	MaxConcurrency     int    `json:"max_concurrency,omitempty" yaml:"max_concurrency"`
	FailFast           *bool  `json:"fail_fast,omitempty" yaml:"fail_fast"`
	PauseWhenFail      bool   `json:"pause_when_fail,omitempty" yaml:"pause_when_fail"`
	OutputDir          string `json:"output_dir,omitempty" yaml:"output_dir"`
	MaxNodeOutputBytes int64  `json:"max_node_output_bytes,omitempty" yaml:"max_node_output_bytes"`
}

type GoToIfSpec struct {
	When   string `json:"when" yaml:"when"`
	Target string `json:"target" yaml:"target"`
}

type NodeKind string

const (
	NodeKindAgent     NodeKind = "agent"
	NodeKindBash      NodeKind = "bash"
	NodeKindTransform NodeKind = "transform"
	NodeKindNoop      NodeKind = "noop"
	NodeKindMap       NodeKind = "map"
)

type NodeSpec struct {
	ID              string      `json:"id" yaml:"id"`
	Kind            NodeKind    `json:"kind" yaml:"kind"`
	DependsOn       []string    `json:"depends_on,omitempty" yaml:"depends_on"`
	When            string      `json:"when,omitempty" yaml:"when"`
	Timeout         int         `json:"timeout,omitempty" yaml:"timeout"`
	Retries         int         `json:"retries,omitempty" yaml:"retries"`
	ContinueOnError bool        `json:"continue_on_error,omitempty" yaml:"continue_on_error"`
	GoToIf          *GoToIfSpec `json:"go_to_if,omitempty" yaml:"go_to_if"`
	ForEach         string      `json:"for_each,omitempty" yaml:"for_each"`
	Concurrency     int         `json:"concurrency,omitempty" yaml:"concurrency"`
	MaxItems        int         `json:"max_items,omitempty" yaml:"max_items"`
	FailFast        *bool       `json:"fail_fast,omitempty" yaml:"fail_fast"`

	Permission   *PermissionSpec `json:"permission,omitempty" yaml:"permission"`
	Provider     string          `json:"provider,omitempty" yaml:"provider"`
	Model        string          `json:"model,omitempty" yaml:"model"`
	Prompt       string          `json:"prompt,omitempty" yaml:"prompt"`
	System       string          `json:"system,omitempty" yaml:"system"`
	Sandbox      SandboxSpec     `json:"sandbox,omitempty" yaml:"sandbox"`
	OutputSchema map[string]any  `json:"output_schema,omitempty" yaml:"output_schema"`

	Command    string            `json:"command,omitempty" yaml:"command"`
	Shell      string            `json:"shell,omitempty" yaml:"shell"`
	WorkingDir string            `json:"working_dir,omitempty" yaml:"working_dir"`
	Env        map[string]string `json:"env,omitempty" yaml:"env"`
	Capture    CaptureSpec       `json:"capture,omitempty" yaml:"capture"`

	Operation string         `json:"operation,omitempty" yaml:"operation"`
	Input     string         `json:"input,omitempty" yaml:"input"`
	With      map[string]any `json:"with,omitempty" yaml:"with"`
	Nodes     []NodeSpec     `json:"nodes,omitempty" yaml:"nodes"`
}

type SandboxSpec struct {
	Mode string `json:"mode,omitempty" yaml:"mode"`
}

type PermissionSpec struct {
	Write *bool `json:"write,omitempty" yaml:"write"`
}

type CaptureSpec struct {
	Stdout   bool `json:"stdout,omitempty" yaml:"stdout"`
	Stderr   bool `json:"stderr,omitempty" yaml:"stderr"`
	ExitCode bool `json:"exit_code,omitempty" yaml:"exit_code"`
}

func (s WorkflowSpec) NodeByID(id string) (NodeSpec, bool) {
	for _, node := range s.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return NodeSpec{}, false
}

// ApplyWorktreeDefaults preenche os valores padrão de worktree quando enabled é true
// mas campos opcionais estão vazios.
func ApplyWorktreeDefaults(spec *WorkflowSpec) {
	if !spec.Worktree.Enabled {
		return
	}
	if spec.Worktree.Provider == "" {
		spec.Worktree.Provider = "pi"
	}
	if spec.Worktree.Base == "" {
		spec.Worktree.Base = "current"
	}
	if spec.Worktree.Merge.Strategy == "" {
		spec.Worktree.Merge.Strategy = "deterministic"
	}
	if spec.Worktree.Merge.OnConflict == "" {
		spec.Worktree.Merge.OnConflict = "agent"
	}
	if spec.Worktree.Cleanup.OnFailure == "" {
		spec.Worktree.Cleanup.OnFailure = "keep"
	}
	if spec.Worktree.Cleanup.OnSuccess == nil {
		v := true
		spec.Worktree.Cleanup.OnSuccess = &v
	}
}
