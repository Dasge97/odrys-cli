package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Dasge97/odrys-cli/internal/server"
)

func main() {
	root, err := resolveRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	host := envOr("ODYRSD_HOST", "127.0.0.1")
	port := envInt("ODYRSD_PORT", 4111)
	address := fmt.Sprintf("%s:%d", host, port)

	svc := server.New(root)
	handler, err := svc.Handler()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("odrys-core listening on http://%s\n", address)
	if err := http.ListenAndServe(address, handler); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
		return "", err
	}
	return filepath.Abs(workingDir)
}
