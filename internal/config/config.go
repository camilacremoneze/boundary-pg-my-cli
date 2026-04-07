// Package config loads runtime configuration from a .env file and environment
// variables.  Call Load() once at program startup; all other packages read from
// the exported Cfg singleton.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Env pairs a human-readable label with a Boundary controller URL and OIDC auth method ID.
type Env struct {
	Label        string
	Addr         string
	AuthMethodID string
}

// Config holds all runtime configuration values.
type Config struct {
	// Envs is the ordered list of Boundary environments available in the UI.
	Envs []Env

	// DefaultEnv is the label of the environment selected on startup.
	DefaultEnv string

	// BoundaryUser is the SSO login hint shown in the UI.
	BoundaryUser string
}

// Cfg is the global singleton populated by Load.
var Cfg Config

// Load reads .env (if present) then overrides with real environment variables.
// It is safe to call multiple times; subsequent calls re-parse everything.
func Load() error {
	// .env is optional – silently ignore if missing.
	_ = godotenv.Load()

	Cfg.DefaultEnv = env("BOUNDARY_DEFAULT_ENV", "")
	Cfg.BoundaryUser = env("BOUNDARY_USER", "")

	raw := os.Getenv("BOUNDARY_ENVS")
	if raw == "" {
		Cfg.Envs = nil
		return nil
	}
	envs, err := parseEnvs(raw)
	if err != nil {
		return fmt.Errorf("config: BOUNDARY_ENVS: %w", err)
	}
	Cfg.Envs = envs
	// If no default was set explicitly, pick the first environment.
	if Cfg.DefaultEnv == "" && len(envs) > 0 {
		Cfg.DefaultEnv = envs[0].Label
	}
	return nil
}

// DefaultAddr returns the controller URL for the configured default environment.
// Falls back to the first entry if the label is not found.
func (c *Config) DefaultAddr() string {
	for _, e := range c.Envs {
		if e.Label == c.DefaultEnv {
			return e.Addr
		}
	}
	if len(c.Envs) > 0 {
		return c.Envs[0].Addr
	}
	return ""
}

// ── helpers ──────────────────────────────────────────────────────────────────

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseEnvs parses "label=url=amoidc_ID,…" into a slice of Env.
// The URL may contain "://" but no bare "="; the auth method ID never contains "=".
// Strategy: split on "=", take parts[0] as label, parts[len-1] as auth method ID,
// and join parts[1:len-1] as the URL (safe because https://… contains no "=").
func parseEnvs(raw string) ([]Env, error) {
	var out []Env
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.Split(pair, "=")
		if len(parts) < 3 {
			return nil, fmt.Errorf("expected label=url=amoidc_ID, got %q", pair)
		}
		label := strings.TrimSpace(parts[0])
		authMethodID := strings.TrimSpace(parts[len(parts)-1])
		addr := strings.TrimSpace(strings.Join(parts[1:len(parts)-1], "="))
		if label == "" || addr == "" || authMethodID == "" {
			return nil, fmt.Errorf("empty field in %q", pair)
		}
		out = append(out, Env{Label: label, Addr: addr, AuthMethodID: authMethodID})
	}
	return out, nil
}
