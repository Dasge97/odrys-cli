package backend

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type rawConfig struct {
	Provider   ProviderConfig               `json:"provider"`
	Workspace  WorkspaceConfig              `json:"workspace"`
	Permission map[string]map[string]string `json:"permission"`
	Worker     rawWorkerConfig              `json:"worker"`
	Session    rawSessionConfig             `json:"session"`
}

type rawWorkerConfig struct {
	MaxReviewLoops    int               `json:"maxReviewLoops"`
	Planner           rawPlannerConfig  `json:"planner"`
	SummarizeOnSuccess *bool            `json:"summarizeOnSuccess"`
}

type rawPlannerConfig struct {
	Auto *bool `json:"auto"`
}

type rawSessionConfig struct {
	AutoResume      *bool `json:"autoResume"`
	ContextMessages int   `json:"contextMessages"`
	ContextRuns     int   `json:"contextRuns"`
	ContextFiles    int   `json:"contextFiles"`
}

var defaultFiles = map[string]string{
	"project/spec.md":            "# Spec\n",
	"project/architecture.md":    "# Architecture\n",
	"project/rules.md":           "# Rules\n",
	"project/checklist.md":       "# Checklist\n",
	"project/project_state.md":   "# Project State\n",
	"logs/.gitkeep":              "",
	".odrys/.gitkeep":            "",
	"agents/odrys_prompt.md":     "Eres `Odrys`.\n",
	"agents/metre_prompt.md":     "Eres `Metre`.\n",
	"agents/cocinero_prompt.md":  "Eres `Cocinero`.\n",
	"agents/auditor_prompt.md":   "Eres `Auditor`.\n",
	"agents/caja_prompt.md":      "Eres `Caja`.\n",
	"system/base_system_prompt.md": "Sistema de agentes.\n",
	"schemas/output_schema_global.json": "{}\n",
}

var defaultConfig = Config{
	Provider: ProviderConfig{
		Name:  "mock",
		Model: "odrys-mock-1",
	},
	Workspace: WorkspaceConfig{
		Path:    ".",
		Include: []string{"go.mod", "README.md", "cmd", "internal", "project"},
		Exclude: []string{".git", "opencode-dev", "dist", "build", "coverage", ".go-cache", ".go-mod-cache", ".go-path"},
	},
	Permission: map[string]map[string]string{
		"read":   {"*": "allow"},
		"edit":   {"*": "ask", ".odrys-sandbox/**": "allow"},
		"list":   {"*": "allow"},
		"search": {"*": "allow"},
		"bash": {
			"*":                           "ask",
			"git status --short":         "allow",
			"git rev-parse --is-inside-work-tree": "allow",
		},
	},
	Worker: WorkerConfig{
		MaxReviewLoops: 2,
		Planner: PlannerConfig{
			Auto: true,
		},
		SummarizeOnSuccess: true,
	},
	Session: SessionConfig{
		AutoResume:      true,
		ContextMessages: 8,
		ContextRuns:     4,
		ContextFiles:    6,
	},
}

func ensureProjectScaffold(root string) error {
	for relative, content := range defaultFiles {
		path := filepath.Join(root, relative)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}

	configPath := filepath.Join(root, "odrys.config.json")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}
	raw, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(raw, '\n'), 0o644)
}

func loadConfig(root string) (Config, error) {
	_ = loadLocalEnv(root)
	raw, err := os.ReadFile(filepath.Join(root, "odrys.config.json"))
	if err != nil {
		return Config{}, err
	}

	var parsed rawConfig
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Config{}, err
	}

	cfg := defaultConfig
	if parsed.Provider.Name != "" {
		cfg.Provider.Name = parsed.Provider.Name
	}
	if parsed.Provider.Model != "" {
		cfg.Provider.Model = parsed.Provider.Model
	}
	if parsed.Workspace.Path != "" {
		cfg.Workspace.Path = parsed.Workspace.Path
	}
	if len(parsed.Workspace.Include) > 0 {
		cfg.Workspace.Include = parsed.Workspace.Include
	}
	if len(parsed.Workspace.Exclude) > 0 {
		cfg.Workspace.Exclude = parsed.Workspace.Exclude
	}
	if parsed.Permission != nil {
		for action, rules := range parsed.Permission {
			cfg.Permission[action] = rules
		}
	}
	if parsed.Worker.MaxReviewLoops != 0 {
		cfg.Worker.MaxReviewLoops = parsed.Worker.MaxReviewLoops
	}
	if parsed.Worker.Planner.Auto != nil {
		cfg.Worker.Planner.Auto = *parsed.Worker.Planner.Auto
	}
	if parsed.Worker.SummarizeOnSuccess != nil {
		cfg.Worker.SummarizeOnSuccess = *parsed.Worker.SummarizeOnSuccess
	}
	if parsed.Session.ContextMessages != 0 {
		cfg.Session.ContextMessages = parsed.Session.ContextMessages
	}
	if parsed.Session.ContextRuns != 0 {
		cfg.Session.ContextRuns = parsed.Session.ContextRuns
	}
	if parsed.Session.ContextFiles != 0 {
		cfg.Session.ContextFiles = parsed.Session.ContextFiles
	}
	if parsed.Session.AutoResume != nil {
		cfg.Session.AutoResume = *parsed.Session.AutoResume
	}
	return cfg, nil
}

func saveConfig(root string, cfg Config) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "odrys.config.json"), append(raw, '\n'), 0o644)
}

func loadLocalEnv(root string) error {
	paths := []string{
		filepath.Join(root, ".env"),
		filepath.Join(root, ".env.local"),
	}
	for _, path := range paths {
		if err := loadEnvFile(path); err != nil {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func saveOpenAIEnv(root, apiKey string) error {
	path := filepath.Join(root, ".env")
	lines := []string{}
	if raw, err := os.ReadFile(path); err == nil {
		existing := strings.Split(string(raw), "\n")
		for _, line := range existing {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "OPENAI_API_KEY=") || strings.HasPrefix(trimmed, "export OPENAI_API_KEY=") {
				continue
			}
			if trimmed == "" && len(lines) > 0 && lines[len(lines)-1] == "" {
				continue
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "OPENAI_API_KEY="+apiKey)
	content := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o600)
}

func saveEnvKey(root, envKey, value string) error {
	path := filepath.Join(root, ".env")
	lines := []string{}
	if raw, err := os.ReadFile(path); err == nil {
		existing := strings.Split(string(raw), "\n")
		for _, line := range existing {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, envKey+"=") || strings.HasPrefix(trimmed, "export "+envKey+"=") {
				continue
			}
			if trimmed == "" && len(lines) > 0 && lines[len(lines)-1] == "" {
				continue
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, envKey+"="+value)
	content := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o600)
}
