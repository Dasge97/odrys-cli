package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var defaultFiles = map[string]string{
	"project/spec.md":            "# Spec\n",
	"project/architecture.md":    "# Architecture\n",
	"project/rules.md":           "# Rules\n",
	"project/checklist.md":       "# Checklist\n",
	"project/project_state.md":   "# Project State\n",
	"logs/.gitkeep":              "",
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
	raw, err := os.ReadFile(filepath.Join(root, "odrys.config.json"))
	if err != nil {
		return Config{}, err
	}

	var parsed Config
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
	cfg.Worker.Planner.Auto = parsed.Worker.Planner.Auto || defaultConfig.Worker.Planner.Auto
	cfg.Worker.SummarizeOnSuccess = parsed.Worker.SummarizeOnSuccess || defaultConfig.Worker.SummarizeOnSuccess
	return cfg, nil
}
