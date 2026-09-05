package auth

import (
	"testing"

	"github.com/izam-mohammed/claim-your-code/internal/crypt"
)

func TestAccountStoreRoundTrip(t *testing.T) {
	// An account entry must hold the token encrypted, never in plaintext.
	token := "ghp_test_abc123" // gitleaks:allow -- a made-up fixture, not a token
	encrypted, err := crypt.Encrypt(token)
	if err != nil {
		t.Fatal(err)
	}

	entry := AccountEntry{
		Username:       "testuser",
		EncryptedToken: encrypted,
		Method:         "oauth",
	}

	if entry.Username != "testuser" {
		t.Errorf("username = %q, want %q", entry.Username, "testuser")
	}
	if entry.EncryptedToken == token {
		t.Error("stored token should be encrypted, not plaintext")
	}

	// Verify decryption
	decrypted, err := crypt.Decrypt(entry.EncryptedToken)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != token {
		t.Errorf("decrypted = %q, want %q", decrypted, token)
	}
}

func TestHasRepoScope(t *testing.T) {
	tests := []struct {
		scopes []string
		want   bool
	}{
		{[]string{"repo", "read:user"}, true},
		{[]string{"repo"}, true},
		{[]string{"read:user", "read:org"}, false},
		{[]string{}, false},
		{nil, false},
	}
	for _, tt := range tests {
		got := HasRepoScope(tt.scopes)
		if got != tt.want {
			t.Errorf("HasRepoScope(%v) = %v, want %v", tt.scopes, got, tt.want)
		}
	}
}
