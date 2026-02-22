package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthCredentialsSetCmd_Run(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Write a valid creds file
	credsFile := filepath.Join(tmp, "creds.json")
	data, _ := json.Marshal(map[string]string{
		"client_id":     "test-id",
		"client_secret": "test-secret",
	})
	os.WriteFile(credsFile, data, 0o600)

	cmd := &AuthCredentialsSetCmd{Path: credsFile}
	flags := &RootFlags{}
	if err := cmd.Run(flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify saved
	saved, err := os.ReadFile(filepath.Join(tmp, ".config", "krocli", "credentials.json"))
	if err != nil {
		t.Fatalf("read saved: %v", err)
	}
	var creds map[string]string
	json.Unmarshal(saved, &creds)
	if creds["client_id"] != "test-id" {
		t.Errorf("client_id = %q", creds["client_id"])
	}
}

func TestAuthCredentialsSetCmd_MissingFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	credsFile := filepath.Join(tmp, "creds.json")
	os.WriteFile(credsFile, []byte(`{"client_id":"only-id"}`), 0o600)

	cmd := &AuthCredentialsSetCmd{Path: credsFile}
	flags := &RootFlags{}
	if err := cmd.Run(flags); err == nil {
		t.Fatal("expected error for missing client_secret")
	}
}

func TestAuthCredentialsSetCmd_BadPath(t *testing.T) {
	cmd := &AuthCredentialsSetCmd{Path: "/nonexistent/creds.json"}
	flags := &RootFlags{}
	if err := cmd.Run(flags); err == nil {
		t.Fatal("expected error for missing file")
	}
}
