package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const slackAPIBaseURL = "https://slack.com/api"

type Client struct {
	appToken string
	botToken string
	baseURL  string
	http     *http.Client
}

type AuthTestResponse struct {
	TeamID string
	UserID string
	URL    string
}

type PostMessageInput struct {
	Channel  string
	ThreadTS string
	Text     string
}

type apiError struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func newClient(appToken, botToken string) *Client {
	return &Client{
		appToken: strings.TrimSpace(appToken),
		botToken: strings.TrimSpace(botToken),
		baseURL:  slackAPIBaseURL,
		http:     &http.Client{},
	}
}

func (c *Client) AuthTest(ctx context.Context) (AuthTestResponse, error) {
	var resp struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		TeamID string `json:"team_id"`
		UserID string `json:"user_id"`
		URL    string `json:"url"`
	}
	if err := c.postJSON(ctx, c.botToken, "auth.test", nil, &resp); err != nil {
		return AuthTestResponse{}, err
	}
	if !resp.OK {
		return AuthTestResponse{}, fmt.Errorf("slack auth.test: %s", resp.Error)
	}
	return AuthTestResponse{TeamID: resp.TeamID, UserID: resp.UserID, URL: resp.URL}, nil
}

func (c *Client) OpenSocketMode(ctx context.Context) (string, error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		URL   string `json:"url"`
	}
	if err := c.postJSON(ctx, c.appToken, "apps.connections.open", nil, &resp); err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("slack apps.connections.open: %s", resp.Error)
	}
	if strings.TrimSpace(resp.URL) == "" {
		return "", errors.New("slack apps.connections.open: missing websocket url")
	}
	return resp.URL, nil
}

func (c *Client) PostMessage(ctx context.Context, input PostMessageInput) error {
	payload := map[string]any{
		"channel": input.Channel,
		"text":    input.Text,
	}
	if strings.TrimSpace(input.ThreadTS) != "" {
		payload["thread_ts"] = input.ThreadTS
	}
	var resp apiError
	if err := c.postJSON(ctx, c.botToken, "chat.postMessage", payload, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("slack chat.postMessage: %s", resp.Error)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, token, method string, body any, out any) error {
	if c == nil {
		return errors.New("slack client is nil")
	}
	if c.http == nil {
		c.http = &http.Client{}
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("slack %s: token is required", method)
	}
	baseURL := strings.TrimSpace(c.baseURL)
	if baseURL == "" {
		baseURL = slackAPIBaseURL
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), method)
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode slack %s request: %w", method, err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return fmt.Errorf("build slack %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call slack %s: %w", method, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read slack %s response: %w", method, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack %s: http %d: %s", method, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode slack %s response: %w", method, err)
	}
	return nil
}
