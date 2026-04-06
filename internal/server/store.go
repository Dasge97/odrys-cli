package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type Store struct {
	root string
	path string
}

func NewStore(root string) *Store {
	return &Store{
		root: root,
		path: filepath.Join(root, ".odrys", "core-state.json"),
	}
}

func (s *Store) Ensure() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(s.path); err == nil {
		return nil
	}
	raw, err := json.MarshalIndent(State{}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o600)
}

func (s *Store) Load() (State, error) {
	if err := s.Ensure(); err != nil {
		return State{}, err
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Store) Save(state State) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o600)
}

func (s *Store) CreateSession(title string) (SessionRecord, error) {
	state, err := s.Load()
	if err != nil {
		return SessionRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	record := SessionRecord{
		ID:        shortID("ses"),
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	state.Sessions = append([]SessionRecord{record}, state.Sessions...)
	if err := s.Save(state); err != nil {
		return SessionRecord{}, err
	}
	return record, nil
}

func (s *Store) ListSessions() ([]SessionRecord, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}
	return state.Sessions, nil
}

func (s *Store) GetSession(id string) (SessionRecord, bool, error) {
	state, err := s.Load()
	if err != nil {
		return SessionRecord{}, false, err
	}
	for _, item := range state.Sessions {
		if item.ID == id {
			return item, true, nil
		}
	}
	return SessionRecord{}, false, nil
}

func (s *Store) UpsertAuthSession(record AuthSessionRecord) error {
	state, err := s.Load()
	if err != nil {
		return err
	}
	index := slices.IndexFunc(state.AuthSessions, func(item AuthSessionRecord) bool {
		return item.ID == record.ID
	})
	if index >= 0 {
		state.AuthSessions[index] = record
	} else {
		state.AuthSessions = append([]AuthSessionRecord{record}, state.AuthSessions...)
	}
	return s.Save(state)
}

func (s *Store) GetAuthSession(id string) (AuthSessionRecord, bool, error) {
	state, err := s.Load()
	if err != nil {
		return AuthSessionRecord{}, false, err
	}
	for _, item := range state.AuthSessions {
		if item.ID == id {
			return item, true, nil
		}
	}
	return AuthSessionRecord{}, false, nil
}

func (s *Store) GetAuthSessionByState(stateValue string) (AuthSessionRecord, bool, error) {
	state, err := s.Load()
	if err != nil {
		return AuthSessionRecord{}, false, err
	}
	for _, item := range state.AuthSessions {
		if item.State == stateValue {
			return item, true, nil
		}
	}
	return AuthSessionRecord{}, false, nil
}

func (s *Store) CreateClientSession(label string) (CoreClientSessionRecord, error) {
	state, err := s.Load()
	if err != nil {
		return CoreClientSessionRecord{}, err
	}
	user := ensureDefaultUser(&state)
	now := time.Now().UTC().Format(time.RFC3339)
	record := CoreClientSessionRecord{
		ID:        shortID("cli"),
		Token:     shortID("tok") + "-" + shortID("tok"),
		UserID:    user.ID,
		Label:     label,
		CreatedAt: now,
		UpdatedAt: now,
	}
	state.ClientSessions = append([]CoreClientSessionRecord{record}, state.ClientSessions...)
	if err := s.Save(state); err != nil {
		return CoreClientSessionRecord{}, err
	}
	return record, nil
}

func (s *Store) GetClientSessionByToken(token string) (CoreClientSessionRecord, bool, error) {
	state, err := s.Load()
	if err != nil {
		return CoreClientSessionRecord{}, false, err
	}
	for _, item := range state.ClientSessions {
		if item.Token == token {
			return item, true, nil
		}
	}
	return CoreClientSessionRecord{}, false, nil
}

func (s *Store) TouchClientSession(token string) error {
	state, err := s.Load()
	if err != nil {
		return err
	}
	index := slices.IndexFunc(state.ClientSessions, func(item CoreClientSessionRecord) bool {
		return item.Token == token
	})
	if index < 0 {
		return nil
	}
	state.ClientSessions[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.Save(state)
}

func (s *Store) UpsertConnection(record ProviderConnectionRecord) error {
	state, err := s.Load()
	if err != nil {
		return err
	}
	index := slices.IndexFunc(state.Connections, func(item ProviderConnectionRecord) bool {
		return item.UserID == record.UserID && item.Provider == record.Provider
	})
	if index >= 0 {
		record.CreatedAt = state.Connections[index].CreatedAt
		state.Connections[index] = record
	} else {
		state.Connections = append([]ProviderConnectionRecord{record}, state.Connections...)
	}
	return s.Save(state)
}

func (s *Store) RemoveConnection(userID, provider string) error {
	state, err := s.Load()
	if err != nil {
		return err
	}
	state.Connections = slices.DeleteFunc(state.Connections, func(item ProviderConnectionRecord) bool {
		return item.UserID == userID && item.Provider == provider
	})
	return s.Save(state)
}

func ensureDefaultUser(state *State) UserRecord {
	if len(state.Users) > 0 {
		return state.Users[0]
	}
	now := time.Now().UTC().Format(time.RFC3339)
	user := UserRecord{
		ID:        shortID("usr"),
		Label:     "local",
		CreatedAt: now,
		UpdatedAt: now,
	}
	state.Users = append(state.Users, user)
	return user
}

func shortID(prefix string) string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "-" + time.Now().UTC().Format("150405")
	}
	return prefix + "-" + hex.EncodeToString(buf)
}
