package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Harness provides an isolated environment for end-to-end tests.
// It creates temporary HOME, workspace, and run-root directories,
// builds the agentflow binaries once per package, and captures
// command output for debuggable failures.
type Harness struct {
	t         *testing.T
	Workspace string
	Home      string
	RunRoot   string

	AgentflowPath  string
	AgentflowdPath string

	mu  sync.Mutex
	env []string
}

// CommandResult holds the output of a executed binary.
type CommandResult struct {
	Cmd      string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// New creates a Harness with isolated temp directories.
// The caller must call Build before running commands.
func New(t *testing.T) *Harness {
	t.Helper()

	h := &Harness{t: t}

	h.Workspace = t.TempDir()

	// macOS limits Unix socket paths to 104 bytes. t.TempDir() on Darwin
	// expands to a long /var/folders/... path that easily exceeds this.
	// Use a short /tmp prefix for HOME so agentflowd can bind its socket.
	if runtime.GOOS == "darwin" {
		var err error
		h.Home, err = os.MkdirTemp("/tmp", "af")
		if err != nil {
			t.Fatalf("create home directory: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(h.Home) })
	} else {
		h.Home = filepath.Join(h.Workspace, "home")
		if err := os.MkdirAll(h.Home, 0o755); err != nil {
			t.Fatalf("create directory %s: %v", h.Home, err)
		}
	}

	h.RunRoot = filepath.Join(h.Workspace, "runs")
	if err := os.MkdirAll(h.RunRoot, 0o755); err != nil {
		t.Fatalf("create directory %s: %v", h.RunRoot, err)
	}

	h.env = os.Environ()
	h.Setenv("HOME", h.Home)
	h.Setenv("AGENTFLOW_HOME", h.Home)

	t.Cleanup(h.Close)
	return h
}

// Setenv overrides an environment variable for all subsequent runs.
func (h *Harness) Setenv(key, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.env = setEnv(h.env, key, value)
}

// Env returns a copy of the current environment.
func (h *Harness) Env() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.env))
	copy(out, h.env)
	return out
}

// Build compiles agentflow and agentflowd into a shared package-level
// temp directory. The build is cached; subsequent calls are no-ops.
func (h *Harness) Build() {
	h.t.Helper()

	if err := ensureBuilt(); err != nil {
		h.t.Fatalf("build binaries: %v", err)
	}
	h.AgentflowPath = pkgAgentflowPath
	h.AgentflowdPath = pkgAgentflowdPath
}

// Run executes the agentflow binary with the given arguments.
func (h *Harness) Run(args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowPath == "" {
		h.t.Fatal("Harness.Build() must be called before Run()")
	}
	return h.runAtDir(h.Workspace, h.AgentflowPath, nil, args...)
}

// RunInDir executes the agentflow binary from an explicit working directory.
func (h *Harness) RunInDir(dir string, args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunInDir()")
	}
	return h.runAtDir(dir, h.AgentflowPath, nil, args...)
}

// RunWithEnv executes the agentflow binary with extra environment variables.
func (h *Harness) RunWithEnv(extra map[string]string, args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunWithEnv()")
	}
	return h.runAtDir(h.Workspace, h.AgentflowPath, extra, args...)
}

// RunWithEnvInDir executes the agentflow binary with extra environment variables from an explicit directory.
func (h *Harness) RunWithEnvInDir(dir string, extra map[string]string, args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunWithEnvInDir()")
	}
	return h.runAtDir(dir, h.AgentflowPath, extra, args...)
}

// RunDaemon executes the agentflowd binary with the given arguments.
func (h *Harness) RunDaemon(args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowdPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunDaemon()")
	}
	return h.runAtDir(h.Workspace, h.AgentflowdPath, nil, args...)
}

// RunDaemonInDir executes the agentflowd binary from an explicit working directory.
func (h *Harness) RunDaemonInDir(dir string, args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowdPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunDaemonInDir()")
	}
	return h.runAtDir(dir, h.AgentflowdPath, nil, args...)
}

// RunDaemonWithEnv executes the agentflowd binary with extra environment variables.
func (h *Harness) RunDaemonWithEnv(extra map[string]string, args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowdPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunDaemonWithEnv()")
	}
	return h.runAtDir(h.Workspace, h.AgentflowdPath, extra, args...)
}

// RunDaemonWithEnvInDir executes the agentflowd binary with extra environment variables from an explicit directory.
func (h *Harness) RunDaemonWithEnvInDir(dir string, extra map[string]string, args ...string) *CommandResult {
	h.t.Helper()
	if h.AgentflowdPath == "" {
		h.t.Fatal("Harness.Build() must be called before RunDaemonWithEnvInDir()")
	}
	return h.runAtDir(dir, h.AgentflowdPath, extra, args...)
}

func (h *Harness) runAtDir(dir, bin string, extra map[string]string, args ...string) *CommandResult {
	h.t.Helper()

	env := h.Env()
	for k, v := range extra {
		env = setEnv(env, k, v)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil
		}
	}

	return &CommandResult{
		Cmd:      strings.Join(append([]string{bin}, args...), " "),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// AssertSuccess fails the test if the command did not exit 0.
func (r *CommandResult) AssertSuccess(t *testing.T) {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("command failed to run: %v\ncommand: %s\nstdout:\n%s\nstderr:\n%s", r.Err, r.Cmd, r.Stdout, r.Stderr)
	}
	if r.ExitCode != 0 {
		t.Fatalf("command exited %d\ncommand: %s\nstdout:\n%s\nstderr:\n%s", r.ExitCode, r.Cmd, r.Stdout, r.Stderr)
	}
}

// AssertExitCode fails the test if the command did not exit with the expected code.
func (r *CommandResult) AssertExitCode(t *testing.T, want int) {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("command failed to run: %v\ncommand: %s\nstdout:\n%s\nstderr:\n%s", r.Err, r.Cmd, r.Stdout, r.Stderr)
	}
	if r.ExitCode != want {
		t.Fatalf("command exited %d, want %d\ncommand: %s\nstdout:\n%s\nstderr:\n%s", r.ExitCode, want, r.Cmd, r.Stdout, r.Stderr)
	}
}

// Close releases resources. Directories are cleaned up automatically by t.TempDir.
func (h *Harness) Close() {}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
