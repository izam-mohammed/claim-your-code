package auth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
)

var (
	cyan   = color.New(color.FgCyan).SprintFunc()
	green  = color.New(color.FgGreen).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	dim    = color.New(color.Faint).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
)

// apiBase is the GitHub API root. Overridden in tests.
var apiBase = "https://api.github.com"

// The prompts and input source below are variables so tests can drive the
// interactive flows without a terminal.

// selectOption asks the user to pick one option.
var selectOption = func(title string, options []huh.Option[string]) (string, error) {
	var choice string
	err := huh.NewSelect[string]().
		Title(title).
		Options(options...).
		Value(&choice).
		Run()
	return choice, err
}

// confirmPrompt asks a yes/no question, answering no if it cannot run.
var confirmPrompt = func(title, affirmative, negative string) bool {
	var yes bool
	err := huh.NewConfirm().
		Title(title).
		Affirmative(affirmative).
		Negative(negative).
		Value(&yes).
		Run()
	if err != nil {
		return false
	}
	return yes
}

// promptInput is where promptPAT reads a pasted token from.
var promptInput io.Reader = os.Stdin

type foundToken struct {
	token  string
	source string
	user   string
}

// GetToken finds or prompts for a GitHub token.
func GetToken() (string, error) {
	var found []foundToken

	// 1. Saved accounts (multi-account store)
	accounts := LoadAllAccounts()
	for _, a := range accounts {
		// EncryptedToken temporarily holds decrypted token from LoadAllAccounts
		token := a.EncryptedToken
		if validateAndGetUser(token) != "" {
			found = append(found, foundToken{token, "saved", a.Username})
		}
	}

	// 2. Env vars
	addEnvToken(&found, "CLAIM_GITHUB_TOKEN")
	addEnvToken(&found, "GITHUB_TOKEN")

	// 3. gh CLI
	if ghCLIAvailable() {
		if token, err := ghAuthToken(); err == nil && token != "" {
			if user := validateAndGetUser(token); user != "" {
				if !isDuplicate(found, token) {
					found = append(found, foundToken{token, "gh CLI", user})
				}
			}
		}
	}

	// If tokens found, let user pick
	if len(found) > 0 {
		options := make([]huh.Option[string], 0, len(found)+2)
		for i, f := range found {
			options = append(options, huh.NewOption(
				fmt.Sprintf("@%s (%s)", f.user, f.source),
				fmt.Sprintf("found_%d", i),
			))
		}
		options = append(options, huh.NewOption("Add another account", "other"))
		options = append(options, huh.NewOption("Continue without auth (public repos only)", "public"))

		choice, err := selectOption("GitHub account", options)
		if err != nil {
			return "", fmt.Errorf("cancelled")
		}

		if choice == "public" {
			fmt.Printf("  %s Continuing without authentication\n", dim("→"))
			return "", nil
		}
		if choice != "other" {
			var idx int
			fmt.Sscanf(choice, "found_%d", &idx)
			if idx >= 0 && idx < len(found) {
				f := found[idx]
				fmt.Printf("  %s Authenticated as %s\n", green("✓"), bold("@"+f.user))
				return f.token, nil
			}
		}
	}

	// No accounts or user wants to add another
	return promptForAuth()
}

func addEnvToken(found *[]foundToken, envVar string) {
	token := os.Getenv(envVar)
	if token == "" {
		return
	}
	if user := validateAndGetUser(token); user != "" {
		if !isDuplicate(*found, token) {
			*found = append(*found, foundToken{token, envVar, user})
		}
	}
}

func isDuplicate(found []foundToken, token string) bool {
	for _, f := range found {
		if f.token == token {
			return true
		}
	}
	return false
}

// promptForAuth shows the interactive method selection and saves the account.
func promptForAuth() (string, error) {
	options := []huh.Option[string]{
		huh.NewOption("GitHub OAuth — opens browser, one-click auth (Recommended)", "oauth"),
	}
	if ghCLIAvailable() {
		options = append(options, huh.NewOption("gh CLI — use existing gh auth session", "gh"))
	}
	options = append(options, huh.NewOption("Personal Access Token — paste a token manually", "pat"))

	method, err := selectOption("Choose authentication method", options)
	if err != nil {
		return "", fmt.Errorf("auth selection cancelled")
	}

	switch method {
	case "oauth":
		token, err := DeviceFlow()
		if err != nil {
			return "", fmt.Errorf("OAuth failed: %w", err)
		}
		saveTokenWithUser(token, "oauth")
		return token, nil

	case "gh":
		token, err := ghAuthToken()
		if err != nil || token == "" {
			return "", fmt.Errorf("failed to get token from gh CLI: %w", err)
		}
		scopes, err := ValidateTokenScopes(token)
		if err != nil {
			return "", fmt.Errorf("gh CLI token is invalid: %w", err)
		}
		if !HasRepoScope(scopes) {
			fmt.Printf("\n  %s gh CLI token is missing the %s scope.\n", yellow("⚠"), cyan("repo"))
			fmt.Printf("  Run: %s\n", cyan("gh auth refresh -s repo"))
		}
		saveTokenWithUser(token, "gh")
		return token, nil

	case "pat":
		return promptPAT()

	default:
		return "", fmt.Errorf("invalid choice")
	}
}

// saveTokenWithUser validates, gets the username, and saves the account.
func saveTokenWithUser(token, method string) {
	user := validateAndGetUser(token)
	if user != "" {
		_ = SaveAccount(user, token, method)
		fmt.Printf("  %s Authenticated as %s\n", green("✓"), bold("@"+user))
	}
}

func promptPAT() (string, error) {
	reader := bufio.NewReader(promptInput)

	for {
		fmt.Println()
		fmt.Printf("  Create a token at: %s\n", cyan("https://github.com/settings/tokens"))
		fmt.Printf("  Required scope: %s\n", cyan("repo"))
		fmt.Println()
		fmt.Print("  Enter token: ")
		token, _ := reader.ReadString('\n')
		token = strings.TrimSpace(token)
		if token == "" {
			return "", fmt.Errorf("no token provided")
		}

		scopes, err := ValidateTokenScopes(token)
		if err != nil {
			fmt.Printf("\n  %s Token is invalid: %v\n", red("✗"), err)
			if !confirmPrompt("Try another token?", "Yes", "No") {
				return "", fmt.Errorf("invalid token")
			}
			continue
		}

		if !HasRepoScope(scopes) {
			fmt.Printf("\n  %s Token is missing the %s scope.\n", yellow("⚠"), cyan("repo"))
			fmt.Printf("  Current scopes: %s\n", dim(strings.Join(scopes, ", ")))
			fmt.Printf("  The %s scope is required to access repository data.\n", cyan("repo"))
			if confirmPrompt("Try another token with the correct scope?", "Yes, enter new token", "No, use this token anyway") {
				continue
			}
		}

		saveTokenWithUser(token, "prompt")
		return token, nil
	}
}

// ValidateToken checks if a token is valid by calling GET /user.
func ValidateToken(token string) error {
	_, err := validateTokenWithScopes(token)
	return err
}

// ValidateTokenScopes checks if a token is valid AND returns its scopes.
func ValidateTokenScopes(token string) ([]string, error) {
	return validateTokenWithScopes(token)
}

func validateTokenWithScopes(token string) ([]string, error) {
	req, err := http.NewRequest("GET", apiBase+"/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token validation failed: HTTP %d", resp.StatusCode)
	}

	scopeHeader := resp.Header.Get("X-OAuth-Scopes")
	var scopes []string
	for _, s := range strings.Split(scopeHeader, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes, nil
}

// HasRepoScope checks if "repo" scope is present.
func HasRepoScope(scopes []string) bool {
	for _, s := range scopes {
		if s == "repo" {
			return true
		}
	}
	return false
}

// validateAndGetUser validates a token and returns the GitHub username.
func validateAndGetUser(token string) string {
	req, err := http.NewRequest("GET", apiBase+"/user", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	body, _ := io.ReadAll(resp.Body)
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return ""
	}
	return user.Login
}

// Logout removes a specific account or all accounts.
// If username is empty, prompts user to select which account to remove.
func Logout(username string) error {
	if username != "" {
		return RemoveAccount(username)
	}

	accounts := LoadAllAccounts()
	if len(accounts) == 0 {
		return nil
	}

	if len(accounts) == 1 {
		return RemoveAccount(accounts[0].Username)
	}

	// Multiple accounts — let user choose
	options := make([]huh.Option[string], 0, len(accounts)+1)
	for _, a := range accounts {
		options = append(options, huh.NewOption(
			fmt.Sprintf("@%s (%s)", a.Username, a.Method),
			a.Username,
		))
	}
	options = append(options, huh.NewOption("Remove all accounts", "__all__"))

	choice, err := selectOption("Which account to remove?", options)
	if err != nil {
		return nil
	}

	if choice == "__all__" {
		return RemoveAllAccounts()
	}
	return RemoveAccount(choice)
}

// ghCLIAvailable checks if the gh CLI is installed.
func ghCLIAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ghAuthToken extracts a token from the gh CLI.
func ghAuthToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// OpenBrowser opens a URL in the user's default browser (cross-platform).
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
