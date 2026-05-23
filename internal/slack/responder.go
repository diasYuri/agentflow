package slack

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/agentchannel/session"
)

type responder struct {
	sessions *session.Sessions
	broker   *events.Broker
	client   *Client
	logger   *slog.Logger
}

func newResponder(sessions *session.Sessions, broker *events.Broker, client *Client, logger *slog.Logger) *responder {
	return &responder{
		sessions: sessions,
		broker:   broker,
		client:   client,
		logger:   logger,
	}
}

func (r *responder) Run(ctx context.Context) {
	if r == nil || r.sessions == nil || r.broker == nil || r.client == nil {
		return
	}
	sub := r.broker.Subscribe("")
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub.C:
			if !ok {
				return
			}
			msg, ok := evt.Payload.(persistence.Message)
			if !ok || msg.Role != persistence.MessageRoleAssistant {
				continue
			}
			if err := r.postAssistant(ctx, msg); err != nil {
				r.logWarn("post assistant reply", err)
			}
		}
	}
}

func (r *responder) postAssistant(ctx context.Context, msg persistence.Message) error {
	sess, err := r.sessions.Get(ctx, msg.SessionID)
	if err != nil {
		return fmt.Errorf("load session %s: %w", msg.SessionID, err)
	}
	text, err := r.messageText(ctx, msg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if strings.TrimSpace(sess.ExternalChannelID) == "" || strings.TrimSpace(sess.ExternalThreadID) == "" {
		return fmt.Errorf("session %s is missing slack channel metadata", sess.ID)
	}
	if err := r.client.PostMessage(ctx, PostMessageInput{
		Channel:  sess.ExternalChannelID,
		ThreadTS: sess.ExternalThreadID,
		Text:     text,
	}); err != nil {
		return fmt.Errorf("post slack message: %w", err)
	}
	return nil
}

func (r *responder) messageText(ctx context.Context, msg persistence.Message) (string, error) {
	if strings.TrimSpace(msg.Content) != "" {
		return msg.Content, nil
	}
	if strings.TrimSpace(msg.PayloadRef) == "" {
		return "", nil
	}
	body, _, err := r.sessions.ResolvePayload(ctx, msg.PayloadRef)
	if err != nil {
		return "", fmt.Errorf("resolve payload %s: %w", msg.PayloadRef, err)
	}
	return string(body), nil
}

func (r *responder) logWarn(msg string, err error) {
	if r == nil || r.logger == nil || err == nil {
		return
	}
	r.logger.Warn(msg, "error", err)
}
