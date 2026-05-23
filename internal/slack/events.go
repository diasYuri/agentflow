package slack

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diasYuri/agentflow/internal/agentchannel"
)

type socketEnvelope struct {
	EnvelopeID string          `json:"envelope_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

type eventsAPIPayload struct {
	Type           string               `json:"type"`
	TeamID         string               `json:"team_id"`
	APIAppID       string               `json:"api_app_id,omitempty"`
	Event          slackEvent           `json:"event"`
	Authorizations []slackAuthorization `json:"authorizations,omitempty"`
	EventContext   string               `json:"event_context,omitempty"`
	EventID        string               `json:"event_id,omitempty"`
	EventTime      int64                `json:"event_time,omitempty"`
	IsExtShared    bool                 `json:"is_ext_shared_channel,omitempty"`
	BotID          string               `json:"bot_id,omitempty"`
}

type slackAuthorization struct {
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
	IsBot  bool   `json:"is_bot"`
}

type slackEvent struct {
	Type        string `json:"type"`
	User        string `json:"user"`
	Text        string `json:"text"`
	Channel     string `json:"channel"`
	ChannelType string `json:"channel_type"`
	Subtype     string `json:"subtype"`
	BotID       string `json:"bot_id"`
	ThreadTS    string `json:"thread_ts"`
	TS          string `json:"ts"`
	EventTS     string `json:"event_ts"`
	Team        string `json:"team"`
}

func (e slackEvent) isSupported() bool {
	if strings.TrimSpace(e.Text) == "" {
		return false
	}
	if strings.TrimSpace(e.Subtype) != "" || strings.TrimSpace(e.BotID) != "" {
		return false
	}
	switch e.Type {
	case "app_mention":
		return true
	case "message":
		return strings.EqualFold(strings.TrimSpace(e.ChannelType), "im")
	default:
		return false
	}
}

func (e slackEvent) content(botUserID string) string {
	text := strings.TrimSpace(e.Text)
	if e.Type != "app_mention" {
		return text
	}
	if botUserID == "" {
		return text
	}
	prefix := "<@" + botUserID + ">"
	text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
	if text == "" {
		return ""
	}
	return text
}

func (e slackEvent) externalKey(teamID string) string {
	threadTS := strings.TrimSpace(e.ThreadTS)
	if threadTS == "" {
		threadTS = strings.TrimSpace(e.TS)
	}
	if teamID == "" {
		teamID = strings.TrimSpace(e.Team)
	}
	return fmt.Sprintf("slack:%s:%s:%s", teamID, strings.TrimSpace(e.Channel), threadTS)
}

func (e slackEvent) toUserMessageInput(projectName, teamID, botUserID, envelopeID string) (agentchannel.UserMessageInput, bool) {
	if !e.isSupported() {
		return agentchannel.UserMessageInput{}, false
	}
	content := e.content(botUserID)
	if strings.TrimSpace(content) == "" {
		return agentchannel.UserMessageInput{}, false
	}
	workspaceID := strings.TrimSpace(teamID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(e.Team)
	}
	threadTS := strings.TrimSpace(e.ThreadTS)
	if threadTS == "" {
		threadTS = strings.TrimSpace(e.TS)
	}
	externalKey := e.externalKey(workspaceID)
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(externalKey) == "" || strings.TrimSpace(e.Channel) == "" || strings.TrimSpace(threadTS) == "" {
		return agentchannel.UserMessageInput{}, false
	}
	input := agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			ProjectName:         projectName,
			Title:               "Slack thread",
			Source:              "slack",
			ExternalKey:         externalKey,
			ExternalWorkspaceID: workspaceID,
			ExternalChannelID:   strings.TrimSpace(e.Channel),
			ExternalThreadID:    threadTS,
			ExternalUserID:      strings.TrimSpace(e.User),
		},
		Content:       content,
		CorrelationID: envelopeID,
		Async:         true,
	}
	return input, true
}

func ackEnvelope(envelopeID string) ([]byte, error) {
	if strings.TrimSpace(envelopeID) == "" {
		return nil, nil
	}
	return json.Marshal(map[string]string{"envelope_id": envelopeID})
}
