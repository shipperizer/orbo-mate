package config

import (
	"unicode/utf8"

	"github.com/kelseyhightower/envconfig"
	"github.com/shipperizer/orbo-mate/pkg/logger"
)

// Config holds all the environment variables.
type Config struct {
	WebhookSecret   string `envconfig:"GITHUB_WEBHOOK_SECRET" required:"true"`
	GitHubToken     string `envconfig:"GITHUB_TOKEN" required:"true"`
	OpenRouterKey   string `envconfig:"OPENROUTER_API_KEY" required:"true"`
	DefaultModel    string `envconfig:"DEFAULT_MODEL" default:"meta-llama/llama-3.1-70b-instruct"`
	BotName         string   `envconfig:"BOT_NAME" default:"@ai-bot"`
	Port            string   `envconfig:"PORT" default:"8080"`
	ContextSentence string   `envconfig:"CONTEXT_SENTENCE"`
	AllowedOrgs     []string `envconfig:"ALLOWED_ORGS" required:"true"`
	LogLevel        string   `envconfig:"LOG_LEVEL" default:"info"`
}

// DefaultContextSentence is the default prompt context sent to OpenRouter.
const DefaultContextSentence = "You are an expert code reviewer. Please review the following git diff payload. Highlight potential bugs, security issues, performance bottlenecks, and structural improvements."

// Load parses environment variables into the Config struct.
func Load() (*Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	if cfg.ContextSentence == "" {
		cfg.ContextSentence = DefaultContextSentence
	}

	// Validate context sentence length (limit to 500 chars)
	if utf8.RuneCountInString(cfg.ContextSentence) > 500 {
		logger.Warnf("Warning: CONTEXT_SENTENCE length is %d, which exceeds the limit of 500 characters. Truncating to 500 characters.", utf8.RuneCountInString(cfg.ContextSentence))
		runes := []rune(cfg.ContextSentence)
		cfg.ContextSentence = string(runes[:500])
	}

	return &cfg, nil
}
