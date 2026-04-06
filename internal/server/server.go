package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Dasge97/odrys-cli/internal/backend"
)

type Server struct {
	root    string
	service *backend.Service
	store   *Store
	callbackMu sync.Mutex
	callbackStarted bool
}

type contextKey string

const clientSessionContextKey contextKey = "core-client-session"

func New(root string) *Server {
	return &Server{
		root:    root,
		service: backend.NewService(root),
		store:   NewStore(root),
	}
}

func (s *Server) Handler() (http.Handler, error) {
	if err := s.service.EnsureScaffold(); err != nil {
		return nil, err
	}
	if err := s.store.Ensure(); err != nil {
		return nil, err
	}
	if err := s.ensureOpenAICallbackServer(); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/core/bootstrap", s.handleCoreBootstrap)
	mux.HandleFunc("/api/v1/sessions", s.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionByID)
	mux.HandleFunc("/api/v1/openai/status", s.handleOpenAIStatus)
	mux.HandleFunc("/api/v1/openai/connect/api-key", s.handleOpenAIAPIKeyConnect)
	mux.HandleFunc("/api/v1/openai/connect/device/start", s.handleOpenAIDeviceStart)
	mux.HandleFunc("/api/v1/openai/connect/device/poll/", s.handleOpenAIDevicePoll)
	mux.HandleFunc("/api/v1/openai/connect/device/launch/", s.handleOpenAIDeviceLaunch)
	mux.HandleFunc("/api/v1/openai/disconnect", s.handleOpenAIDisconnect)
	protected := s.requireClientSession(mux)
	return s.withJSON(protected), nil
}

func (s *Server) ensureOpenAICallbackServer() error {
	s.callbackMu.Lock()
	defer s.callbackMu.Unlock()
	if s.callbackStarted {
		return nil
	}
	callbackMux := http.NewServeMux()
	callbackMux.HandleFunc("/auth/callback", s.handleOpenAIOAuthCallback)
	server := &http.Server{
		Addr:    "127.0.0.1:1455",
		Handler: callbackMux,
	}
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("odrys-core oauth callback server error: %v\n", err)
		}
	}()
	s.callbackStarted = true
	return nil
}

func (s *Server) withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireClientSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/api/v1/core/bootstrap" || strings.HasPrefix(r.URL.Path, "/api/v1/openai/connect/device/launch/") {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "falta sesion de odrys-core")
			return
		}
		session, ok, err := s.store.GetClientSessionByToken(token)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "sesion de odrys-core invalida")
			return
		}
		_ = s.store.TouchClientSession(token)
		ctx := context.WithValue(r.Context(), clientSessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{
		Status: "ok",
		Name:   "odrys-core",
		Root:   s.root,
	})
}

func (s *Server) handleCoreBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	var body BootstrapClientRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	session, err := s.store.CreateClientSession(strings.TrimSpace(body.Label))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	state, err := s.store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	user := ensureDefaultUser(&state)
	if err := s.store.Save(state); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := s.service.OpenAIStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"session": session,
		"user":    user,
		"capabilities": map[string]any{
			"openai": status,
		},
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.service.Sessions(24)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var body CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "json invalido")
			return
		}
		record, err := s.service.CreateSession(strings.TrimSpace(body.Title))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, record)
	default:
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
	}
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	id := path.Base(r.URL.Path)
	item, err := s.service.LoadSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "sesion no encontrada")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleOpenAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	status, err := s.service.OpenAIStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
	})
}

func (s *Server) handleOpenAIAPIKeyConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	var body OpenAIAPIKeyConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "json invalido")
		return
	}
	if err := s.service.ConnectOpenAI(body.APIKey, body.Model); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	clientSession, _ := r.Context().Value(clientSessionContextKey).(CoreClientSessionRecord)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.store.UpsertConnection(ProviderConnectionRecord{
		ID:        shortID("con"),
		UserID:    clientSession.UserID,
		Provider:  "openai",
		Method:    "api_key",
		Status:    "connected",
		Model:     fallback(body.Model, "gpt-4.1-mini"),
		CreatedAt: now,
		UpdatedAt: now,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": "openai",
		"model":    fallback(body.Model, "gpt-4.1-mini"),
		"method":   "api_key",
		"status":   "connected",
	})
}

func (s *Server) handleOpenAIDeviceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	var body OpenAIDeviceStartRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, "json invalido")
			return
	}
	session, err := s.service.StartOpenAIDeviceCode(body.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	clientSession, _ := r.Context().Value(clientSessionContextKey).(CoreClientSessionRecord)
	now := time.Now().UTC().Format(time.RFC3339)
	record := AuthSessionRecord{
		ID:              shortID("auth"),
		UserID:          clientSession.UserID,
		ClientSessionID: clientSession.ID,
		Provider:        "openai",
		Method:          "oauth_browser",
		Status:          "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
		State:           session.State,
		CodeVerifier:    session.CodeVerifier,
		RedirectURL:     session.RedirectURL,
		DeviceAuth:      session.DeviceAuthID,
		UserCode:        session.UserCode,
		VerifyURL:       fallback(session.AuthorizationURL, session.VerificationURL),
		Model:           session.Model,
		ExpiresAt:       session.ExpiresAt,
	}
	if err := s.store.UpsertAuthSession(record); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	session.LaunchURL = fmt.Sprintf("%s://%s/api/v1/openai/connect/device/launch/%s", scheme, r.Host, record.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       record.ID,
		"provider": record.Provider,
		"method":   record.Method,
		"status":   record.Status,
		"device":   session,
	})
}

func (s *Server) handleOpenAIDeviceLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	id := path.Base(r.URL.Path)
	record, ok, err := s.store.GetAuthSession(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "auth session no encontrada", http.StatusNotFound)
		return
	}
	target := strings.TrimSpace(record.VerifyURL)
	if target == "" {
		http.Error(w, "sesion sin URL de autorizacion", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) handleOpenAIDevicePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	id := path.Base(r.URL.Path)
	record, ok, err := s.store.GetAuthSession(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "auth session no encontrada")
		return
	}
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	expiry, expiryErr := time.Parse(time.RFC3339, record.ExpiresAt)
	if expiryErr == nil && record.Status == "pending" && time.Now().After(expiry) {
		record.Status = "expired"
		record.LastError = "codigo expirado"
		_ = s.store.UpsertAuthSession(record)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      record.ID,
		"status":  record.Status,
		"email":   record.Email,
		"provider": "openai",
	})
}

func (s *Server) handleOpenAIOAuthCallback(w http.ResponseWriter, r *http.Request) {
	errorCode := strings.TrimSpace(r.URL.Query().Get("error"))
	errorDescription := strings.TrimSpace(r.URL.Query().Get("error_description"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	stateValue := strings.TrimSpace(r.URL.Query().Get("state"))

	record, ok, err := s.store.GetAuthSessionByState(stateValue)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if errorCode != "" {
		record.Status = "error"
		record.LastError = fallback(errorDescription, errorCode)
		_ = s.store.UpsertAuthSession(record)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<html><body><h1>Authorization failed</h1><p>" + htmlEscape(record.LastError) + "</p></body></html>"))
		return
	}
	if code == "" {
		record.Status = "error"
		record.LastError = "missing authorization code"
		_ = s.store.UpsertAuthSession(record)
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	tokens, err := backend.ExchangeOpenAIAuthorizationCodeForOAuth(record.CodeVerifier, record.RedirectURL, code)
	if err != nil {
		record.Status = "error"
		record.LastError = err.Error()
		_ = s.store.UpsertAuthSession(record)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body><h1>Authorization failed</h1><p>" + htmlEscape(err.Error()) + "</p></body></html>"))
		return
	}

	accountID := backend.ExtractOpenAIAccountIDFromTokens(tokens)
	auth := backend.ProviderAuthInfo{
		Type:         "oauth",
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UnixMilli(),
		AccountID:    accountID,
		ClientID:     backend.OpenAICodexCompatibilityClientID(),
	}
	if err := backend.SaveProviderAuthForServer(s.root, "openai", auth); err != nil {
		record.Status = "error"
		record.LastError = err.Error()
		_ = s.store.UpsertAuthSession(record)
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	_ = s.service.SaveProvider(backend.ProviderConfig{Name: "openai", Model: fallback(record.Model, "gpt-4.1-mini")})
	email := ""
	if info, infoErr := backend.FetchOpenAIUserForOAuth(tokens.AccessToken); infoErr == nil {
		email = info.Email
	}
	record.Status = "success"
	record.Email = email
	record.LastError = ""
	_ = s.store.UpsertAuthSession(record)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.store.UpsertConnection(ProviderConnectionRecord{
		ID:        shortID("con"),
		UserID:    record.UserID,
		Provider:  "openai",
		Method:    record.Method,
		Status:    "connected",
		Model:     fallback(record.Model, "gpt-4.1-mini"),
		AccountID: accountID,
		CreatedAt: now,
		UpdatedAt: now,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<html><body><h1>Authorization successful</h1><p>Puedes cerrar esta ventana y volver a Odrys.</p><script>setTimeout(()=>window.close(),1500)</script></body></html>"))
}

func (s *Server) handleOpenAIDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no soportado")
		return
	}
	if err := backend.DisconnectOpenAI(s.root); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	clientSession, _ := r.Context().Value(clientSessionContextKey).(CoreClientSessionRecord)
	if clientSession.UserID != "" {
		_ = s.store.RemoveConnection(clientSession.UserID, "openai")
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"provider": "openai",
		"status":   "disconnected",
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
	})
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}
