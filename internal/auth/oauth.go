package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth App client ID for claim-your-code.
// This is a public client ID — it's safe to embed in the binary.
// To use OAuth device flow, register a GitHub OAuth App and put its client ID here.
// For now, this is a placeholder — OAuth will gracefully fail and fall through to prompt.
const oauthClientID = ""

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

// DeviceFlow runs the GitHub OAuth 2.0 Device Authorization Grant.
// Returns an access token or error.
func DeviceFlow() (string, error) {
	if oauthClientID == "" {
		return "", fmt.Errorf("OAuth not configured")
	}

	// Step 1: Request device code
	resp, err := http.PostForm("https://github.com/login/device/code", url.Values{
		"client_id": {oauthClientID},
		"scope":     {"repo"},
	})
	if err != nil {
		return "", fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	var dcr deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return "", fmt.Errorf("failed to parse device code response: %w", err)
	}

	// Step 2: Show user the code
	fmt.Println()
	fmt.Printf("  To authenticate, open this URL in your browser:\n")
	fmt.Printf("  %s\n\n", dcr.VerificationURI)
	fmt.Printf("  And enter code: %s\n\n", dcr.UserCode)
	fmt.Printf("  Waiting for authorization...")

	// Step 3: Poll for token
	interval := dcr.Interval
	if interval < 5 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(dcr.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		token, err := pollForToken(dcr.DeviceCode)
		if err != nil {
			continue
		}
		if token != "" {
			fmt.Println(" done!")
			return token, nil
		}
	}

	return "", fmt.Errorf("device flow timed out")
}

func pollForToken(deviceCode string) (string, error) {
	resp, err := http.PostForm("https://github.com/login/oauth/access_token", url.Values{
		"client_id":   {oauthClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// GitHub returns form-encoded by default, request JSON
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])

	// Try JSON first
	var tr tokenResponse
	if err := json.NewDecoder(strings.NewReader(bodyStr)).Decode(&tr); err == nil {
		if tr.AccessToken != "" {
			return tr.AccessToken, nil
		}
		if tr.Error == "authorization_pending" {
			return "", nil // keep polling
		}
		if tr.Error == "slow_down" {
			return "", nil // keep polling, interval will be longer
		}
		if tr.Error != "" {
			return "", fmt.Errorf("oauth error: %s", tr.Error)
		}
	}

	// Try form-encoded
	values, err := url.ParseQuery(bodyStr)
	if err == nil {
		if token := values.Get("access_token"); token != "" {
			return token, nil
		}
		if errVal := values.Get("error"); errVal == "authorization_pending" || errVal == "slow_down" {
			return "", nil
		}
	}

	return "", nil
}
