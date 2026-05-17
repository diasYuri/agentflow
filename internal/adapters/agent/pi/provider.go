package pi

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

	"github.com/diasYuri/agentflow/internal/core/ports"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

const (
	maxErrorOutputBytes        = 4096
	maxStructuredOutputRetries = 5
	readOnlyTools              = "read,grep,find,ls"
)

type Provider struct {
	piPath string
}

func New(piPath string) *Provider {
	return &Provider{piPath: piPath}
}

func (p *Provider) Run(ctx context.Context, req ports.AgentRequest) (ports.AgentResult, error) {
	args := buildArgs(req)
	cmd := exec.CommandContext(ctx, resolvePiPath(p.piPath), args...)
	cmd.Env = envMapToList(mergePiEnv(req.Env))
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ports.AgentResult{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ports.AgentResult{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return ports.AgentResult{}, fmt.Errorf("start pi: %w", err)
	}
	waited := false
	defer func() {
		if !waited && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = waitForProcess(ctx, cmd)
		}
	}()

	session := &rpcSession{
		stdin:     stdin,
		reader:    bufio.NewReader(stdout),
		rawEvents: []ports.AgentEvent{},
	}

	prompt := req.Prompt
	if req.OutputSchema != nil {
		prompt = appendJSONOnlyInstruction(prompt)
	}
	text, assistantTextResp, err := session.runTurn(ctx, "agentflow-prompt", prompt)
	if err != nil {
		return ports.AgentResult{}, finishWithError(ctx, cmd, &waited, stderr.String(), "run pi prompt", err)
	}
	if strings.TrimSpace(text) == "" && session.lastAgentError != "" {
		return ports.AgentResult{}, finishWithError(ctx, cmd, &waited, stderr.String(), "run pi agent", errors.New(session.lastAgentError))
	}

	var parsed any
	var statsResp map[string]any
	if req.OutputSchema != nil {
		var firstValidationErr error
		var lastValidationErr error
		parsed, err = validateStructuredOutput(text, req.OutputSchema)
		if err != nil {
			firstValidationErr = err
			lastValidationErr = err
			for retry := 1; retry <= maxStructuredOutputRetries; retry++ {
				retryPrompt := buildStructuredOutputRetryPrompt(req.OutputSchema, lastValidationErr, retry, maxStructuredOutputRetries)
				text, assistantTextResp, err = session.runTurn(ctx, "agentflow-prompt", retryPrompt)
				if err != nil {
					return ports.AgentResult{}, finishWithError(ctx, cmd, &waited, stderr.String(), "run pi structured output retry", err)
				}
				if strings.TrimSpace(text) == "" && session.lastAgentError != "" {
					return ports.AgentResult{}, finishWithError(ctx, cmd, &waited, stderr.String(), "run pi agent", errors.New(session.lastAgentError))
				}
				parsed, err = validateStructuredOutput(text, req.OutputSchema)
				if err == nil {
					break
				}
				lastValidationErr = err
			}
			if err != nil {
				return ports.AgentResult{}, finishWithError(ctx, cmd, &waited, stderr.String(), "validate pi structured output", fmt.Errorf("initial response invalid: %v; retry response invalid: %v; final text=%q", firstValidationErr, lastValidationErr, truncateOutput(text)))
			}
		}
	}
	statsResp, err = session.requestResponse(ctx, "agentflow-stats", "get_session_stats")
	if err != nil {
		return ports.AgentResult{}, finishWithError(ctx, cmd, &waited, stderr.String(), "get pi session stats", err)
	}

	result := ports.AgentResult{
		Text:      text,
		RawEvents: session.rawEvents,
		Metadata: map[string]any{
			"pi": map[string]any{
				"assistant_text": assistantTextResp,
				"session_stats":  statsResp,
			},
		},
	}
	if req.OutputSchema != nil {
		result.JSON = parsed
	}
	if usage := extractUsage(statsResp); usage != nil {
		result.Usage = usage
	}

	_ = stdin.Close()
	waitErr, ctxErr := waitForProcess(ctx, cmd)
	waited = true
	if ctxErr != nil {
		return ports.AgentResult{}, ctxErr
	}
	if waitErr != nil {
		return ports.AgentResult{}, fmt.Errorf("run pi: %w: stderr=%q", waitErr, truncateOutput(stderr.String()))
	}
	return result, nil
}

type rpcSession struct {
	stdin             io.WriteCloser
	reader            *bufio.Reader
	rawEvents         []ports.AgentEvent
	lastAssistantText string
	lastAgentError    string
}

func (s *rpcSession) runTurn(ctx context.Context, promptID string, prompt string) (string, map[string]any, error) {
	s.lastAssistantText = ""
	s.lastAgentError = ""
	if err := s.writeCommand(map[string]any{
		"id":      promptID,
		"type":    "prompt",
		"message": prompt,
	}); err != nil {
		return "", nil, err
	}
	if err := s.waitForPromptAccepted(ctx, promptID); err != nil {
		return "", nil, err
	}
	if err := s.waitForAgentEnd(ctx); err != nil {
		return "", nil, err
	}
	textResp, err := s.requestResponse(ctx, "agentflow-text", "get_last_assistant_text")
	if err != nil {
		return "", nil, err
	}
	text := ""
	if data, ok := textResp["data"].(map[string]any); ok {
		if value, ok := data["text"].(string); ok {
			text = value
		}
	}
	if strings.TrimSpace(text) == "" {
		text = s.lastAssistantText
	}
	return text, textResp, nil
}

func buildArgs(req ports.AgentRequest) []string {
	args := []string{"--mode", "rpc", "--no-session"}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if strings.TrimSpace(req.System) != "" {
		args = append(args, "--append-system-prompt", req.System)
	}
	if strings.TrimSpace(req.Sandbox.Mode) == "read-only" {
		args = append(args, "--tools", readOnlyTools)
	}
	return args
}

func resolvePiPath(override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	if envPath := strings.TrimSpace(os.Getenv("AGENTFLOW_PI_PATH")); envPath != "" {
		return envPath
	}
	if envPath := strings.TrimSpace(os.Getenv("PI_PATH")); envPath != "" {
		return envPath
	}
	if path, err := exec.LookPath("pi"); err == nil {
		return path
	}
	return "pi"
}

func mergePiEnv(overrides map[string]string) map[string]string {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	for key, value := range overrides {
		env[key] = value
	}
	return env
}

func envMapToList(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	sort.Strings(items)
	return items
}

func (s *rpcSession) writeCommand(command map[string]any) error {
	data, err := json.Marshal(command)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = s.stdin.Write(data)
	return err
}

func (s *rpcSession) waitForPromptAccepted(ctx context.Context, id string) error {
	for {
		msg, err := s.readMessage(ctx)
		if err != nil {
			return err
		}
		if stringField(msg, "type") != "response" {
			s.captureEvent(msg)
			continue
		}
		if stringField(msg, "id") != id || stringField(msg, "command") != "prompt" {
			continue
		}
		if success, ok := msg["success"].(bool); ok && success {
			return nil
		}
		return responseError(msg)
	}
}

func (s *rpcSession) waitForAgentEnd(ctx context.Context) error {
	for {
		msg, err := s.readMessage(ctx)
		if err != nil {
			return err
		}
		if stringField(msg, "type") == "response" {
			continue
		}
		s.captureEvent(msg)
		if stringField(msg, "type") == "agent_end" {
			return nil
		}
	}
}

func (s *rpcSession) requestResponse(ctx context.Context, id string, command string) (map[string]any, error) {
	if err := s.writeCommand(map[string]any{"id": id, "type": command}); err != nil {
		return nil, err
	}
	for {
		msg, err := s.readMessage(ctx)
		if err != nil {
			return nil, err
		}
		if stringField(msg, "type") != "response" {
			s.captureEvent(msg)
			continue
		}
		if stringField(msg, "id") != id || stringField(msg, "command") != command {
			continue
		}
		if success, ok := msg["success"].(bool); ok && success {
			return msg, nil
		}
		return nil, responseError(msg)
	}
}

func (s *rpcSession) readMessage(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	line, err := readJSONLRecord(s.reader)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	var msg map[string]any
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("parse pi rpc line: %w: line=%q", err, truncateOutput(string(line)))
	}
	return msg, nil
}

func readJSONLRecord(reader *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		part, err := reader.ReadSlice('\n')
		out = append(out, part...)
		if err == nil {
			out = bytes.TrimSuffix(out, []byte{'\n'})
			out = bytes.TrimSuffix(out, []byte{'\r'})
			return out, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(out) > 0 {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
}

func (s *rpcSession) captureEvent(msg map[string]any) {
	s.rawEvents = append(s.rawEvents, ports.AgentEvent{
		Type: stringField(msg, "type"),
		Data: msg,
	})
	if text := extractAssistantText(msg); strings.TrimSpace(text) != "" {
		s.lastAssistantText = text
		s.lastAgentError = ""
	}
	if errMsg := extractAgentError(msg); errMsg != "" {
		s.lastAgentError = errMsg
	}
}

func validateStructuredOutput(text string, schema map[string]any) (any, error) {
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("%w: final text=%q", err, truncateOutput(text))
	}
	if err := coreworkflow.ValidateSchema(parsed, schema, "output_schema"); err != nil {
		return nil, err
	}
	return parsed, nil
}

func extractAssistantText(event map[string]any) string {
	if message, ok := event["message"].(map[string]any); ok && messageRole(message) == "assistant" {
		return messageText(message)
	}
	if messages, ok := event["messages"].([]any); ok {
		for i := len(messages) - 1; i >= 0; i-- {
			message, ok := messages[i].(map[string]any)
			if ok && messageRole(message) == "assistant" {
				return messageText(message)
			}
		}
	}
	return ""
}

func extractAgentError(event map[string]any) string {
	if message, ok := event["message"].(map[string]any); ok && messageRole(message) == "assistant" {
		if errMsg := assistantError(message); errMsg != "" {
			return errMsg
		}
	}
	if messages, ok := event["messages"].([]any); ok {
		for i := len(messages) - 1; i >= 0; i-- {
			message, ok := messages[i].(map[string]any)
			if ok && messageRole(message) == "assistant" {
				return assistantError(message)
			}
		}
	}
	return ""
}

func messageRole(message map[string]any) string {
	return stringField(message, "role")
}

func assistantError(message map[string]any) string {
	errMsg := stringField(message, "errorMessage", "error")
	if errMsg == "" {
		return ""
	}
	if stopReason := stringField(message, "stopReason", "stop_reason"); stopReason != "" && stopReason != "error" {
		return ""
	}
	return errMsg
}

func messageText(message map[string]any) string {
	switch content := message["content"].(type) {
	case string:
		return content
	case []any:
		var parts []string
		for _, item := range content {
			switch value := item.(type) {
			case string:
				parts = append(parts, value)
			case map[string]any:
				if text, ok := value["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func responseError(msg map[string]any) error {
	if errMsg := stringField(msg, "error", "message"); errMsg != "" {
		return fmt.Errorf("pi rpc %s failed: %s", stringField(msg, "command"), errMsg)
	}
	if data, ok := msg["data"].(map[string]any); ok {
		if errMsg := stringField(data, "error", "message"); errMsg != "" {
			return fmt.Errorf("pi rpc %s failed: %s", stringField(msg, "command"), errMsg)
		}
	}
	return fmt.Errorf("pi rpc %s failed", stringField(msg, "command"))
}

func extractUsage(payload map[string]any) *ports.Usage {
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil
	}
	tokens, ok := data["tokens"].(map[string]any)
	if !ok {
		return nil
	}
	usage := &ports.Usage{
		InputTokens:  intField(tokens, "input"),
		OutputTokens: intField(tokens, "output"),
		TotalTokens:  intField(tokens, "total"),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

func intField(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case float64:
			return int(value)
		case int:
			return value
		case json.Number:
			number, _ := value.Int64()
			return int(number)
		}
	}
	return 0
}

func stringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok {
			return value
		}
	}
	return ""
}

func appendJSONOnlyInstruction(prompt string) string {
	return prompt + "\n\nReturn only the final assistant message as JSON matching the requested output schema. Do not include Markdown fences, commentary, or surrounding text."
}

func buildStructuredOutputRetryPrompt(schema map[string]any, validationErr error, retry int, maxRetries int) string {
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		schemaJSON = []byte(`{}`)
	}
	return fmt.Sprintf(
		"The previous response did not match the requested JSON schema.\n\nRequested JSON schema:\n%s\n\nValidation error: %s\n\nRetry attempt %d of %d.\nReturn a corrected final assistant message as JSON only. Do not include Markdown fences, commentary, or surrounding text.",
		string(schemaJSON),
		validationErr.Error(),
		retry,
		maxRetries,
	)
}

func finishWithError(ctx context.Context, cmd *exec.Cmd, waited *bool, stderr string, label string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_, _ = waitForProcess(ctx, cmd)
		*waited = true
		return ctxErr
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_, _ = waitForProcess(ctx, cmd)
	*waited = true
	if stderr != "" {
		return fmt.Errorf("%s: %w: stderr=%q", label, err, truncateOutput(stderr))
	}
	return fmt.Errorf("%s: %w", label, err)
}

func waitForProcess(ctx context.Context, cmd *exec.Cmd) (error, error) {
	err := cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return err, ctxErr
	}
	return err, nil
}

func truncateOutput(value string) string {
	if len(value) <= maxErrorOutputBytes {
		return value
	}
	return value[:maxErrorOutputBytes] + "...(truncated)"
}
