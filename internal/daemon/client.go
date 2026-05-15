package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	socketPath string
	http       *http.Client
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultConfig().SocketPath
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		socketPath: socketPath,
		http:       &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}
}

func (c *Client) Status(ctx context.Context) (DaemonStatus, error) {
	var out DaemonStatus
	err := c.do(ctx, http.MethodGet, "/v1/daemon/status", nil, &out)
	return out, err
}

func (c *Client) Stop(ctx context.Context) (StopResponse, error) {
	var out StopResponse
	err := c.do(ctx, http.MethodPost, "/v1/daemon/stop", nil, &out)
	return out, err
}

func (c *Client) RunWorkflow(ctx context.Context, req RunWorkflowRequest) (RunWorkflowResponse, error) {
	var out RunWorkflowResponse
	err := c.do(ctx, http.MethodPost, "/v1/workflows", req, &out)
	return out, err
}

func (c *Client) ListWorkflows(ctx context.Context) (ListWorkflowsResponse, error) {
	var out ListWorkflowsResponse
	err := c.do(ctx, http.MethodGet, "/v1/workflows", nil, &out)
	return out, err
}

func (c *Client) WorkflowStatus(ctx context.Context, runID string) (RunWorkflowResponse, error) {
	var out RunWorkflowResponse
	err := c.do(ctx, http.MethodGet, "/v1/workflows/"+runID, nil, &out)
	return out, err
}

func (c *Client) WorkflowLogs(ctx context.Context, runID string) (LogsResponse, error) {
	var out LogsResponse
	err := c.do(ctx, http.MethodGet, "/v1/workflows/"+runID+"/logs", nil, &out)
	return out, err
}

func (c *Client) CancelWorkflow(ctx context.Context, runID string) (CancelWorkflowResponse, error) {
	var out CancelWorkflowResponse
	err := c.do(ctx, http.MethodPost, "/v1/workflows/"+runID+"/cancel", nil, &out)
	return out, err
}

func (c *Client) do(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return daemonUnavailableError{socketPath: c.socketPath, err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var payload map[string]string
		if err := json.Unmarshal(data, &payload); err == nil && payload["error"] != "" {
			return errors.New(payload["error"])
		}
		return fmt.Errorf("agentflowd returned %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

type daemonUnavailableError struct {
	socketPath string
	err        error
}

func (e daemonUnavailableError) Error() string {
	return fmt.Sprintf("agentflowd is not running; start it with agentflow daemon start (socket: %s)", e.socketPath)
}

func (e daemonUnavailableError) Unwrap() error {
	return e.err
}

func IsDaemonUnavailable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "agentflowd is not running")
}
