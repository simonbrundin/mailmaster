package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the mail agent
type Config struct {
	Mail     []MailAccount  `yaml:"mail"`
	AI       AIConfig       `yaml:"ai"`
	Rules    RulesConfig    `yaml:"rules"`
	Logging  LoggingConfig  `yaml:"logging"`
	Fallback FallbackConfig `yaml:"fallback"`
}

// MailAccount holds settings for a single mail account.
type MailAccount struct {
	Name             string        `yaml:"name"` // Display name (e.g., "Sales", "Michael")
	Server           string        `yaml:"server"`
	Port             int           `yaml:"port"`
	SMTPServer       string        `yaml:"smtp_server"`        // Optional SMTP hostname
	SMTPPort         int           `yaml:"smtp_port"`          // Defaults to 587
	Username         string        `yaml:"username"`           // Login username (e.g., michael.brundin@soulmate.se)
	PasswordEnv      string        `yaml:"password_env"`       // Environment variable for password/token
	AzureClientID    string        `yaml:"azure_client_id"`    // Azure AD app client ID (for OAuth2)
	AzureTenantID    string        `yaml:"azure_tenant_id"`    // Azure AD tenant ID (for OAuth2)
	OAuthRedirectURI string        `yaml:"oauth_redirect_uri"` // Local callback for interactive login
	OAuthCachePath   string        `yaml:"oauth_cache_path"`   // Optional token cache path
	Folder           string        `yaml:"folder"`             // Default folder to check
	SharedMailbox    string        `yaml:"shared_mailbox"`     // For Microsoft 365 shared mailboxes
	CheckInterval    time.Duration `yaml:"check_interval"`
	UseTLS           bool          `yaml:"use_tls"`
	UseOAuth2        bool          `yaml:"use_oauth2"` // Use OAuth2 instead of basic auth
}

// AIConfig holds AI model settings
type AIConfig struct {
	Provider       string `yaml:"provider"`        // "openai-compatible" for Luna
	Endpoint      string `yaml:"endpoint"`       // e.g., "http://localhost:11434/v1"
	Model         string `yaml:"model"`          // e.g., "gpt-5.6-luna"
	ReasoningEffort string `yaml:"reasoning_effort"` // "low", "medium", "high" (GPT-5.6 Luna)
	APIKeyEnv     string `yaml:"api_key_env"`
}

// RulesConfig holds rule file settings
type RulesConfig struct {
	File string `yaml:"file"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// FallbackConfig holds fallback settings for uncertain AI decisions
type FallbackConfig struct {
	Enabled     bool   `yaml:"enabled"`
	NotifyEmail string `yaml:"notify_email"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults for each mail account
	for i := range cfg.Mail {
		if cfg.Mail[i].Port == 0 {
			cfg.Mail[i].Port = 993
		}
		if cfg.Mail[i].Folder == "" {
			cfg.Mail[i].Folder = "INBOX"
		}
		if cfg.Mail[i].CheckInterval == 0 {
			cfg.Mail[i].CheckInterval = 5 * time.Minute
		}
	}

	if cfg.AI.Endpoint == "" {
		cfg.AI.Endpoint = "http://localhost:11434/v1"
	}

	for i := range cfg.Mail {
		if cfg.Mail[i].UseOAuth2 && cfg.Mail[i].OAuthRedirectURI == "" {
			cfg.Mail[i].OAuthRedirectURI = "http://localhost:8400/callback"
		}
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}

	return &cfg, nil
}

// GetMailPassword returns the mail password from environment variable for first account
func (c *Config) GetMailPassword() (string, error) {
	if len(c.Mail) == 0 {
		return "", fmt.Errorf("no mail accounts configured")
	}
	return c.Mail[0].GetPassword()
}

// GetPassword returns the password for this mail account from environment variable
func (m *MailAccount) GetPassword() (string, error) {
	val, ok := os.LookupEnv(m.PasswordEnv)
	if !ok {
		return "", fmt.Errorf("environment variable %s not set", m.PasswordEnv)
	}
	return val, nil
}

// GetSMTPServer returns the configured SMTP server, with provider defaults.
func (m *MailAccount) GetSMTPServer() string {
	if m.SMTPServer != "" {
		return m.SMTPServer
	}
	if m.UseOAuth2 {
		return "smtp.office365.com"
	}
	if m.Server == "imap.one.com" {
		return "send.one.com" // one.com SMTP relay (IPv4: 46.30.211.141)
	}
	return m.Server
}

// GetSMTPPort returns the configured SMTP port, defaulting to submission/TLS.
func (m *MailAccount) GetSMTPPort() int {
	if m.SMTPPort == 0 {
		return 587
	}
	return m.SMTPPort
}

// GetAPIKey returns the AI API key from environment variable
func (c *Config) GetAPIKey() (string, error) {
	if c.AI.APIKeyEnv == "" {
		return "", nil
	}
	val, ok := os.LookupEnv(c.AI.APIKeyEnv)
	if !ok {
		return "", nil // API key is optional
	}
	return val, nil
}
