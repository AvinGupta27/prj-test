package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

	// URLs in .env use %s as a placeholder for the environment name.
	// e.g. "https://bff.%s.munna-bhai.xyz" → "https://bff.preprod.munna-bhai.xyz"
	sub := func(key string) string {
		return strings.ReplaceAll(os.Getenv(key), "%s", env)
	}

	cfg := &Config{
		Env:           env,
		SpinnerBFFURL: sub("SPINNER_BFF_BASE_URL"),
		FcBFFURL:      sub("FC_BFF_BASE_URL"),
		FrontendURL:   sub("FRONTEND_BASE_URL"),
		ProxyURL:      sub("PROXY_URL"),
	}

	return cfg, nil
}

// DataPath returns the absolute path to a file inside the project's data/ directory.
// Example: config.DataPath("pack_buy_reveal.json") → <root>/data/pack_buy_reveal.json
func DataPath(filename string) string {
	return filepath.Join(projectRoot(), "data", filename)
}

// LoadJSON reads the JSON file at path and unmarshals it into dst.
// dst must be a pointer to a struct.
func LoadJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	return nil
}
