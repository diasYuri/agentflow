package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/diasYuri/agentflow/internal/web/chatagent"
	"github.com/diasYuri/agentflow/internal/web/diagnostics"
	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
	"github.com/diasYuri/agentflow/internal/web/session"
)

const (
	defaultChatAgentTimeout    = 60 * time.Second
	defaultChatAgentHistoryLen = 40
)

func (s *Service) scheduleChatAgent(sessionID string, userMessage persistence.Message) {
	if s == nil || s.ChatAgent == nil || userMessage.Role != persistence.MessageRoleUser {
		return
	}
	go s.runChatAgent(sessionID, userMessage)
}

func (s *Service) runChatAgent(sessionID string, userMessage persistence.Message) {
	timeout := s.chatAgentTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sess, err := s.Sessions.Get(ctx, sessionID)
	if err != nil {
		s.logChatAgentError("load session", sessionID, userMessage.ID, err)
		s.recordChatAgentFailure(ctx, sessionID, userMessage.CorrelationID, err)
		return
	}

	recorder := newSessionToolCallRecorder(s.Sessions, s.Broker, sessionID, userMessage.ID)
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
	s.Broker.Publish(sessionID, events.KindMessage, assistant, assistant.CorrelationID)
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
	broker     *events.Broker
	sessionID  string
	messageID  string
	mu         sync.Mutex
	activeCall map[string]persistence.ToolCall
}

func newSessionToolCallRecorder(sessions *session.Sessions, broker *events.Broker, sessionID, messageID string) *sessionToolCallRecorder {
	return &sessionToolCallRecorder{
		sessions:   sessions,
		broker:     broker,
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
	if r != nil && r.broker != nil {
		r.broker.Publish(r.sessionID, events.KindToolCall, call, call.CorrelationID)
	}
}
