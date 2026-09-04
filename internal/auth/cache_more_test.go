package auth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateStore points the account store at a throwaway directory.
func isolateStore(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", tmp)
	case "windows":
		t.Setenv("LocalAppData", tmp)
	default:
		t.Setenv("XDG_DATA_HOME", tmp)
	}
	path, err := storePath()
	if err != nil {
		t.Fatalf("storePath: %v", err)
	}
	return path
}

func TestStorePathSitsBesideTheReportsDir(t *testing.T) {
	path := isolateStore(t)
	if filepath.Base(path) != tokenFile {
		t.Errorf("store file = %q, want %q", filepath.Base(path), tokenFile)
	}
	if filepath.Base(filepath.Dir(path)) != "claim" {
		t.Errorf("store lives in %q, want it under the claim data dir", filepath.Dir(path))
	}
}

func TestLoadAllAccountsEmptyWhenNothingSaved(t *testing.T) {
	isolateStore(t)
	if got := LoadAllAccounts(); len(got) != 0 {
		t.Errorf("LoadAllAccounts = %+v, want empty", got)
	}
	if got := ListAccountUsernames(); len(got) != 0 {
		t.Errorf("ListAccountUsernames = %v, want empty", got)
	}
}

func TestSaveAccountRoundTripDecryptsToken(t *testing.T) {
	isolateStore(t)

	if err := SaveAccount("izam-mohammed", "ghp_secret_token", "oauth"); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}

	accounts := LoadAllAccounts()
	if len(accounts) != 1 {
		t.Fatalf("LoadAllAccounts returned %d accounts, want 1", len(accounts))
	}
	a := accounts[0]
	if a.Username != "izam-mohammed" {
		t.Errorf("Username = %q", a.Username)
	}
	if a.Method != "oauth" {
		t.Errorf("Method = %q", a.Method)
	}
	// LoadAllAccounts hands back the decrypted token in EncryptedToken.
	if a.EncryptedToken != "ghp_secret_token" {
		t.Errorf("token = %q, want the decrypted value", a.EncryptedToken)
	}
	if a.CreatedAt.IsZero() {
		t.Error("CreatedAt was not stamped")
	}
}

func TestSaveAccountWritesEncrypted(t *testing.T) {
	path := isolateStore(t)
	if err := SaveAccount("izam", "ghp_plaintext_marker", "pat"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("store file is empty")
	}
	if contains(string(raw), "ghp_plaintext_marker") {
		t.Error("the token was written in plaintext — it must be encrypted at rest")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}

func TestSaveAccountFilePermissionsAreRestrictive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	path := isolateStore(t)
	if err := SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store permissions = %o, want 0600 — it holds credentials", perm)
	}
}

func TestSaveAccountUpdatesExistingUsername(t *testing.T) {
	isolateStore(t)

	if err := SaveAccount("izam", "first-token", "gh"); err != nil {
		t.Fatal(err)
	}
	if err := SaveAccount("izam", "second-token", "oauth"); err != nil {
		t.Fatal(err)
	}

	accounts := LoadAllAccounts()
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want the existing one updated rather than duplicated", len(accounts))
	}
	if accounts[0].EncryptedToken != "second-token" {
		t.Errorf("token = %q, want the newer one", accounts[0].EncryptedToken)
	}
	if accounts[0].Method != "oauth" {
		t.Errorf("Method = %q, want it updated too", accounts[0].Method)
	}
}

func TestSaveAccountKeepsMultipleUsers(t *testing.T) {
	isolateStore(t)
	for _, name := range []string{"alice", "bob", "carol"} {
		if err := SaveAccount(name, "token-"+name, "pat"); err != nil {
			t.Fatal(err)
		}
	}
	names := ListAccountUsernames()
	if len(names) != 3 {
		t.Fatalf("ListAccountUsernames = %v, want 3 accounts", names)
	}
}

func TestRemoveAccount(t *testing.T) {
	isolateStore(t)
	for _, name := range []string{"alice", "bob"} {
		if err := SaveAccount(name, "t-"+name, "pat"); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemoveAccount("alice"); err != nil {
		t.Fatalf("RemoveAccount: %v", err)
	}
	names := ListAccountUsernames()
	if len(names) != 1 || names[0] != "bob" {
		t.Errorf("after removing alice, accounts = %v", names)
	}
}

func TestRemoveAccountUnknownUserIsANoop(t *testing.T) {
	isolateStore(t)
	if err := SaveAccount("alice", "t", "pat"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAccount("nobody"); err != nil {
		t.Errorf("RemoveAccount(nobody) = %v, want nil", err)
	}
	if len(ListAccountUsernames()) != 1 {
		t.Error("removing an unknown account changed the store")
	}
}

func TestRemoveAllAccounts(t *testing.T) {
	path := isolateStore(t)
	if err := SaveAccount("alice", "t", "pat"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAllAccounts(); err != nil {
		t.Fatalf("RemoveAllAccounts: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("RemoveAllAccounts left the store file behind")
	}
	if len(LoadAllAccounts()) != 0 {
		t.Error("accounts survived RemoveAllAccounts")
	}
}

func TestRemoveAllAccountsWhenNothingSaved(t *testing.T) {
	isolateStore(t)
	if err := RemoveAllAccounts(); err != nil {
		t.Errorf("RemoveAllAccounts on an empty store = %v, want nil", err)
	}
}

func TestLoadStoreTreatsCorruptFileAsEmpty(t *testing.T) {
	path := isolateStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json at all"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := loadStore()
	if err != nil {
		t.Fatalf("loadStore on a corrupt file = %v, want it to recover", err)
	}
	if len(store.Accounts) != 0 {
		t.Errorf("corrupt store decoded as %+v, want empty", store.Accounts)
	}

	// Saving over a corrupt store must still work.
	if err := SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatalf("SaveAccount over a corrupt store: %v", err)
	}
	if len(ListAccountUsernames()) != 1 {
		t.Error("could not recover the store by saving a new account")
	}
}

func TestLoadAllAccountsSkipsUndecryptableEntries(t *testing.T) {
	path := isolateStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	// One decryptable account, one that was encrypted on another machine.
	if err := SaveAccount("good", "real-token", "pat"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var store AccountStore
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatal(err)
	}
	// Valid base64, but not something this machine's key can open — built at
	// runtime so no high-entropy literal ends up in the source.
	foreign := base64.StdEncoding.EncodeToString([]byte("encrypted on another machine"))
	store.Accounts = append(store.Accounts, AccountEntry{
		Username:       "foreign",
		EncryptedToken: foreign,
		Method:         "oauth",
	})
	out, err := json.Marshal(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}

	accounts := LoadAllAccounts()
	if len(accounts) != 1 || accounts[0].Username != "good" {
		t.Errorf("LoadAllAccounts = %+v, want only the decryptable account", accounts)
	}
}

func TestStoreOperationsFailWithoutADataDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("LocalAppData", "")
	} else {
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("HOME", "")
	}

	if _, err := storePath(); err == nil {
		t.Error("storePath succeeded with no home directory")
	}
	if err := SaveAccount("x", "t", "pat"); err == nil {
		t.Error("SaveAccount succeeded with no home directory")
	}
	if err := RemoveAccount("x"); err == nil {
		t.Error("RemoveAccount succeeded with no home directory")
	}
	if err := RemoveAllAccounts(); err == nil {
		t.Error("RemoveAllAccounts succeeded with no home directory")
	}
	if got := LoadAllAccounts(); got != nil {
		t.Errorf("LoadAllAccounts = %+v, want nil", got)
	}
}

func TestSaveAccountFailsWhenTheStoreDirIsAFile(t *testing.T) {
	path := isolateStore(t)
	// Put a regular file where the claim data directory needs to be.
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(path), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveAccount("izam", "tok", "pat"); err == nil {
		t.Error("SaveAccount succeeded even though the store directory is a file")
	}
}

func TestRemoveAllAccountsFailsWhenTheStoreIsADirectory(t *testing.T) {
	path := isolateStore(t)
	// A directory where the store file belongs cannot be removed by os.Remove.
	if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAllAccounts(); err == nil {
		t.Error("RemoveAllAccounts succeeded even though the store path is a non-empty directory")
	}
}

func TestLoadStoreSurfacesAReadFailure(t *testing.T) {
	path := isolateStore(t)
	// A directory at the store path makes ReadFile fail with something other
	// than "not exist", which must be reported rather than treated as empty.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStore(); err == nil {
		t.Error("loadStore succeeded even though the store path is a directory")
	}
}
