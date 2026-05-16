package runtime

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
)

// RunEvent representa um evento lido do JSONL.
type RunEvent struct {
	Cursor     int            `json:"cursor"`
	Timestamp  time.Time      `json:"timestamp"`
	RunID      string         `json:"run_id"`
	Type       string         `json:"type"`
	NodeID     string         `json:"node_id,omitempty"`
	InstanceID string         `json:"instance_id,omitempty"`
	Path       []string       `json:"path,omitempty"`
	Attempt    int            `json:"attempt,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// EventsResponse retorna eventos paginados de uma run.
type EventsResponse struct {
	RunID      string     `json:"run_id"`
	Events     []RunEvent `json:"events"`
	NextCursor int        `json:"next_cursor"`
	HasMore    bool       `json:"has_more"`
}

const (
	defaultEventLimit = 100
	maxEventLimit     = 1000
)

// GetRunEvents le eventos de events.jsonl com paginacao por cursor.
func GetRunEvents(runDir string, cursor, limit int) (EventsResponse, error) {
	if limit <= 0 {
		limit = defaultEventLimit
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}
	if cursor < 0 {
		cursor = 0
	}

	resp := EventsResponse{
		NextCursor: cursor,
	}

	eventsPath := filepath.Join(runDir, "events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return resp, nil
		}
		return EventsResponse{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineIndex := 0
	collected := 0

	for scanner.Scan() {
		if lineIndex < cursor {
			lineIndex++
			continue
		}
		if collected >= limit {
			resp.HasMore = true
			resp.NextCursor = lineIndex
			break
		}

		line := scanner.Bytes()
		var event corerun.Event
		if err := json.Unmarshal(line, &event); err != nil {
			lineIndex++
			resp.NextCursor = lineIndex
			continue
		}

		resp.Events = append(resp.Events, RunEvent{
			Cursor:     lineIndex,
			Timestamp:  event.Timestamp,
			RunID:      event.RunID,
			Type:       event.Type,
			NodeID:     event.NodeID,
			InstanceID: event.InstanceID,
			Path:       event.Path,
			Attempt:    event.Attempt,
			Data:       event.Data,
		})
		collected++
		lineIndex++
		resp.NextCursor = lineIndex
	}

	if err := scanner.Err(); err != nil {
		return EventsResponse{}, err
	}

	return resp, nil
}

// runProgress resume o estado atual de uma run a partir de artefatos em disco.
type runProgress struct {
	CurrentStep    string
	CompletedSteps []string
	PendingSteps   []string
	TotalSteps     int
	TerminalError  string
	RecentEvents   []string
	Paused         bool
	PauseReason    string
}

// loadProgress le plan.json e events.jsonl para reconstruir progresso.
func loadProgress(runDir string) (runProgress, error) {
	progress := runProgress{}
	if runDir == "" {
		return progress, os.ErrNotExist
	}
	planPath := filepath.Join(runDir, "plan.json")
	data, err := os.ReadFile(planPath)
	if err != nil {
		return progress, err
	}
	var plan struct {
		Order []string `json:"order"`
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		return progress, err
	}
	progress.TotalSteps = len(plan.Order)
	progress.PendingSteps = append([]string(nil), plan.Order...)

	completed := map[string]struct{}{}
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if lines, err := tailLines(eventsPath, 20); err == nil {
		progress.RecentEvents = lines
	}
	if events, err := loadRunEvents(eventsPath); err == nil {
		for _, event := range events {
			switch event.Type {
			case "node.started", "node.instance.started", "node.ready", "node.expanded":
				if event.NodeID != "" && progress.CurrentStep == "" {
					progress.CurrentStep = event.NodeID
				}
			case "node.skipped", "node.completed", "node.failed", "node.instance.completed", "node.instance.failed":
				if event.NodeID != "" {
					completed[event.NodeID] = struct{}{}
				}
			case "run.pausing":
				if event.Data != nil {
					if reason, _ := event.Data["reason"].(string); reason != "" {
						progress.PauseReason = reason
					}
				}
				if event.NodeID != "" {
					progress.CurrentStep = event.NodeID
				}
			case "run.paused":
				progress.Paused = true
				if event.Data != nil {
					if reason, _ := event.Data["reason"].(string); reason != "" {
						progress.PauseReason = reason
					}
				}
			case "run.resumed":
				progress.Paused = false
				progress.PauseReason = ""
			case "run.failed":
				if event.Data != nil {
					if status, _ := event.Data["status"].(string); status != "" {
						progress.TerminalError = status
					}
				}
			}
		}
	}
	progress.CompletedSteps = progress.CompletedSteps[:0]
	progress.PendingSteps = progress.PendingSteps[:0]
	for _, id := range plan.Order {
		if _, ok := completed[id]; ok {
			progress.CompletedSteps = append(progress.CompletedSteps, id)
			continue
		}
		progress.PendingSteps = append(progress.PendingSteps, id)
		if progress.CurrentStep == "" {
			progress.CurrentStep = id
		}
	}
	return progress, nil
}

type jsonlEvent struct {
	Type   string         `json:"type"`
	NodeID string         `json:"node_id"`
	Data   map[string]any `json:"data"`
}

func loadRunEvents(path string) ([]jsonlEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []jsonlEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event jsonlEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func tailLines(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func sortedNodeIDs(nodes map[string]corerun.NodeResult) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
