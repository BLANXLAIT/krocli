package krogerapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blanxlait/krocli/internal/config"
	"github.com/blanxlait/krocli/internal/secrets"
	"github.com/blanxlait/krocli/internal/ui"
	"golang.org/x/oauth2"
)

const (
	tokenKeyClient = "client_credentials"
	tokenKeyUser   = "authorization_code"
	authURL        = "https://api.kroger.com/v1/connect/oauth2/authorize"
)

// tokenURL is a var so tests can override it with httptest.Server URLs.
var tokenURL = "https://api.kroger.com/v1/connect/oauth2/token"

// tokenHTTPClient is the HTTP client used for token exchange. Tests override this.
var tokenHTTPClient = http.DefaultClient

func oauthConfig(creds *config.Credentials, redirectURL string, scopes ...string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
		RedirectURL: redirectURL,
		Scopes:      scopes,
	}
}

func ClearClientToken() {
	_ = secrets.DeleteToken(tokenKeyClient)
}

func GetClientToken(creds *config.Credentials) (*oauth2.Token, error) {
	td, err := secrets.LoadToken(tokenKeyClient)
	if err == nil && td.Expiry.After(time.Now()) {
		return &oauth2.Token{
			AccessToken: td.AccessToken,
			TokenType:   td.TokenType,
			Expiry:      td.Expiry,
		}, nil
	}

	var tok *oauth2.Token
	if creds == nil {
		tok, err = hostedClientCredentialsExchange()
	} else {
		tok, err = clientCredentialsExchange(creds, "product.compact")
	}
	if err != nil {
		return nil, fmt.Errorf("client credentials exchange: %w", err)
	}

	_ = secrets.StoreToken(tokenKeyClient, &secrets.TokenData{
		AccessToken: tok.AccessToken,
		TokenType:   tok.TokenType,
		Expiry:      tok.Expiry,
	})
	return tok, nil
}

func clientCredentialsExchange(creds *config.Credentials, scope string) (*oauth2.Token, error) {
	data := url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {scope},
	}
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(creds.ClientID, creds.ClientSecret)

	resp, err := tokenHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	return &oauth2.Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		Expiry:      time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

func GetUserToken(creds *config.Credentials) (*oauth2.Token, error) {
	td, err := secrets.LoadToken(tokenKeyUser)
	if err != nil {
		return nil, fmt.Errorf("not logged in; run: krocli auth login")
	}
	if td.Expiry.After(time.Now()) {
		return &oauth2.Token{
			AccessToken:  td.AccessToken,
			RefreshToken: td.RefreshToken,
			TokenType:    td.TokenType,
			Expiry:       td.Expiry,
		}, nil
	}
	if td.RefreshToken == "" {
		return nil, fmt.Errorf("token expired and no refresh token; run: krocli auth login")
	}

	var tok *oauth2.Token
	if creds == nil {
		tok, err = hostedRefreshToken(td.RefreshToken)
	} else {
		cfg := oauthConfig(creds, "", "cart.basic:write", "profile.compact")
		tok, err = cfg.TokenSource(context.Background(), &oauth2.Token{RefreshToken: td.RefreshToken}).Token()
	}
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w; run: krocli auth login", err)
	}

	_ = secrets.StoreToken(tokenKeyUser, &secrets.TokenData{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		Expiry:       tok.Expiry,
	})
	return tok, nil
}

func LoginFlow(creds *config.Credentials, openURL func(string) error) error {
	if creds == nil {
		return hostedLoginFlow(openURL)
	}
	const callbackPort = 8080
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", callbackPort)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return fmt.Errorf("listen on port %d (is something else using it?): %w", callbackPort, err)
	}

	cfg := oauthConfig(creds, redirectURL, "cart.basic:write", "profile.compact")

	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	ui.Info("Opening browser for Kroger login...")
	if err := openURL(url); err != nil {
		ui.Warn("Could not open browser. Visit this URL:\n%s", url)
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "no code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = callbackTmpl.Execute(w, nil)
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		_ = srv.Close()
		return err
	case <-time.After(2 * time.Minute):
		_ = srv.Close()
		return fmt.Errorf("login timed out")
	}
	_ = srv.Close()

	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}

	return secrets.StoreToken(tokenKeyUser, &secrets.TokenData{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		Expiry:       tok.Expiry,
	})
}

// --- hosted mode functions ---

func hostedClientCredentialsExchange() (*oauth2.Token, error) {
	resp, err := tokenHTTPClient.Post(config.ProxyBaseURL+"/tokenClient", "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hosted token endpoint %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		Expiry:      time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

func hostedLoginFlow(openURL func(string) error) error {
	sessionID := generateSessionID()

	loginURL := config.ProxyBaseURL + "/authorize?session_id=" + sessionID + "&source=cli"

	ui.Info("Opening browser for Kroger login...")
	if err := openURL(loginURL); err != nil {
		ui.Warn("Could not open browser. Visit this URL:\n%s", loginURL)
	}

	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("login timed out")
		case <-ticker.C:
			tok, err := pollForUserToken(sessionID)
			if err != nil {
				continue // still pending
			}
			return secrets.StoreToken(tokenKeyUser, &secrets.TokenData{
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				TokenType:    tok.TokenType,
				Expiry:       tok.Expiry,
			})
		}
	}
}

func pollForUserToken(sessionID string) (*oauth2.Token, error) {
	resp, err := tokenHTTPClient.Get(config.ProxyBaseURL + "/tokenUser?session_id=" + sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 202 {
		return nil, fmt.Errorf("pending")
	}

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hosted user token %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

func hostedRefreshToken(refreshToken string) (*oauth2.Token, error) {
	payload, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	resp, err := tokenHTTPClient.Post(config.ProxyBaseURL+"/tokenRefresh", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hosted refresh %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var callbackTmpl = template.Must(template.New("callback").Parse(callbackHTML))

const callbackHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>krocli — Logged In</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: #0f1117;
    color: #e1e4e8;
    display: flex;
    justify-content: center;
    padding: 3rem 1rem;
    line-height: 1.6;
  }
  .container { max-width: 640px; width: 100%; }
  .card {
    background: #161b22;
    border: 1px solid #30363d;
    border-radius: 12px;
    padding: 2.5rem;
    text-align: center;
    margin-bottom: 2rem;
  }
  .checkmark {
    width: 64px; height: 64px;
    margin: 0 auto 1.25rem;
    background: #238636;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    animation: pop 0.4s ease;
  }
  @keyframes pop {
    0% { transform: scale(0); }
    70% { transform: scale(1.15); }
    100% { transform: scale(1); }
  }
  .checkmark svg { width: 32px; height: 32px; }
  h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.5rem; }
  .subtitle { color: #8b949e; font-size: 0.95rem; }
  h2 {
    font-size: 1.1rem;
    font-weight: 600;
    margin-bottom: 1rem;
    color: #c9d1d9;
    text-align: left;
  }
  .commands {
    background: #161b22;
    border: 1px solid #30363d;
    border-radius: 12px;
    padding: 1.5rem;
    text-align: left;
  }
  .cmd-group { margin-bottom: 1.25rem; }
  .cmd-group:last-child { margin-bottom: 0; }
  .cmd-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #8b949e;
    margin-bottom: 0.4rem;
  }
  .cmd {
    background: #0d1117;
    border: 1px solid #21262d;
    border-radius: 6px;
    padding: 0.6rem 0.85rem;
    font-family: "SF Mono", "Fira Code", "Fira Mono", Menlo, monospace;
    font-size: 0.85rem;
    color: #79c0ff;
    overflow-x: auto;
    white-space: nowrap;
  }
  .cmd .flag { color: #d2a8ff; }
  .cmd .str { color: #a5d6ff; }
  .cmd .comment { color: #8b949e; }
  .close-hint {
    text-align: center;
    color: #484f58;
    font-size: 0.8rem;
    margin-top: 1.5rem;
  }
</style>
</head>
<body>
<div class="container">
  <div class="card">
    <div class="checkmark">
      <svg fill="none" viewBox="0 0 24 24" stroke="white" stroke-width="3">
        <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
      </svg>
    </div>
    <h1>You&#39;re logged in!</h1>
    <p class="subtitle">krocli is authenticated with your Kroger account.</p>
  </div>

  <div class="commands">
    <h2>What you can do now</h2>

    <div class="cmd-group">
      <div class="cmd-label">Search products</div>
      <div class="cmd">krocli products search <span class="str">"organic milk"</span></div>
    </div>

    <div class="cmd-group">
      <div class="cmd-label">Find nearby stores</div>
      <div class="cmd">krocli locations search <span class="flag">--near</span> <span class="str">"45202"</span></div>
    </div>

    <div class="cmd-group">
      <div class="cmd-label">View your cart</div>
      <div class="cmd">krocli cart list</div>
    </div>

    <div class="cmd-group">
      <div class="cmd-label">Add to cart</div>
      <div class="cmd">krocli cart add <span class="flag">--upc</span> <span class="str">0001111042010</span> <span class="flag">--qty</span> 2</div>
    </div>

    <div class="cmd-group">
      <div class="cmd-label">Check your profile</div>
      <div class="cmd">krocli identity profile</div>
    </div>

    <div class="cmd-group">
      <div class="cmd-label">Output as JSON</div>
      <div class="cmd">krocli <span class="flag">-j</span> products search <span class="str">"bread"</span> <span class="comment"># pipe to jq, etc.</span></div>
    </div>
  </div>

  <p class="close-hint">You can close this tab and return to your terminal.</p>
</div>
</body>
</html>`

func AuthStatus() (clientOK, userOK bool) {
	if td, err := secrets.LoadToken(tokenKeyClient); err == nil && td.Expiry.After(time.Now()) {
		clientOK = true
	}
	if td, err := secrets.LoadToken(tokenKeyUser); err == nil && (td.Expiry.After(time.Now()) || td.RefreshToken != "") {
		userOK = true
	}
	return
}
