package pi

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

func TestResolvePiPath(t *testing.T) {
	t.Setenv("AGENTFLOW_PI_PATH", "/tmp/from-agentflow-env")
	t.Setenv("PI_PATH", "/tmp/from-pi-env")

	if actual := resolvePiPath("/tmp/from-constructor"); actual != "/tmp/from-constructor" {
		t.Fatalf("constructor path did not win: %q", actual)
	}
	if actual := resolvePiPath(""); actual != "/tmp/from-agentflow-env" {
		t.Fatalf("agentflow env path did not win: %q", actual)
	}

	t.Setenv("AGENTFLOW_PI_PATH", "")
	if actual := resolvePiPath(""); actual != "/tmp/from-pi-env" {
		t.Fatalf("pi env path did not win: %q", actual)
	}
}

func TestMergePiEnvPreservesProcessEnv(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("PI_API_KEY", "from-process")

	env := mergePiEnv(map[string]string{
		"CUSTOM_ENV": "preserved",
		"PI_API_KEY": "from-workflow",
	})

	if env["CUSTOM_ENV"] != "preserved" {
		t.Fatalf("custom env lost: %#v", env["CUSTOM_ENV"])
	}
	if env["PATH"] != "/usr/bin" {
		t.Fatalf("path not preserved: %#v", env["PATH"])
	}
	if env["PI_API_KEY"] != "from-workflow" {
		t.Fatalf("workflow env did not override process env: %#v", env["PI_API_KEY"])
	}
}

func TestBuildArgsRestrictsReadOnlyTools(t *testing.T) {
	args := buildArgs(ports.AgentRequest{
		Model:   "openai/gpt-4o",
		System:  "Stay focused.",
		Sandbox: workflow.SandboxSpec{Mode: "read-only"},
	})

	assertArgPair(t, args, "--mode", "rpc")
	assertArg(t, args, "--no-session")
	assertArgPair(t, args, "--model", "openai/gpt-4o")
	assertArgPair(t, args, "--append-system-prompt", "Stay focused.")
	assertArgPair(t, args, "--tools", readOnlyTools)
}

func TestBuildArgsUsesPiDefaultToolsForWriteModes(t *testing.T) {
	for _, mode := range []string{"", "workspace-write", "danger-full-access"} {
		args := buildArgs(ports.AgentRequest{Sandbox: workflow.SandboxSpec{Mode: mode}})
		if indexOf(args, "--tools") != -1 {
			t.Fatalf("expected PI default tools for mode %q, got %#v", mode, args)
		}
	}
}

func TestProviderContractForwardsPiExecutionOptions(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	envPath := filepath.Join(dir, "env.txt")
	pwdPath := filepath.Join(dir, "pwd.txt")
	promptPath := filepath.Join(dir, "prompt.txt")
	workingDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fakePi := writeFakePi(t, dir, fakePiSuccess)
	provider := New(fakePi)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := provider.Run(ctx, ports.AgentRequest{
		Model:      "openai/gpt-4o",
		System:     "Do not run shell commands.",
		Prompt:     "return structured\ncontract output",
		WorkingDir: workingDir,
		Env: map[string]string{
			"FAKE_PI_ARGS_FILE":   argsPath,
			"FAKE_PI_ENV_FILE":    envPath,
			"FAKE_PI_PWD_FILE":    pwdPath,
			"FAKE_PI_PROMPT_FILE": promptPath,
			"AGENTFLOW_PI_ENV":    "forwarded",
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

	args := readArgs(t, argsPath)
	assertArgPair(t, args, "--mode", "rpc")
	assertArg(t, args, "--no-session")
	assertArgPair(t, args, "--model", "openai/gpt-4o")
	assertArgPair(t, args, "--append-system-prompt", "Do not run shell commands.")
	assertArgPair(t, args, "--tools", readOnlyTools)
	if env := strings.TrimSpace(readFile(t, envPath)); env != "forwarded" {
		t.Fatalf("env was not forwarded, got %q", env)
	}
	if pwd := strings.TrimSpace(readFile(t, pwdPath)); !samePath(t, pwd, workingDir) {
		t.Fatalf("working dir mismatch: %q", pwd)
	}
	promptLine := readFile(t, promptPath)
	if !strings.Contains(promptLine, "return structured\\ncontract output") {
		t.Fatalf("prompt was not sent over RPC: %q", promptLine)
	}
	if !strings.Contains(promptLine, "Return only the final assistant message as JSON") {
		t.Fatalf("structured output instruction missing from RPC prompt: %q", promptLine)
	}

	if result.Text != `{"status":"ok","source":"fake-pi"}` {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{"status": "ok", "source": "fake-pi"}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}
	if result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 11 || result.Usage.TotalTokens != 18 {
		t.Fatalf("usage mismatch: %#v", result.Usage)
	}
	if len(result.RawEvents) != 3 || result.RawEvents[0].Type != "agent_start" || result.RawEvents[2].Type != "agent_end" {
		t.Fatalf("raw events mismatch: %#v", result.RawEvents)
	}
}

func TestProviderReturnsPromptRejection(t *testing.T) {
	provider := New(writeFakePi(t, t.TempDir(), fakePiRejectPrompt))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := provider.Run(ctx, ports.AgentRequest{Prompt: "reject"})
	if err == nil {
		t.Fatal("expected prompt rejection error")
	}
	if !strings.Contains(err.Error(), "rejected by fake pi") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderReturnsMalformedJSONError(t *testing.T) {
	provider := New(writeFakePi(t, t.TempDir(), fakePiMalformedJSON))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := provider.Run(ctx, ports.AgentRequest{
		Prompt:       "malformed",
		OutputSchema: map[string]any{"type": "object"},
	})
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if !strings.Contains(err.Error(), "final text=") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderReturnsStructuredParseError(t *testing.T) {
	provider := New(writeFakePi(t, t.TempDir(), fakePiInvalidStructuredText))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := provider.Run(ctx, ports.AgentRequest{
		Prompt: "invalid structured",
		OutputSchema: map[string]any{
			"type":     "object",
			"required": []any{"summary"},
		},
	})
	if err == nil {
		t.Fatal("expected structured parse error")
	}
	if !strings.Contains(err.Error(), "final text=") {
		t.Fatalf("expected final text in error, got %v", err)
	}
}

func TestProviderFallsBackToAssistantTextFromEvents(t *testing.T) {
	provider := New(writeFakePi(t, t.TempDir(), fakePiTextInEvents))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := provider.Run(ctx, ports.AgentRequest{
		Prompt:       "event text",
		OutputSchema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != `{"status":"ok","source":"event"}` {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{"status": "ok", "source": "event"}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}
}

func TestProviderRetriesStructuredOutputInSameSession(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.txt")
	provider := New(writeFakePi(t, dir, fakePiStructuredRetry))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := provider.Run(ctx, ports.AgentRequest{
		Prompt: "implement the plan",
		Env: map[string]string{
			"FAKE_PI_PROMPT_FILE": promptPath,
		},
		OutputSchema: map[string]any{
			"type": "object",
			"required": []any{
				"plan_path",
				"status",
				"summary",
				"files_changed",
				"validation",
				"follow_ups",
			},
			"properties": map[string]any{
				"plan_path":     map[string]any{"type": "string"},
				"status":        map[string]any{"type": "string"},
				"summary":       map[string]any{"type": "string"},
				"files_changed": map[string]any{"type": "array"},
				"validation":    map[string]any{"type": "array"},
				"follow_ups":    map[string]any{"type": "array"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Text != `{"plan_path":"001-provider-core.md","status":"success","summary":"Provider refatorado com fallback de schema","files_changed":["internal/adapters/agent/pi/provider.go","internal/adapters/agent/pi/provider_test.go"],"validation":["go test ./internal/adapters/agent/pi/..."],"follow_ups":[]}` {
		t.Fatalf("text mismatch: %q", result.Text)
	}
	if !reflect.DeepEqual(result.JSON, map[string]any{
		"plan_path":     "001-provider-core.md",
		"status":        "success",
		"summary":       "Provider refatorado com fallback de schema",
		"files_changed": []any{"internal/adapters/agent/pi/provider.go", "internal/adapters/agent/pi/provider_test.go"},
		"validation":    []any{"go test ./internal/adapters/agent/pi/..."},
		"follow_ups":    []any{},
	}) {
		t.Fatalf("json mismatch: %#v", result.JSON)
	}

	promptLog := readFile(t, promptPath)
	if strings.Count(promptLog, "---PROMPT---") != 2 {
		t.Fatalf("expected 2 prompt turns, got log %q", promptLog)
	}
	if !strings.Contains(promptLog, "missing required property \\\"summary\\\"") {
		t.Fatalf("expected schema validation error in retry prompt, got %q", promptLog)
	}
}

func TestProviderReturnsAgentEndErrorWhenTextIsEmpty(t *testing.T) {
	provider := New(writeFakePi(t, t.TempDir(), fakePiAgentEndError))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := provider.Run(ctx, ports.AgentRequest{
		Prompt:       "agent error",
		OutputSchema: map[string]any{"type": "object"},
	})
	if err == nil {
		t.Fatal("expected agent error")
	}
	if !strings.Contains(err.Error(), "Connection error.") {
		t.Fatalf("expected PI error in provider error, got %v", err)
	}
}

type fakePiMode string

const (
	fakePiSuccess               fakePiMode = "success"
	fakePiRejectPrompt          fakePiMode = "reject"
	fakePiMalformedJSON         fakePiMode = "malformed"
	fakePiInvalidStructuredText fakePiMode = "invalid-structured"
	fakePiStructuredRetry       fakePiMode = "structured-retry"
	fakePiTextInEvents          fakePiMode = "text-in-events"
	fakePiAgentEndError         fakePiMode = "agent-end-error"
)

func writeFakePi(t *testing.T, dir string, mode fakePiMode) string {
	t.Helper()
	path := filepath.Join(dir, "pi-fake")
	script := `#!/bin/sh
set -eu

turn=0
assistant_text='{"status":"ok","source":"fake-pi"}'

if [ -n "${FAKE_PI_ARGS_FILE:-}" ]; then
  : > "$FAKE_PI_ARGS_FILE"
  for arg in "$@"; do
    printf '%s\n---ARG---\n' "$arg" >> "$FAKE_PI_ARGS_FILE"
  done
fi
if [ -n "${FAKE_PI_ENV_FILE:-}" ]; then
  printf '%s\n' "${AGENTFLOW_PI_ENV:-}" > "$FAKE_PI_ENV_FILE"
fi
if [ -n "${FAKE_PI_PWD_FILE:-}" ]; then
  pwd > "$FAKE_PI_PWD_FILE"
fi

while IFS= read -r line; do
  case "$line" in
    *'"type":"prompt"'*)
      turn=$((turn + 1))
      if [ -n "${FAKE_PI_PROMPT_FILE:-}" ]; then
        printf '%s\n---PROMPT---\n' "$line" >> "$FAKE_PI_PROMPT_FILE"
      fi
      assistant_text='{"status":"ok","source":"fake-pi"}'
      assistant_text_json=$(printf '%s' "$assistant_text" | sed 's/\\/\\\\/g; s/"/\\"/g')
      case "` + string(mode) + `" in
        reject)
          printf '%s\n' '{"id":"agentflow-prompt","type":"response","command":"prompt","success":false,"error":"rejected by fake pi"}'
          exit 0
          ;;
        malformed)
          assistant_text='not json'
          ;;
        invalid-structured)
          assistant_text='{"plan_path":"001-provider-core.md","status":"success","files_changed":["internal/adapters/agent/pi/provider.go"],"validation":["go test ./internal/adapters/agent/pi/..."],"follow_ups":[]}'
          ;;
        structured-retry)
          if [ "$turn" -eq 1 ]; then
            assistant_text='{"plan_path":"001-provider-core.md","status":"success","files_changed":["internal/adapters/agent/pi/provider.go"],"validation":["go test ./internal/adapters/agent/pi/..."],"follow_ups":[]}'
          else
            assistant_text='{"plan_path":"001-provider-core.md","status":"success","summary":"Provider refatorado com fallback de schema","files_changed":["internal/adapters/agent/pi/provider.go","internal/adapters/agent/pi/provider_test.go"],"validation":["go test ./internal/adapters/agent/pi/..."],"follow_ups":[]}'
          fi
          ;;
        text-in-events|agent-end-error)
          assistant_text='not json'
          ;;
      esac
      assistant_text_json=$(printf '%s' "$assistant_text" | sed 's/\\/\\\\/g; s/"/\\"/g')
      printf '%s\n' '{"id":"agentflow-prompt","type":"response","command":"prompt","success":true}'
      printf '%s\n' '{"type":"agent_start","session_id":"contract-session"}'
      printf '%s\n' '{"type":"message_update","delta":"working"}'
      case "` + string(mode) + `" in
        text-in-events)
          printf '%s\n' '{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"{\"status\":\"ok\",\"source\":\"event\"}"}]}]}'
          ;;
        agent-end-error)
          printf '%s\n' '{"type":"agent_end","messages":[{"role":"assistant","content":[],"stopReason":"error","errorMessage":"Connection error."}]}'
          ;;
        *)
          printf '%s\n' '{"type":"agent_end","session_id":"contract-session"}'
          ;;
      esac
      ;;
    *'"type":"get_last_assistant_text"'*)
      case "` + string(mode) + `" in
        text-in-events|agent-end-error)
          printf '%s\n' '{"id":"agentflow-text","type":"response","command":"get_last_assistant_text","success":true,"data":{}}'
          ;;
        *)
          printf '%s\n' '{"id":"agentflow-text","type":"response","command":"get_last_assistant_text","success":true,"data":{"text":"'"$assistant_text_json"'"}}'
          ;;
      esac
      ;;
    *'"type":"get_session_stats"'*)
      printf '%s\n' '{"id":"agentflow-stats","type":"response","command":"get_session_stats","success":true,"data":{"tokens":{"input":7,"output":11,"total":18}}}'
      exit 0
      ;;
  esac
done
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

func TestExtractUsage(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"tokens": map[string]any{
				"input":  float64(3),
				"output": float64(4),
				"total":  float64(7),
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	usage := extractUsage(decoded)
	if usage == nil || usage.InputTokens != 3 || usage.OutputTokens != 4 || usage.TotalTokens != 7 {
		t.Fatalf("usage mismatch: %#v", usage)
	}
}
