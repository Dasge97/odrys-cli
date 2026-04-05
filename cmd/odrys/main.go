package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Dasge97/odrys-cli/internal/app"
)

func main() {
	root, err := resolveRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	program := tea.NewProgram(app.NewModel(root), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveRoot() (string, error) {
	if fromEnv := os.Getenv("ODYRS_ROOT"); fromEnv != "" {
		return filepath.Abs(fromEnv)
	}

	executable, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(executable)
		candidates := []string{
			execDir,
			filepath.Join(execDir, ".."),
			filepath.Join(execDir, "..", ".."),
		}
		for _, candidate := range candidates {
			absolute, absoluteErr := filepath.Abs(candidate)
			if absoluteErr != nil {
				continue
			}
			if _, statErr := os.Stat(filepath.Join(absolute, "src", "cli.js")); statErr == nil {
				return absolute, nil
			}
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver el root de Odrys: %w", err)
	}

	return filepath.Abs(workingDir)
}
