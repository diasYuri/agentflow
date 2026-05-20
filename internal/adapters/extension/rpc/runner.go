package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

const (
	defaultAdapterCommand = "agentflow-extension-rpc"
	adapterEnvVar         = "AGENTFLOW_EXTENSION_RPC"
	methodExtensionRun    = "extension.run"
	methodShutdown        = "shutdown"
)

type Runner struct {
	command string
	mu      sync.Mutex
	servers map[string]*serverSession
}

func New(command string) *Runner {
	return &Runner{command: command, servers: map[string]*serverSession{}}
}

func (r *Runner) Run(ctx context.Context, req ports.ExtensionRequest) (ports.ExtensionResult, error) {
	if req.Mode == "server" {
		return r.runServer(ctx, req)
	}
	return r.runOneShot(ctx, req)
}

func (r *Runner) CloseRun(ctx context.Context, runID string) error {
	r.mu.Lock()
	var sessions []*serverSession
	for key, session := range r.servers {
		if session.runID == runID {
			sessions = append(sessions, session)
			delete(r.servers, key)
		}
	}
	r.mu.Unlock()
	var firstErr error
	for _, session := range sessions {
		if err := session.close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Runner) runOneShot(ctx context.Context, req ports.ExtensionRequest) (ports.ExtensionResult, error) {
	start := time.Now()
	rpcReq := rpcRequest{JSONRPC: "2.0", ID: requestID(req), Method: methodExtensionRun, Params: rpcParams(req)}
	stdin, err := json.Marshal(rpcReq)
	if err != nil {
		return ports.ExtensionResult{}, fmt.Errorf("marshal extension rpc request: %w", err)
	}
	var stdout, stderr limitedBuffer
	stdout.limit = req.MaxOutputBytes
	stderr.limit = req.MaxOutputBytes
	cmd := exec.CommandContext(ctx, r.adapterCommand(), "run", "--extension-dir", req.ExtensionDir, "--script", req.Script)
	cmd.Dir = req.WorkingDir
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = extensionEnv(req.Env)
	err = cmd.Run()
	exitCode := exitCode(err)
	result := ports.ExtensionResult{Stdout: "", Stderr: stderr.String(), ExitCode: exitCode, Duration: time.Since(start)}
	if err != nil {
		if exitCode == -1 {
			return result, fmt.Errorf("failed to start extension rpc adapter: %w", err)
		}
		return result, fmt.Errorf("extension rpc adapter exited with code %d: %s", exitCode, stderr.String())
	}
	output, parsedStderr, err := parseRPCOutput(stdout.Bytes())
	if err != nil {
		return result, err
	}
	result.Output = output
	result.Stderr = parsedStderr
	return result, nil
}

func (r *Runner) runServer(ctx context.Context, req ports.ExtensionRequest) (ports.ExtensionResult, error) {
	session, err := r.server(ctx, req)
	if err != nil {
		return ports.ExtensionResult{}, err
	}
	result, runErr := session.run(ctx, req)
	if runErr != nil {
		r.dropServer(session)
		_ = session.close(context.Background())
	}
	return result, runErr
}

func (r *Runner) server(ctx context.Context, req ports.ExtensionRequest) (*serverSession, error) {
	key := sessionKey(req)
	r.mu.Lock()
	session := r.servers[key]
	r.mu.Unlock()
	if session != nil {
		return session, nil
	}
	cmd := exec.CommandContext(context.Background(), r.adapterCommand(), "serve", "--extension-dir", req.ExtensionDir)
	cmd.Dir = req.WorkingDir
	cmd.Env = extensionEnv(req.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open extension rpc stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open extension rpc stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open extension rpc stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start extension rpc adapter: %w", err)
	}
	session = &serverSession{
		key:    key,
		runID:  req.RunID,
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		stderr: &limitedBuffer{limit: req.MaxOutputBytes},
	}
	go func() {
		_, _ = io.Copy(session.stderr, stderr)
	}()
	r.mu.Lock()
	if existing := r.servers[key]; existing != nil {
		r.mu.Unlock()
		_ = session.close(context.Background())
		return existing, nil
	}
	r.servers[key] = session
	r.mu.Unlock()
	return session, nil
}

func (r *Runner) dropServer(session *serverSession) {
	r.mu.Lock()
	if current := r.servers[session.key]; current == session {
		delete(r.servers, session.key)
	}
	r.mu.Unlock()
}

func (r *Runner) adapterCommand() string {
	if r.command != "" {
		return r.command
	}
	if fromEnv := os.Getenv(adapterEnvVar); fromEnv != "" {
		return fromEnv
	}
	return defaultAdapterCommand
}

type serverSession struct {
	key    string
	runID  string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	stderr *limitedBuffer
	mu     sync.Mutex
}

func (s *serverSession) run(ctx context.Context, req ports.ExtensionRequest) (ports.ExtensionResult, error) {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	rpcReq := rpcRequest{JSONRPC: "2.0", ID: requestID(req), Method: methodExtensionRun, Params: rpcParams(req)}
	if err := s.write(rpcReq); err != nil {
		return ports.ExtensionResult{Stderr: s.stderr.String(), ExitCode: -1, Duration: time.Since(start)}, err
	}
	resp, err := s.read(ctx)
	result := ports.ExtensionResult{ExitCode: 0, Duration: time.Since(start)}
	if err != nil {
		result.ExitCode = -1
		result.Stderr = s.stderr.String()
		return result, err
	}
	output, stderr, err := responseOutput(resp)
	if err != nil {
		result.ExitCode = 1
		result.Stderr = stderr
		return result, err
	}
	result.Output = output
	result.Stderr = stderr
	return result, nil
}

func (s *serverSession) close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.write(rpcRequest{JSONRPC: "2.0", ID: "shutdown", Method: methodShutdown})
	done := make(chan error, 1)
	go func() {
		_ = s.stdin.Close()
		done <- s.cmd.Wait()
	}()
	select {
	case <-ctx.Done():
		_ = s.cmd.Process.Kill()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (s *serverSession) write(req rpcRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal extension rpc request: %w", err)
	}
	data = append(data, '\n')
	if _, err := s.stdin.Write(data); err != nil {
		return fmt.Errorf("write extension rpc request: %w", err)
	}
	return nil
}

func (s *serverSession) read(ctx context.Context) (rpcResponse, error) {
	type readResult struct {
		resp rpcResponse
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			ch <- readResult{err: fmt.Errorf("read extension rpc response: %w", err)}
			return
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			ch <- readResult{err: fmt.Errorf("parse extension rpc response: %w: line=%q", err, string(line))}
			return
		}
		ch <- readResult{resp: resp}
	}()
	select {
	case <-ctx.Done():
		_ = s.cmd.Process.Kill()
		_ = s.stdin.Close()
		return rpcResponse{}, ctx.Err()
	case result := <-ch:
		return result.resp, result.err
	}
}

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type rpcResult struct {
	OK     bool      `json:"ok"`
	Output any       `json:"output"`
	Stderr string    `json:"stderr,omitempty"`
	Error  *rpcError `json:"error,omitempty"`
}

func requestID(req ports.ExtensionRequest) string {
	return fmt.Sprintf("%s:%s:%s:%d", req.RunID, req.NodeID, req.InstanceID, req.Attempt)
}

func sessionKey(req ports.ExtensionRequest) string {
	return req.RunID + "\x00" + req.Extension + "\x00" + req.WorkingDir + "\x00" + envSignature(req.Env)
}

func rpcParams(req ports.ExtensionRequest) map[string]any {
	params := make(map[string]any, len(req.Payload)+3)
	for key, value := range req.Payload {
		params[key] = value
	}
	params["script"] = req.Script
	if req.Operation != "" {
		params["operation"] = req.Operation
	}
	return params
}

func parseRPCOutput(stdout []byte) (any, string, error) {
	lines := bytes.Split(bytes.TrimSpace(stdout), []byte("\n"))
	if len(lines) == 0 || len(lines[0]) == 0 {
		return nil, "", fmt.Errorf("extension rpc adapter produced no response")
	}
	var resp rpcResponse
	if err := json.Unmarshal(lines[len(lines)-1], &resp); err != nil {
		return nil, "", fmt.Errorf("parse extension rpc response: %w: stdout=%q", err, string(stdout))
	}
	return responseOutput(resp)
}

func responseOutput(resp rpcResponse) (any, string, error) {
	var result rpcResult
	if len(resp.Result) == 0 {
		if resp.Error != nil {
			return nil, "", rpcErrorToErr(resp.Error)
		}
		return nil, "", nil
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, "", fmt.Errorf("parse extension rpc result: %w", err)
	}
	if !result.OK {
		if result.Error != nil {
			return nil, result.Stderr, rpcErrorToErr(result.Error)
		}
		return nil, result.Stderr, fmt.Errorf("extension rpc failed")
	}
	return result.Output, result.Stderr, nil
}

func extensionEnv(env map[string]string) []string {
	out := os.Environ()
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func envSignature(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte('\x1f')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(env[key])
	}
	return b.String()
}

func rpcErrorToErr(errObj *rpcError) error {
	if errObj == nil {
		return nil
	}
	if errObj.Code != "" {
		return fmt.Errorf("extension rpc %s: %s", errObj.Code, errObj.Message)
	}
	return fmt.Errorf("extension rpc failed: %s", errObj.Message)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
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
