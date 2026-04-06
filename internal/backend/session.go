package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func sessionsDir(root string) string {
	return filepath.Join(root, "logs", "sessions")
}

func ensureSessionsDir(root string) error {
	return os.MkdirAll(sessionsDir(root), 0o755)
}

func sessionPath(root, sessionID string) string {
	return filepath.Join(sessionsDir(root), sessionID+".json")
}

func newSession(goal string) Session {
	now := time.Now().UTC().Format(time.RFC3339)
	title := strings.TrimSpace(goal)
	if title == "" {
		title = "New session"
	}
	id := time.Now().UTC().Format(time.RFC3339Nano)
	id = strings.NewReplacer(":", "-", ".", "-").Replace(id)
	return Session{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "",
		Messages:  []SessionMessage{},
	}
}

func loadSession(root, sessionID string) (Session, error) {
	raw, err := os.ReadFile(sessionPath(root, sessionID))
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func saveSession(root string, session Session) error {
	if err := ensureSessionsDir(root); err != nil {
		return err
	}
	session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(root, session.ID), append(raw, '\n'), 0o644)
}

func appendSessionMessage(root, sessionID, role, content string) (Session, error) {
	session, err := loadOrCreateSession(root, sessionID, content)
	if err != nil {
		return Session{}, err
	}
	appendSessionMessageToState(&session, role, content)
	if err := saveSession(root, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func appendSessionAssistantMemory(root, sessionID, content, note string, files []string) (Session, error) {
	session, err := loadOrCreateSession(root, sessionID, content)
	if err != nil {
		return Session{}, err
	}
	appendSessionMessageToState(&session, "assistant", content)
	for _, file := range files {
		session.RecentFiles = appendUniqueRecent(session.RecentFiles, file, 12)
	}
	if strings.TrimSpace(note) != "" {
		session.RecentNotes = appendUniqueRecent(session.RecentNotes, note, 12)
	}
	if err := saveSession(root, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func appendSessionMessageToState(session *Session, role, content string) {
	session.Messages = append(session.Messages, SessionMessage{
		Role:      role,
		Content:   strings.TrimSpace(content),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if role == "user" && strings.TrimSpace(session.Title) == "" {
		session.Title = summarizeTitle(content)
	}
	if role == "user" {
		session.RecentGoals = appendUniqueRecent(session.RecentGoals, content, 8)
	}
}

func updateSessionSummary(root, sessionID, summary string) error {
	session, err := loadSession(root, sessionID)
	if err != nil {
		return err
	}
	session.Summary = strings.TrimSpace(summary)
	return saveSession(root, session)
}

func loadOrCreateSession(root, sessionID, title string) (Session, error) {
	if strings.TrimSpace(sessionID) == "" {
		session := newSession(summarizeTitle(title))
		return session, saveSession(root, session)
	}
	session, err := loadSession(root, sessionID)
	if err == nil {
		return session, nil
	}
	if !os.IsNotExist(err) {
		return Session{}, err
	}
	session = newSession(summarizeTitle(title))
	session.ID = sessionID
	return session, saveSession(root, session)
}

func listSessions(root string, limit int) ([]SessionSummary, error) {
	if err := ensureSessionsDir(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(sessionsDir(root))
	if err != nil {
		return nil, err
	}
	var items []SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		session, err := loadSession(root, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		items = append(items, SessionSummary{
			ID:        session.ID,
			Title:     session.Title,
			UpdatedAt: session.UpdatedAt,
			Summary:   session.Summary,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func latestSession(root string) (*SessionSummary, error) {
	sessions, err := listSessions(root, 1)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

func summarizeTitle(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "New session"
	}
	runes := []rune(trimmed)
	if len(runes) > 48 {
		return string(runes[:48]) + "..."
	}
	return trimmed
}

func appendUniqueRecent(items []string, value string, limit int) []string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return items
	}
	filtered := make([]string, 0, len(items)+1)
	filtered = append(filtered, normalized)
	for _, item := range items {
		if strings.TrimSpace(item) == normalized {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}
