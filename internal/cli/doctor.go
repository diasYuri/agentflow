package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/version"
)

// doctorDeps abstracts external dependencies so checks stay testable.
type doctorDeps struct {
	lookPath         func(string) (string, error)
	getenv           func(string) string
	stat             func(string) (os.FileInfo, error)
	mkdirAll         func(string, os.FileMode) error
	writeFile        func(string, []byte, os.FileMode) error
	remove           func(string) error
	daemonStatus     func(context.Context) (daemon.DaemonStatus, error)
	findAgentflowd   func() (string, error)
	userHomeDir      func() (string, error)
	workflowFiles    func(dir string) ([]string, error)
	validateWorkflow func(ctx context.Context, ref string) error
}

func defaultDoctorDeps() doctorDeps {
	return doctorDeps{
		lookPath:       exec.LookPath,
		getenv:         os.Getenv,
		stat:           os.Stat,
		mkdirAll:       os.MkdirAll,
		writeFile:      os.WriteFile,
		remove:         os.Remove,
		daemonStatus:   defaultDoctorDaemonStatus,
		findAgentflowd: findAgentflowd,
		userHomeDir:    os.UserHomeDir,
		workflowFiles:  defaultWorkflowFiles,
		validateWorkflow: func(ctx context.Context, ref string) error {
			uc, err := buildUseCase(&options{logFormat: "text"})
			if err != nil {
				return err
			}
			_, err = uc.Validate(ctx, ref)
			return err
		},
	}
}

func defaultDoctorDaemonStatus(ctx context.Context) (daemon.DaemonStatus, error) {
	return newDaemonClient("").Status(ctx)
}

func defaultWorkflowFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out, nil
}

// doctorCheck represents a single environment check.
type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

// doctorGroup groups related checks under a heading.
type doctorGroup struct {
	Title  string        `json:"title"`
	Checks []doctorCheck `json:"checks"`
}

// doctorReport is the aggregate output of all checks.
type doctorReport struct {
	Groups []doctorGroup `json:"groups"`
}

func newDoctorCommandWithDeps(deps doctorDeps) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the local environment for common problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := runDoctorChecks(cmd.Context(), deps)
			if jsonOutput {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				renderDoctorReport(cmd.OutOrStdout(), report, isInteractiveWriter(cmd.OutOrStdout()))
			}
			return doctorReportError(report)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output report in JSON format")
	return cmd
}

func newDoctorCommand() *cobra.Command {
	return newDoctorCommandWithDeps(defaultDoctorDeps())
}

func runDoctorChecks(ctx context.Context, deps doctorDeps) doctorReport {
	var groups []doctorGroup
	groups = append(groups, checkAgentflow(deps))
	groups = append(groups, checkAgentflowd(ctx, deps))
	groups = append(groups, checkEnvironment(ctx, deps))
	groups = append(groups, checkBinaries(deps))
	groups = append(groups, checkWorkflows(ctx, deps))
	return doctorReport{Groups: groups}
}

func checkAgentflow(deps doctorDeps) doctorGroup {
	info := version.GetInfo()
	return doctorGroup{
		Title: "Agentflow",
		Checks: []doctorCheck{
			{
				Name:    "version",
				Status:  "ok",
				Message: info.String(),
			},
		},
	}
}

func checkAgentflowd(ctx context.Context, deps doctorDeps) doctorGroup {
	var checks []doctorCheck

	path, err := deps.findAgentflowd()
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "binary",
			Status:  "fail",
			Message: "agentflowd binary not found",
			Action:  "build with: go build ./cmd/agentflowd",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "binary",
			Status:  "ok",
			Message: path,
		})
	}

	status, err := deps.daemonStatus(ctx)
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "daemon",
			Status:  "warn",
			Message: "agentflowd is not running",
			Action:  "start with: agentflow daemon start",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "daemon",
			Status:  "ok",
			Message: fmt.Sprintf("running (pid %d, socket %s)", status.PID, status.Socket),
		})
	}

	return doctorGroup{Title: "Daemon", Checks: checks}
}

func checkEnvironment(_ context.Context, deps doctorDeps) doctorGroup {
	var checks []doctorCheck

	home, err := deps.userHomeDir()
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "home directory",
			Status:  "fail",
			Message: "unable to determine user home directory",
			Action:  "ensure HOME is set",
		})
		return doctorGroup{Title: "Environment", Checks: checks}
	}

	agentflowDir := filepath.Join(home, ".agentflow")
	if _, err := deps.stat(agentflowDir); err != nil {
		if err := deps.mkdirAll(agentflowDir, 0o755); err != nil {
			checks = append(checks, doctorCheck{
				Name:    "data directory",
				Status:  "fail",
				Message: fmt.Sprintf("%s does not exist and cannot be created: %v", agentflowDir, err),
				Action:  "check permissions on your home directory",
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    "data directory",
				Status:  "ok",
				Message: fmt.Sprintf("created %s", agentflowDir),
			})
		}
	} else {
		probe := filepath.Join(agentflowDir, ".doctor-write-test")
		if err := deps.writeFile(probe, []byte("probe"), 0o644); err != nil {
			checks = append(checks, doctorCheck{
				Name:    "data directory",
				Status:  "fail",
				Message: fmt.Sprintf("%s is not writable: %v", agentflowDir, err),
				Action:  "fix permissions on " + agentflowDir,
			})
		} else {
			_ = deps.remove(probe)
			checks = append(checks, doctorCheck{
				Name:    "data directory",
				Status:  "ok",
				Message: agentflowDir,
			})
		}
	}

	return doctorGroup{Title: "Environment", Checks: checks}
}

func checkBinaries(deps doctorDeps) doctorGroup {
	var checks []doctorCheck
	getenv := deps.getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	for _, name := range []string{"codex", "claude", "bun", "agentflow-extension-rpc"} {
		if name == "agentflow-extension-rpc" {
			if path := getenv("AGENTFLOW_EXTENSION_RPC"); path != "" {
				checks = append(checks, doctorCheck{
					Name:    name,
					Status:  "ok",
					Message: path,
				})
				continue
			}
		}
		if path, err := deps.lookPath(name); err != nil {
			checks = append(checks, doctorCheck{
				Name:    name,
				Status:  "warn",
				Message: fmt.Sprintf("%s not found in PATH", name),
				Action:  "install or add to PATH",
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    name,
				Status:  "ok",
				Message: path,
			})
		}
	}

	if isDevelopmentBuild() {
		if path, err := deps.lookPath("go"); err != nil {
			checks = append(checks, doctorCheck{
				Name:    "go",
				Status:  "warn",
				Message: "go not found in PATH",
				Action:  "install Go or add to PATH for development builds",
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    "go",
				Status:  "ok",
				Message: path,
			})
		}
	}

	return doctorGroup{Title: "Binaries", Checks: checks}
}

func isDevelopmentBuild() bool {
	return version.GetInfo().Version == "dev"
}

func doctorReportError(report doctorReport) error {
	failures := doctorFailureCount(report)
	if failures == 0 {
		return nil
	}
	return fmt.Errorf("doctor found %d failing check(s); fix the reported failures and run again", failures)
}

func doctorFailureCount(report doctorReport) int {
	var failures int
	for _, group := range report.Groups {
		for _, check := range group.Checks {
			if check.Status == "fail" {
				failures++
			}
		}
	}
	return failures
}

func checkWorkflows(ctx context.Context, deps doctorDeps) doctorGroup {
	var checks []doctorCheck

	home, err := deps.userHomeDir()
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "workflow directory",
			Status:  "warn",
			Message: "unable to determine user home directory",
		})
		return doctorGroup{Title: "Workflows", Checks: checks}
	}

	workflowDir := filepath.Join(home, ".agentflow", "workflows")
	files, err := deps.workflowFiles(workflowDir)
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "workflow directory",
			Status:  "warn",
			Message: fmt.Sprintf("unable to read %s: %v", workflowDir, err),
		})
		return doctorGroup{Title: "Workflows", Checks: checks}
	}

	if len(files) == 0 {
		checks = append(checks, doctorCheck{
			Name:    "local workflows",
			Status:  "ok",
			Message: "no workflows found in " + workflowDir,
		})
		return doctorGroup{Title: "Workflows", Checks: checks}
	}

	var invalid int
	for _, f := range files {
		if err := deps.validateWorkflow(ctx, f); err != nil {
			invalid++
			checks = append(checks, doctorCheck{
				Name:    filepath.Base(f),
				Status:  "fail",
				Message: err.Error(),
				Action:  "fix the workflow file",
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    filepath.Base(f),
				Status:  "ok",
				Message: "valid",
			})
		}
	}

	return doctorGroup{Title: "Workflows", Checks: checks}
}

func renderDoctorReport(w io.Writer, report doctorReport, colored bool) {
	f := newCLIFormat(colored)
	for _, group := range report.Groups {
		fmt.Fprintln(w, f.section(group.Title))
		for _, check := range group.Checks {
			icon := "✓"
			if check.Status == "warn" {
				icon = "!"
			} else if check.Status == "fail" {
				icon = "✗"
			}
			line := fmt.Sprintf("  %s %s: %s", icon, check.Name, check.Message)
			if colored {
				switch check.Status {
				case "ok":
					line = fmt.Sprintf("  %s %s: %s", f.render(f.successStyle, icon), check.Name, check.Message)
				case "warn":
					line = fmt.Sprintf("  %s %s: %s", f.render(f.warningStyle, icon), check.Name, check.Message)
				case "fail":
					line = fmt.Sprintf("  %s %s: %s", f.render(f.dangerStyle, icon), check.Name, check.Message)
				}
			}
			fmt.Fprintln(w, line)
			if check.Action != "" {
				fmt.Fprintf(w, "    %s\n", f.note("→ "+check.Action))
			}
		}
		fmt.Fprintln(w)
	}
}
