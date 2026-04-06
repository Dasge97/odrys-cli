package server

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SessionRecord struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type AuthSessionRecord struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id,omitempty"`
	ClientSessionID string `json:"client_session_id,omitempty"`
	Provider   string `json:"provider"`
	Method     string `json:"method"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	State      string `json:"state,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
	DeviceAuth string `json:"device_auth,omitempty"`
	UserCode   string `json:"user_code,omitempty"`
	VerifyURL  string `json:"verify_url,omitempty"`
	Model      string `json:"model,omitempty"`
	Email      string `json:"email,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type CoreClientSessionRecord struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type UserRecord struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ProviderConnectionRecord struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Provider  string `json:"provider"`
	Method    string `json:"method"`
	Status    string `json:"status"`
	Model     string `json:"model,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type State struct {
	Users         []UserRecord               `json:"users"`
	Sessions      []SessionRecord            `json:"sessions"`
	AuthSessions  []AuthSessionRecord        `json:"auth_sessions"`
	Connections   []ProviderConnectionRecord `json:"connections"`
	ClientSessions []CoreClientSessionRecord `json:"client_sessions"`
}

type HealthResponse struct {
	Status string `json:"status"`
	Name   string `json:"name"`
	Root   string `json:"root,omitempty"`
}

type CreateSessionRequest struct {
	Title string `json:"title"`
}

type OpenAIAPIKeyConnectRequest struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

type OpenAIDeviceStartRequest struct {
	Model string `json:"model"`
}

type BootstrapClientRequest struct {
	Label string `json:"label"`
}
