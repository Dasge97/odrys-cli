package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Dasge97/odrys-cli/internal/app"
	"github.com/Dasge97/odrys-cli/internal/backend"
)

func main() {
	root, err := resolveRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := run(root, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, args []string) error {
	command, flags, positionals := parseArgs(args)
	service := backend.NewService(root)

	switch command {
	case "", "tui":
		if err := service.EnsureScaffold(); err != nil {
			return err
		}
		program := tea.NewProgram(app.NewModel(root), tea.WithAltScreen())
		_, err := program.Run()
		return err
	case "init":
		if err := service.EnsureScaffold(); err != nil {
			return err
		}
		fmt.Printf("Scaffold listo en %s\n", root)
		return nil
	case "doctor":
		payload, err := service.Doctor()
		if err != nil {
			return err
		}
		return printJSON(payload)
	case "run":
		goal := strings.TrimSpace(strings.Join(positionals, " "))
		if goal == "" {
			return errors.New("debes indicar un objetivo. Ejemplo: odrys run \"crear una API de tareas\"")
		}
		result, err := service.Run(goal)
		if err != nil {
			return err
		}
		return printJSON(result)
	case "workspace":
		subcommand := "scan"
		if len(positionals) > 0 {
			subcommand = positionals[0]
		}
		switch subcommand {
		case "scan":
			snapshot, err := service.Scan()
			if err != nil {
				return err
			}
			return printJSON(snapshot)
		case "write":
			if len(positionals) < 2 {
				return errors.New("uso: odrys workspace write ruta.txt --content \"texto\"")
			}
			content := flags["content"]
			result, err := service.Write(positionals[1], content)
			if err != nil {
				return err
			}
			return printJSON(result)
		case "patch":
			patchFile := flags["file"]
			if patchFile == "" {
				return errors.New("uso: odrys workspace patch --file ./cambio.patch")
			}
			raw, err := os.ReadFile(filepath.Join(root, patchFile))
			if err != nil {
				return err
			}
			result, err := service.ApplyPatch(string(raw))
			if err != nil {
				return err
			}
			return printJSON(result)
		default:
			return errors.New("subcomando de workspace no soportado. Usa: scan, write o patch")
		}
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		printHelp()
		return nil
	}
}

func parseArgs(args []string) (string, map[string]string, []string) {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	flags := map[string]string{}
	var positionals []string
	for index := 1; index < len(args); index++ {
		item := args[index]
		if strings.HasPrefix(item, "--") {
			key := strings.TrimPrefix(item, "--")
			if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
				flags[key] = args[index+1]
				index++
			} else {
				flags[key] = "true"
			}
			continue
		}
		positionals = append(positionals, item)
	}
	return command, flags, positionals
}

func printJSON(value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func printHelp() {
	fmt.Println(`Odrys CLI

Uso:
  odrys
  odrys init
  odrys doctor
  odrys run "tu objetivo"
  odrys workspace scan
  odrys workspace write ruta.txt --content "hola"
  odrys workspace patch --file ./cambio.patch`)
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
			if _, statErr := os.Stat(filepath.Join(absolute, "odrys.config.json")); statErr == nil {
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
