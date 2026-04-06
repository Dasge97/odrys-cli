package serverclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dasge97/odrys-cli/internal/backend"
)

type Client struct {
	baseURL string
	http    *http.Client
	token   string
	root    string
	userID  string
}

func New() *Client {
	baseURL := strings.TrimRight(envOr("ODYRSD_URL", "http://127.0.0.1:4111"), "/")
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
		root: envOr("ODYRS_ROOT", "."),
	}
}

func (c *Client) Available() bool {
	_ = c.ensureSession()
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	var payload struct {
		Status string `json:"status"`
		Name   string `json:"name"`
		Root   string `json:"root"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false
	}
	if strings.TrimSpace(payload.Root) == "" {
		return true
	}
	expected, err := filepath.Abs(c.root)
	if err != nil {
		return false
	}
	actual, err := filepath.Abs(payload.Root)
	if err != nil {
		return false
	}
	return expected == actual
}

func (c *Client) ensureSession() error {
	if c.token != "" {
		return nil
	}
	if token, err := c.loadToken(); err == nil && token != "" {
		c.token = token
		return nil
	}
	var payload struct {
		Session struct {
			Token string `json:"token"`
		} `json:"session"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := c.postWithoutAuth("/api/v1/core/bootstrap", map[string]string{
		"label": "odrys-cli",
	}, &payload); err != nil {
		return err
	}
	c.token = payload.Session.Token
	c.userID = payload.User.ID
	return c.saveToken(c.token)
}

func (c *Client) ListSessions() ([]backend.SessionSummary, error) {
	var payload struct {
		Items []backend.SessionSummary `json:"items"`
	}
	if err := c.get("/api/v1/sessions", &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func (c *Client) CreateSession(title string) (backend.Session, error) {
	var session backend.Session
	if err := c.post("/api/v1/sessions", map[string]string{"title": title}, &session); err != nil {
		return backend.Session{}, err
	}
	return session, nil
}

func (c *Client) LoadSession(id string) (backend.Session, error) {
	var session backend.Session
	if err := c.get("/api/v1/sessions/"+id, &session); err != nil {
		return backend.Session{}, err
	}
	return session, nil
}

func (c *Client) OpenAIStatus() (backend.OpenAIAuthStatus, error) {
	var payload struct {
		Status backend.OpenAIAuthStatus `json:"status"`
	}
	if err := c.get("/api/v1/openai/status", &payload); err != nil {
		return backend.OpenAIAuthStatus{}, err
	}
	return payload.Status, nil
}

func (c *Client) ConnectOpenAIAPIKey(apiKey, model string) error {
	return c.post("/api/v1/openai/connect/api-key", map[string]string{
		"api_key": apiKey,
		"model":   model,
	}, nil)
}

func (c *Client) StartOpenAIDevice(model string) (string, backend.OpenAIDeviceCodeSession, error) {
	var payload struct {
		ID     string                        `json:"id"`
		Device backend.OpenAIDeviceCodeSession `json:"device"`
	}
	if err := c.post("/api/v1/openai/connect/device/start", map[string]string{
		"model": model,
	}, &payload); err != nil {
		return "", backend.OpenAIDeviceCodeSession{}, err
	}
	return payload.ID, payload.Device, nil
}

func (c *Client) PollOpenAIDevice(id string) (backend.OpenAIDeviceCodePollResult, error) {
	var payload struct {
		Status string `json:"status"`
		Email  string `json:"email"`
	}
	if err := c.get("/api/v1/openai/connect/device/poll/"+id, &payload); err != nil {
		return backend.OpenAIDeviceCodePollResult{}, err
	}
	return backend.OpenAIDeviceCodePollResult{
		Status: payload.Status,
		Email:  payload.Email,
	}, nil
}

func (c *Client) DisconnectOpenAI() error {
	return c.post("/api/v1/openai/disconnect", map[string]string{}, nil)
}

func (c *Client) get(path string, out any) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, out)
}

func (c *Client) post(path string, body any, out any) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, out)
}

func (c *Client) postWithoutAuth(path string, body any, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, out)
}

func decodeResponse(resp *http.Response, out any) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if message, _ := payload["error"].(string); message != "" {
				return fmt.Errorf(message)
			}
		}
		return fmt.Errorf("error del backend: %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (c *Client) tokenPath() string {
	return filepath.Join(c.root, ".odrys", "core-client.json")
}

func (c *Client) loadToken() (string, error) {
	raw, err := os.ReadFile(c.tokenPath())
	if err != nil {
		return "", err
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	return payload.Token, nil
}

func (c *Client) saveToken(token string) error {
	if err := os.MkdirAll(filepath.Dir(c.tokenPath()), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(map[string]string{"token": token}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenPath(), append(raw, '\n'), 0o600)
}
