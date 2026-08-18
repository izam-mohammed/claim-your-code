package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/izam-mohammed/claim-your-code/internal/report"

	"github.com/izam-mohammed/claim-your-code/internal/crypt"
)

const tokenFile = "accounts.json"

// AccountEntry stores a single encrypted account.
type AccountEntry struct {
	Username       string    `json:"username"`
	EncryptedToken string    `json:"encrypted_token"`
	Method         string    `json:"method"` // "oauth", "gh", "env", "prompt"
	CreatedAt      time.Time `json:"created_at"`
}

// AccountStore holds all saved accounts.
type AccountStore struct {
	Accounts []AccountEntry `json:"accounts"`
}

func storePath() (string, error) {
	dir, err := report.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dir), tokenFile), nil
}

// loadStore reads the account store from disk.
func loadStore() (*AccountStore, error) {
	path, err := storePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AccountStore{}, nil
		}
		return nil, err
	}
	var store AccountStore
	if err := json.Unmarshal(data, &store); err != nil {
		// Corrupted file — start fresh
		return &AccountStore{}, nil
	}
	return &store, nil
}

// saveStore writes the account store to disk.
func saveStore(store *AccountStore) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadAllAccounts returns all saved accounts with decrypted tokens.
// Accounts that fail to decrypt are silently skipped.
func LoadAllAccounts() []AccountEntry {
	store, err := loadStore()
	if err != nil {
		return nil
	}
	var valid []AccountEntry
	for _, a := range store.Accounts {
		token, err := crypt.Decrypt(a.EncryptedToken)
		if err != nil {
			continue
		}
		valid = append(valid, AccountEntry{
			Username:       a.Username,
			EncryptedToken: token, // temporarily holds decrypted token for caller
			Method:         a.Method,
			CreatedAt:      a.CreatedAt,
		})
	}
	return valid
}

// SaveAccount encrypts and stores an account. Updates if username already exists.
func SaveAccount(username, token, method string) error {
	store, err := loadStore()
	if err != nil {
		store = &AccountStore{}
	}

	encrypted, err := crypt.Encrypt(token)
	if err != nil {
		return err
	}

	entry := AccountEntry{
		Username:       username,
		EncryptedToken: encrypted,
		Method:         method,
		CreatedAt:      time.Now().UTC(),
	}

	// Update existing or append
	found := false
	for i, a := range store.Accounts {
		if a.Username == username {
			store.Accounts[i] = entry
			found = true
			break
		}
	}
	if !found {
		store.Accounts = append(store.Accounts, entry)
	}

	return saveStore(store)
}

// RemoveAccount removes a specific account by username.
func RemoveAccount(username string) error {
	store, err := loadStore()
	if err != nil {
		return err
	}

	var filtered []AccountEntry
	found := false
	for _, a := range store.Accounts {
		if a.Username == username {
			found = true
			continue
		}
		filtered = append(filtered, a)
	}

	if !found {
		return nil
	}

	store.Accounts = filtered
	return saveStore(store)
}

// RemoveAllAccounts clears all saved accounts.
func RemoveAllAccounts() error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListAccountUsernames returns just the usernames of saved accounts.
func ListAccountUsernames() []string {
	accounts := LoadAllAccounts()
	names := make([]string, len(accounts))
	for i, a := range accounts {
		names[i] = a.Username
	}
	return names
}
