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
	maxRPCRecordBytes          = 1 << 20  // 1 MiB
	maxTextBytes               = 10 << 20 // 10 MiB
	rawEventsEnvVar            = "AGENTFLOW_PI_CAPTURE_RAW_EVENTS"
)

type Provider struct {
	piPath           string
	captureRawEvents bool
}

func New(piPath string) *Provider {
	return &Provider{
		piPath:           piPath,
		captureRawEvents: envBool(rawEventsEnvVar),
	}
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
		stdin:            stdin,
		reader:           bufio.NewReader(stdout),
		captureRawEvents: p.captureRawEvents,
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
	var statsResp *rpcMessage
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
				"assistant_text": msgToMap(assistantTextResp),
				"session_stats":  msgToMap(statsResp),
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
	captureRawEvents  bool
	lastAssistantText string
	lastAgentError    string
}

type rpcMessage struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Command  string          `json:"command"`
	Success  bool            `json:"success"`
	Error    string          `json:"error"`
	Data     json.RawMessage `json:"data"`
	Message  json.RawMessage `json:"message"`
	Messages json.RawMessage `json:"messages"`
}

type rpcData struct {
	Text    string          `json:"text"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Tokens  json.RawMessage `json:"tokens"`
}

type rpcPayloadEnvelope struct {
	Text     string          `json:"text"`
	Error    string          `json:"error"`
	Message  json.RawMessage `json:"message"`
	Messages json.RawMessage `json:"messages"`
}

type rpcMessageObj struct {
	Role          string          `json:"role"`
	Content       json.RawMessage `json:"content"`
	StopReason    string          `json:"stopReason"`
	StopReasonAlt string          `json:"stop_reason"`
	ErrorMessage  string          `json:"errorMessage"`
	ErrorAlt      string          `json:"error"`
}

type rpcContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type rpcTokens struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

func (s *rpcSession) runTurn(ctx context.Context, promptID string, prompt string) (string, *rpcMessage, error) {
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
	if len(textResp.Data) > 0 {
		var data rpcData
		if err := json.Unmarshal(textResp.Data, &data); err == nil {
			text = data.Text
		}
	}
	if len(text) > maxTextBytes {
		return "", nil, fmt.Errorf("assistant text exceeds %d bytes", maxTextBytes)
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
		msg, raw, err := s.readMessage(ctx)
		if err != nil {
			return err
		}
		if msg.Type != "response" {
			if err := s.processEvent(msg, raw); err != nil {
				return err
			}
			continue
		}
		if msg.ID != id || msg.Command != "prompt" {
			continue
		}
		if msg.Success {
			return nil
		}
		return responseError(msg)
	}
}

func (s *rpcSession) waitForAgentEnd(ctx context.Context) error {
	for {
		msg, raw, err := s.readMessage(ctx)
		if err != nil {
			return err
		}
		if msg.Type == "response" {
			continue
		}
		if err := s.processEvent(msg, raw); err != nil {
			return err
		}
		if msg.Type == "agent_end" {
			return nil
		}
	}
}

func (s *rpcSession) requestResponse(ctx context.Context, id string, command string) (*rpcMessage, error) {
	if err := s.writeCommand(map[string]any{"id": id, "type": command}); err != nil {
		return nil, err
	}
	for {
		msg, raw, err := s.readMessage(ctx)
		if err != nil {
			return nil, err
		}
		if msg.Type != "response" {
			if err := s.processEvent(msg, raw); err != nil {
				return nil, err
			}
			continue
		}
		if msg.ID != id || msg.Command != command {
			continue
		}
		if msg.Success {
			return msg, nil
		}
		return nil, responseError(msg)
	}
}

func (s *rpcSession) readMessage(ctx context.Context) (*rpcMessage, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	line, err := readJSONLRecord(s.reader, maxRPCRecordBytes)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, nil, fmt.Errorf("parse pi rpc line: %w: line=%q", err, truncateOutput(string(line)))
	}
	return &msg, line, nil
}

func readJSONLRecord(reader *bufio.Reader, limit int) ([]byte, error) {
	var out []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(out)+len(part) > limit {
			return nil, fmt.Errorf("rpc record exceeds %d bytes", limit)
		}
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

func (s *rpcSession) processEvent(msg *rpcMessage, raw []byte) error {
	if s.captureRawEvents {
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err == nil {
			s.rawEvents = append(s.rawEvents, ports.AgentEvent{
				Type: msg.Type,
				Data: data,
			})
		}
	}
	if text := extractAssistantText(msg); strings.TrimSpace(text) != "" {
		if len(text) > maxTextBytes {
			return fmt.Errorf("assistant text exceeds %d bytes", maxTextBytes)
		}
		s.lastAssistantText = text
		s.lastAgentError = ""
	}
	if errMsg := extractAgentError(msg); errMsg != "" {
		s.lastAgentError = errMsg
	}
	return nil
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

func extractAssistantText(msg *rpcMessage) string {
	if text := extractAssistantTextFromRaw(msg.Data); text != "" {
		return text
	}
	if len(msg.Message) > 0 {
		if text := extractAssistantTextFromRaw(msg.Message); text != "" {
			return text
		}
	}
	if len(msg.Messages) > 0 {
		return extractAssistantTextFromMessages(msg.Messages)
	}
	return ""
}

func extractAgentError(msg *rpcMessage) string {
	if errMsg := extractAgentErrorFromRaw(msg.Data); errMsg != "" {
		return errMsg
	}
	if len(msg.Message) > 0 {
		if errMsg := extractAgentErrorFromRaw(msg.Message); errMsg != "" {
			return errMsg
		}
	}
	if len(msg.Messages) > 0 {
		return extractAgentErrorFromMessages(msg.Messages)
	}
	return ""
}

func assistantError(m *rpcMessageObj) string {
	errMsg := m.ErrorMessage
	if errMsg == "" {
		errMsg = m.ErrorAlt
	}
	if errMsg == "" {
		return ""
	}
	stopReason := m.StopReason
	if stopReason == "" {
		stopReason = m.StopReasonAlt
	}
	if stopReason != "" && stopReason != "error" {
		return ""
	}
	return errMsg
}

func messageText(m *rpcMessageObj) string {
	if len(m.Content) == 0 {
		return ""
	}
	return contentText(m.Content)
}

func extractAssistantTextFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var envelope rpcPayloadEnvelope
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if envelope.Text != "" {
			return envelope.Text
		}
		if text := extractAssistantTextFromRaw(envelope.Message); text != "" {
			return text
		}
		if text := extractAssistantTextFromMessages(envelope.Messages); text != "" {
			return text
		}
	}
	var msg rpcMessageObj
	if err := json.Unmarshal(raw, &msg); err == nil {
		if msg.Role == "assistant" {
			return messageText(&msg)
		}
		return ""
	}
	return extractAssistantTextFromMessages(raw)
}

func extractAssistantTextFromMessages(raw json.RawMessage) string {
	msgs, ok := decodeRawMessages(raw)
	if !ok {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if text := extractAssistantTextFromRaw(msgs[i]); text != "" {
			return text
		}
	}
	return ""
}

func extractAgentErrorFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var errText string
	if err := json.Unmarshal(raw, &errText); err == nil {
		return errText
	}
	var envelope rpcPayloadEnvelope
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if envelope.Error != "" {
			return envelope.Error
		}
		if errMsg := extractAgentErrorFromRaw(envelope.Message); errMsg != "" {
			return errMsg
		}
		if errMsg := extractAgentErrorFromMessages(envelope.Messages); errMsg != "" {
			return errMsg
		}
	}
	var msg rpcMessageObj
	if err := json.Unmarshal(raw, &msg); err == nil {
		if msg.Role == "assistant" {
			return assistantError(&msg)
		}
		return ""
	}
	return extractAgentErrorFromMessages(raw)
}

func extractAgentErrorFromMessages(raw json.RawMessage) string {
	msgs, ok := decodeRawMessages(raw)
	if !ok {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if errMsg := extractAgentErrorFromRaw(msgs[i]); errMsg != "" {
			return errMsg
		}
	}
	return ""
}

func decodeRawMessages(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, false
	}
	return msgs, true
}

func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	msgs, ok := decodeRawMessages(raw)
	if ok {
		var parts []string
		for _, item := range msgs {
			parts = append(parts, contentText(item))
		}
		return strings.Join(parts, "")
	}
	var item rpcContentItem
	if err := json.Unmarshal(raw, &item); err == nil {
		if item.Type == "" || item.Type == "text" {
			return item.Text
		}
	}
	var textOnly struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &textOnly); err == nil {
		return textOnly.Text
	}
	return ""
}

func responseError(msg *rpcMessage) error {
	cmd := msg.Command
	errMsg := msg.Error
	if errMsg == "" && len(msg.Message) > 0 {
		var s string
		if err := json.Unmarshal(msg.Message, &s); err == nil {
			errMsg = s
		}
	}
	if errMsg != "" {
		return fmt.Errorf("pi rpc %s failed: %s", cmd, errMsg)
	}
	if len(msg.Data) > 0 {
		var data rpcData
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			if data.Error != "" {
				return fmt.Errorf("pi rpc %s failed: %s", cmd, data.Error)
			}
			if data.Message != "" {
				return fmt.Errorf("pi rpc %s failed: %s", cmd, data.Message)
			}
		}
	}
	return fmt.Errorf("pi rpc %s failed", cmd)
}

func extractUsage(msg *rpcMessage) *ports.Usage {
	if len(msg.Data) == 0 {
		return nil
	}
	var data rpcData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil
	}
	if len(data.Tokens) == 0 {
		return nil
	}
	var tokens rpcTokens
	if err := json.Unmarshal(data.Tokens, &tokens); err != nil {
		return nil
	}
	usage := &ports.Usage{
		InputTokens:  tokens.Input,
		OutputTokens: tokens.Output,
		TotalTokens:  tokens.Total,
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

func msgToMap(msg *rpcMessage) map[string]any {
	if msg == nil {
		return nil
	}
	m := map[string]any{
		"type": msg.Type,
	}
	if msg.ID != "" {
		m["id"] = msg.ID
	}
	if msg.Command != "" {
		m["command"] = msg.Command
	}
	if msg.Success {
		m["success"] = true
	}
	if msg.Error != "" {
		m["error"] = msg.Error
	}
	if len(msg.Data) > 0 {
		var data any
		_ = json.Unmarshal(msg.Data, &data)
		m["data"] = data
	}
	if len(msg.Message) > 0 {
		var msgAny any
		_ = json.Unmarshal(msg.Message, &msgAny)
		m["message"] = msgAny
	}
	if len(msg.Messages) > 0 {
		var msgsAny any
		_ = json.Unmarshal(msg.Messages, &msgsAny)
		m["messages"] = msgsAny
	}
	return m
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

func envBool(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
