package backend

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const authStoreRelativePath = ".odrys/auth.json"

type ProviderAuthInfo struct {
	Type         string `json:"type"`
	APIKey       string `json:"apiKey,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	AccountID    string `json:"accountId,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
}

type authStore struct {
	Providers map[string]ProviderAuthInfo `json:"providers"`
}

type OpenAIAuthStatus struct {
	Connected           bool           `json:"connected"`
	Method              string         `json:"method,omitempty"`
	Provider            ProviderConfig `json:"provider"`
	DeviceCodeAvailable bool           `json:"deviceCodeAvailable"`
	DeviceCodeMessage   string         `json:"deviceCodeMessage,omitempty"`
}

type OpenAIDeviceCodeSession struct {
	ClientID        string `json:"clientId"`
	DeviceAuthID    string `json:"deviceAuthId"`
	UserCode        string `json:"userCode"`
	VerificationURL string `json:"verificationUrl"`
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	LaunchURL       string `json:"launchUrl,omitempty"`
	IntervalSeconds int    `json:"intervalSeconds"`
	ExpiresAt       string `json:"expiresAt"`
	Model           string `json:"model"`
	State           string `json:"-"`
	CodeVerifier    string `json:"-"`
	RedirectURL     string `json:"-"`
}

type OpenAIDeviceCodePollResult struct {
	Status string `json:"status"`
	Email  string `json:"email,omitempty"`
}

func authStorePath(root string) string {
	if override := strings.TrimSpace(os.Getenv("ODYRS_AUTH_STORE")); override != "" {
		return override
	}
	if global, err := globalAuthStorePath(); err == nil && global != "" {
		return global
	}
	return filepath.Join(root, authStoreRelativePath)
}

func ensureAuthStore(root string) error {
	storePath, migrateFrom := resolveAuthStorePath(root)
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(storePath); err == nil {
		return nil
	}
	if migrateFrom != "" {
		if raw, err := os.ReadFile(migrateFrom); err == nil {
			return os.WriteFile(storePath, raw, 0o600)
		}
	}
	raw, marshalErr := json.MarshalIndent(authStore{Providers: map[string]ProviderAuthInfo{}}, "", "  ")
	if marshalErr != nil {
		return marshalErr
	}
	return os.WriteFile(storePath, append(raw, '\n'), 0o600)
}

func loadAuthStore(root string) (authStore, error) {
	if err := ensureAuthStore(root); err != nil {
		return authStore{}, err
	}
	raw, err := os.ReadFile(authStorePath(root))
	if err != nil {
		return authStore{}, err
	}
	var store authStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return authStore{}, err
	}
	if store.Providers == nil {
		store.Providers = map[string]ProviderAuthInfo{}
	}
	return store, nil
}

func saveAuthStore(root string, store authStore) error {
	if store.Providers == nil {
		store.Providers = map[string]ProviderAuthInfo{}
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(authStorePath(root), append(raw, '\n'), 0o600)
}

func loadProviderAuth(root, provider string) (ProviderAuthInfo, bool, error) {
	store, err := loadAuthStore(root)
	if err != nil {
		return ProviderAuthInfo{}, false, err
	}
	info, ok := store.Providers[provider]
	return info, ok, nil
}

func saveProviderAuth(root, provider string, info ProviderAuthInfo) error {
	store, err := loadAuthStore(root)
	if err != nil {
		return err
	}
	store.Providers[provider] = info
	return saveAuthStore(root, store)
}

func SaveProviderAuthForServer(root, provider string, info ProviderAuthInfo) error {
	return saveProviderAuth(root, provider, info)
}

func removeProviderAuth(root, provider string) error {
	store, err := loadAuthStore(root)
	if err != nil {
		return err
	}
	delete(store.Providers, provider)
	return saveAuthStore(root, store)
}

func DisconnectOpenAI(root string) error {
	return removeProviderAuth(root, "openai")
}

func decodeJWTPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil
	}
	return claims
}

func extractOpenAIAccountID(tokens openAITokenResponse) string {
	candidates := []string{tokens.IDToken, tokens.AccessToken}
	for _, token := range candidates {
		if token == "" {
			continue
		}
		claims := decodeJWTPayload(token)
		if claims == nil {
			continue
		}
		if value, _ := claims["chatgpt_account_id"].(string); value != "" {
			return value
		}
		if nested, _ := claims["https://api.openai.com/auth"].(map[string]any); nested != nil {
			if value, _ := nested["chatgpt_account_id"].(string); value != "" {
				return value
			}
		}
		if orgs, _ := claims["organizations"].([]any); len(orgs) > 0 {
			first, _ := orgs[0].(map[string]any)
			if value, _ := first["id"].(string); value != "" {
				return value
			}
		}
	}
	return ""
}

func parseRFC3339Time(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, errors.New("fecha vacia")
	}
	return time.Parse(time.RFC3339, value)
}

func globalAuthStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "odrys", "auth.json"), nil
}

func resolveAuthStorePath(root string) (string, string) {
	target := authStorePath(root)
	if _, err := os.Stat(target); err == nil {
		return target, ""
	}
	for _, legacy := range legacyAuthCandidates(root) {
		if legacy == "" || legacy == target {
			continue
		}
		if _, err := os.Stat(legacy); err == nil {
			return target, legacy
		}
	}
	return target, ""
}

func legacyAuthCandidates(root string) []string {
	candidates := []string{
		filepath.Join(root, authStoreRelativePath),
	}
	if executable, err := os.Executable(); err == nil {
		execDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(execDir, "..", ".odrys", "auth.json"),
			filepath.Join(execDir, "..", "..", ".odrys", "auth.json"),
		)
	}
	return candidates
}
