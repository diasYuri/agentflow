// Package agentchannel coordinates channel-neutral conversations with the
// AgentFlow assistant. Delivery adapters such as web or Slack translate their
// transport-specific requests into this package and keep channel details out of
// the core flow.
package agentchannel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/chatagent"
	"github.com/diasYuri/agentflow/internal/agentchannel/diagnostics"
	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/agentchannel/session"
)

const (
	defaultChatAgentTimeout    = 60 * time.Second
	defaultChatAgentHistoryLen = 40
)

// ChatAgent is the narrow interface the channel service uses to produce
// assistant responses. Nil means chat is not configured.
type ChatAgent interface {
	Run(ctx context.Context, req chatagent.RunRequest) (chatagent.RunResponse, error)
}

// EventSink receives channel-neutral conversation events. Web adapts this to
// SSE; other channels can map the same events to their own delivery mechanism.
type EventSink interface {
	Publish(sessionID string, kind events.Kind, payload any, correlationID string) events.Event
}

// Options bundles dependencies for Service.
type Options struct {
	Sessions              *session.Sessions
	Diagnostics           *diagnostics.Recorder
	Events                EventSink
	WorkflowDefinitions   chatagent.WorkflowDefinitionClient
	WorkflowRuns          chatagent.WorkflowRunClient
	ChatAgent             ChatAgent
	ChatAgentTimeout      time.Duration
	ChatAgentHistoryLimit int
	Logger                *slog.Logger
}

// Service owns the channel-neutral message-to-agent orchestration.
type Service struct {
	Sessions              *session.Sessions
	Diagnostics           *diagnostics.Recorder
	Events                EventSink
	WorkflowDefinitions   chatagent.WorkflowDefinitionClient
	WorkflowRuns          chatagent.WorkflowRunClient
	ChatAgent             ChatAgent
	ChatAgentTimeout      time.Duration
	ChatAgentHistoryLimit int
	logger                *slog.Logger
}

// ConversationInput identifies a channel-neutral conversation. Channel
// adapters own the meaning of the external fields and pass only opaque values
// into this layer.
type ConversationInput struct {
	ProjectName         string
	Title               string
	Provider            string
	Model               string
	Source              string
	ExternalKey         string
	ExternalWorkspaceID string
	ExternalChannelID   string
	ExternalThreadID    string
	ExternalUserID      string
	Metadata            map[string]any
}

// UserMessageInput is the command adapters use to send a user turn into the
// shared agent channel.
type UserMessageInput struct {
	SessionID     string
	Conversation  ConversationInput
	Content       string
	CorrelationID string
	Metadata      map[string]any
	Async         bool
}

// UserMessageResult is the durable result of accepting a user message.
type UserMessageResult struct {
	Session persistence.Session
	Message persistence.Message
}

// NewService wires a channel-neutral service.
func NewService(opts Options) (*Service, error) {
	if opts.Sessions == nil {
		return nil, errors.New("agentchannel: Sessions is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Service{
		Sessions:              opts.Sessions,
		Diagnostics:           opts.Diagnostics,
		Events:                opts.Events,
		WorkflowDefinitions:   opts.WorkflowDefinitions,
		WorkflowRuns:          opts.WorkflowRuns,
		ChatAgent:             opts.ChatAgent,
		ChatAgentTimeout:      opts.ChatAgentTimeout,
		ChatAgentHistoryLimit: opts.ChatAgentHistoryLimit,
		logger:                opts.Logger,
	}, nil
}

// ResolveConversation returns an existing external conversation or creates a
// new session for it. Web callers may skip this and use explicit session IDs.
func (s *Service) ResolveConversation(ctx context.Context, input ConversationInput) (persistence.Session, error) {
	return s.Sessions.ResolveOrCreateByExternalKey(ctx, session.CreateInput{
		ProjectName:         input.ProjectName,
		Title:               input.Title,
		Provider:            input.Provider,
		Model:               input.Model,
		Source:              input.Source,
		ExternalKey:         input.ExternalKey,
		ExternalWorkspaceID: input.ExternalWorkspaceID,
		ExternalChannelID:   input.ExternalChannelID,
		ExternalThreadID:    input.ExternalThreadID,
		ExternalUserID:      input.ExternalUserID,
		Metadata:            input.Metadata,
	})
}

// SubmitUserMessage persists a user message, publishes it, and optionally
// starts assistant processing. It is the preferred entrypoint for non-web
// adapters such as Slack.
func (s *Service) SubmitUserMessage(ctx context.Context, input UserMessageInput) (UserMessageResult, error) {
	var (
		sess persistence.Session
		err  error
	)
	if input.SessionID != "" {
		sess, err = s.Sessions.Get(ctx, input.SessionID)
	} else {
		sess, err = s.ResolveConversation(ctx, input.Conversation)
	}
	if err != nil {
		return UserMessageResult{}, err
	}
	stored, err := s.Sessions.AppendMessage(ctx, sess.ID, session.AppendInput{
		Role:          persistence.MessageRoleUser,
		Content:       input.Content,
		CorrelationID: input.CorrelationID,
		Metadata:      input.Metadata,
	})
	if err != nil {
		return UserMessageResult{}, err
	}
	s.PublishMessage(stored)
	if input.Async {
		s.ScheduleUserMessage(sess.ID, stored)
	} else {
		s.HandleUserMessage(ctx, sess.ID, stored)
	}
	return UserMessageResult{Session: sess, Message: stored}, nil
}

// PublishMessage emits a persisted message to interested channel adapters.
func (s *Service) PublishMessage(message persistence.Message) {
	if s != nil && s.Events != nil {
		s.Events.Publish(message.SessionID, events.KindMessage, message, message.CorrelationID)
	}
}

// ScheduleUserMessage starts assistant processing for a user message in the
// background. Non-user messages and unconfigured agents are ignored.
func (s *Service) ScheduleUserMessage(sessionID string, userMessage persistence.Message) {
	if s == nil || s.ChatAgent == nil || userMessage.Role != persistence.MessageRoleUser {
		return
	}
	go s.HandleUserMessage(context.Background(), sessionID, userMessage)
}

// HandleUserMessage runs the assistant for a persisted user message.
func (s *Service) HandleUserMessage(ctx context.Context, sessionID string, userMessage persistence.Message) {
	timeout := s.chatAgentTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sess, err := s.Sessions.Get(ctx, sessionID)
	if err != nil {
		s.logChatAgentError("load session", sessionID, userMessage.ID, err)
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	recorder := newSessionToolCallRecorder(s.Sessions, s.Events, sessionID, userMessage.ID)
	mainCallID, err := recorder.Start(ctx, "agentflow.chat", map[string]any{
		"session_id": sessionID,
		"message_id": userMessage.ID,
		"project":    sess.ProjectName,
	})
	if err != nil {
		s.logChatAgentError("start tool call", sessionID, userMessage.ID, err)
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	userText, err := s.resolveMessageText(ctx, userMessage)
	if err != nil {
		s.logChatAgentError("resolve user message", sessionID, userMessage.ID, err)
		_ = recorder.Finish(ctx, mainCallID, chatagent.ToolCallFailed, nil, err.Error())
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	history, err := s.loadChatHistory(ctx, sessionID, userMessage.ID)
	if err != nil {
		s.logChatAgentError("load history", sessionID, userMessage.ID, err)
		_ = recorder.Finish(ctx, mainCallID, chatagent.ToolCallFailed, nil, err.Error())
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	s.logChatAgentStart(sessionID, userMessage.ID, sess)
	resp, err := s.ChatAgent.Run(ctx, chatagent.RunRequest{
		SessionID:     sessionID,
		ProjectPath:   sess.ProjectPath,
		ProjectName:   sess.ProjectName,
		Provider:      sess.Provider,
		Model:         sess.Model,
		UserMessage:   userText,
		History:       history,
		HistoryLimit:  s.chatAgentHistoryLimit(),
		CorrelationID: userMessage.CorrelationID,
		ToolEnvironment: &chatagent.ToolEnvironment{
			SessionID:     sessionID,
			ProjectPath:   sess.ProjectPath,
			ProjectName:   sess.ProjectName,
			Definitions:   s.WorkflowDefinitions,
			Runs:          s.WorkflowRuns,
			ProjectReader: chatagent.NewProjectReader(sess.ProjectPath),
			Recorder:      recorder,
		},
	})
	if err != nil {
		s.logChatAgentError("run chat agent", sessionID, userMessage.ID, err)
		_ = recorder.Finish(ctx, mainCallID, chatagent.ToolCallFailed, nil, err.Error())
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	assistant, err := s.Sessions.AppendMessage(ctx, sessionID, session.AppendInput{
		Role:          persistence.MessageRoleAssistant,
		Content:       resp.Text,
		CorrelationID: userMessage.CorrelationID,
		Metadata:      resp.Metadata,
	})
	if err != nil {
		s.logChatAgentError("store assistant reply", sessionID, userMessage.ID, err)
		_ = recorder.Finish(ctx, mainCallID, chatagent.ToolCallFailed, nil, err.Error())
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	if err := recorder.Finish(ctx, mainCallID, chatagent.ToolCallSucceeded, assistant, ""); err != nil {
		s.logChatAgentError("finish tool call", sessionID, userMessage.ID, err)
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}
	s.logChatAgentComplete(sessionID, userMessage.ID, assistant.ID, resp.Metadata)
	s.PublishMessage(assistant)
}

func (s *Service) resolveMessageText(ctx context.Context, msg persistence.Message) (string, error) {
	if msg.Content != "" {
		return msg.Content, nil
	}
	if msg.PayloadRef == "" {
		return "", nil
	}
	body, _, err := s.Sessions.ResolvePayload(ctx, msg.PayloadRef)
	if err != nil {
		return "", fmt.Errorf("resolve payload %s: %w", msg.PayloadRef, err)
	}
	return string(body), nil
}

func (s *Service) loadChatHistory(ctx context.Context, sessionID, currentMessageID string) ([]chatagent.Message, error) {
	msgs, err := s.Sessions.ListMessages(ctx, sessionID, 0)
	if err != nil {
		return nil, err
	}
	if currentMessageID != "" {
		filtered := msgs[:0]
		for _, msg := range msgs {
			if msg.ID == currentMessageID {
				continue
			}
			filtered = append(filtered, msg)
		}
		msgs = filtered
	}
	limit := s.chatAgentHistoryLimit()
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	history := make([]chatagent.Message, 0, len(msgs))
	for _, msg := range msgs {
		text, err := s.resolveMessageText(ctx, msg)
		if err != nil {
			return nil, err
		}
		history = append(history, chatagent.Message{
			Role:    string(msg.Role),
			Content: text,
		})
	}
	return history, nil
}

func (s *Service) chatAgentTimeout() time.Duration {
	if s != nil && s.ChatAgentTimeout > 0 {
		return s.ChatAgentTimeout
	}
	return defaultChatAgentTimeout
}

func (s *Service) chatAgentHistoryLimit() int {
	if s != nil && s.ChatAgentHistoryLimit > 0 {
		return s.ChatAgentHistoryLimit
	}
	return defaultChatAgentHistoryLen
}

func (s *Service) recordChatAgentFailure(ctx context.Context, sessionID, correlationID string, err error) {
	if err == nil || s == nil || s.Diagnostics == nil {
		return
	}
	_, _ = s.Diagnostics.Record(ctx, persistence.Diagnostic{
		SessionID:     sessionID,
		Level:         persistence.DiagnosticLevelError,
		Source:        "chat_agent",
		Code:          "chat_agent_failed",
		Message:       err.Error(),
		CorrelationID: correlationID,
	})
}

func (s *Service) logChatAgentStart(sessionID, messageID string, sess persistence.Session) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(
		"chat agent started",
		"session_id", sessionID,
		"message_id", messageID,
		"project", sess.ProjectName,
		"provider", sess.Provider,
		"model", sess.Model,
	)
}

func (s *Service) logChatAgentComplete(sessionID, messageID, assistantMessageID string, metadata map[string]any) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(
		"chat agent completed",
		"session_id", sessionID,
		"message_id", messageID,
		"assistant_message_id", assistantMessageID,
		"provider", metadataString(metadata, "provider"),
		"model", metadataString(metadata, "model"),
	)
}

func (s *Service) logChatAgentError(stage, sessionID, messageID string, err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.Error(
		"chat agent failed",
		"stage", stage,
		"session_id", sessionID,
		"message_id", messageID,
		"error", err,
	)
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key]; ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return ""
}

type sessionToolCallRecorder struct {
	sessions   *session.Sessions
	events     EventSink
	sessionID  string
	messageID  string
	mu         sync.Mutex
	activeCall map[string]persistence.ToolCall
}

func newSessionToolCallRecorder(sessions *session.Sessions, events EventSink, sessionID, messageID string) *sessionToolCallRecorder {
	return &sessionToolCallRecorder{
		sessions:   sessions,
		events:     events,
		sessionID:  sessionID,
		messageID:  messageID,
		activeCall: map[string]persistence.ToolCall{},
	}
}

func (r *sessionToolCallRecorder) Start(ctx context.Context, name string, request any) (string, error) {
	if r == nil || r.sessions == nil {
		return "", errors.New("tool call recorder is not configured")
	}
	call := persistence.ToolCall{
		SessionID:     r.sessionID,
		MessageID:     r.messageID,
		Name:          name,
		Status:        persistence.ToolCallStatusRunning,
		CorrelationID: diagnostics.NewCorrelationID(),
	}
	stored, err := r.sessions.RecordToolCall(ctx, call)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.activeCall[stored.ID] = stored
	r.mu.Unlock()
	r.publish(stored)
	return stored.ID, nil
}

func (r *sessionToolCallRecorder) Finish(ctx context.Context, id string, status chatagent.ToolCallStatus, response any, errMsg string) error {
	if r == nil || r.sessions == nil {
		return errors.New("tool call recorder is not configured")
	}
	persistedStatus := persistence.ToolCallStatus(status)
	if err := r.sessions.UpdateToolCallStatus(ctx, id, persistedStatus, "", errMsg); err != nil {
		return err
	}
	call := persistence.ToolCall{ID: id, SessionID: r.sessionID, MessageID: r.messageID, Status: persistedStatus, Error: errMsg, FinishedAt: time.Now().UTC()}
	r.mu.Lock()
	if existing, ok := r.activeCall[id]; ok {
		call = existing
		call.Status = persistedStatus
		call.Error = errMsg
		call.FinishedAt = time.Now().UTC()
		delete(r.activeCall, id)
	}
	r.mu.Unlock()
	if call.Name == "" {
		if stored, err := r.sessions.GetToolCall(ctx, id); err == nil {
			call = stored
			call.Status = persistedStatus
			call.Error = errMsg
			call.FinishedAt = time.Now().UTC()
		}
	}
	r.publish(call)
	return nil
}

func (r *sessionToolCallRecorder) publish(call persistence.ToolCall) {
	if r != nil && r.events != nil {
		r.events.Publish(r.sessionID, events.KindToolCall, call, call.CorrelationID)
	}
}
