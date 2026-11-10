package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	// Ensure relevant env vars are clear
	os.Unsetenv("TELEGRAM_APP_ID")
	os.Unsetenv("STORAGE_DSN")
	os.Unsetenv("STORAGE_DRIVER")
	os.Unsetenv("GEMINI_API_KEY")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("expected default driver to be sqlite, got %s", cfg.Storage.Driver)
	}

	if cfg.HasLocalAI() {
		t.Errorf("expected HasLocalAI to be false without GEMINI_API_KEY")
	}

	home, _ := os.UserHomeDir()
	expectedDB := filepath.Join(home, ".config/telegram-feed/feed.db")
	if cfg.Storage.DSN != expectedDB {
		t.Errorf("expected DSN %s, got %s", expectedDB, cfg.Storage.DSN)
	}
}

func TestPostgresAutoDetection(t *testing.T) {
	os.Setenv("STORAGE_DSN", "postgres://user:pass@localhost:5432/tf_db?sslmode=disable")
	defer os.Unsetenv("STORAGE_DSN")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Storage.Driver != "postgres" {
		t.Errorf("expected driver to be auto-detected as postgres, got %s", cfg.Storage.Driver)
	}

	if cfg.Storage.DSN != "postgres://user:pass@localhost:5432/tf_db?sslmode=disable" {
		t.Errorf("expected postgres DSN to remain intact, got %s", cfg.Storage.DSN)
	}
}

func TestGeminiCapabilityCheck(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "test-api-key-12345")
	defer os.Unsetenv("GEMINI_API_KEY")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.HasLocalAI() {
		t.Errorf("expected HasLocalAI to be true when GEMINI_API_KEY is set")
	}
}
