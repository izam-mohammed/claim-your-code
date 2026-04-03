package auth

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// GetToken tries all authentication methods in priority order:
// 1. Cached token from disk
// 2. CLAIM_GITHUB_TOKEN env var
// 3. GITHUB_TOKEN env var
// 4. gh CLI auth token
// 5. OAuth 2.0 Device Flow
// 6. Interactive prompt (paste a PAT)
func GetToken() (string, error) {
	// 1. Cached token
	if token, err := LoadCachedToken(); err == nil && token != "" {
		if ValidateToken(token) == nil {
			return token, nil
		}
		// Cached token is invalid/expired — continue to other methods
	}

	// 2. CLAIM_GITHUB_TOKEN env var
	if token := os.Getenv("CLAIM_GITHUB_TOKEN"); token != "" {
		if err := ValidateToken(token); err == nil {
			_ = CacheToken(token, "env")
			return token, nil
		}
	}

	// 3. GITHUB_TOKEN env var
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		if err := ValidateToken(token); err == nil {
			_ = CacheToken(token, "env")
			return token, nil
		}
	}

	// 4. gh CLI
	if token, err := ghAuthToken(); err == nil && token != "" {
		if ValidateToken(token) == nil {
			_ = CacheToken(token, "gh")
			return token, nil
		}
	}

	// 5. OAuth Device Flow
	token, err := DeviceFlow()
	if err == nil && token != "" {
		_ = CacheToken(token, "oauth")
		return token, nil
	}

	// 6. Interactive prompt
	fmt.Print("Enter GitHub Personal Access Token: ")
	reader := bufio.NewReader(os.Stdin)
	token, _ = reader.ReadString('\n')
	token = strings.TrimSpace(token)
	if token != "" {
		if err := ValidateToken(token); err == nil {
			_ = CacheToken(token, "prompt")
			return token, nil
		}
		return "", fmt.Errorf("invalid token")
	}

	return "", fmt.Errorf("no GitHub authentication available — set GITHUB_TOKEN, install gh CLI, or provide a PAT")
}

// ValidateToken checks if a token is valid by calling GET /user.
func ValidateToken(token string) error {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("token validation failed: HTTP %d", resp.StatusCode)
	}
	return nil
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
