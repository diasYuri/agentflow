package app

import "testing"

func TestNewRunWorkflowUseCaseRegistersAgentProviders(t *testing.T) {
	uc, err := NewRunWorkflowUseCase(RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, provider := range []string{"codex", "claude"} {
		if !uc.Agents.HasProvider(provider) {
			t.Fatalf("expected provider %q to be registered", provider)
		}
	}
}
