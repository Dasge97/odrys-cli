package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Bridge struct {
	Root       string
	NodeBinary string
}

type DoctorPayload struct {
	Root      string         `json:"root"`
	ConfigPath string        `json:"configPath"`
	Provider  ProviderConfig `json:"provider"`
	Workspace WorkspaceState `json:"workspace"`
}

type ProviderConfig struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type WorkspaceState struct {
	Path string `json:"path"`
}

type RunResult struct {
	Goal     string     `json:"goal"`
	Planned  bool       `json:"planned"`
	LogPath  string     `json:"log_path"`
	Tasks    []RunTask  `json:"tasks"`
}

type RunTask struct {
	Task              string              `json:"task"`
	Executor          ActorResult         `json:"executor"`
	Reviewer          ActorResult         `json:"reviewer"`
	AppliedOperations []AppliedOperation  `json:"applied_operations"`
}

type ActorResult struct {
	Summary string `json:"summary"`
}

type AppliedOperation struct {
	Tool string `json:"tool"`
	Path string `json:"path"`
}

func NewBridge(root string) *Bridge {
	return &Bridge{
		Root:       root,
		NodeBinary: "node",
	}
}

func (b *Bridge) Doctor(ctx context.Context) (DoctorPayload, error) {
	var payload DoctorPayload
	if err := b.execJSON(ctx, &payload, "src/cli.js", "doctor"); err != nil {
		return DoctorPayload{}, err
	}
	return payload, nil
}

func (b *Bridge) Run(ctx context.Context, goal string) (RunResult, error) {
	var payload RunResult
	if err := b.execJSON(ctx, &payload, "src/cli.js", "run", goal); err != nil {
		return RunResult{}, err
	}
	return payload, nil
}

func (b *Bridge) Scan(ctx context.Context) (string, error) {
	output, err := b.execRaw(ctx, "src/cli.js", "workspace", "scan")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (b *Bridge) execJSON(ctx context.Context, target any, args ...string) error {
	output, err := b.execRaw(ctx, args...)
	if err != nil {
		return err
	}
	if parseErr := json.Unmarshal([]byte(output), target); parseErr != nil {
		return fmt.Errorf("no se pudo parsear la salida JSON del backend Node: %w", parseErr)
	}
	return nil
}

func (b *Bridge) execRaw(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, b.NodeBinary, args...)
	command.Dir = b.Root

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("backend Node fallo: %s", message)
	}

	return stdout.String(), nil
}
