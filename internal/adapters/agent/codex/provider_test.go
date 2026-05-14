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
