package auth

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
)

// stubSelect answers the next selection prompts with the given values in order.
// It records the option values each prompt was offered.
func stubSelect(t *testing.T, answers ...string) *[][]string {
	t.Helper()
	var offered [][]string
	i := 0
	prev := selectOption
	selectOption = func(_ string, options []huh.Option[string]) (string, error) {
		var values []string
		for _, o := range options {
			values = append(values, o.Value)
		}
		offered = append(offered, values)
		if i >= len(answers) {
			return "", fmt.Errorf("no answer configured")
		}
		a := answers[i]
		i++
		return a, nil
	}
	t.Cleanup(func() { selectOption = prev })
	return &offered
}

// stubSelectErr makes every selection prompt fail, as it does with no terminal.
func stubSelectErr(t *testing.T) {
	t.Helper()
	prev := selectOption
	selectOption = func(string, []huh.Option[string]) (string, error) {
		return "", fmt.Errorf("no terminal")
	}
	t.Cleanup(func() { selectOption = prev })
}

// stubConfirm answers confirmation prompts with the given values in order.
func stubConfirm(t *testing.T, answers ...bool) {
	t.Helper()
	i := 0
	prev := confirmPrompt
	confirmPrompt = func(string, string, string) bool {
		if i >= len(answers) {
			return false
		}
		a := answers[i]
		i++
		return a
	}
	t.Cleanup(func() { confirmPrompt = prev })
}

// stubStdin feeds promptPAT the given input.
func stubStdin(t *testing.T, input string) {
	t.Helper()
	prev := promptInput
	promptInput = strings.NewReader(input)
	t.Cleanup(func() { promptInput = prev })
}

// noGhCLI removes gh from PATH so the auth flows do not shell out to it.
func noGhCLI(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// clearTokenEnv removes the token env vars the discovery step reads.
func clearTokenEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CLAIM_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
}

func TestGetTokenPicksASavedAccount(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	clearTokenEnv(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam-mohammed"}`))
	})
	if err := SaveAccount("izam-mohammed", "saved-token", "oauth"); err != nil {
		t.Fatal(err)
	}

	offered := stubSelect(t, "found_0")

	token, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token != "saved-token" {
		t.Errorf("GetToken = %q, want the saved token", token)
	}
	if len(*offered) != 1 {
		t.Fatalf("expected exactly one prompt, got %d", len(*offered))
	}
	keys := (*offered)[0]
	if len(keys) != 3 || keys[0] != "found_0" || keys[1] != "other" || keys[2] != "public" {
		t.Errorf("options = %v, want the account plus \"other\" and \"public\"", keys)
	}
}

func TestGetTokenContinueWithoutAuth(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	clearTokenEnv(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam"}`))
	})
	if err := SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}
	stubSelect(t, "public")

	token, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token != "" {
		t.Errorf("GetToken = %q, want an empty token for public-only access", token)
	}
}

func TestGetTokenPrefersEnvVarWhenNoAccountSaved(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	t.Setenv("CLAIM_GITHUB_TOKEN", "env-token")
	t.Setenv("GITHUB_TOKEN", "")
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"env-user"}`))
	})
	stubSelect(t, "found_0")

	token, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token != "env-token" {
		t.Errorf("GetToken = %q, want the token from the environment", token)
	}
}

func TestGetTokenDeduplicatesEnvVars(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	// The same token exposed under both names must appear once.
	t.Setenv("CLAIM_GITHUB_TOKEN", "same")
	t.Setenv("GITHUB_TOKEN", "same")
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"u"}`))
	})
	offered := stubSelect(t, "found_0")

	if _, err := GetToken(); err != nil {
		t.Fatal(err)
	}
	keys := (*offered)[0]
	if len(keys) != 3 {
		t.Errorf("options = %v, want one account plus the two trailing choices", keys)
	}
}

func TestGetTokenUsesGhCLIToken(t *testing.T) {
	isolateStore(t)
	clearTokenEnv(t)
	dir := t.TempDir()
	if err := writeStub(dir, "gh", "#!/bin/sh\necho ghp_from_gh\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"gh-user"}`))
	})
	offered := stubSelect(t, "found_0")

	token, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token != "ghp_from_gh" {
		t.Errorf("GetToken = %q, want the gh CLI token", token)
	}
	if !strings.Contains(strings.Join((*offered)[0], ","), "found_0") {
		t.Errorf("options = %v", (*offered)[0])
	}
}

func TestGetTokenCancelledSelection(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	clearTokenEnv(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam"}`))
	})
	if err := SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}
	stubSelectErr(t)

	if _, err := GetToken(); err == nil {
		t.Error("GetToken succeeded even though the picker was cancelled")
	}
}

func TestGetTokenFallsThroughToPromptWhenAddingAnother(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	clearTokenEnv(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam"}`))
	})
	if err := SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}
	// First prompt: "other". Second prompt (promptForAuth): PAT.
	stubSelect(t, "other", "pat")
	stubStdin(t, "\n") // empty token ends promptPAT immediately

	if _, err := GetToken(); err == nil {
		t.Error("GetToken succeeded with no token entered")
	}
}

func TestPromptForAuthViaPAT(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo")
		w.Write([]byte(`{"login":"izam-mohammed"}`))
	})
	stubSelect(t, "pat")
	stubStdin(t, "ghp_pasted_token\n")

	token, err := promptForAuth()
	if err != nil {
		t.Fatalf("promptForAuth: %v", err)
	}
	if token != "ghp_pasted_token" {
		t.Errorf("token = %q", token)
	}
	if names := ListAccountUsernames(); len(names) != 1 || names[0] != "izam-mohammed" {
		t.Errorf("accounts = %v, want the pasted token saved", names)
	}
}

func TestPromptForAuthOffersGhCLIOnlyWhenInstalled(t *testing.T) {
	isolateStore(t)

	noGhCLI(t)
	offered := stubSelect(t, "")
	_, _ = promptForAuth()
	if strings.Contains(strings.Join((*offered)[0], ","), "gh") {
		t.Errorf("options = %v, want no gh entry when gh is not installed", (*offered)[0])
	}

	dir := t.TempDir()
	if err := writeStub(dir, "gh", "#!/bin/sh\necho tok\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	offered2 := stubSelect(t, "")
	_, _ = promptForAuth()
	if !strings.Contains(strings.Join((*offered2)[0], ","), "gh") {
		t.Errorf("options = %v, want a gh entry when gh is installed", (*offered2)[0])
	}
}

func TestPromptForAuthViaGhCLI(t *testing.T) {
	isolateStore(t)
	dir := t.TempDir()
	if err := writeStub(dir, "gh", "#!/bin/sh\necho ghp_gh_token\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo")
		w.Write([]byte(`{"login":"gh-user"}`))
	})
	stubSelect(t, "gh")

	token, err := promptForAuth()
	if err != nil {
		t.Fatalf("promptForAuth: %v", err)
	}
	if token != "ghp_gh_token" {
		t.Errorf("token = %q", token)
	}
	if names := ListAccountUsernames(); len(names) != 1 {
		t.Errorf("accounts = %v, want the gh token saved", names)
	}
}

func TestPromptForAuthGhCLIWarnsOnMissingRepoScope(t *testing.T) {
	isolateStore(t)
	dir := t.TempDir()
	if err := writeStub(dir, "gh", "#!/bin/sh\necho ghp_gh_token\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "read:org")
		w.Write([]byte(`{"login":"gh-user"}`))
	})
	stubSelect(t, "gh")

	token, err := promptForAuth()
	if err != nil {
		t.Fatalf("promptForAuth = %v, want it to warn but carry on", err)
	}
	if token != "ghp_gh_token" {
		t.Errorf("token = %q", token)
	}
}

func TestPromptForAuthGhCLIFailure(t *testing.T) {
	isolateStore(t)
	dir := t.TempDir()
	if err := writeStub(dir, "gh", "#!/bin/sh\nexit 1\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	stubSelect(t, "gh")

	if _, err := promptForAuth(); err == nil {
		t.Error("promptForAuth succeeded even though gh CLI failed")
	}
}

func TestPromptForAuthGhCLITokenRejected(t *testing.T) {
	isolateStore(t)
	dir := t.TempDir()
	if err := writeStub(dir, "gh", "#!/bin/sh\necho stale\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	stubSelect(t, "gh")

	if _, err := promptForAuth(); err == nil {
		t.Error("promptForAuth accepted a gh token the API rejected")
	}
}

func TestPromptForAuthViaOAuth(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":30,"interval":1}`))
			return
		}
		w.Write([]byte(`{"access_token":"ghp_oauth_token"}`))
	})
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"oauth-user"}`))
	})
	stubSelect(t, "oauth")

	token, err := promptForAuth()
	if err != nil {
		t.Fatalf("promptForAuth: %v", err)
	}
	if token != "ghp_oauth_token" {
		t.Errorf("token = %q", token)
	}
	if names := ListAccountUsernames(); len(names) != 1 || names[0] != "oauth-user" {
		t.Errorf("accounts = %v, want the OAuth account saved", names)
	}
}

func TestPromptForAuthOAuthFailure(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`)) // no device code
	})
	stubSelect(t, "oauth")

	if _, err := promptForAuth(); err == nil {
		t.Error("promptForAuth succeeded even though OAuth failed")
	}
}

func TestPromptForAuthCancelled(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	stubSelectErr(t)
	if _, err := promptForAuth(); err == nil {
		t.Error("promptForAuth succeeded even though the picker was cancelled")
	}
}

func TestPromptForAuthUnknownChoice(t *testing.T) {
	isolateStore(t)
	noGhCLI(t)
	stubSelect(t, "something-else")
	if _, err := promptForAuth(); err == nil {
		t.Error("promptForAuth accepted an unrecognised choice")
	}
}

func TestPromptPATRetriesAfterAnInvalidToken(t *testing.T) {
	isolateStore(t)
	var calls int
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-OAuth-Scopes", "repo")
		w.Write([]byte(`{"login":"izam"}`))
	})
	stubStdin(t, "bad-token\ngood-token\n")
	stubConfirm(t, true) // yes, try another token

	token, err := promptPAT()
	if err != nil {
		t.Fatalf("promptPAT: %v", err)
	}
	if token != "good-token" {
		t.Errorf("promptPAT = %q, want the second token", token)
	}
}

func TestPromptPATGivesUpWhenRetryDeclined(t *testing.T) {
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	stubStdin(t, "bad-token\n")
	stubConfirm(t, false)

	if _, err := promptPAT(); err == nil {
		t.Error("promptPAT succeeded after the user declined to retry")
	}
}

func TestPromptPATEmptyInput(t *testing.T) {
	isolateStore(t)
	stubStdin(t, "\n")
	_, err := promptPAT()
	if err == nil {
		t.Fatal("promptPAT accepted an empty token")
	}
	if !strings.Contains(err.Error(), "no token provided") {
		t.Errorf("error = %v", err)
	}
}

func TestPromptPATAcceptsTokenMissingRepoScope(t *testing.T) {
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "read:org")
		w.Write([]byte(`{"login":"izam"}`))
	})
	stubStdin(t, "narrow-token\n")
	stubConfirm(t, false) // no, use this token anyway

	token, err := promptPAT()
	if err != nil {
		t.Fatalf("promptPAT: %v", err)
	}
	if token != "narrow-token" {
		t.Errorf("token = %q", token)
	}
}

func TestPromptPATRetriesForABetterScope(t *testing.T) {
	isolateStore(t)
	var calls int
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("X-OAuth-Scopes", "read:org")
		} else {
			w.Header().Set("X-OAuth-Scopes", "repo")
		}
		w.Write([]byte(`{"login":"izam"}`))
	})
	stubStdin(t, "narrow-token\nwide-token\n")
	stubConfirm(t, true) // yes, enter a new token

	token, err := promptPAT()
	if err != nil {
		t.Fatalf("promptPAT: %v", err)
	}
	if token != "wide-token" {
		t.Errorf("promptPAT = %q, want the token with the repo scope", token)
	}
}

func TestLogoutPicksFromMultipleAccounts(t *testing.T) {
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"x"}`))
	})
	for _, name := range []string{"alice", "bob"} {
		if err := SaveAccount(name, "t-"+name, "pat"); err != nil {
			t.Fatal(err)
		}
	}
	offered := stubSelect(t, "alice")

	if err := Logout(""); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	names := ListAccountUsernames()
	if len(names) != 1 || names[0] != "bob" {
		t.Errorf("accounts = %v, want alice removed", names)
	}
	keys := (*offered)[0]
	if len(keys) != 3 || keys[2] != "__all__" {
		t.Errorf("options = %v, want both accounts plus a remove-all entry", keys)
	}
}

func TestLogoutRemoveAll(t *testing.T) {
	isolateStore(t)
	for _, name := range []string{"alice", "bob"} {
		if err := SaveAccount(name, "t-"+name, "pat"); err != nil {
			t.Fatal(err)
		}
	}
	stubSelect(t, "__all__")

	if err := Logout(""); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if len(ListAccountUsernames()) != 0 {
		t.Error("accounts survived a remove-all logout")
	}
}

func TestLogoutCancelledLeavesAccountsAlone(t *testing.T) {
	isolateStore(t)
	for _, name := range []string{"alice", "bob"} {
		if err := SaveAccount(name, "t-"+name, "pat"); err != nil {
			t.Fatal(err)
		}
	}
	stubSelectErr(t)

	if err := Logout(""); err != nil {
		t.Errorf("Logout = %v, want nil when cancelled", err)
	}
	if len(ListAccountUsernames()) != 2 {
		t.Error("a cancelled logout removed accounts")
	}
}
