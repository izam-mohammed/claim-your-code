package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fatih/color"
)

// OAuth App client ID for claim-your-code.
// This is a public client ID — safe to embed in the binary.
// Registered at: https://github.com/settings/applications
//
// The gitleaks:allow below clears the secret gate: a client ID ships inside
// every distributed binary and is useless without the user completing the
// device flow, so it is not a credential.
const oauthClientID = "Ov23liGrayvWtyIWfuvZ" // gitleaks:allow

// oauthBase is the GitHub OAuth host. Overridden in tests.
var oauthBase = "https://github.com"

// openBrowser launches the verification URL. Tests replace it so no
// browser window opens during a run.
var openBrowser = OpenBrowser

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// oauthError is an error GitHub reported through the OAuth error field, as
// opposed to a transport failure. These are terminal: the user denied the
// request, or the device code expired, and no amount of polling will help.
type oauthError struct{ code string }

func (e *oauthError) Error() string { return "oauth error: " + e.code }

// errSlowDown asks the caller to poll less often, per RFC 8628 section 3.5.
var errSlowDown = errors.New("slow_down")

// minPollInterval is the floor, in seconds, for the gap between polls. Tests
// lower it so they do not spend their run time asleep.
var minPollInterval = 5

// pollUnit is what an interval count is multiplied by. Tests shrink it.
var pollUnit = time.Second

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
	req, err := http.NewRequest("POST", oauthBase+"/login/device/code",
		strings.NewReader(url.Values{
			"client_id": {oauthClientID},
			"scope":     {"repo"},
		}.Encode()))
	if err != nil {
		return "", fmt.Errorf("device code request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	var dcr deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return "", fmt.Errorf("failed to parse device code response: %w", err)
	}

	if dcr.DeviceCode == "" || dcr.UserCode == "" {
		return "", fmt.Errorf("GitHub did not return a device code — is Device Flow enabled on the OAuth App?")
	}

	// Step 2: Show user the code and open browser
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	yellowBold := color.New(color.Bold, color.FgYellow).SprintFunc()

	fmt.Println()
	fmt.Printf("  Your code: %s\n\n", yellowBold(dcr.UserCode))

	// Auto-open browser
	if err := openBrowser(dcr.VerificationURI); err == nil {
		fmt.Printf("  %s Browser opened → %s\n", green("✓"), cyan(dcr.VerificationURI))
	} else {
		fmt.Printf("  Open this URL in your browser:\n")
		fmt.Printf("  %s\n", cyan(dcr.VerificationURI))
	}

	fmt.Printf("\n  %s", dim("Waiting for authorization..."))

	// Step 3: Poll for token
	interval := dcr.Interval
	if interval < minPollInterval {
		interval = minPollInterval
	}
	deadline := time.Now().Add(time.Duration(dcr.ExpiresIn) * pollUnit)

	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * pollUnit)

		token, err := pollForToken(dcr.DeviceCode)
		switch {
		case errors.Is(err, errSlowDown):
			// GitHub wants a longer gap between polls.
			interval += minPollInterval
			continue
		case err != nil:
			var oe *oauthError
			if errors.As(err, &oe) {
				// The user denied the request, or the code expired. Say so
				// rather than sitting here until the deadline.
				fmt.Println()
				return "", err
			}
			// A transport hiccup — keep polling until the code expires.
			continue
		}
		if token != "" {
			fmt.Printf(" %s\n", green("done!"))
			return token, nil
		}
	}

	return "", fmt.Errorf("device flow timed out")
}

func pollForToken(deviceCode string) (string, error) {
	req, err := http.NewRequest("POST", oauthBase+"/login/oauth/access_token",
		strings.NewReader(url.Values{
			"client_id":   {oauthClientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	if tr.AccessToken != "" {
		return tr.AccessToken, nil
	}
	switch tr.Error {
	case "authorization_pending":
		return "", nil // keep polling
	case "slow_down":
		return "", errSlowDown
	case "":
		return "", nil
	default:
		return "", &oauthError{code: tr.Error}
	}
}
