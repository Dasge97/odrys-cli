package backend

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Service struct {
	Root string
}

const (
	openAIAuthIssuer              = "https://auth.openai.com"
	openAIDeviceVerificationURL   = "https://auth.openai.com/codex/device"
	openAIDeviceCallbackURL       = "https://auth.openai.com/deviceauth/callback"
	openAIOAuthExpirySafetyWindow = 5 * time.Minute
	openAICodexCompatibilityClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexLocalRedirectURL      = "http://localhost:1455/auth/callback"
)

type openAIDeviceCodeStartResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	Interval     string `json:"interval"`
}

type openAIDeviceCodePollResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

type openAITokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type openAIUserInfo struct {
	Email string `json:"email"`
}

func NewService(root string) *Service {
	return &Service{Root: root}
}

func (s *Service) EnsureScaffold() error {
	return ensureProjectScaffold(s.Root)
}

func (s *Service) LoadConfig() (Config, error) {
	if err := s.EnsureScaffold(); err != nil {
		return Config{}, err
	}
	return loadConfig(s.Root)
}

func (s *Service) SaveProvider(provider ProviderConfig) error {
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Provider = provider
	return saveConfig(s.Root, cfg)
}

func (s *Service) ConnectOpenAI(apiKey, model string) error {
	if strings.TrimSpace(apiKey) == "" {
		return errors.New("debes indicar una OPENAI_API_KEY")
	}
	if err := saveProviderAuth(s.Root, "openai", ProviderAuthInfo{
		Type:   "api",
		APIKey: apiKey,
	}); err != nil {
		return err
	}
	return s.SaveProvider(ProviderConfig{
		Name:  "openai",
		Model: fallbackProviderModel(model, "gpt-4.1-mini"),
	})
}

func (s *Service) OpenAIStatus() (OpenAIAuthStatus, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return OpenAIAuthStatus{}, err
	}
	status := OpenAIAuthStatus{
		Provider:            cfg.Provider,
		DeviceCodeAvailable: true,
		DeviceCodeMessage:   "ChatGPT Plus/Pro usa un flujo experimental compatible con Codex desde odrys-core",
	}
	if auth, ok, err := loadProviderAuth(s.Root, "openai"); err == nil && ok {
		if auth.Type == "api" && strings.TrimSpace(auth.APIKey) != "" {
			status.Connected = true
			status.Method = "api_key"
			return status, nil
		}
		if auth.Type == "oauth" && strings.TrimSpace(auth.AccessToken) != "" {
			status.Connected = true
			status.Method = "device_code"
			return status, nil
		}
	}
	if envFirst("OPENAI_API_KEY", "ODYRS_API_KEY") != "" {
		status.Connected = true
		status.Method = "env_api_key"
	}
	return status, nil
}

func (s *Service) StartOpenAIDeviceCode(model string) (OpenAIDeviceCodeSession, error) {
	pkceVerifier, pkceChallenge, err := generateOpenAIPKCE()
	if err != nil {
		return OpenAIDeviceCodeSession{}, err
	}
	state, err := generateOpenAIState()
	if err != nil {
		return OpenAIDeviceCodeSession{}, err
	}
	authURL := buildOpenAIAuthorizeURL(openAICodexLocalRedirectURL, pkceChallenge, state)
	return OpenAIDeviceCodeSession{
		ClientID:         openAICodexCompatibilityClientID,
		VerificationURL:  authURL,
		AuthorizationURL: authURL,
		IntervalSeconds:  2,
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
		Model:            fallbackProviderModel(model, "gpt-4.1-mini"),
		State:            state,
		CodeVerifier:     pkceVerifier,
		RedirectURL:      openAICodexLocalRedirectURL,
	}, nil
}

func (s *Service) PollOpenAIDeviceCode(session OpenAIDeviceCodeSession) (OpenAIDeviceCodePollResult, error) {
	expiry, err := parseRFC3339Time(session.ExpiresAt)
	if err == nil && time.Now().After(expiry) {
		return OpenAIDeviceCodePollResult{Status: "expired"}, nil
	}

	body := map[string]string{
		"device_auth_id": session.DeviceAuthID,
		"user_code":      session.UserCode,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, openAIAuthIssuer+"/api/accounts/deviceauth/token", bytes.NewReader(raw))
	if err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "odrys/dev")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return OpenAIDeviceCodePollResult{Status: "pending"}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OpenAIDeviceCodePollResult{}, fmt.Errorf("error consultando device code: %d %s", resp.StatusCode, string(respRaw))
	}

	var poll openAIDeviceCodePollResponse
	if err := json.Unmarshal(respRaw, &poll); err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}

	tokens, err := exchangeOpenAIAuthorizationCode(session.ClientID, poll.AuthorizationCode, poll.CodeVerifier)
	if err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}
	accountID := extractOpenAIAccountID(tokens)
	auth := ProviderAuthInfo{
		Type:         "oauth",
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UnixMilli(),
		AccountID:    accountID,
		ClientID:     session.ClientID,
	}
	if err := saveProviderAuth(s.Root, "openai", auth); err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}
	if err := s.SaveProvider(ProviderConfig{Name: "openai", Model: session.Model}); err != nil {
		return OpenAIDeviceCodePollResult{}, err
	}
	email := ""
	if info, infoErr := fetchOpenAIUser(tokens.AccessToken); infoErr == nil {
		email = info.Email
	}
	return OpenAIDeviceCodePollResult{Status: "success", Email: email}, nil
}

func (s *Service) Doctor() (DoctorPayload, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return DoctorPayload{}, err
	}
	openAIStatus, statusErr := s.OpenAIStatus()
	if statusErr != nil {
		openAIStatus = OpenAIAuthStatus{Provider: cfg.Provider}
	}
	return DoctorPayload{
		Root:       s.Root,
		ConfigPath: filepath.Join(s.Root, "odrys.config.json"),
		Provider:   cfg.Provider,
		OpenAIConfigured: openAIStatus.Connected,
		OpenAIAuth: openAIStatus,
		Workspace:  cfg.Workspace,
		Permission: cfg.Permission,
		Worker:     cfg.Worker,
		Session:    cfg.Session,
	}, nil
}

func (s *Service) Run(goal string) (RunResult, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return RunResult{}, err
	}
	return runWorker(s.Root, goal, cfg, RunOptions{})
}

func (s *Service) Chat(goal string) (ChatResult, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return ChatResult{}, err
	}
	return runDirectChat(s.Root, goal, cfg, RunOptions{})
}

func (s *Service) RunWithOptions(goal string, options RunOptions) (RunResult, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return RunResult{}, err
	}
	return runWorker(s.Root, goal, cfg, options)
}

func (s *Service) ChatWithOptions(goal string, options RunOptions) (ChatResult, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return ChatResult{}, err
	}
	return runDirectChat(s.Root, goal, cfg, options)
}

func (s *Service) Scan() (map[string]any, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}
	workspace, err := resolveWorkspace(s.Root, cfg.Workspace)
	if err != nil {
		return nil, err
	}
	return scanWorkspace(workspace, cfg.Permission)
}

func (s *Service) Sessions(limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 10
	}
	return listSessions(s.Root, limit)
}

func (s *Service) LoadSession(sessionID string) (Session, error) {
	return loadSession(s.Root, sessionID)
}

func (s *Service) LatestSession() (*SessionSummary, error) {
	return latestSession(s.Root)
}

func (s *Service) CreateSession(title string) (Session, error) {
	return loadOrCreateSession(s.Root, "", title)
}

func (s *Service) Write(relativePath, content string) (map[string]any, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}
	workspace, err := resolveWorkspace(s.Root, cfg.Workspace)
	if err != nil {
		return nil, err
	}
	return writeTextFile(workspace, relativePath, content, cfg.Permission, nil)
}

func (s *Service) ApplyPatch(patchText string) ([]map[string]any, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}
	workspace, err := resolveWorkspace(s.Root, cfg.Workspace)
	if err != nil {
		return nil, err
	}
	return applyPatch(workspace, patchText, cfg.Permission, nil)
}

func readPrompt(root, relative string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func readAgentPrompt(root, role string) (string, error) {
	agent := getAgent(role)
	return readPrompt(root, "agents/"+agent.Slug+"_prompt.md")
}

func readProjectState(root string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, "project", "project_state.md"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func updateProjectState(root, content string) error {
	return os.WriteFile(filepath.Join(root, "project", "project_state.md"), []byte(strings.TrimSpace(content)+"\n"), 0o644)
}

func writeRunLog(root string, payload any) (string, error) {
	dir := filepath.Join(root, "logs", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamp := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
	path := filepath.Join(dir, stamp+".json")
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func listRunLogs(root string, limit int) ([]string, error) {
	dir := filepath.Join(root, "logs", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var items []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			items = append(items, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(items)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func readRunLog(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildContext(root, goal, mode string, workspace *Workspace, permission map[string]map[string]string, session *Session, sessionCfg SessionConfig) ([]string, error) {
	files := []string{
		"project/spec.md",
		"project/architecture.md",
		"project/rules.md",
		"project/project_state.md",
	}
	if mode == "review" {
		files = append(files, "project/checklist.md")
	}
	var items []string
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return nil, err
		}
		items = append(items, fmt.Sprintf("## %s\n\n%s", file, strings.TrimSpace(string(content))))
	}
	if session != nil {
		if strings.TrimSpace(session.Summary) != "" {
			items = append(items, "## session/summary.md\n\n"+strings.TrimSpace(session.Summary))
		}
		if len(session.Messages) > 0 {
			limit := sessionCfg.ContextMessages
			if limit <= 0 {
				limit = 8
			}
			fragments := selectSessionMessages(goal, session.Messages, limit)
			if len(fragments) > 0 {
				items = append(items, "## session/relevant_messages.md\n\n"+strings.Join(fragments, "\n\n"))
			}
		}
		if len(session.RecentGoals) > 0 {
			runLimit := sessionCfg.ContextRuns
			if runLimit <= 0 {
				runLimit = 4
			}
			goals := selectRelevantStrings(goal, session.RecentGoals, runLimit)
			items = append(items, "## session/recent_goals.md\n\n- "+strings.Join(goals, "\n- "))
		}
		if len(session.RecentFiles) > 0 {
			fileLimit := sessionCfg.ContextFiles
			if fileLimit <= 0 {
				fileLimit = 6
			}
			files := session.RecentFiles
			if len(files) > fileLimit {
				files = files[:fileLimit]
			}
			items = append(items, "## session/recent_files.md\n\n- "+strings.Join(files, "\n- "))
		}
		if len(session.RecentNotes) > 0 {
			notes := selectRelevantStrings(goal, session.RecentNotes, 6)
			items = append(items, "## session/recent_notes.md\n\n- "+strings.Join(notes, "\n- "))
		}
	}
	runLogs, err := listRunLogs(root, 3)
	if err == nil && len(runLogs) > 0 {
		var fragments []string
		for _, path := range runLogs {
			payload, readErr := readRunLog(path)
			if readErr != nil {
				continue
			}
			goal, _ := payload["goal"].(string)
			result, _ := payload["result"].(map[string]any)
			status, _ := result["status"].(string)
			if strings.TrimSpace(goal) == "" {
				goal = "sin objetivo"
			}
			if strings.TrimSpace(status) == "" {
				status = "desconocido"
			}
			fragments = append(fragments, fmt.Sprintf("- %s\n  status: %s\n  goal: %s", filepath.Base(path), status, trimForContext(goal, 140)))
		}
		if len(fragments) > 0 {
			items = append(items, "## session/recent_runs.md\n\n"+strings.Join(fragments, "\n"))
		}
	}
	if workspace != nil {
		snapshot, err := scanWorkspace(*workspace, permission)
		if err != nil {
			return nil, err
		}
		raw, _ := json.MarshalIndent(snapshot, "", "  ")
		items = append(items, "## workspace/snapshot.json\n\n"+string(raw))
	}
	return items, nil
}

func selectSessionMessages(goal string, messages []SessionMessage, limit int) []string {
	type scoredMessage struct {
		index   int
		score   int
		content string
	}
	keywords := keywordSet(goal)
	var scored []scoredMessage
	for index, message := range messages {
		trimmed := strings.TrimSpace(message.Content)
		if trimmed == "" {
			continue
		}
		score := scoreText(trimmed, keywords)
		switch message.Role {
		case "user":
			score += 3
		case "assistant":
			score += 2
		default:
			score += 1
		}
		score += index / 3
		scored = append(scored, scoredMessage{
			index:   index,
			score:   score,
			content: fmt.Sprintf("[%s] %s", message.Role, compactMessageForContext(message.Role, trimmed)),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].index > scored[j].index
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].index < scored[j].index
	})
	out := make([]string, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.content)
	}
	return out
}

func selectRelevantStrings(goal string, items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	type scoredItem struct {
		index int
		score int
		text  string
	}
	keywords := keywordSet(goal)
	var scored []scoredItem
	for index, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		scored = append(scored, scoredItem{
			index: index,
			score: scoreText(trimmed, keywords) + len(items) - index,
			text:  trimForContext(trimmed, 160),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].index < scored[j].index
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]string, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.text)
	}
	return out
}

func keywordSet(text string) map[string]struct{} {
	normalized := strings.ToLower(text)
	fields := strings.FieldsFunc(normalized, func(r rune) bool {
		return !(r == '_' || r == '-' || r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := map[string]struct{}{}
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		out[field] = struct{}{}
	}
	return out
}

func scoreText(text string, keywords map[string]struct{}) int {
	normalized := strings.ToLower(text)
	score := 0
	for keyword := range keywords {
		if strings.Contains(normalized, keyword) {
			score += 4
		}
	}
	return score
}

func compactMessageForContext(role, text string) string {
	switch role {
	case "assistant":
		return trimForContext(oneLineText(text), 220)
	case "system":
		return trimForContext(oneLineText(text), 120)
	default:
		return trimForContext(text, 180)
	}
}

func trimForContext(text string, maxRunes int) string {
	trimmed := strings.TrimSpace(text)
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return string(runes[:maxRunes]) + "..."
}

func oneLineText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func formatContext(context []string) string {
	return strings.TrimSpace(strings.Join(context, "\n\n"))
}

func shouldUsePlanner(goal string) bool {
	normalized := strings.TrimSpace(goal)
	if len(normalized) > 120 {
		return true
	}
	lower := strings.ToLower(normalized)
	for _, token := range []string{" y ", " luego ", " después ", " after ", ",", ";"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func plannerTaskList(plan PlannerOutput) []string {
	var tasks []string
	for _, phase := range plan.Phases {
		for _, task := range phase.Tasks {
			if task.Description != "" {
				tasks = append(tasks, task.Description)
			} else if task.ID != "" {
				tasks = append(tasks, task.ID)
			}
		}
	}
	if len(tasks) == 0 {
		return []string{plan.Summary}
	}
	return tasks
}

func reviewHasBlockingErrors(review ReviewerOutput) bool {
	for _, item := range review.Errors {
		if item.Severity == "critical" || item.Severity == "major" {
			return true
		}
	}
	return false
}

type providerInput struct {
	Agent      string
	AgentName  string
	SystemPrompt string
	Context    string
	Goal       string
	Task       string
	Feedback   any
}

type provider interface {
	Run(context.Context, providerInput) (json.RawMessage, error)
	Chat(context.Context, providerInput) (string, error)
}

type mockProvider struct {
	config ProviderConfig
}

func buildPlan(goal string) PlannerOutput {
	return PlannerOutput{
		Status:  "planned",
		Summary: "Plan base para: " + goal,
		Phases: []PlannerPhase{{
			Name: "base",
			Goal: "establecer la estructura inicial",
			Tasks: []PlannerTask{{
				ID:          "task-1",
				Description: goal,
				DoneWhen:    []string{"la tarea principal tiene una primera implementacion o propuesta concreta"},
				DependsOn:   []string{},
			}},
		}},
		NextAction: "execute",
	}
}

func buildOperations(task string) []ExecutorOperation {
	createMatch := regexp.MustCompile(`(?i)crear archivo\s+([^\s]+)\s+con contenido\s+(.+)`).FindStringSubmatch(task)
	if len(createMatch) > 0 {
		return []ExecutorOperation{{Tool: "write", Path: createMatch[1], Content: createMatch[2]}}
	}
	writeMatch := regexp.MustCompile(`(?i)escribir en\s+([^\s]+)\s*:\s*(.+)`).FindStringSubmatch(task)
	if len(writeMatch) > 0 {
		return []ExecutorOperation{{Tool: "write", Path: writeMatch[1], Content: writeMatch[2]}}
	}
	return nil
}

func (m mockProvider) Run(_ context.Context, input providerInput) (json.RawMessage, error) {
	switch input.Agent {
	case "planner":
		return json.Marshal(buildPlan(input.Goal))
	case "executor":
		retry := "Primera ejecucion."
		if input.Feedback != nil {
			retry = "Corrige errores previos y reintenta."
		}
		return json.Marshal(ExecutorOutput{
			Status:  "success",
			Summary: input.AgentName + " preparado para trabajar la tarea: " + input.Task + ". " + retry,
			Changes: []string{
				"Se ha producido una propuesta estructurada de implementacion",
				"Se ha generado una salida trazable para revision",
			},
			Assumptions:   []string{"La v1 prioriza arquitectura y control de flujo sobre automatizacion total"},
			OpenQuestions: []string{},
			Operations:    buildOperations(input.Task),
			NextAction:    "review",
		})
	case "reviewer":
		return json.Marshal(ReviewerOutput{
			Status:  "approved",
			Summary: input.AgentName + ` considera que la propuesta para la tarea "` + input.Task + `" cumple el contrato minimo de la v1`,
			Errors:  []ReviewError{},
			VerifiedAgainst: []string{
				"project/spec.md",
				"project/architecture.md",
				"project/rules.md",
				"project/checklist.md",
			},
			NextAction: "finish",
		})
	case "summarizer":
		return json.Marshal(SummarizerOutput{
			Status: "updated",
			ProjectState: "# Project State\n\nUltima actualizacion automatica.\n\n- Objetivo reciente: " + input.Goal + "\n- Estado: se ejecuto el flujo completo del worker\n- Proximo paso natural: continuar desde la sesion persistente mas reciente\n",
			NextAction: "store_state",
		})
	default:
		return nil, fmt.Errorf("agente no soportado por mock: %s", input.Agent)
	}
}

func (m mockProvider) Chat(_ context.Context, input providerInput) (string, error) {
	if strings.TrimSpace(input.Task) == "" {
		return "Estoy listo para ayudarte.", nil
	}
	return "Recibido: " + strings.TrimSpace(input.Task), nil
}

type openAICompatibleProvider struct {
	config  ProviderConfig
	baseURL string
	apiKey  string
}

type openAIOAuthProvider struct {
	root   string
	config ProviderConfig
	auth   ProviderAuthInfo
}

func (p openAICompatibleProvider) Run(ctx context.Context, input providerInput) (json.RawMessage, error) {
	if p.apiKey == "" {
		switch p.config.Name {
		case "openai":
			return nil, errors.New("falta OPENAI_API_KEY para usar openai")
		case "minimax":
			return nil, errors.New("falta MINIMAX_API_KEY para usar minimax")
		default:
			return nil, errors.New("falta ODYRS_API_KEY u OPENAI_API_KEY para usar openai-compatible")
		}
	}
	body := map[string]any{
		"model":       p.config.Model,
		"temperature": 0.1,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": input.SystemPrompt},
			{"role": "user", "content": fmt.Sprintf("Objetivo:\n%s\n\nTarea actual:\n%s\n\nContexto:\n%s\n\nFeedback previo:\n%v\n\nResponde solo con JSON valido segun el contrato del agente %s.", input.Goal, input.Task, input.Context, input.Feedback, input.Agent)},
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respRaw, err := ioReadAll(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error del proveedor: %d %s", resp.StatusCode, string(respRaw))
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respRaw, &payload); err != nil {
		return nil, err
	}
	text, err := extractProviderText(payload.Choices)
	if err != nil {
		return nil, err
	}
	return parseProviderJSON(text)
}

func (p openAICompatibleProvider) Chat(ctx context.Context, input providerInput) (string, error) {
	if p.apiKey == "" {
		switch p.config.Name {
		case "openai":
			return "", errors.New("falta OPENAI_API_KEY para usar openai")
		case "minimax":
			return "", errors.New("falta MINIMAX_API_KEY para usar minimax")
		default:
			return "", errors.New("falta ODYRS_API_KEY u OPENAI_API_KEY para usar openai-compatible")
		}
	}
	body := map[string]any{
		"model":       p.config.Model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": input.SystemPrompt},
			{"role": "user", "content": fmt.Sprintf("Peticion actual:\n%s\n\nContexto:\n%s\n\nResponde en texto claro y directo.", input.Task, input.Context)},
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respRaw, err := ioReadAll(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("error del proveedor: %d %s", resp.StatusCode, string(respRaw))
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respRaw, &payload); err != nil {
		return "", err
	}
	return extractProviderText(payload.Choices)
}

func (p *openAIOAuthProvider) ensureAccessToken() (ProviderAuthInfo, error) {
	auth := p.auth
	if auth.ClientID == "" {
		return auth, fmt.Errorf("faltan metadatos del cliente OAuth para OpenAI")
	}
	if auth.AccessToken != "" && auth.ExpiresAt > time.Now().Add(openAIOAuthExpirySafetyWindow).UnixMilli() {
		return auth, nil
	}
	tokens, err := refreshOpenAIAccessToken(auth.ClientID, auth.RefreshToken)
	if err != nil {
		return auth, err
	}
	auth.AccessToken = tokens.AccessToken
	auth.RefreshToken = tokens.RefreshToken
	auth.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UnixMilli()
	if auth.AccountID == "" {
		auth.AccountID = extractOpenAIAccountID(tokens)
	}
	if err := saveProviderAuth(p.root, "openai", auth); err != nil {
		return auth, err
	}
	p.auth = auth
	return auth, nil
}

func (p *openAIOAuthProvider) Run(ctx context.Context, input providerInput) (json.RawMessage, error) {
	auth, err := p.ensureAccessToken()
	if err != nil {
		return nil, err
	}
	userInput := fmt.Sprintf("Objetivo:\n%s\n\nTarea actual:\n%s\n\nContexto:\n%s\n\nFeedback previo:\n%v\n\nResponde solo con JSON valido segun el contrato del agente %s.", input.Goal, input.Task, input.Context, input.Feedback, input.Agent)
	instructions := strings.TrimSpace(input.SystemPrompt) + "\n\nREGLA CRITICA:\n- Debes devolver exclusivamente un unico objeto JSON valido.\n- No uses markdown.\n- No uses bloques ```.\n- No expliques nada fuera del JSON.\n- La primera letra de tu respuesta debe ser { y la ultima debe ser }."
	body := map[string]any{
		"model":        p.config.Model,
		"store":        false,
		"stream":       true,
		"instructions": instructions,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{"type": "input_text", "text": userInput},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("originator", "odrys")
	req.Header.Set("User-Agent", "odrys/dev")
	if auth.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error de OpenAI ChatGPT auth: %d %s", resp.StatusCode, string(respRaw))
	}
	text, err := extractResponsesOutputTextFromStream(respRaw)
	if err != nil {
		return nil, err
	}
	parsed, err := parseProviderJSON(text)
	if err != nil {
		return nil, fmt.Errorf("%w\nmuestra del modelo: %s", err, trimForContext(text, 500))
	}
	return parsed, nil
}

func (p *openAIOAuthProvider) Chat(ctx context.Context, input providerInput) (string, error) {
	auth, err := p.ensureAccessToken()
	if err != nil {
		return "", err
	}
	instructions := strings.TrimSpace(input.SystemPrompt) + "\n\nResponde en texto libre, claro y directo. No uses JSON salvo que el usuario lo pida expresamente."
	body := map[string]any{
		"model":        p.config.Model,
		"store":        false,
		"stream":       true,
		"instructions": instructions,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{"type": "input_text", "text": fmt.Sprintf("Peticion actual:\n%s\n\nContexto:\n%s", input.Task, input.Context)},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("originator", "odrys")
	req.Header.Set("User-Agent", "odrys/dev")
	if auth.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("error de OpenAI ChatGPT auth: %d %s", resp.StatusCode, string(respRaw))
	}
	return extractResponsesOutputTextFromStream(respRaw)
}

func ioReadAll(resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}

func extractProviderText(choices []struct {
	Message struct {
		Content any `json:"content"`
	} `json:"message"`
}) (string, error) {
	if len(choices) == 0 {
		return "", errors.New("la respuesta del proveedor no contiene opciones")
	}
	switch value := choices[0].Message.Content.(type) {
	case string:
		return value, nil
	default:
		raw, _ := json.Marshal(value)
		return string(raw), nil
	}
}

func parseProviderJSON(text string) (json.RawMessage, error) {
	var probe any
	if err := json.Unmarshal([]byte(text), &probe); err == nil {
		return json.RawMessage(text), nil
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	var first any
	if err := decoder.Decode(&first); err == nil {
		raw, marshalErr := json.Marshal(first)
		if marshalErr == nil {
			return json.RawMessage(raw), nil
		}
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		slice := text[start : end+1]
		if err := json.Unmarshal([]byte(slice), &probe); err == nil {
			return json.RawMessage(slice), nil
		}
	}
	return nil, errors.New("no se pudo parsear JSON de la respuesta del modelo")
}

func generateOpenAIPKCE() (string, string, error) {
	verifier, err := generateRandomURLString(43)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func generateOpenAIState() (string, error) {
	return generateRandomURLString(32)
}

func generateRandomURLString(length int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func buildOpenAIAuthorizeURL(redirectURI, codeChallenge, state string) string {
	query := strings.Join([]string{
		"response_type=code",
		"client_id=" + urlQueryEscape(openAICodexCompatibilityClientID),
		"redirect_uri=" + urlQueryEscape(redirectURI),
		"scope=" + urlQueryEscape("openid profile email offline_access"),
		"code_challenge=" + urlQueryEscape(codeChallenge),
		"code_challenge_method=S256",
		"id_token_add_organizations=true",
		"codex_cli_simplified_flow=true",
		"state=" + urlQueryEscape(state),
		"originator=odrys",
	}, "&")
	return openAIAuthIssuer + "/oauth/authorize?" + query
}

func exchangeOpenAIAuthorizationCode(clientID, authorizationCode, codeVerifier string) (openAITokenResponse, error) {
	form := fmt.Sprintf(
		"grant_type=authorization_code&code=%s&redirect_uri=%s&client_id=%s&code_verifier=%s",
		urlQueryEscape(authorizationCode),
		urlQueryEscape(openAIDeviceCallbackURL),
		urlQueryEscape(clientID),
		urlQueryEscape(codeVerifier),
	)
	req, err := http.NewRequest(http.MethodPost, openAIAuthIssuer+"/oauth/token", strings.NewReader(form))
	if err != nil {
		return openAITokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAITokenResponse{}, err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return openAITokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAITokenResponse{}, fmt.Errorf("token exchange failed: %d %s", resp.StatusCode, string(respRaw))
	}
	var tokens openAITokenResponse
	if err := json.Unmarshal(respRaw, &tokens); err != nil {
		return openAITokenResponse{}, err
	}
	return tokens, nil
}

func ExchangeOpenAIAuthorizationCodeForOAuth(codeVerifier, redirectURI, authorizationCode string) (openAITokenResponse, error) {
	form := fmt.Sprintf(
		"grant_type=authorization_code&code=%s&redirect_uri=%s&client_id=%s&code_verifier=%s",
		urlQueryEscape(authorizationCode),
		urlQueryEscape(redirectURI),
		urlQueryEscape(openAICodexCompatibilityClientID),
		urlQueryEscape(codeVerifier),
	)
	req, err := http.NewRequest(http.MethodPost, openAIAuthIssuer+"/oauth/token", strings.NewReader(form))
	if err != nil {
		return openAITokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAITokenResponse{}, err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return openAITokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAITokenResponse{}, fmt.Errorf("token exchange failed: %d %s", resp.StatusCode, string(respRaw))
	}
	var tokens openAITokenResponse
	if err := json.Unmarshal(respRaw, &tokens); err != nil {
		return openAITokenResponse{}, err
	}
	return tokens, nil
}

func refreshOpenAIAccessToken(clientID, refreshToken string) (openAITokenResponse, error) {
	form := fmt.Sprintf(
		"grant_type=refresh_token&refresh_token=%s&client_id=%s",
		urlQueryEscape(refreshToken),
		urlQueryEscape(clientID),
	)
	req, err := http.NewRequest(http.MethodPost, openAIAuthIssuer+"/oauth/token", strings.NewReader(form))
	if err != nil {
		return openAITokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAITokenResponse{}, err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return openAITokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAITokenResponse{}, fmt.Errorf("token refresh failed: %d %s", resp.StatusCode, string(respRaw))
	}
	var tokens openAITokenResponse
	if err := json.Unmarshal(respRaw, &tokens); err != nil {
		return openAITokenResponse{}, err
	}
	return tokens, nil
}

func fetchOpenAIUser(accessToken string) (openAIUserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/user", nil)
	if err != nil {
		return openAIUserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "odrys/dev")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAIUserInfo{}, err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return openAIUserInfo{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAIUserInfo{}, fmt.Errorf("error obteniendo usuario openai: %d %s", resp.StatusCode, string(respRaw))
	}
	var user openAIUserInfo
	if err := json.Unmarshal(respRaw, &user); err != nil {
		return openAIUserInfo{}, err
	}
	return user, nil
}

func FetchOpenAIUserForOAuth(accessToken string) (openAIUserInfo, error) {
	return fetchOpenAIUser(accessToken)
}

func ExtractOpenAIAccountIDFromTokens(tokens openAITokenResponse) string {
	return extractOpenAIAccountID(tokens)
}

func OpenAICodexCompatibilityClientID() string {
	return openAICodexCompatibilityClientID
}

func extractResponsesOutputText(raw []byte) (string, error) {
	var payload struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	var parts []string
	for _, output := range payload.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
	}
	if len(parts) == 0 {
		return "", errors.New("la respuesta de OpenAI no contiene output_text")
	}
	return strings.Join(parts, "\n"), nil
}

func extractResponsesOutputTextFromStream(raw []byte) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", errors.New("respuesta vacia de OpenAI")
	}
	if strings.HasPrefix(text, "{") {
		return extractResponsesOutputText(raw)
	}

	var parts []string
	sawDelta := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" {
			continue
		}
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Text  string `json:"text"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			if strings.TrimSpace(event.Delta) != "" || event.Delta != "" {
				sawDelta = true
				parts = append(parts, event.Delta)
			}
		case "response.output_text.done":
			if !sawDelta && (strings.TrimSpace(event.Text) != "" || event.Text != "") {
				parts = append(parts, event.Text)
			}
		}
	}
	if len(parts) == 0 {
		return "", errors.New("no se pudo reconstruir texto desde el stream SSE de OpenAI")
	}
	return strings.Join(parts, ""), nil
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"!", "%21",
		"\"", "%22",
		"#", "%23",
		"$", "%24",
		"&", "%26",
		"'", "%27",
		"(", "%28",
		")", "%29",
		"*", "%2A",
		"+", "%2B",
		",", "%2C",
		"/", "%2F",
		":", "%3A",
		";", "%3B",
		"<", "%3C",
		"=", "%3D",
		">", "%3E",
		"?", "%3F",
		"@", "%40",
		"[", "%5B",
		"\\", "%5C",
		"]", "%5D",
		"^", "%5E",
		"`", "%60",
		"{", "%7B",
		"|", "%7C",
		"}", "%7D",
	)
	return replacer.Replace(value)
}

func createProvider(root string, cfg ProviderConfig) provider {
	switch cfg.Name {
	case "openai":
		if auth, ok, err := loadProviderAuth(root, "openai"); err == nil && ok {
			if auth.Type == "oauth" && strings.TrimSpace(auth.AccessToken) != "" {
				return &openAIOAuthProvider{
					root:   root,
					config: cfg,
					auth:   auth,
				}
			}
			if auth.Type == "api" && strings.TrimSpace(auth.APIKey) != "" {
				return openAICompatibleProvider{
					config:  cfg,
					baseURL: "https://api.openai.com/v1",
					apiKey:  auth.APIKey,
				}
			}
		}
		return openAICompatibleProvider{
			config:  cfg,
			baseURL: "https://api.openai.com/v1",
			apiKey:  envFirst("OPENAI_API_KEY", "ODYRS_API_KEY"),
		}
	case "minimax":
		return openAICompatibleProvider{
			config:  cfg,
			baseURL: "https://api.minimax.io/v1",
			apiKey:  envFirst("MINIMAX_API_KEY", "OPENAI_API_KEY", "ODYRS_API_KEY"),
		}
	case "openai-compatible":
		return openAICompatibleProvider{
			config:  cfg,
			baseURL: envOr("ODYRS_BASE_URL", "https://api.openai.com/v1"),
			apiKey:  envFirst("ODYRS_API_KEY", "OPENAI_API_KEY"),
		}
	default:
		return mockProvider{config: cfg}
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func fallbackProviderModel(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func validateAgentOutput(agent string, raw json.RawMessage) (json.RawMessage, error) {
	if normalized, err := normalizeAgentOutput(agent, raw); err == nil {
		raw = normalized
	}
	agentName := getAgent(agent).Name
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	required := map[string][]string{
		"planner":    {"status", "summary", "phases", "next_action"},
		"executor":   {"status", "summary", "changes", "assumptions", "open_questions", "operations", "next_action"},
		"reviewer":   {"status", "summary", "errors", "verified_against", "next_action"},
		"summarizer": {"status", "project_state", "next_action"},
	}
	for _, key := range required[agent] {
		if _, ok := probe[key]; !ok {
			return nil, fmt.Errorf("salida de %s invalida: falta el campo %q", agentName, key)
		}
	}
	return raw, nil
}

func normalizeAgentOutput(agent string, raw json.RawMessage) (json.RawMessage, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	if _, ok := probe["status"]; ok {
		return raw, nil
	}
	switch agent {
	case "executor":
		if _, ok := probe["summary"]; ok {
			probe["status"] = "completed"
		}
		normalizeStringArrayField(probe, "changes")
		normalizeStringArrayField(probe, "assumptions")
		normalizeStringArrayField(probe, "open_questions")
	case "reviewer":
		if _, ok := probe["summary"]; ok {
			probe["status"] = "approved"
		}
		normalizeReviewerErrorsField(probe, "errors")
		normalizeStringArrayField(probe, "verified_against")
	case "summarizer":
		if _, ok := probe["project_state"]; ok {
			probe["status"] = "updated"
		}
	case "planner":
		normalizePlannerPhasesField(probe, "phases")
	}
	normalized, err := json.Marshal(probe)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeStringArrayField(probe map[string]any, key string) {
	value, ok := probe[key]
	if !ok || value == nil {
		probe[key] = []any{}
		return
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			probe[key] = []any{}
		} else {
			probe[key] = []any{trimmed}
		}
	}
}

func normalizeReviewerErrorsField(probe map[string]any, key string) {
	value, ok := probe[key]
	if !ok || value == nil {
		probe[key] = []any{}
		return
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			probe[key] = []any{}
			return
		}
		probe[key] = []any{
			map[string]any{
				"severity": "medium",
				"message":  trimmed,
			},
		}
	case []any:
		var out []any
		for _, item := range typed {
			switch current := item.(type) {
			case string:
				trimmed := strings.TrimSpace(current)
				if trimmed == "" {
					continue
				}
				out = append(out, map[string]any{
					"severity": "medium",
					"message":  trimmed,
				})
			case map[string]any:
				if _, ok := current["severity"]; !ok {
					current["severity"] = "medium"
				}
				out = append(out, current)
			}
		}
		probe[key] = out
	}
}

func normalizePlannerPhasesField(probe map[string]any, key string) {
	value, ok := probe[key]
	if !ok || value == nil {
		probe[key] = []any{}
		return
	}
	if _, ok := value.([]any); ok {
		return
	}
	probe[key] = []any{}
}

func normalizeExecutorOutputForTask(raw json.RawMessage, task string) (json.RawMessage, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}

	for _, key := range []string{"changes", "assumptions", "open_questions"} {
		switch value := probe[key].(type) {
		case nil:
			probe[key] = []any{}
		case string:
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				probe[key] = []any{}
			} else {
				probe[key] = []any{trimmed}
			}
		}
	}

	probe["operations"] = normalizeExecutorOperationsValue(probe["operations"], task)

	normalized, err := json.Marshal(probe)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeExecutorOperationsValue(value any, task string) any {
	fallback := executorOperationsToAny(buildOperations(task))
	switch typed := value.(type) {
	case nil:
		if len(fallback) > 0 {
			return fallback
		}
		return []any{}
	case string:
		if operations := inferExecutorOperationsFromString(typed, task); len(operations) > 0 {
			return executorOperationsToAny(operations)
		}
		if len(fallback) > 0 {
			return fallback
		}
		return []any{}
	case []any:
		var normalized []any
		for _, item := range typed {
			switch current := item.(type) {
			case map[string]any:
				normalized = append(normalized, current)
			case string:
				normalized = append(normalized, executorOperationsToAny(inferExecutorOperationsFromString(current, task))...)
			}
		}
		if len(normalized) == 0 {
			if len(fallback) > 0 {
				return fallback
			}
			return []any{}
		}
		return normalized
	default:
		if len(fallback) > 0 {
			return fallback
		}
		return []any{}
	}
}

func inferExecutorOperationsFromString(value, task string) []ExecutorOperation {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "apply_patch") || strings.Contains(lower, "*** begin patch") {
		return []ExecutorOperation{{Tool: "apply_patch", Patch: trimmed}}
	}
	if operations := buildOperations(trimmed); len(operations) > 0 {
		return operations
	}
	if operations := buildOperations(task); len(operations) > 0 {
		return operations
	}
	return nil
}

func executorOperationsToAny(operations []ExecutorOperation) []any {
	out := make([]any, 0, len(operations))
	for _, operation := range operations {
		item := map[string]any{
			"tool": operation.Tool,
		}
		if strings.TrimSpace(operation.Path) != "" {
			item["path"] = operation.Path
		}
		if strings.TrimSpace(operation.Content) != "" {
			item["content"] = operation.Content
		}
		if strings.TrimSpace(operation.Patch) != "" {
			item["patch"] = operation.Patch
		}
		out = append(out, item)
	}
	return out
}

func callAgent(ctx context.Context, root string, provider provider, name, goal, task, contextText string, feedback any) (json.RawMessage, error) {
	agent := getAgent(name)
	base, err := readPrompt(root, "system/base_system_prompt.md")
	if err != nil {
		return nil, err
	}
	agentPrompt, err := readAgentPrompt(root, name)
	if err != nil {
		return nil, err
	}
	systemPrompt := strings.TrimSpace(base) + "\n\n" + strings.TrimSpace(agentPrompt)
	if name == "executor" {
		systemPrompt += "\n\nCONTRATO JSON OBLIGATORIO DEL EXECUTOR:\n" +
			"{\"status\":\"completed\",\"summary\":\"string\",\"changes\":[],\"assumptions\":[],\"open_questions\":[],\"operations\":[],\"next_action\":\"string\"}\n" +
			"Debes incluir exactamente esas claves siempre, aunque algunas esten vacias."
	}
	if name == "reviewer" {
		systemPrompt += "\n\nCONTRATO JSON OBLIGATORIO DEL REVIEWER:\n" +
			"{\"status\":\"approved\",\"summary\":\"string\",\"errors\":[],\"verified_against\":[],\"next_action\":\"string\"}\n" +
			"Debes incluir exactamente esas claves siempre, aunque algunas esten vacias."
	}
	if name == "summarizer" {
		systemPrompt += "\n\nCONTRATO JSON OBLIGATORIO DE CAJA:\n" +
			"{\"status\":\"updated\",\"project_state\":\"string\",\"next_action\":\"string\"}\n" +
			"Debes incluir exactamente esas claves siempre."
	}
	response, err := provider.Run(ctx, providerInput{
		Agent:        name,
		AgentName:    agent.Name,
		SystemPrompt: systemPrompt,
		Context:      contextText,
		Goal:         goal,
		Task:         task,
		Feedback:     feedback,
	})
	if err != nil {
		return nil, err
	}
	return validateAgentOutput(name, response)
}

func chatWithAgent(ctx context.Context, root string, provider provider, name, goal, task, contextText string) (string, error) {
	agent := getAgent(name)
	base, err := readPrompt(root, "system/base_system_prompt.md")
	if err != nil {
		return "", err
	}
	agentPrompt, err := readAgentPrompt(root, name)
	if err != nil {
		return "", err
	}
	systemPrompt := strings.TrimSpace(base) + "\n\n" + strings.TrimSpace(agentPrompt)
	return provider.Chat(ctx, providerInput{
		Agent:        name,
		AgentName:    agent.Name,
		SystemPrompt: systemPrompt,
		Context:      contextText,
		Goal:         goal,
		Task:         task,
	})
}

func runDirectChat(root, goal string, config Config, options RunOptions) (ChatResult, error) {
	provider := createProvider(root, config.Provider)
	workspace, err := resolveWorkspace(root, config.Workspace)
	if err != nil {
		return ChatResult{}, err
	}
	var session *Session
	if strings.TrimSpace(options.SessionID) != "" {
		loaded, loadErr := loadOrCreateSession(root, options.SessionID, goal)
		if loadErr != nil {
			return ChatResult{}, loadErr
		}
		session = &loaded
	}
	contextList, err := buildContext(root, goal, "execute", &workspace, config.Permission, session, config.Session)
	if err != nil {
		return ChatResult{}, err
	}
	reply, err := chatWithAgent(context.Background(), root, provider, "general", goal, goal, formatContext(contextList))
	if err != nil {
		return ChatResult{}, err
	}
	var summary *SummarizerOutput
	if shouldUpdateProjectStateFromDirectChat(goal, reply) {
		currentState, stateErr := readProjectState(root)
		if stateErr != nil {
			return ChatResult{}, stateErr
		}
		rawSummary, summaryErr := callAgent(
			context.Background(),
			root,
			provider,
			"summarizer",
			goal,
			"actualizar estado del proyecto tras una conversacion directa",
			formatContext(contextList)+"\n\n## current_project_state\n\n"+currentState,
			map[string]any{
				"mode":  "chat",
				"goal":  goal,
				"reply": reply,
			},
		)
		if summaryErr != nil {
			return ChatResult{}, summaryErr
		}
		var out SummarizerOutput
		if err := json.Unmarshal(rawSummary, &out); err != nil {
			return ChatResult{}, err
		}
		if err := updateProjectState(root, out.ProjectState); err != nil {
			return ChatResult{}, err
		}
		summary = &out
	}
	persistedSession, err := appendSessionMessage(root, options.SessionID, "user", goal)
	if err != nil {
		return ChatResult{}, err
	}
	if _, err := appendSessionAssistantMemory(root, persistedSession.ID, reply, trimForContext(reply, 180), nil); err != nil {
		return ChatResult{}, err
	}
	if _, err := appendSessionMessage(root, persistedSession.ID, "system", "Chat completado para: "+goal); err != nil {
		return ChatResult{}, err
	}
	if summary != nil {
		if err := updateSessionSummary(root, persistedSession.ID, summary.ProjectState); err != nil {
			return ChatResult{}, err
		}
	}
	result := ChatResult{
		Status:    "completed",
		Goal:      goal,
		Reply:     strings.TrimSpace(reply),
		SessionID: persistedSession.ID,
	}
	logPath, err := writeRunLog(root, map[string]any{
		"started_at": time.Now().UTC().Format(time.RFC3339),
		"mode":       "chat",
		"goal":       goal,
		"result":     result,
	})
	if err != nil {
		return ChatResult{}, err
	}
	result.LogPath = logPath
	return result, nil
}

func shouldUpdateProjectStateFromDirectChat(goal, reply string) bool {
	joined := strings.ToLower(strings.TrimSpace(goal + "\n" + reply))
	if joined == "" {
		return false
	}
	for _, token := range []string{
		"actualiza el state",
		"actualiza project_state",
		"actualiza el estado",
		"guarda esto en el estado",
		"recuerda esto",
		"decidimos",
		"a partir de ahora",
		"el siguiente paso",
		"los siguientes pasos",
		"queda decidido",
		"nuevo criterio",
		"nueva regla",
		"cambia la arquitectura",
		"actualiza la arquitectura",
		"actualiza la spec",
		"actualiza la especificacion",
	} {
		if strings.Contains(joined, token) {
			return true
		}
	}
	return false
}

func runWorker(root, goal string, config Config, options RunOptions) (RunResult, error) {
	provider := createProvider(root, config.Provider)
	workspace, err := resolveWorkspace(root, config.Workspace)
	if err != nil {
		return RunResult{}, err
	}
	var session *Session
	if strings.TrimSpace(options.SessionID) != "" {
		loaded, loadErr := loadOrCreateSession(root, options.SessionID, goal)
		if loadErr != nil {
			return RunResult{}, loadErr
		}
		session = &loaded
	}
	executeContextList, err := buildContext(root, goal, "execute", &workspace, config.Permission, session, config.Session)
	if err != nil {
		return RunResult{}, err
	}
	reviewContextList, err := buildContext(root, goal, "review", &workspace, config.Permission, session, config.Session)
	if err != nil {
		return RunResult{}, err
	}
	executeContext := formatContext(executeContextList)
	reviewContext := formatContext(reviewContextList)
	var plan PlannerOutput
	mustPlan := config.Worker.Planner.Auto && shouldUsePlanner(goal)
	if mustPlan {
		raw, err := callAgent(context.Background(), root, provider, "planner", goal, goal, executeContext, nil)
		if err != nil {
			return RunResult{}, err
		}
		if err := json.Unmarshal(raw, &plan); err != nil {
			return RunResult{}, err
		}
	}

	tasks := []string{goal}
	if mustPlan {
		tasks = plannerTaskList(plan)
	}

	var taskResults []RunTask
	for _, task := range tasks {
		var executorOutput ExecutorOutput
		var reviewerOutput ReviewerOutput
		var feedback any
		var applied []AppliedOperation
		for attempt := 0; attempt <= config.Worker.MaxReviewLoops; attempt++ {
			rawExec, err := callAgent(context.Background(), root, provider, "executor", goal, task, executeContext, feedback)
			if err != nil {
				return RunResult{}, err
			}
			rawExec, err = normalizeExecutorOutputForTask(rawExec, task)
			if err != nil {
				return RunResult{}, err
			}
			if err := json.Unmarshal(rawExec, &executorOutput); err != nil {
				return RunResult{}, err
			}
			applied, err = applyExecutorOperations(workspace, config.Permission, executorOutput.Operations, options.Overrides)
			if err != nil {
				return RunResult{}, err
			}
			rawReview, err := callAgent(context.Background(), root, provider, "reviewer", goal, task, reviewContext, map[string]any{
				"executor":           executorOutput,
				"applied_operations": applied,
			})
			if err != nil {
				return RunResult{}, err
			}
			if err := json.Unmarshal(rawReview, &reviewerOutput); err != nil {
				return RunResult{}, err
			}
			if reviewerOutput.Status == "approved" && !reviewHasBlockingErrors(reviewerOutput) {
				break
			}
			feedback = map[string]any{
				"executor":           executorOutput,
				"reviewer":           reviewerOutput,
				"applied_operations": applied,
			}
		}
		taskResults = append(taskResults, RunTask{
			Task:              task,
			Executor:          executorOutput,
			AppliedOperations: applied,
			Reviewer:          reviewerOutput,
		})
	}

	var summary *SummarizerOutput
	if config.Worker.SummarizeOnSuccess {
		currentState, err := readProjectState(root)
		if err != nil {
			return RunResult{}, err
		}
		rawSummary, err := callAgent(context.Background(), root, provider, "summarizer", goal, "actualizar estado del proyecto", executeContext+"\n\n## current_project_state\n\n"+currentState, taskResults)
		if err != nil {
			return RunResult{}, err
		}
		var out SummarizerOutput
		if err := json.Unmarshal(rawSummary, &out); err != nil {
			return RunResult{}, err
		}
		if err := updateProjectState(root, out.ProjectState); err != nil {
			return RunResult{}, err
		}
		summary = &out
	}

	persistedSession, err := appendSessionMessage(root, options.SessionID, "user", goal)
	if err != nil {
		return RunResult{}, err
	}
	for _, taskResult := range taskResults {
		if _, err := appendSessionAssistantMemory(
			root,
			persistedSession.ID,
			renderTaskSessionMessage(taskResult.Task, taskResult.Executor, taskResult.Reviewer, taskResult.AppliedOperations),
			renderTaskNote(taskResult.Task, taskResult.Executor, taskResult.Reviewer),
			operationPaths(taskResult.AppliedOperations),
		); err != nil {
			return RunResult{}, err
		}
	}
	if _, err := appendSessionMessage(root, persistedSession.ID, "system", "Run completado para: "+goal); err != nil {
		return RunResult{}, err
	}
	if summary != nil {
		if err := updateSessionSummary(root, persistedSession.ID, summary.ProjectState); err != nil {
			return RunResult{}, err
		}
	}

	result := RunResult{
		Status:  "completed",
		Goal:    goal,
		Planned: mustPlan,
		Tasks:   taskResults,
		Summary: summary,
		SessionID: persistedSession.ID,
	}
	logPath, err := writeRunLog(root, map[string]any{
		"started_at": time.Now().UTC().Format(time.RFC3339),
		"goal":       goal,
		"result":     result,
	})
	if err != nil {
		return RunResult{}, err
	}
	result.LogPath = logPath
	return result, nil
}

func renderTaskSessionMessage(task string, executor ExecutorOutput, reviewer ReviewerOutput, applied []AppliedOperation) string {
	lines := []string{
		"Tarea: " + strings.TrimSpace(task),
		"Cocinero: " + strings.TrimSpace(executor.Summary),
	}
	if len(applied) > 0 {
		var ops []string
		for _, item := range applied {
			label := item.Tool
			if strings.TrimSpace(item.Path) != "" {
				label += " " + item.Path
			}
			ops = append(ops, label)
		}
		lines = append(lines, "Operaciones: "+strings.Join(ops, ", "))
	}
	lines = append(lines, "Auditor: "+strings.TrimSpace(reviewer.Summary))
	return strings.Join(lines, "\n")
}

func renderTaskNote(task string, executor ExecutorOutput, reviewer ReviewerOutput) string {
	return strings.Join([]string{
		"Tarea: " + trimForContext(task, 100),
		"Cocinero: " + trimForContext(executor.Summary, 140),
		"Auditor: " + trimForContext(reviewer.Summary, 140),
	}, " | ")
}

func operationPaths(applied []AppliedOperation) []string {
	var out []string
	for _, item := range applied {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		out = append(out, item.Path)
	}
	return out
}

func escapeRegex(text string) string {
	replacer := strings.NewReplacer(
		"|", "\\|", "\\", "\\\\", "{", "\\{", "}", "\\}", "(", "\\(", ")", "\\)",
		"[", "\\[", "]", "\\]", "^", "\\^", "$", "\\$", "+", "\\+", ".", "\\.",
	)
	return replacer.Replace(text)
}

func patternToRegex(pattern string) *regexp.Regexp {
	var builder strings.Builder
	builder.WriteString("^")
	for _, char := range pattern {
		switch char {
		case '*':
			builder.WriteString(".*")
		case '?':
			builder.WriteString(".")
		default:
			builder.WriteString(escapeRegex(string(char)))
		}
	}
	builder.WriteString("$")
	return regexp.MustCompile(builder.String())
}

func evaluatePermission(config map[string]map[string]string, action, target string) string {
	rules, ok := config[action]
	if !ok {
		return "ask"
	}
	decision := "ask"
	for pattern, value := range rules {
		if patternToRegex(pattern).MatchString(target) {
			decision = value
		}
	}
	return decision
}

func assertPermission(config map[string]map[string]string, action, target string, overrides map[string]map[string]string) error {
	decision := evaluatePermission(config, action, target)
	if actionOverrides, ok := overrides[action]; ok {
		if value, exists := actionOverrides[target]; exists {
			decision = value
		}
	}
	switch decision {
	case "allow":
		return nil
	case "deny":
		return fmt.Errorf("permiso denegado para %s: %s", action, target)
	default:
		return &PermissionRequiredError{
			Prompt: PermissionPrompt{
				Action:  action,
				Target:  target,
				Message: fmt.Sprintf("Odrys necesita permiso para %s: %s", action, target),
			},
		}
	}
}

func resolveWorkspace(root string, cfg WorkspaceConfig) (Workspace, error) {
	path := filepath.Join(root, cfg.Path)
	info, err := os.Stat(path)
	if err != nil {
		return Workspace{}, err
	}
	if !info.IsDir() {
		return Workspace{}, fmt.Errorf("el workspace debe ser un directorio: %s", path)
	}
	return Workspace{Root: path, Include: cfg.Include, Exclude: cfg.Exclude}, nil
}

func listDirectory(workspace Workspace, relativePath string, permission map[string]map[string]string, overrides map[string]map[string]string) ([]map[string]string, error) {
	if err := assertPermission(permission, "list", relativePath, overrides); err != nil {
		return nil, err
	}
	var output []map[string]string
	base := filepath.Join(workspace.Root, relativePath)
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == base {
			return nil
		}
		rel, _ := filepath.Rel(workspace.Root, path)
		rel = filepath.ToSlash(rel)
		for _, excluded := range workspace.Exclude {
			if rel == excluded || strings.HasPrefix(rel, excluded+"/") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		entryType := "file"
		if d.IsDir() {
			entryType = "directory"
		}
		output = append(output, map[string]string{"path": rel, "type": entryType})
		return nil
	})
	return output, err
}

func readTextFile(workspace Workspace, relativePath string, permission map[string]map[string]string, overrides map[string]map[string]string) (string, error) {
	if err := assertPermission(permission, "read", relativePath, overrides); err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(workspace.Root, relativePath))
	return string(content), err
}

func writeTextFile(workspace Workspace, relativePath, content string, permission map[string]map[string]string, overrides map[string]map[string]string) (map[string]any, error) {
	if err := assertPermission(permission, "edit", relativePath, overrides); err != nil {
		return nil, err
	}
	path := filepath.Join(workspace.Root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": relativePath, "bytes": len([]byte(content))}, nil
}

func runCommand(workspace Workspace, command string, permission map[string]map[string]string, overrides map[string]map[string]string) (map[string]string, error) {
	if err := assertPermission(permission, "bash", command, overrides); err != nil {
		return nil, err
	}
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return nil, errors.New("comando vacio")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workspace.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return map[string]string{
		"command": command,
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
	}, err
}

func searchInWorkspace(workspace Workspace, pattern string, permission map[string]map[string]string, limit int, overrides map[string]map[string]string) ([]map[string]any, error) {
	if err := assertPermission(permission, "search", pattern, overrides); err != nil {
		return nil, err
	}
	files, err := listDirectory(workspace, ".", permission, overrides)
	if err != nil {
		return nil, err
	}
	regex, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}
	var matches []map[string]any
	for _, item := range files {
		if item["type"] != "file" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(workspace.Root, item["path"]))
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		for index, line := range lines {
			if !regex.MatchString(line) {
				continue
			}
			matches = append(matches, map[string]any{"path": item["path"], "line": index + 1, "text": strings.TrimSpace(line)})
			if len(matches) >= limit {
				return matches, nil
			}
		}
	}
	return matches, nil
}

func safeRead(workspace Workspace, relativePath string, permission map[string]map[string]string) string {
	content, err := readTextFile(workspace, relativePath, permission, nil)
	if err != nil {
		return fmt.Sprintf("No se pudo leer %s: %v", relativePath, err)
	}
	return content
}

func scanWorkspace(workspace Workspace, permission map[string]map[string]string) (map[string]any, error) {
	files, err := listDirectory(workspace, ".", permission, nil)
	if err != nil {
		return nil, err
	}
	selected := make([]map[string]string, 0)
	for _, item := range files {
		for _, include := range workspace.Include {
			if item["path"] == include || strings.HasPrefix(item["path"], include+"/") {
				selected = append(selected, item)
				break
			}
		}
		if len(selected) >= 40 {
			break
		}
	}

	keyFiles := make([]map[string]string, 0)
	keyPattern := regexp.MustCompile(`(?i)(package\.json|README|tsconfig|pyproject|Cargo\.toml|go\.mod|pom\.xml|Gemfile|requirements)`)
	for _, item := range selected {
		if item["type"] != "file" || !keyPattern.MatchString(item["path"]) {
			continue
		}
		keyFiles = append(keyFiles, map[string]string{
			"path":    item["path"],
			"content": safeRead(workspace, item["path"], permission),
		})
	}

	var git map[string]string
	status, err := runCommand(workspace, "git status --short", permission, nil)
	if err == nil {
		inside, _ := runCommand(workspace, "git rev-parse --is-inside-work-tree", permission, nil)
		git = map[string]string{
			"inside_work_tree": strings.TrimSpace(inside["stdout"]),
			"status":           strings.TrimSpace(status["stdout"]),
		}
	} else {
		git = map[string]string{"error": err.Error()}
	}

	hints, _ := searchInWorkspace(workspace, "TODO|FIXME|HACK", permission, 20, nil)
	return map[string]any{
		"root":           workspace.Root,
		"file_count":     len(files),
		"selected_files": selected,
		"key_files":      keyFiles,
		"git":            git,
		"search_hints":   hints,
	}, nil
}

func parseBlocks(patchText string) []map[string]any {
	lines := strings.Split(strings.ReplaceAll(patchText, "\r\n", "\n"), "\n")
	var blocks []map[string]any
	var current map[string]any
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			if current != nil {
				blocks = append(blocks, current)
			}
			current = map[string]any{"type": "update", "path": strings.TrimPrefix(line, "*** Update File: "), "hunks": []map[string]any{}}
		case strings.HasPrefix(line, "*** Add File: "):
			if current != nil {
				blocks = append(blocks, current)
			}
			current = map[string]any{"type": "add", "path": strings.TrimPrefix(line, "*** Add File: "), "lines": []string{}}
		default:
			if current == nil {
				continue
			}
			if current["type"] == "add" {
				if strings.HasPrefix(line, "+") {
					current["lines"] = append(current["lines"].([]string), strings.TrimPrefix(line, "+"))
				}
				continue
			}
			hunks := current["hunks"].([]map[string]any)
			if strings.HasPrefix(line, "@@") {
				hunks = append(hunks, map[string]any{"search": "", "replace": []string{}})
				current["hunks"] = hunks
				continue
			}
			if len(hunks) == 0 {
				hunks = append(hunks, map[string]any{"search": "", "replace": []string{}})
			}
			hunk := hunks[len(hunks)-1]
			switch {
			case strings.HasPrefix(line, "-"):
				hunk["search"] = hunk["search"].(string) + strings.TrimPrefix(line, "-") + "\n"
			case strings.HasPrefix(line, "+"):
				hunk["replace"] = append(hunk["replace"].([]string), strings.TrimPrefix(line, "+"))
			case strings.HasPrefix(line, " "):
				hunk["search"] = hunk["search"].(string) + strings.TrimPrefix(line, " ") + "\n"
				hunk["replace"] = append(hunk["replace"].([]string), strings.TrimPrefix(line, " "))
			}
			hunks[len(hunks)-1] = hunk
			current["hunks"] = hunks
		}
	}
	if current != nil {
		blocks = append(blocks, current)
	}
	return blocks
}

func applyUpdate(original string, hunks []map[string]any, path string) (string, error) {
	output := original
	for _, hunk := range hunks {
		search := strings.TrimSuffix(hunk["search"].(string), "\n")
		replace := strings.Join(hunk["replace"].([]string), "\n")
		if search == "" {
			continue
		}
		if !strings.Contains(output, search) {
			return "", fmt.Errorf("no se encontro el bloque a reemplazar en %s", path)
		}
		output = strings.Replace(output, search, replace, 1)
	}
	return output, nil
}

func applyPatch(workspace Workspace, patchText string, permission map[string]map[string]string, overrides map[string]map[string]string) ([]map[string]any, error) {
	blocks := parseBlocks(patchText)
	var changes []map[string]any
	for _, block := range blocks {
		switch block["type"] {
		case "add":
			result, err := writeTextFile(workspace, block["path"].(string), strings.Join(block["lines"].([]string), "\n"), permission, overrides)
			if err != nil {
				return nil, err
			}
			result["type"] = "add"
			changes = append(changes, result)
		case "update":
			original, err := readTextFile(workspace, block["path"].(string), permission, overrides)
			if err != nil {
				return nil, err
			}
			next, err := applyUpdate(original, block["hunks"].([]map[string]any), block["path"].(string))
			if err != nil {
				return nil, err
			}
			result, err := writeTextFile(workspace, block["path"].(string), next, permission, overrides)
			if err != nil {
				return nil, err
			}
			result["type"] = "update"
			changes = append(changes, result)
		}
	}
	return changes, nil
}

func applyExecutorOperations(workspace Workspace, permission map[string]map[string]string, operations []ExecutorOperation, overrides map[string]map[string]string) ([]AppliedOperation, error) {
	var applied []AppliedOperation
	for _, operation := range operations {
		switch operation.Tool {
		case "write":
			result, err := writeTextFile(workspace, operation.Path, operation.Content, permission, overrides)
			if err != nil {
				return nil, err
			}
			content, _ := readTextFile(workspace, operation.Path, permission, overrides)
			applied = append(applied, AppliedOperation{
				Tool:    "write",
				Path:    operation.Path,
				Result:  result,
				Content: content,
			})
		case "apply_patch":
			result, err := applyPatch(workspace, operation.Patch, permission, overrides)
			if err != nil {
				return nil, err
			}
			applied = append(applied, AppliedOperation{
				Tool:   "apply_patch",
				Result: result,
			})
		default:
			return nil, fmt.Errorf("operacion no soportada: %s", operation.Tool)
		}
	}
	return applied, nil
}
