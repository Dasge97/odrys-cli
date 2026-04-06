package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Dasge97/odrys-cli/internal/app"
	"github.com/Dasge97/odrys-cli/internal/backend"
	"github.com/Dasge97/odrys-cli/internal/server"
	"github.com/Dasge97/odrys-cli/internal/serverclient"
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
	serverAPI := serverclient.New()
	serverAvailable := serverAPI.Available()

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
	case "sessions":
		subcommand := "list"
		if len(positionals) > 0 {
			subcommand = positionals[0]
		}
		switch subcommand {
		case "list":
			var (
				items []backend.SessionSummary
				err error
			)
			if serverAvailable {
				items, err = serverAPI.ListSessions()
			} else {
				items, err = service.Sessions(12)
			}
			if err != nil {
				return err
			}
			return printJSON(items)
		case "latest":
			item, err := service.LatestSession()
			if err != nil {
				return err
			}
			return printJSON(item)
		case "show":
			if len(positionals) < 2 {
				return errors.New("uso: odrys sessions show <session_id>")
			}
			item, err := service.LoadSession(positionals[1])
			if err != nil {
				return err
			}
			return printJSON(item)
		default:
			return errors.New("subcomando de sessions no soportado. Usa: list, latest o show")
		}
	case "provider":
		subcommand := "current"
		if len(positionals) > 0 {
			subcommand = positionals[0]
		}
		switch subcommand {
		case "current":
			payload, err := service.Doctor()
			if err != nil {
				return err
			}
			return printJSON(payload.Provider)
		case "set":
			if len(positionals) < 3 {
				return errors.New("uso: odrys provider set <provider> <model>")
			}
			err := service.SaveProvider(backend.ProviderConfig{
				Name:  positionals[1],
				Model: positionals[2],
			})
			if err != nil {
				return err
			}
			return printJSON(map[string]string{
				"provider": positionals[1],
				"model":    positionals[2],
			})
		default:
			return errors.New("subcomando de provider no soportado. Usa: current o set")
		}
	case "openai":
		subcommand := "status"
		if len(positionals) > 0 {
			subcommand = positionals[0]
		}
		switch subcommand {
		case "status":
			var (
				status backend.OpenAIAuthStatus
				err error
			)
			if serverAvailable {
				status, err = serverAPI.OpenAIStatus()
			} else {
				status, err = service.OpenAIStatus()
			}
			if err != nil {
				return err
			}
			return printJSON(status)
		case "connect":
			apiKey := flags["api-key"]
			model := flags["model"]
			if serverAvailable {
				err := serverAPI.ConnectOpenAIAPIKey(apiKey, model)
				if err != nil {
					return err
				}
			} else if err := service.ConnectOpenAI(apiKey, model); err != nil {
				return err
			}
			return printJSON(map[string]string{
				"provider": "openai",
				"model":    fallbackString(model, "gpt-4.1-mini"),
				"status":   "connected",
			})
		default:
			return errors.New("subcomando de openai no soportado. Usa: status o connect")
		}
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
	case "server":
		host := fallbackString(flags["host"], envOr("ODYRSD_HOST", "127.0.0.1"))
		port := fallbackInt(flags["port"], envInt("ODYRSD_PORT", 4111))
		svc := server.New(root)
		handler, err := svc.Handler()
		if err != nil {
			return err
		}
		address := fmt.Sprintf("%s:%d", host, port)
		fmt.Printf("odrys-core listening on http://%s\n", address)
		return http.ListenAndServe(address, handler)
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
  odrys sessions
  odrys sessions latest
  odrys sessions show <session_id>
  odrys provider current
  odrys provider set <provider> <model>
  odrys openai status
  odrys openai connect --api-key "sk-..." --model gpt-4.1-mini
  odrys server --host 127.0.0.1 --port 4111
  odrys workspace scan
  odrys workspace write ruta.txt --content "hola"
  odrys workspace patch --file ./cambio.patch`)
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fallbackInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func resolveRoot() (string, error) {
	if fromEnv := os.Getenv("ODYRS_ROOT"); fromEnv != "" {
		return filepath.Abs(fromEnv)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver el root de Odrys: %w", err)
	}
	return filepath.Abs(workingDir)
}
