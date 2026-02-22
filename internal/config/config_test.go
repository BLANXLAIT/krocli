package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	// Use a temp dir as config home
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmp)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	creds := &Credentials{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
	}

	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	if loaded.ClientID != creds.ClientID {
		t.Errorf("ClientID = %q, want %q", loaded.ClientID, creds.ClientID)
	}
	if loaded.ClientSecret != creds.ClientSecret {
		t.Errorf("ClientSecret = %q, want %q", loaded.ClientSecret, creds.ClientSecret)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := LoadCredentials()
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestLoadCredentials_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".config", "krocli")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("{not json"), 0o600)

	_, err := LoadCredentials()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadCredentials_MissingFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".config", "krocli")
	_ = os.MkdirAll(dir, 0o700)
	data, _ := json.Marshal(Credentials{ClientID: "id-only"})
	_ = os.WriteFile(filepath.Join(dir, "credentials.json"), data, 0o600)

	_, err := LoadCredentials()
	if err == nil {
		t.Fatal("expected error for missing client_secret")
	}
}

func TestDir_CreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
