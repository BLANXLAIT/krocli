package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"

	"github.com/blanxlait/krocli/internal/config"
	"github.com/blanxlait/krocli/internal/krogerapi"
	"github.com/blanxlait/krocli/internal/ui"
)

type AuthCmd struct {
	Login       AuthLoginCmd       `cmd:"" help:"Login via browser OAuth flow."`
	Status      AuthStatusCmd      `cmd:"" help:"Show current auth state."`
	Credentials AuthCredentialsCmd `cmd:"" help:"Manage API credentials."`
}

type AuthLoginCmd struct{}

func (c *AuthLoginCmd) Run(flags *RootFlags) error {
	creds, err := config.LoadCredentials()
	if err != nil && !errors.Is(err, config.ErrNoCredentials) {
		return err
	}
	// creds is nil in hosted mode
	return krogerapi.LoginFlow(creds, openBrowser)
}

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(flags *RootFlags) error {
	if config.IsHostedMode() {
		ui.Info("Mode: hosted")
	} else {
		ui.Info("Mode: local")
	}
	clientOK, userOK := krogerapi.AuthStatus()
	if clientOK {
		ui.Success("Client token: valid")
	} else {
		ui.Warn("Client token: not available")
	}
	if userOK {
		ui.Success("User token: valid")
	} else {
		ui.Warn("User token: not available (run: krocli auth login)")
	}
	return nil
}

type AuthCredentialsCmd struct {
	Set AuthCredentialsSetCmd `cmd:"" help:"Import credentials from a JSON file."`
}

type AuthCredentialsSetCmd struct {
	Path string `arg:"" help:"Path to JSON file with client_id and client_secret."`
}

func (c *AuthCredentialsSetCmd) Run(flags *RootFlags) error {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return err
	}
	var creds config.Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return err
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return errMissingCreds
	}
	if err := config.SaveCredentials(&creds); err != nil {
		return err
	}
	ui.Success("Credentials saved.")
	return nil
}

var errMissingCreds = errString("JSON must contain client_id and client_secret")

type errString string

func (e errString) Error() string { return string(e) }

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}
