package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/diasYuri/agentflow/internal/agentchannel"
)

type socketRunner struct {
	client    *Client
	processor *processor
	logger    *slog.Logger
}

func newSocketRunner(client *Client, processor *processor, logger *slog.Logger) *socketRunner {
	return &socketRunner{client: client, processor: processor, logger: logger}
}

func (r *socketRunner) Run(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("slack socket runner is not configured")
	}
	backoff := time.Second
	for ctx.Err() == nil {
		socketURL, err := r.client.OpenSocketMode(ctx)
		if err != nil {
			if isFatalSlackAPIError(err) {
				return err
			}
			r.logWarn("open socket mode", err)
			if !sleepWithContext(ctx, backoff) {
				return ctx.Err()
			}
			backoff = growBackoff(backoff)
			continue
		}
		conn, _, err := websocket.Dial(ctx, socketURL, nil)
		if err != nil {
			r.logWarn("dial socket mode", err)
			if !sleepWithContext(ctx, backoff) {
				return ctx.Err()
			}
			backoff = growBackoff(backoff)
			continue
		}
		err = r.serve(ctx, conn)
		_ = conn.Close(websocket.StatusNormalClosure, "shutdown")
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			r.logWarn("socket loop", err)
			if !sleepWithContext(ctx, backoff) {
				return ctx.Err()
			}
			backoff = growBackoff(backoff)
			continue
		}
		backoff = time.Second
	}
	return ctx.Err()
}

func (r *socketRunner) serve(ctx context.Context, conn *websocket.Conn) error {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var envelope socketEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			r.logWarn("decode socket envelope", err)
			continue
		}
		if envelope.Type == "disconnect" {
			return errors.New("slack requested reconnect")
		}
		if ack, err := ackEnvelope(envelope.EnvelopeID); err != nil {
			r.logWarn("ack envelope", err)
		} else if len(ack) > 0 {
			if err := conn.Write(ctx, websocket.MessageText, ack); err != nil {
				return fmt.Errorf("ack envelope %s: %w", envelope.EnvelopeID, err)
			}
		}
		if envelope.Type != "events_api" {
			continue
		}
		if err := r.handleEventsAPI(ctx, envelope); err != nil {
			r.logWarn("handle events api", err)
		}
	}
}

func (r *socketRunner) handleEventsAPI(ctx context.Context, envelope socketEnvelope) error {
	var payload eventsAPIPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("decode events api payload: %w", err)
	}
	input, ok := payload.Event.toUserMessageInput(
		r.processor.projectName,
		r.processor.teamID(payload.TeamID),
		r.processor.botUserID,
		envelope.EnvelopeID,
	)
	if !ok {
		return nil
	}
	_, err := r.processor.submitter.SubmitUserMessage(ctx, input)
	if err != nil {
		return fmt.Errorf("submit slack message: %w", err)
	}
	return nil
}

func (r *socketRunner) logWarn(msg string, err error) {
	if r == nil || r.logger == nil || err == nil {
		return
	}
	r.logger.Warn(msg, "error", err)
}

type processor struct {
	submitter   submitter
	projectName string
	teamIDFn    func(payloadTeamID string) string
	botUserID   string
}

type submitter interface {
	SubmitUserMessage(context.Context, agentchannel.UserMessageInput) (agentchannel.UserMessageResult, error)
}

func (p *processor) teamID(payloadTeamID string) string {
	if p == nil || p.teamIDFn == nil {
		return strings.TrimSpace(payloadTeamID)
	}
	return p.teamIDFn(payloadTeamID)
}

func isFatalSlackAPIError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid_auth") ||
		strings.Contains(msg, "not_authed") ||
		strings.Contains(msg, "missing_scope") ||
		strings.Contains(msg, "invalid_app_id") ||
		strings.Contains(msg, "token_revoked")
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func growBackoff(backoff time.Duration) time.Duration {
	next := backoff * 2
	if next > 15*time.Second {
		return 15 * time.Second
	}
	if next < time.Second {
		return time.Second
	}
	return next
}
