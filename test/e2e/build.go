package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	buildOnce         sync.Once
	buildErr          error
	pkgBuildDir       string
	pkgAgentflowPath  string
	pkgAgentflowdPath string
)

func ensureBuilt() error {
	buildOnce.Do(func() {
		pkgBuildDir, buildErr = os.MkdirTemp("", "agentflow-e2e-build-*")
		if buildErr != nil {
			return
		}
		pkgAgentflowPath = filepath.Join(pkgBuildDir, exe("agentflow"))
		pkgAgentflowdPath = filepath.Join(pkgBuildDir, exe("agentflowd"))

		buildErr = buildBinary(pkgAgentflowPath, "./cmd/agentflow")
		if buildErr == nil {
			buildErr = buildBinary(pkgAgentflowdPath, "./cmd/agentflowd")
		}
	})
	return buildErr
}

func buildBinary(out, pkg string) error {
	modRoot, err := moduleRoot()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, pkg)
	cmd.Dir = modRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build %s: %w\n%s", pkg, err, stderr.String())
	}
	return nil
}

func moduleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get caller")
	}
	// file is .../test/e2e/build.go
	return filepath.Join(filepath.Dir(file), "..", ".."), nil
}
