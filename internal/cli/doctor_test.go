package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/version"
)

func TestDoctorCommandOutputsGroups(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Agentflow", "Daemon", "Environment", "Binaries", "Workflows"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in doctor output, got %q", want, got)
		}
	}
}

func TestDoctorCommandJSON(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"groups"`) {
		t.Fatalf("expected groups key in json, got %q", got)
	}
	if !strings.Contains(got, `"status"`) {
		t.Fatalf("expected status key in json, got %q", got)
	}
}

func TestDoctorCommandFailsOnFailures(t *testing.T) {
	cmd := newDoctorCommandWithDeps(doctorDeps{
		findAgentflowd: func() (string, error) {
			return "", errors.New("not found")
		},
		daemonStatus: func(context.Context) (daemon.DaemonStatus, error) {
			return daemon.DaemonStatus{}, errors.New("not running")
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		stat: func(string) (os.FileInfo, error) {
			return nil, nil
		},
		writeFile: func(string, []byte, os.FileMode) error {
			return nil
		},
		remove: func(string) error { return nil },
		lookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		workflowFiles: func(string) ([]string, error) {
			return []string{"/home/test/.agentflow/workflows/bad.yaml"}, nil
		},
		validateWorkflow: func(context.Context, string) error {
			return errors.New("invalid workflow")
		},
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected doctor command to fail")
	}
	if !strings.Contains(err.Error(), "failing check") {
		t.Fatalf("expected actionable failure error, got %v", err)
	}
}

func TestDoctorCheckAgentflow(t *testing.T) {
	oldVersion := version.Version
	version.Version = "test-1.0"
	t.Cleanup(func() { version.Version = oldVersion })

	deps := defaultDoctorDeps()
	group := checkAgentflow(deps)
	if group.Title != "Agentflow" {
		t.Fatalf("unexpected group title: %q", group.Title)
	}
	if len(group.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "ok" {
		t.Fatalf("expected ok status, got %q", group.Checks[0].Status)
	}
	if !strings.Contains(group.Checks[0].Message, "test-1.0") {
		t.Fatalf("expected version in message, got %q", group.Checks[0].Message)
	}
}

func TestDoctorCheckAgentflowdBinaryMissing(t *testing.T) {
	deps := doctorDeps{
		findAgentflowd: func() (string, error) {
			return "", errors.New("not found")
		},
		daemonStatus: func(context.Context) (daemon.DaemonStatus, error) {
			return daemon.DaemonStatus{}, errors.New("not running")
		},
	}
	group := checkAgentflowd(context.Background(), deps)
	if len(group.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "fail" {
		t.Fatalf("expected fail for binary, got %q", group.Checks[0].Status)
	}
	if group.Checks[1].Status != "warn" {
		t.Fatalf("expected warn for daemon, got %q", group.Checks[1].Status)
	}
}

func TestDoctorCheckAgentflowdRunning(t *testing.T) {
	deps := doctorDeps{
		findAgentflowd: func() (string, error) {
			return "/usr/local/bin/agentflowd", nil
		},
		daemonStatus: func(context.Context) (daemon.DaemonStatus, error) {
			return daemon.DaemonStatus{Running: true, PID: 42, Socket: "/tmp/agentflowd.sock"}, nil
		},
	}
	group := checkAgentflowd(context.Background(), deps)
	if len(group.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "ok" {
		t.Fatalf("expected ok for binary, got %q", group.Checks[0].Status)
	}
	if group.Checks[1].Status != "ok" {
		t.Fatalf("expected ok for daemon, got %q", group.Checks[1].Status)
	}
}

func TestDoctorCheckEnvironmentWritable(t *testing.T) {
	deps := doctorDeps{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		stat:        func(string) (os.FileInfo, error) { return nil, errors.New("not exists") },
		mkdirAll:    func(string, os.FileMode) error { return nil },
	}
	group := checkEnvironment(context.Background(), deps)
	if len(group.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "ok" {
		t.Fatalf("expected ok for created dir, got %q", group.Checks[0].Status)
	}
}

func TestDoctorCheckEnvironmentNotWritable(t *testing.T) {
	deps := doctorDeps{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		stat:        func(string) (os.FileInfo, error) { return nil, nil },
		writeFile:   func(string, []byte, os.FileMode) error { return errors.New("permission denied") },
		remove:      func(string) error { return nil },
	}
	group := checkEnvironment(context.Background(), deps)
	if len(group.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "fail" {
		t.Fatalf("expected fail for unwritable dir, got %q", group.Checks[0].Status)
	}
	if !strings.Contains(group.Checks[0].Action, "permissions") {
		t.Fatalf("expected action hint, got %q", group.Checks[0].Action)
	}
}

func TestDoctorCheckBinaries(t *testing.T) {
	deps := doctorDeps{
		lookPath: func(name string) (string, error) {
			if name == "go" {
				return "/usr/local/go/bin/go", nil
			}
			return "", errors.New("not found")
		},
	}
	group := checkBinaries(deps)
	if len(group.Checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(group.Checks))
	}
	if group.Checks[0].Name != "codex" || group.Checks[0].Status != "warn" {
		t.Fatalf("expected codex warn, got %q %q", group.Checks[0].Name, group.Checks[0].Status)
	}
	if group.Checks[1].Name != "claude" || group.Checks[1].Status != "warn" {
		t.Fatalf("expected claude warn, got %q %q", group.Checks[1].Name, group.Checks[1].Status)
	}
	if group.Checks[2].Name != "go" || group.Checks[2].Status != "ok" {
		t.Fatalf("expected go ok, got %q %q", group.Checks[2].Name, group.Checks[2].Status)
	}
}

func TestDoctorCheckBinariesSkipsGoOutsideDevelopmentBuild(t *testing.T) {
	oldVersion := version.Version
	version.Version = "1.2.3"
	t.Cleanup(func() { version.Version = oldVersion })

	deps := doctorDeps{
		lookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
	}
	group := checkBinaries(deps)
	if len(group.Checks) != 2 {
		t.Fatalf("expected 2 checks when not in development mode, got %d", len(group.Checks))
	}
	for _, check := range group.Checks {
		if check.Name == "go" {
			t.Fatalf("did not expect go check outside development builds")
		}
	}
}

func TestDoctorCheckWorkflowsValidatesFiles(t *testing.T) {
	deps := doctorDeps{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		workflowFiles: func(string) ([]string, error) {
			return []string{"/home/test/.agentflow/workflows/a.yaml"}, nil
		},
		validateWorkflow: func(context.Context, string) error { return nil },
	}
	group := checkWorkflows(context.Background(), deps)
	if len(group.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "ok" {
		t.Fatalf("expected ok for valid workflow, got %q", group.Checks[0].Status)
	}
}

func TestDoctorCheckWorkflowsInvalidFile(t *testing.T) {
	deps := doctorDeps{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		workflowFiles: func(string) ([]string, error) {
			return []string{"/home/test/.agentflow/workflows/bad.yaml"}, nil
		},
		validateWorkflow: func(context.Context, string) error { return errors.New("missing nodes") },
	}
	group := checkWorkflows(context.Background(), deps)
	if len(group.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(group.Checks))
	}
	if group.Checks[0].Status != "fail" {
		t.Fatalf("expected fail for invalid workflow, got %q", group.Checks[0].Status)
	}
}

func TestDoctorRenderReport(t *testing.T) {
	report := doctorReport{
		Groups: []doctorGroup{
			{
				Title: "Test",
				Checks: []doctorCheck{
					{Name: "ok-check", Status: "ok", Message: "fine"},
					{Name: "warn-check", Status: "warn", Message: "careful", Action: "do something"},
					{Name: "fail-check", Status: "fail", Message: "broken", Action: "fix it"},
				},
			},
		},
	}
	var out bytes.Buffer
	renderDoctorReport(&out, report, false)
	got := out.String()
	if !strings.Contains(got, "ok-check") {
		t.Fatalf("expected ok-check in output, got %q", got)
	}
	if !strings.Contains(got, "warn-check") {
		t.Fatalf("expected warn-check in output, got %q", got)
	}
	if !strings.Contains(got, "fail-check") {
		t.Fatalf("expected fail-check in output, got %q", got)
	}
	if !strings.Contains(got, "fix it") {
		t.Fatalf("expected action in output, got %q", got)
	}
}
