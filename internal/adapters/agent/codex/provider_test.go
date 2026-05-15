package codex

import (
	"testing"

	codexsdk "github.com/diasYuri/go-codex-sdk"
)

func TestNormalizeSandboxMode(t *testing.T) {
	tests := map[string]codexsdk.SandboxMode{
		"":                   "",
		"read-only":          codexsdk.SandboxModeReadOnly,
		"workspace-write":    codexsdk.SandboxModeWorkspaceWrite,
		"danger-full-access": codexsdk.SandboxModeDangerFullAccess,
		"seatbelt":           codexsdk.SandboxModeWorkspaceWrite,
		"unsupported":        "",
	}

	for input, expected := range tests {
		if actual := normalizeSandboxMode(input); actual != expected {
			t.Fatalf("normalizeSandboxMode(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestMergeCodexEnvPreservesProcessEnv(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("OPENAI_API_KEY", "from-process")

	env := mergeCodexEnv(map[string]string{
		"CUSTOM_ENV":     "preserved",
		"OPENAI_API_KEY": "from-workflow",
	})

	if env["CUSTOM_ENV"] != "preserved" {
		t.Fatalf("custom env lost: %#v", env["CUSTOM_ENV"])
	}
	if env["PATH"] != "/usr/bin" {
		t.Fatalf("path not preserved: %#v", env["PATH"])
	}
	if env["CODEX_HOME"] != codexHome {
		t.Fatalf("CODEX_HOME not preserved: %#v", env["CODEX_HOME"])
	}
	if env["OPENAI_API_KEY"] != "from-workflow" {
		t.Fatalf("workflow env did not override process env: %#v", env["OPENAI_API_KEY"])
	}
}
