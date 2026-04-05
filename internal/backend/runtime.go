package backend

import (
	"bytes"
	"context"
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

func (s *Service) Doctor() (DoctorPayload, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return DoctorPayload{}, err
	}
	return DoctorPayload{
		Root:       s.Root,
		ConfigPath: filepath.Join(s.Root, "odrys.config.json"),
		Provider:   cfg.Provider,
		Workspace:  cfg.Workspace,
		Permission: cfg.Permission,
		Worker:     cfg.Worker,
	}, nil
}

func (s *Service) Run(goal string) (RunResult, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return RunResult{}, err
	}
	return runWorker(s.Root, goal, cfg)
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

func (s *Service) Write(relativePath, content string) (map[string]any, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}
	workspace, err := resolveWorkspace(s.Root, cfg.Workspace)
	if err != nil {
		return nil, err
	}
	return writeTextFile(workspace, relativePath, content, cfg.Permission)
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
	return applyPatch(workspace, patchText, cfg.Permission)
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

func buildContext(root, mode string, workspace *Workspace, permission map[string]map[string]string) ([]string, error) {
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
			ProjectState: "# Project State\n\nUltima actualizacion automatica.\n\n- Objetivo reciente: " + input.Goal + "\n- Estado: se ejecuto el flujo completo del worker\n- Proximo paso natural: habilitar cambios automaticos y aprobaciones interactivas\n",
			NextAction: "store_state",
		})
	default:
		return nil, fmt.Errorf("agente no soportado por mock: %s", input.Agent)
	}
}

type openAICompatibleProvider struct {
	config  ProviderConfig
	baseURL string
	apiKey  string
}

func (p openAICompatibleProvider) Run(ctx context.Context, input providerInput) (json.RawMessage, error) {
	if p.apiKey == "" {
		return nil, errors.New("falta ODYRS_API_KEY u OPENAI_API_KEY para usar openai-compatible")
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

func createProvider(cfg ProviderConfig) provider {
	if cfg.Name == "openai-compatible" {
		return openAICompatibleProvider{
			config:  cfg,
			baseURL: envOr("ODYRS_BASE_URL", "https://api.openai.com/v1"),
			apiKey:  envFirst("ODYRS_API_KEY", "OPENAI_API_KEY"),
		}
	}
	return mockProvider{config: cfg}
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

func validateAgentOutput(agent string, raw json.RawMessage) error {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return err
	}
	required := map[string][]string{
		"planner":    {"status", "summary", "phases", "next_action"},
		"executor":   {"status", "summary", "changes", "assumptions", "open_questions", "operations", "next_action"},
		"reviewer":   {"status", "summary", "errors", "verified_against", "next_action"},
		"summarizer": {"status", "project_state", "next_action"},
	}
	for _, key := range required[agent] {
		if _, ok := probe[key]; !ok {
			return fmt.Errorf("salida de %s invalida", agent)
		}
	}
	return nil
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
	response, err := provider.Run(ctx, providerInput{
		Agent:        name,
		AgentName:    agent.Name,
		SystemPrompt: strings.TrimSpace(base) + "\n\n" + strings.TrimSpace(agentPrompt),
		Context:      contextText,
		Goal:         goal,
		Task:         task,
		Feedback:     feedback,
	})
	if err != nil {
		return nil, err
	}
	return response, validateAgentOutput(name, response)
}

func runWorker(root, goal string, config Config) (RunResult, error) {
	provider := createProvider(config.Provider)
	workspace, err := resolveWorkspace(root, config.Workspace)
	if err != nil {
		return RunResult{}, err
	}
	executeContextList, err := buildContext(root, "execute", &workspace, config.Permission)
	if err != nil {
		return RunResult{}, err
	}
	reviewContextList, err := buildContext(root, "review", &workspace, config.Permission)
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
			if err := json.Unmarshal(rawExec, &executorOutput); err != nil {
				return RunResult{}, err
			}
			applied, err = applyExecutorOperations(workspace, config.Permission, executorOutput.Operations)
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

	result := RunResult{
		Status:  "completed",
		Goal:    goal,
		Planned: mustPlan,
		Tasks:   taskResults,
		Summary: summary,
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

func assertPermission(config map[string]map[string]string, action, target string) error {
	switch evaluatePermission(config, action, target) {
	case "allow":
		return nil
	case "deny":
		return fmt.Errorf("permiso denegado para %s: %s", action, target)
	default:
		return fmt.Errorf("aprobacion requerida para %s: %s", action, target)
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

func listDirectory(workspace Workspace, relativePath string, permission map[string]map[string]string) ([]map[string]string, error) {
	if err := assertPermission(permission, "list", relativePath); err != nil {
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

func readTextFile(workspace Workspace, relativePath string, permission map[string]map[string]string) (string, error) {
	if err := assertPermission(permission, "read", relativePath); err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(workspace.Root, relativePath))
	return string(content), err
}

func writeTextFile(workspace Workspace, relativePath, content string, permission map[string]map[string]string) (map[string]any, error) {
	if err := assertPermission(permission, "edit", relativePath); err != nil {
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

func runCommand(workspace Workspace, command string, permission map[string]map[string]string) (map[string]string, error) {
	if err := assertPermission(permission, "bash", command); err != nil {
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

func searchInWorkspace(workspace Workspace, pattern string, permission map[string]map[string]string, limit int) ([]map[string]any, error) {
	if err := assertPermission(permission, "search", pattern); err != nil {
		return nil, err
	}
	files, err := listDirectory(workspace, ".", permission)
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
	content, err := readTextFile(workspace, relativePath, permission)
	if err != nil {
		return fmt.Sprintf("No se pudo leer %s: %v", relativePath, err)
	}
	return content
}

func scanWorkspace(workspace Workspace, permission map[string]map[string]string) (map[string]any, error) {
	files, err := listDirectory(workspace, ".", permission)
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
	status, err := runCommand(workspace, "git status --short", permission)
	if err == nil {
		inside, _ := runCommand(workspace, "git rev-parse --is-inside-work-tree", permission)
		git = map[string]string{
			"inside_work_tree": strings.TrimSpace(inside["stdout"]),
			"status":           strings.TrimSpace(status["stdout"]),
		}
	} else {
		git = map[string]string{"error": err.Error()}
	}

	hints, _ := searchInWorkspace(workspace, "TODO|FIXME|HACK", permission, 20)
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

func applyPatch(workspace Workspace, patchText string, permission map[string]map[string]string) ([]map[string]any, error) {
	blocks := parseBlocks(patchText)
	var changes []map[string]any
	for _, block := range blocks {
		switch block["type"] {
		case "add":
			result, err := writeTextFile(workspace, block["path"].(string), strings.Join(block["lines"].([]string), "\n"), permission)
			if err != nil {
				return nil, err
			}
			result["type"] = "add"
			changes = append(changes, result)
		case "update":
			original, err := readTextFile(workspace, block["path"].(string), permission)
			if err != nil {
				return nil, err
			}
			next, err := applyUpdate(original, block["hunks"].([]map[string]any), block["path"].(string))
			if err != nil {
				return nil, err
			}
			result, err := writeTextFile(workspace, block["path"].(string), next, permission)
			if err != nil {
				return nil, err
			}
			result["type"] = "update"
			changes = append(changes, result)
		}
	}
	return changes, nil
}

func applyExecutorOperations(workspace Workspace, permission map[string]map[string]string, operations []ExecutorOperation) ([]AppliedOperation, error) {
	var applied []AppliedOperation
	for _, operation := range operations {
		switch operation.Tool {
		case "write":
			result, err := writeTextFile(workspace, operation.Path, operation.Content, permission)
			if err != nil {
				return nil, err
			}
			content, _ := readTextFile(workspace, operation.Path, permission)
			applied = append(applied, AppliedOperation{
				Tool:    "write",
				Path:    operation.Path,
				Result:  result,
				Content: content,
			})
		case "apply_patch":
			result, err := applyPatch(workspace, operation.Patch, permission)
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
