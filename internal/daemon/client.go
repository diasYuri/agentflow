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
	"net/url"
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

func (c *Client) WorkflowEvents(ctx context.Context, runID string, cursor int, limit int) (WorkflowEventsResponse, error) {
	var out WorkflowEventsResponse
	query := fmt.Sprintf("?cursor=%d&limit=%d", cursor, limit)
	err := c.do(ctx, http.MethodGet, "/v1/workflows/"+runID+"/events"+query, nil, &out)
	return out, err
}

func (c *Client) CancelWorkflow(ctx context.Context, runID string) (CancelWorkflowResponse, error) {
	var out CancelWorkflowResponse
	err := c.do(ctx, http.MethodPost, "/v1/workflows/"+runID+"/cancel", nil, &out)
	return out, err
}

func (c *Client) PauseWorkflow(ctx context.Context, runID string) (PauseWorkflowResponse, error) {
	var out PauseWorkflowResponse
	err := c.do(ctx, http.MethodPost, "/v1/workflows/"+runID+"/pause", nil, &out)
	return out, err
}

func (c *Client) ResumeWorkflow(ctx context.Context, runID string) (ResumeWorkflowResponse, error) {
	var out ResumeWorkflowResponse
	err := c.do(ctx, http.MethodPost, "/v1/workflows/"+runID+"/resume", nil, &out)
	return out, err
}

func (c *Client) WorkflowArtifacts(ctx context.Context, runID string) (WorkflowArtifactsResponse, error) {
	var out WorkflowArtifactsResponse
	err := c.do(ctx, http.MethodGet, "/v1/workflows/"+runID+"/artifacts", nil, &out)
	return out, err
}

func (c *Client) WorkflowArtifact(ctx context.Context, runID string, artifactID string) (WorkflowArtifactResponse, error) {
	var out WorkflowArtifactResponse
	path := "/v1/workflows/" + runID + "/artifacts/" + url.PathEscape(artifactID)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) WorkflowNodes(ctx context.Context, runID string) (WorkflowNodesResponse, error) {
	var out WorkflowNodesResponse
	err := c.do(ctx, http.MethodGet, "/v1/workflows/"+runID+"/nodes", nil, &out)
	return out, err
}

func (c *Client) WorkflowNode(ctx context.Context, runID string, nodeID string) (WorkflowNodeResponse, error) {
	var out WorkflowNodeResponse
	path := "/v1/workflows/" + runID + "/nodes/" + url.PathEscape(nodeID)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) WorkflowPlan(ctx context.Context, runID string) (WorkflowPlanResponse, error) {
	var out WorkflowPlanResponse
	err := c.do(ctx, http.MethodGet, "/v1/workflows/"+runID+"/plan", nil, &out)
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
