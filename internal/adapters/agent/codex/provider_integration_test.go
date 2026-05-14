//go:build codex_integration

package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

func TestProviderContractForwardsCodexExecutionOptions(t *testing.T) {
	requireCodexAvailable(t)

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	schemaPath := filepath.Join(dir, "schema.json")
	envPath := filepath.Join(dir, "env.txt")
	workingDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeCodex := writeFakeCodex(t, dir)
	provider := New(fakeCodex)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := provider.Run(ctx, ports.AgentRequest{
		Model:      "codex-contract-model",
		System:     "Do not run shell commands.",
		Prompt:     "return structured contract output",
		WorkingDir: workingDir,
		Env: map[string]string{
			"FAKE_CODEX_ARGS_FILE":   argsPath,
			"FAKE_CODEX_SCHEMA_FILE": schemaPath,
			"FAKE_CODEX_ENV_FILE":    envPath,
			"agentflow_CONTRACT_ENV": "forwarded",
		},
		Sandbox: workflow.SandboxSpec{Mode: "read-only"},
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

	args := readLines(t, argsPath)
	assertArgPair(t, args, "--model", "codex-contract-model")
	assertArgPair(t, args, "--sandbox", "read-only")
	assertArgPair(t, args, "--cd", workingDir)
	if indexOf(args, "--skip-git-repo-check") == -1 {
		t.Fatalf("missing --skip-git-repo-check in args: %#v", args)
	}
	if indexOf(args, "--output-schema") == -1 {
		t.Fatalf("missing --output-schema in args: %#v", args)
	}
	if indexOf(args, "--config") == -1 || indexOf(args, `approval_policy="never"`) == -1 {
		t.Fatalf("missing approval policy in args: %#v", args)
	}
	if got := args[len(args)-1]; got != "User:\nreturn structured contract output" {
		t.Fatalf("prompt mismatch: %#v", args)
	}
	if prompt := strings.Join(args[len(args)-1:], "\n"); !strings.Contains(prompt, "System:\nDo not run shell commands.") {
		t.Fatalf("system prompt was not forwarded: %#v", args)
	}
	if env := strings.TrimSpace(readFile(t, envPath)); env != "forwarded" {
		t.Fatalf("env was not forwarded, got %q", env)
	}

	var forwardedSchema map[string]any
	if err := json.Unmarshal([]byte(readFile(t, schemaPath)), &forwardedSchema); err != nil {
		t.Fatalf("schema was not valid JSON: %v", err)
	}
	if forwardedSchema["type"] != "object" {
		t.Fatalf("schema type mismatch: %#v", forwardedSchema)
	}

	if result.Text != `{"status":"ok","source":"fake-codex"}` {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{"status": "ok", "source": "fake-codex"}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}
	if result.Usage == nil || result.Usage.InputTokens != 3 || result.Usage.OutputTokens != 5 || result.Usage.TotalTokens != 8 {
		t.Fatalf("usage mismatch: %#v", result.Usage)
	}
	if len(result.RawEvents) != 1 || result.RawEvents[0].Type != "agent_message" {
		t.Fatalf("raw events mismatch: %#v", result.RawEvents)
	}
}

func requireCodexAvailable(t *testing.T) {
	t.Helper()
	if os.Getenv("CODEX_PATH") != "" {
		return
	}
	if _, err := exec.LookPath("codex"); err == nil {
		return
	} else if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("look up codex: %v", err)
	}
	t.Skip("set CODEX_PATH or install codex to run codex integration tests")
}

func writeFakeCodex(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "codex-fake")
	script := `#!/bin/sh
set -eu

if [ -n "${FAKE_CODEX_ARGS_FILE:-}" ]; then
  : > "$FAKE_CODEX_ARGS_FILE"
  for arg in "$@"; do
    printf '%s\n' "$arg" >> "$FAKE_CODEX_ARGS_FILE"
  done
fi
if [ -n "${FAKE_CODEX_ENV_FILE:-}" ]; then
  printf '%s\n' "${agentflow_CONTRACT_ENV:-}" > "$FAKE_CODEX_ENV_FILE"
fi
if [ -n "${FAKE_CODEX_SCHEMA_FILE:-}" ]; then
  previous=""
  for arg in "$@"; do
    if [ "$previous" = "--output-schema" ]; then
      cat "$arg" > "$FAKE_CODEX_SCHEMA_FILE"
      break
    fi
    previous="$arg"
  done
fi

printf '%s\n' '{"type":"thread.started","thread_id":"contract-thread"}'
printf '%s\n' '{"type":"item.completed","item":{"id":"msg_1","type":"agent_message","text":"{\"status\":\"ok\",\"source\":\"fake-codex\"}"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":3,"cached_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}'
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

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data := strings.TrimSpace(readFile(t, path))
	if data == "" {
		return nil
	}
	return strings.Split(data, "\n")
}
