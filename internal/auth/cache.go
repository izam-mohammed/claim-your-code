package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/izam-mohammed/claim-your-code/internal/report"
)

const tokenFile = "auth.json"

type CachedAuth struct {
	Token     string    `json:"token"`
	Method    string    `json:"method"` // "oauth", "gh", "env", "prompt"
	CreatedAt time.Time `json:"created_at"`
}

func tokenPath() (string, error) {
	dir, err := report.DataDir()
	if err != nil {
		return "", err
	}
	// Store auth alongside reports in the parent dir
	return filepath.Join(filepath.Dir(dir), tokenFile), nil
}

// LoadCachedToken reads a previously stored token.
func LoadCachedToken() (string, error) {
	path, err := tokenPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var cached CachedAuth
	if err := json.Unmarshal(data, &cached); err != nil {
		return "", err
	}
	if cached.Token == "" {
		return "", os.ErrNotExist
	}
	return cached.Token, nil
}

// CacheToken stores a token to disk for future use.
func CacheToken(token, method string) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cached := CachedAuth{
		Token:     token,
		Method:    method,
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600) // restrictive permissions for token file
}
