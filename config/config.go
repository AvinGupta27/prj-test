package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// Config holds all environment-specific configuration derived from .env.
type Config struct {
	Env           string
	SpinnerBFFURL string // Base URL for Spinner BFF (pack reveals, etc.)
	FcBFFURL      string // Base URL for FC BFF (auth, etc.)
	FrontendURL   string
	ProxyURL      string // Base URL for the proxy service (supply, etc.)
}

// projectRoot returns the absolute path of the project root by walking up from
// this file's compile-time location (config/config.go → project root).
func projectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filename))
}

// Root returns the absolute path to the project root directory.
// Use this to resolve output directories, data paths, etc. from any package.
func Root() string {
	return projectRoot()
}

// Load reads the .env file at the project root and returns a populated Config.
// Existing environment variables are NOT overridden (godotenv.Load semantics).
func Load() (*Config, error) {
	root := projectRoot()
	envPath := filepath.Join(root, ".env")

	if err := godotenv.Load(envPath); err != nil {
		return nil, fmt.Errorf("config: failed to load .env at %s: %w", envPath, err)
	}

	env := os.Getenv("ENV")
	if env == "" {
		return nil, fmt.Errorf("config: ENV is not set in .env")
	}

	cfg := &Config{
		Env:           env,
		SpinnerBFFURL: fmt.Sprintf(os.Getenv("SPINNER_BFF_BASE_URL"), env),
		FcBFFURL:      fmt.Sprintf(os.Getenv("FC_BFF_BASE_URL"), env),
		FrontendURL:   fmt.Sprintf(os.Getenv("FRONTEND_BASE_URL"), env),
		ProxyURL:      fmt.Sprintf(os.Getenv("PROXY_URL"), env),
	}

	return cfg, nil
}

// DataPath returns the absolute path to a file inside the project's data/ directory.
// Example: config.DataPath("ids.json") → <root>/data/ids.json
func DataPath(filename string) string {
	return filepath.Join(projectRoot(), "data", filename)
}
