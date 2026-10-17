package config

import (
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("ATLAS_ENV", "local")
	t.Setenv("ATLAS_LOG_FORMAT", "text")
	t.Setenv("ATLAS_LOG_LEVEL", "debug")
	t.Setenv("ATLAS_DATABASE_URL", "postgres://user:pass@localhost:55433/atlas?sslmode=disable")
	t.Setenv("ATLAS_HTTP_ADDR", ":9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if !cfg.IsLocal() {
		t.Error("IsLocal() = false, want true")
	}
	if cfg.ReadTimeout == 0 || cfg.ShutdownGrace == 0 {
		t.Error("timeouts should have non-zero defaults")
	}
	if cfg.GitHubWebhookSecret != "" {
		t.Errorf("GitHubWebhookSecret = %q, want empty by default", cfg.GitHubWebhookSecret)
	}
}

func TestLoadReadsGitHubWebhookSecret(t *testing.T) {
	t.Setenv("ATLAS_ENV", "local")
	t.Setenv("ATLAS_DATABASE_URL", "postgres://user:pass@localhost:55433/atlas?sslmode=disable")
	t.Setenv("ATLAS_GITHUB_WEBHOOK_SECRET", "shh")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GitHubWebhookSecret != "shh" {
		t.Fatalf("GitHubWebhookSecret = %q, want shh", cfg.GitHubWebhookSecret)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ATLAS_ENV", "local")
	t.Setenv("ATLAS_DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() should fail when ATLAS_DATABASE_URL is empty")
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	t.Setenv("ATLAS_ENV", "galaxy")
	t.Setenv("ATLAS_DATABASE_URL", "postgres://x")

	if _, err := Load(); err == nil {
		t.Fatal("Load() should reject an unknown ATLAS_ENV")
	}
}

func TestLoadRequiresJSONLogsInProduction(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_LOG_FORMAT", "text")
	t.Setenv("ATLAS_DATABASE_URL", "postgres://x")

	if _, err := Load(); err == nil {
		t.Fatal("Load() should require JSON logs in production")
	}
}
