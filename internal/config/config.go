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
