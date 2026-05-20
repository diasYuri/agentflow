package shell

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Run(ctx context.Context, req ports.ShellRequest) (ports.ShellResult, error) {
	start := time.Now()
	cmd := command(ctx, req)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}
	cmd.Env = os.Environ()
	for key, value := range req.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr limitedBuffer
	stdout.limit = req.MaxOutputBytes
	stderr.limit = req.MaxOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return ports.ShellResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode, Duration: time.Since(start)}, err
}

func command(ctx context.Context, req ports.ShellRequest) *exec.Cmd {
	if len(req.Args) > 0 {
		return exec.CommandContext(ctx, req.Args[0], req.Args[1:]...)
	}
	shell := req.Shell
	if shell == "" {
		shell = "bash"
	}
	return exec.CommandContext(ctx, shell, "-lc", req.Command)
}

type limitedBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return b.Buffer.Write(p)
	}
	remaining := b.limit - int64(b.Buffer.Len())
	if remaining <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
