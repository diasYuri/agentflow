package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

func TestResolveClaudePath(t *testing.T) {
	t.Setenv("CLAUDE_PATH", "/tmp/from-env")

	if actual := resolveClaudePath("/tmp/from-constructor"); actual != "/tmp/from-constructor" {
		t.Fatalf("constructor path did not win: %q", actual)
	}
	if actual := resolveClaudePath(""); actual != "/tmp/from-env" {
		t.Fatalf("env path did not win: %q", actual)
	}
}

func TestMergeClaudeEnvPreservesProcessEnv(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("ANTHROPIC_API_KEY", "from-process")

	env := mergeClaudeEnv(map[string]string{
		"CUSTOM_ENV":        "preserved",
		"ANTHROPIC_API_KEY": "from-workflow",
	})

	if env["CUSTOM_ENV"] != "preserved" {
		t.Fatalf("custom env lost: %#v", env["CUSTOM_ENV"])
	}
	if env["PATH"] != "/usr/bin" {
		t.Fatalf("path not preserved: %#v", env["PATH"])
	}
	if env["ANTHROPIC_API_KEY"] != "from-workflow" {
		t.Fatalf("workflow env did not override process env: %#v", env["ANTHROPIC_API_KEY"])
	}
}

func TestParseClaudeResult(t *testing.T) {
	result, err := parseClaudeResult([]byte(`{
		"type": "result",
		"subtype": "success",
		"session_id": "session-1",
		"result": "{\"status\":\"ok\",\"source\":\"fake-claude\"}",
		"usage": {"input_tokens": 7, "output_tokens": 11}
	}`), true)
	if err != nil {
		t.Fatal(err)
	}

	if result.Text != `{"status":"ok","source":"fake-claude"}` {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{"status": "ok", "source": "fake-claude"}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}
	if result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 11 || result.Usage.TotalTokens != 18 {
		t.Fatalf("usage mismatch: %#v", result.Usage)
	}
	if result.Metadata["session_id"] != "session-1" {
		t.Fatalf("session metadata mismatch: %#v", result.Metadata)
	}
	if len(result.RawEvents) != 1 || result.RawEvents[0].Type != "result" {
		t.Fatalf("raw event mismatch: %#v", result.RawEvents)
	}
}

func TestParseClaudeResultPrefersStructuredOutput(t *testing.T) {
	result, err := parseClaudeResult([]byte(`{
		"type": "result",
		"session_id": "session-structured",
		"result": "The review passed.",
		"structured_output": {"status": "ok", "findings": []}
	}`), true)
	if err != nil {
		t.Fatal(err)
	}

	if result.Text != "The review passed." {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{"status": "ok", "findings": []any{}}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}
}

func TestSandboxArgsReadOnlyRestrictsClaudeTools(t *testing.T) {
	args := sandboxArgs("read-only")

	assertArgPair(t, args, "--tools", "Read,Glob,Grep,LS")
	assertArgPair(t, args, "--allowedTools", "Read,Glob,Grep,LS")
	joinedArgs := strings.Join(args, " ")
	for _, tool := range []string{"Bash", "Write", "Edit", "MultiEdit"} {
		if strings.Contains(joinedArgs, tool) {
			t.Fatalf("read-only args include write-capable tool %q: %#v", tool, args)
		}
	}
}

func TestProviderContractForwardsReadOnlyToolRestrictions(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	fakeClaude := writeFakeClaude(t, dir)
	provider := New(fakeClaude)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := provider.Run(ctx, ports.AgentRequest{
		Prompt: "read only",
		Env: map[string]string{
			"FAKE_CLAUDE_ARGS_FILE": argsPath,
		},
		Sandbox: workflow.SandboxSpec{Mode: "read-only"},
	})
	if err != nil {
		t.Fatal(err)
	}

	args := readArgs(t, argsPath)
	assertArgPair(t, args, "--tools", "Read,Glob,Grep,LS")
	assertArgPair(t, args, "--allowedTools", "Read,Glob,Grep,LS")
	joinedArgs := strings.Join(args, " ")
	for _, tool := range []string{"Bash", "Write", "Edit", "MultiEdit"} {
		if strings.Contains(joinedArgs, tool) {
			t.Fatalf("read-only args include write-capable tool %q: %#v", tool, args)
		}
	}
}

func TestProviderContractForwardsClaudeExecutionOptions(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	schemaPath := filepath.Join(dir, "schema.json")
	envPath := filepath.Join(dir, "env.txt")
	pwdPath := filepath.Join(dir, "pwd.txt")
	workingDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeClaude := writeFakeClaude(t, dir)
	provider := New(fakeClaude)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := provider.Run(ctx, ports.AgentRequest{
		Model:      "claude-contract-model",
		System:     "Do not run shell commands.",
		Prompt:     "return structured\ncontract output",
		WorkingDir: workingDir,
		Env: map[string]string{
			"FAKE_CLAUDE_ARGS_FILE":   argsPath,
			"FAKE_CLAUDE_SCHEMA_FILE": schemaPath,
			"FAKE_CLAUDE_ENV_FILE":    envPath,
			"FAKE_CLAUDE_PWD_FILE":    pwdPath,
			"AGENTFLOW_CONTRACT_ENV":  "forwarded",
		},
		Sandbox: workflow.SandboxSpec{Mode: "workspace-write"},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
				"source": map[string]any{"type": "string"},
			},
			"required":             []any{"status", "source"},
			"additionalProperties": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	args := readArgs(t, argsPath)
	if len(args) < 1 {
		t.Fatalf("no args recorded")
	}
	assertArg(t, args, "-p")
	assertArgPair(t, args, "--output-format", "json")
	assertArgPair(t, args, "--model", "claude-contract-model")
	assertArgPair(t, args, "--append-system-prompt", "Do not run shell commands.")
	assertArgPair(t, args, "--allowedTools", "Read,Write,Edit,MultiEdit,Bash,Glob,Grep,LS")
	if indexOf(args, "--json-schema") == -1 {
		t.Fatalf("missing --json-schema in args: %#v", args)
	}
	if got := args[len(args)-1]; got != "return structured\ncontract output" {
		t.Fatalf("prompt mismatch: %#v", got)
	}
	if env := strings.TrimSpace(readFile(t, envPath)); env != "forwarded" {
		t.Fatalf("env was not forwarded, got %q", env)
	}
	if pwd := strings.TrimSpace(readFile(t, pwdPath)); !samePath(t, pwd, workingDir) {
		t.Fatalf("working dir mismatch: %q", pwd)
	}

	var forwardedSchema map[string]any
	if err := json.Unmarshal([]byte(readFile(t, schemaPath)), &forwardedSchema); err != nil {
		t.Fatalf("schema was not valid JSON: %v", err)
	}
	if forwardedSchema["type"] != "object" {
		t.Fatalf("schema type mismatch: %#v", forwardedSchema)
	}

	if result.Text != "structured response emitted by Claude" {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{"status": "ok", "source": "fake-claude"}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}
	if result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 11 || result.Usage.TotalTokens != 18 {
		t.Fatalf("usage mismatch: %#v", result.Usage)
	}
	if result.Metadata["session_id"] != "contract-session" {
		t.Fatalf("metadata mismatch: %#v", result.Metadata)
	}
}

func writeFakeClaude(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "claude-fake")
	script := `#!/bin/sh
set -eu

if [ -n "${FAKE_CLAUDE_ARGS_FILE:-}" ]; then
  : > "$FAKE_CLAUDE_ARGS_FILE"
  for arg in "$@"; do
    printf '%s\n---ARG---\n' "$arg" >> "$FAKE_CLAUDE_ARGS_FILE"
  done
fi
if [ -n "${FAKE_CLAUDE_ENV_FILE:-}" ]; then
  printf '%s\n' "${AGENTFLOW_CONTRACT_ENV:-}" > "$FAKE_CLAUDE_ENV_FILE"
fi
if [ -n "${FAKE_CLAUDE_PWD_FILE:-}" ]; then
  pwd > "$FAKE_CLAUDE_PWD_FILE"
fi
if [ -n "${FAKE_CLAUDE_SCHEMA_FILE:-}" ]; then
  previous=""
  for arg in "$@"; do
    if [ "$previous" = "--json-schema" ]; then
      printf '%s\n' "$arg" > "$FAKE_CLAUDE_SCHEMA_FILE"
      break
    fi
    previous="$arg"
  done
fi

printf '%s\n' '{"type":"result","subtype":"success","session_id":"contract-session","result":"structured response emitted by Claude","structured_output":{"status":"ok","source":"fake-claude"},"usage":{"input_tokens":7,"output_tokens":11}}'
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	data := strings.TrimSuffix(readFile(t, path), "\n---ARG---\n")
	if data == "" {
		return nil
	}
	return strings.Split(data, "\n---ARG---\n")
}

func samePath(t *testing.T, actual string, expected string) bool {
	t.Helper()
	actualResolved, err := filepath.EvalSymlinks(actual)
	if err != nil {
		t.Fatal(err)
	}
	expectedResolved, err := filepath.EvalSymlinks(expected)
	if err != nil {
		t.Fatal(err)
	}
	return actualResolved == expectedResolved
}

func assertArg(t *testing.T, args []string, key string) {
	t.Helper()
	if indexOf(args, key) == -1 {
		t.Fatalf("arg %s not found in %#v", key, args)
	}
}

func assertArgPair(t *testing.T, args []string, key string, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return
		}
	}
	t.Fatalf("pair %s %s not found in %#v", key, value, args)
}

func indexOf(args []string, value string) int {
	for i, arg := range args {
		if arg == value {
			return i
		}
	}
	return -1
}
