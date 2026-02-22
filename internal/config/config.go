package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const ProxyBaseURL = "https://us-central1-krocli.cloudfunctions.net"

var ErrNoCredentials = errors.New("no credentials file found")

func IsHostedMode() bool {
	path, err := CredentialsPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "krocli")
	return dir, os.MkdirAll(dir, 0o700)
}

func CredentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func LoadCredentials() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoCredentials
		}
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return nil, fmt.Errorf("credentials file missing client_id or client_secret")
	}
	return &c, nil
}

func SaveCredentials(c *Credentials) error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ErrNoTelegramConfig is returned when no Telegram configuration file is found.
var ErrNoTelegramConfig = errors.New("no telegram config found")

// TelegramConfig holds the Telegram Bot API credentials for sending login URLs.
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

func TelegramConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "telegram.json"), nil
}

func LoadTelegramConfig() (*TelegramConfig, error) {
	path, err := TelegramConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoTelegramConfig
		}
		return nil, err
	}
	var c TelegramConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.BotToken == "" || c.ChatID == "" {
		return nil, fmt.Errorf("telegram config missing bot_token or chat_id")
	}
	return &c, nil
}

func SaveTelegramConfig(c *TelegramConfig) error {
	path, err := TelegramConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// OpenClawEnvBotToken and OpenClawEnvChatID are the environment variables set
// by an OpenClaw bot orchestrator to enable native integration mode.
const OpenClawEnvBotToken = "OPENCLAW_BOT_TOKEN"
const OpenClawEnvChatID = "OPENCLAW_CHAT_ID"

// OpenClawIntegration returns a TelegramConfig sourced from environment
// variables when krocli is running under an OpenClaw bot orchestrator.
// It returns nil when the required environment variables are not set,
// signalling that the caller should fall back to standalone behaviour.
func OpenClawIntegration() *TelegramConfig {
	token := os.Getenv(OpenClawEnvBotToken)
	chatID := os.Getenv(OpenClawEnvChatID)
	if token == "" || chatID == "" {
		return nil
	}
	return &TelegramConfig{BotToken: token, ChatID: chatID}
}
