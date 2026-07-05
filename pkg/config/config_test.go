package config

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLoad_Success(t *testing.T) {
	os.Setenv("GITHUB_WEBHOOK_SECRET", "super-secret")
	os.Setenv("GITHUB_TOKEN", "token-123")
	os.Setenv("OPENROUTER_API_KEY", "key-xyz")
	os.Setenv("ALLOWED_ORGS", "test-org,another-org")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("GITHUB_WEBHOOK_SECRET")
		os.Unsetenv("GITHUB_TOKEN")
		os.Unsetenv("OPENROUTER_API_KEY")
		os.Unsetenv("ALLOWED_ORGS")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if cfg.WebhookSecret != "super-secret" {
		t.Errorf("Expected WebhookSecret to be 'super-secret', got %s", cfg.WebhookSecret)
	}
	if cfg.GitHubToken != "token-123" {
		t.Errorf("Expected GitHubToken to be 'token-123', got %s", cfg.GitHubToken)
	}
	if cfg.OpenRouterKey != "key-xyz" {
		t.Errorf("Expected OpenRouterKey to be 'key-xyz', got %s", cfg.OpenRouterKey)
	}
	if cfg.DefaultModel != "meta-llama/llama-3.1-70b-instruct" {
		t.Errorf("Expected DefaultModel to be default, got %s", cfg.DefaultModel)
	}
	if cfg.ContextSentence != DefaultContextSentence {
		t.Errorf("Expected ContextSentence to be default, got %s", cfg.ContextSentence)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel to be 'debug', got %s", cfg.LogLevel)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("Expected MaxTokens to be 4096, got %d", cfg.MaxTokens)
	}
}

func TestLoad_ContextSentenceTruncation(t *testing.T) {
	os.Setenv("GITHUB_WEBHOOK_SECRET", "secret")
	os.Setenv("GITHUB_TOKEN", "token")
	os.Setenv("OPENROUTER_API_KEY", "key")
	os.Setenv("ALLOWED_ORGS", "test-org")

	longSentence := strings.Repeat("A", 600)
	os.Setenv("CONTEXT_SENTENCE", longSentence)

	defer func() {
		os.Unsetenv("GITHUB_WEBHOOK_SECRET")
		os.Unsetenv("GITHUB_TOKEN")
		os.Unsetenv("OPENROUTER_API_KEY")
		os.Unsetenv("CONTEXT_SENTENCE")
		os.Unsetenv("ALLOWED_ORGS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	charCount := utf8.RuneCountInString(cfg.ContextSentence)
	if charCount != 500 {
		t.Errorf("Expected truncated ContextSentence to be 500 characters, got %d", charCount)
	}
}
