package config

import (
	"os"
	"testing"
)

// parseEnvs and DefaultAddr are tested here as white-box (same package).

func TestParseEnvs_valid(t *testing.T) {
	raw := "testing=https://boundary.testing.example.com=amoidc_AAA," +
		"staging=https://boundary.staging.example.com=amoidc_BBB," +
		"production=https://boundary.example.com=amoidc_CCC"

	envs, err := parseEnvs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 3 {
		t.Fatalf("want 3 envs, got %d", len(envs))
	}

	cases := []struct {
		label, addr, authMethodID string
	}{
		{"testing", "https://boundary.testing.example.com", "amoidc_AAA"},
		{"staging", "https://boundary.staging.example.com", "amoidc_BBB"},
		{"production", "https://boundary.example.com", "amoidc_CCC"},
	}
	for i, c := range cases {
		e := envs[i]
		if e.Label != c.label {
			t.Errorf("[%d] Label = %q, want %q", i, e.Label, c.label)
		}
		if e.Addr != c.addr {
			t.Errorf("[%d] Addr = %q, want %q", i, e.Addr, c.addr)
		}
		if e.AuthMethodID != c.authMethodID {
			t.Errorf("[%d] AuthMethodID = %q, want %q", i, e.AuthMethodID, c.authMethodID)
		}
	}
}

func TestParseEnvs_orderPreserved(t *testing.T) {
	raw := "z=https://z.example.com=amoidc_Z,a=https://a.example.com=amoidc_A"
	envs, err := parseEnvs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if envs[0].Label != "z" || envs[1].Label != "a" {
		t.Errorf("order not preserved: got %v, %v", envs[0].Label, envs[1].Label)
	}
}

func TestParseEnvs_trailingComma(t *testing.T) {
	raw := "testing=https://boundary.example.com=amoidc_AAA,"
	envs, err := parseEnvs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("want 1 env, got %d", len(envs))
	}
}

func TestParseEnvs_missingAuthMethodID(t *testing.T) {
	// Old 2-part format – must fail.
	_, err := parseEnvs("testing=https://boundary.example.com")
	if err == nil {
		t.Error("expected error for 2-part entry, got nil")
	}
}

func TestParseEnvs_empty(t *testing.T) {
	// An empty string produces an empty slice, not an error.
	envs, err := parseEnvs("")
	if err != nil {
		t.Errorf("parseEnvs(%q): unexpected error: %v", "", err)
	}
	if len(envs) != 0 {
		t.Errorf("parseEnvs(%q): want 0 envs, got %d", "", len(envs))
	}
}

func TestParseEnvs_emptyFields(t *testing.T) {
	cases := []string{
		"=https://boundary.example.com=amoidc_AAA", // empty label
		"testing==amoidc_AAA",                      // empty addr
		"testing=https://boundary.example.com=",    // empty auth method ID
	}
	for _, raw := range cases {
		_, err := parseEnvs(raw)
		if err == nil {
			t.Errorf("parseEnvs(%q): expected error, got nil", raw)
		}
	}
}

func TestDefaultAddr(t *testing.T) {
	cfg := Config{
		DefaultEnv: "staging",
		Envs: []Env{
			{Label: "testing", Addr: "https://testing.example.com", AuthMethodID: "amoidc_T"},
			{Label: "staging", Addr: "https://staging.example.com", AuthMethodID: "amoidc_S"},
			{Label: "production", Addr: "https://production.example.com", AuthMethodID: "amoidc_P"},
		},
	}

	t.Run("returns matching env addr", func(t *testing.T) {
		got := cfg.DefaultAddr()
		if got != "https://staging.example.com" {
			t.Errorf("DefaultAddr() = %q, want %q", got, "https://staging.example.com")
		}
	})

	t.Run("falls back to first when default not found", func(t *testing.T) {
		cfg2 := cfg
		cfg2.DefaultEnv = "nonexistent"
		got := cfg2.DefaultAddr()
		if got != "https://testing.example.com" {
			t.Errorf("DefaultAddr() = %q, want first entry %q", got, "https://testing.example.com")
		}
	})

	t.Run("returns empty string when no envs", func(t *testing.T) {
		cfg3 := Config{DefaultEnv: "testing", Envs: nil}
		got := cfg3.DefaultAddr()
		if got != "" {
			t.Errorf("DefaultAddr() = %q, want empty", got)
		}
	})
}

func TestLoad_noEnvVar(t *testing.T) {
	// Ensure BOUNDARY_ENVS is unset so Load returns with no environments.
	os.Unsetenv("BOUNDARY_ENVS")
	os.Unsetenv("BOUNDARY_DEFAULT_ENV")
	os.Unsetenv("BOUNDARY_USER")

	if err := Load(); err != nil {
		t.Fatalf("Load() with no BOUNDARY_ENVS: unexpected error: %v", err)
	}
	if len(Cfg.Envs) != 0 {
		t.Errorf("Load() with no BOUNDARY_ENVS: want 0 envs, got %d", len(Cfg.Envs))
	}
}

func TestLoad_subsetOfEnvs(t *testing.T) {
	// Only production – no staging, no testing.
	os.Setenv("BOUNDARY_ENVS", "production=https://boundary.example.com=amoidc_CCC")
	os.Unsetenv("BOUNDARY_DEFAULT_ENV")
	t.Cleanup(func() {
		os.Unsetenv("BOUNDARY_ENVS")
		os.Unsetenv("BOUNDARY_DEFAULT_ENV")
	})

	if err := Load(); err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if len(Cfg.Envs) != 1 {
		t.Fatalf("want 1 env, got %d", len(Cfg.Envs))
	}
	if Cfg.Envs[0].Label != "production" {
		t.Errorf("want label %q, got %q", "production", Cfg.Envs[0].Label)
	}
	// Default should be auto-selected to the first (and only) env.
	if Cfg.DefaultEnv != "production" {
		t.Errorf("want DefaultEnv %q, got %q", "production", Cfg.DefaultEnv)
	}
}
