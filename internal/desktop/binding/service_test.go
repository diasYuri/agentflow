package binding

import (
	"context"
	"testing"

	"github.com/diasYuri/agentflow/internal/desktop/adapter"
)

func TestDesktopService_Health(t *testing.T) {
	svc := NewDesktopService(nil)
	h := svc.Health()

	if h.Status != "ok" {
		t.Errorf("expected status ok, got %s", h.Status)
	}
	if h.Version == "" {
		t.Error("expected non-empty version")
	}
	if h.GoVersion == "" {
		t.Error("expected non-empty goVersion")
	}
	if h.OS == "" {
		t.Error("expected non-empty os")
	}
	if h.Arch == "" {
		t.Error("expected non-empty arch")
	}
	if h.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestDesktopService_Greet(t *testing.T) {
	svc := NewDesktopService(nil)

	t.Run("with name", func(t *testing.T) {
		got := svc.Greet("Agentflow")
		want := "Hello, Agentflow!"
		if got != want {
			t.Errorf("Greet(Agentflow) = %q, want %q", got, want)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		got := svc.Greet("")
		want := "Hello, World!"
		if got != want {
			t.Errorf("Greet(\"\") = %q, want %q", got, want)
		}
	})
}

func TestDesktopService_AdapterDelegation(t *testing.T) {
	// Usa o adapter default; os metodos que precisam de filesystem vao retornar
	// erro controlado quando o path nao existe, o que ja valida o binding.
	t.Setenv("HOME", t.TempDir())
	svc := NewDesktopService(nil)
	if err := svc.ServiceStartup(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	t.Run("ValidateWorkflow missing path", func(t *testing.T) {
		got := svc.ValidateWorkflow("non-existent.yaml")
		if got.Valid {
			t.Error("expected invalid for missing workflow")
		}
		if len(got.Errors) == 0 {
			t.Error("expected errors for missing workflow")
		}
	})

	t.Run("GenerateGraph missing path", func(t *testing.T) {
		got := svc.GenerateGraph("non-existent.yaml")
		if got.Valid {
			t.Error("expected invalid for missing workflow")
		}
		if len(got.Errors) == 0 {
			t.Error("expected errors for missing workflow")
		}
	})

	t.Run("LoadWorkflow missing path", func(t *testing.T) {
		_, err := svc.LoadWorkflow("non-existent.yaml")
		if err == nil {
			t.Error("expected error for missing workflow")
		}
	})

	t.Run("GetAppSettings", func(t *testing.T) {
		settings, err := svc.GetAppSettings()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.Theme != "system" {
			t.Errorf("expected default theme system, got %s", settings.Theme)
		}
	})

	t.Run("Project CRUD", func(t *testing.T) {
		if err := svc.AddProject("demo", t.TempDir()); err != nil {
			t.Fatalf("add project: %v", err)
		}
		projects, err := svc.ListProjects()
		if err != nil {
			t.Fatalf("list projects: %v", err)
		}
		if len(projects) != 1 || projects[0].Name != "demo" {
			t.Fatalf("unexpected projects: %#v", projects)
		}
		if err := svc.RemoveProject("demo"); err != nil {
			t.Fatalf("remove project: %v", err)
		}
	})
}

func TestDesktopService_RunBindingErrors(t *testing.T) {
	// Usa adapter sem runtime inicializado para validar que o binding
	// delega corretamente e retorna erros normalizados.
	svc := NewDesktopService(adapter.NewAdapter(nil, nil, nil, nil, nil, nil))
	if err := svc.ServiceStartup(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	t.Run("RunWorkflow", func(t *testing.T) {
		_, err := svc.RunWorkflow(adapter.RunWorkflowRequest{WorkflowRef: "test.yaml"})
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("ListRuns", func(t *testing.T) {
		_, err := svc.ListRuns()
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRun", func(t *testing.T) {
		_, err := svc.GetRun("run-1")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("CancelRun", func(t *testing.T) {
		_, err := svc.CancelRun("run-1")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunEvents", func(t *testing.T) {
		_, err := svc.GetRunEvents("run-1", 0, 10)
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunArtifacts", func(t *testing.T) {
		_, err := svc.GetRunArtifacts("run-1")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunArtifact", func(t *testing.T) {
		_, err := svc.GetRunArtifact("run-1", "file.txt")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunNodes", func(t *testing.T) {
		_, err := svc.GetRunNodes("run-1")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunNode", func(t *testing.T) {
		_, err := svc.GetRunNode("run-1", "a")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunPlan", func(t *testing.T) {
		_, err := svc.GetRunPlan("run-1")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})

	t.Run("GetRunLogs", func(t *testing.T) {
		_, err := svc.GetRunLogs("run-1")
		if err == nil {
			t.Error("expected error for nil runtime")
		}
	})
}
