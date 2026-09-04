package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// serveAPI points the package's GitHub API base at a test server.
func serveAPI(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := apiBase
	apiBase = srv.URL
	t.Cleanup(func() {
		apiBase = prev
		srv.Close()
	})
	return srv
}

// serveOAuth points the package's OAuth host at a test server, stops
// DeviceFlow from launching a real browser, and shrinks the poll interval so
// tests do not spend their run time asleep.
func serveOAuth(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	prevBase, prevOpen, prevUnit := oauthBase, openBrowser, pollUnit
	oauthBase = srv.URL
	openBrowser = func(string) error { return nil }
	pollUnit = time.Millisecond
	t.Cleanup(func() {
		oauthBase, openBrowser, pollUnit = prevBase, prevOpen, prevUnit
		srv.Close()
	})
	return srv
}

func TestValidateTokenAcceptsAGoodToken(t *testing.T) {
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer good-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.Write([]byte(`{"login":"izam-mohammed"}`))
	})

	if err := ValidateToken("good-token"); err != nil {
		t.Errorf("ValidateToken = %v, want nil", err)
	}
}

func TestValidateTokenScopesParsesHeader(t *testing.T) {
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", " repo ,  read:org , ")
		w.Write([]byte(`{"login":"izam"}`))
	})

	scopes, err := ValidateTokenScopes("tok")
	if err != nil {
		t.Fatal(err)
	}
	if !scopes.Listed {
		t.Error("Listed = false, but GitHub sent a scope header")
	}
	if len(scopes.List) != 2 || scopes.List[0] != "repo" || scopes.List[1] != "read:org" {
		t.Errorf("scopes = %v, want [repo read:org] with whitespace and blanks trimmed", scopes.List)
	}
	if scopes.MissingRepoScope() {
		t.Error("MissingRepoScope = true for a token that lists repo")
	}
}

func TestValidateTokenScopesNoHeader(t *testing.T) {
	// GitHub sends no X-OAuth-Scopes header for fine-grained tokens.
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam"}`))
	})
	scopes, err := ValidateTokenScopes("github_pat_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes.List) != 0 {
		t.Errorf("scopes = %v, want none", scopes.List)
	}
	if scopes.Listed {
		t.Error("Listed = true, but no scope header was sent")
	}
	if scopes.MissingRepoScope() {
		t.Error("a token GitHub listed no scopes for must not be reported as missing repo")
	}
}

func TestValidateTokenScopesEmptyHeaderIsStillAList(t *testing.T) {
	// A classic token with no scopes at all sends the header, empty. That is
	// evidence, unlike the header being absent.
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "")
		w.Write([]byte(`{"login":"izam"}`))
	})
	scopes, err := ValidateTokenScopes("ghp_noscopes")
	if err != nil {
		t.Fatal(err)
	}
	if !scopes.Listed {
		t.Error("Listed = false, but the header was present")
	}
	if !scopes.MissingRepoScope() {
		t.Error("a classic token with an empty scope list is missing repo")
	}
}

func TestTokenScopesMissingRepoScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes TokenScopes
		want   bool
	}{
		{"classic with repo", TokenScopes{List: []string{"repo", "gist"}, Listed: true}, false},
		{"classic without repo", TokenScopes{List: []string{"gist"}, Listed: true}, true},
		{"classic with no scopes", TokenScopes{Listed: true}, true},
		{"fine-grained, nothing listed", TokenScopes{Listed: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scopes.MissingRepoScope(); got != tt.want {
				t.Errorf("MissingRepoScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTokenRejectsBadStatus(t *testing.T) {
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := ValidateToken("bad")
	if err == nil {
		t.Fatal("ValidateToken accepted a token the API rejected")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want it to carry the status code", err)
	}
}

func TestValidateTokenUnreachableAPI(t *testing.T) {
	prev := apiBase
	apiBase = "http://127.0.0.1:1"
	t.Cleanup(func() { apiBase = prev })
	if err := ValidateToken("tok"); err == nil {
		t.Error("ValidateToken succeeded against a dead API")
	}
}

func TestHasRepoScopeCases(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{"has repo", []string{"read:org", "repo"}, true},
		{"only repo", []string{"repo"}, true},
		{"missing", []string{"read:org", "gist"}, false},
		{"empty", nil, false},
		{"public_repo is not repo", []string{"public_repo"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasRepoScope(tt.scopes); got != tt.want {
				t.Errorf("HasRepoScope(%v) = %v, want %v", tt.scopes, got, tt.want)
			}
		})
	}
}

func TestValidateAndGetUser(t *testing.T) {
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam-mohammed"}`))
	})
	if got := validateAndGetUser("tok"); got != "izam-mohammed" {
		t.Errorf("validateAndGetUser = %q, want the login", got)
	}
}

func TestValidateAndGetUserReturnsEmptyOnFailure(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		if got := validateAndGetUser("tok"); got != "" {
			t.Errorf("validateAndGetUser = %q, want empty on a 403", got)
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{not json`))
		})
		if got := validateAndGetUser("tok"); got != "" {
			t.Errorf("validateAndGetUser = %q, want empty on malformed JSON", got)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		prev := apiBase
		apiBase = "http://127.0.0.1:1"
		t.Cleanup(func() { apiBase = prev })
		if got := validateAndGetUser("tok"); got != "" {
			t.Errorf("validateAndGetUser = %q, want empty when the API is down", got)
		}
	})
}

func TestIsDuplicate(t *testing.T) {
	found := []foundToken{{token: "a"}, {token: "b"}}
	if !isDuplicate(found, "b") {
		t.Error("isDuplicate missed a token already in the list")
	}
	if isDuplicate(found, "c") {
		t.Error("isDuplicate flagged a token that is not in the list")
	}
	if isDuplicate(nil, "a") {
		t.Error("isDuplicate on an empty list should be false")
	}
}

func TestAddEnvToken(t *testing.T) {
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"env-user"}`))
	})
	t.Setenv("CLAIM_TEST_TOKEN", "env-token")

	var found []foundToken
	addEnvToken(&found, "CLAIM_TEST_TOKEN")
	if len(found) != 1 {
		t.Fatalf("addEnvToken produced %d entries, want 1", len(found))
	}
	if found[0].source != "CLAIM_TEST_TOKEN" || found[0].user != "env-user" {
		t.Errorf("entry = %+v", found[0])
	}

	// The same token from another variable must not be added twice.
	t.Setenv("CLAIM_TEST_TOKEN_2", "env-token")
	addEnvToken(&found, "CLAIM_TEST_TOKEN_2")
	if len(found) != 1 {
		t.Errorf("addEnvToken added a duplicate token: %+v", found)
	}
}

func TestAddEnvTokenIgnoresUnsetVar(t *testing.T) {
	var found []foundToken
	addEnvToken(&found, "CLAIM_DEFINITELY_UNSET_VAR")
	if len(found) != 0 {
		t.Errorf("addEnvToken = %+v for an unset variable, want none", found)
	}
}

func TestAddEnvTokenIgnoresInvalidToken(t *testing.T) {
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	t.Setenv("CLAIM_TEST_TOKEN", "rejected")

	var found []foundToken
	addEnvToken(&found, "CLAIM_TEST_TOKEN")
	if len(found) != 0 {
		t.Errorf("addEnvToken kept a token the API rejected: %+v", found)
	}
}

func TestLogoutNamedAccount(t *testing.T) {
	isolateStore(t)
	if err := SaveAccount("alice", "t", "pat"); err != nil {
		t.Fatal(err)
	}
	if err := SaveAccount("bob", "t2", "pat"); err != nil {
		t.Fatal(err)
	}

	if err := Logout("alice"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	names := ListAccountUsernames()
	if len(names) != 1 || names[0] != "bob" {
		t.Errorf("accounts after logout = %v, want just bob", names)
	}
}

func TestLogoutWithNoAccountsIsANoop(t *testing.T) {
	isolateStore(t)
	if err := Logout(""); err != nil {
		t.Errorf("Logout on an empty store = %v, want nil", err)
	}
}

func TestLogoutWithASingleAccountSkipsThePrompt(t *testing.T) {
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"solo"}`))
	})
	if err := SaveAccount("solo", "t", "pat"); err != nil {
		t.Fatal(err)
	}

	if err := Logout(""); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if len(ListAccountUsernames()) != 0 {
		t.Error("the only saved account should have been removed without prompting")
	}
}

func TestGhCLIAvailable(t *testing.T) {
	// Whatever the answer, it must agree with PATH lookup and not panic.
	t.Setenv("PATH", t.TempDir())
	if ghCLIAvailable() {
		t.Error("ghCLIAvailable = true with an empty PATH")
	}
}

func TestGhAuthTokenFailsWithoutTheCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := ghAuthToken(); err == nil {
		t.Error("ghAuthToken succeeded with no gh on PATH")
	}
}

func TestGhAuthTokenTrimsOutput(t *testing.T) {
	dir := t.TempDir()
	stub := dir + "/gh"
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho '  ghp_from_cli  '\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if !ghCLIAvailable() {
		t.Fatal("ghCLIAvailable = false with a gh stub on PATH")
	}
	token, err := ghAuthToken()
	if err != nil {
		t.Fatalf("ghAuthToken: %v", err)
	}
	if token != "ghp_from_cli" {
		t.Errorf("ghAuthToken = %q, want the output trimmed", token)
	}
}

func TestOpenBrowserUsesAStubCommand(t *testing.T) {
	// Replace whatever opener this OS uses with a no-op on PATH.
	dir := t.TempDir()
	for _, name := range []string{"open", "xdg-open", "rundll32"} {
		if err := os.WriteFile(dir+"/"+name, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)

	if err := OpenBrowser("https://example.com"); err != nil {
		t.Errorf("OpenBrowser = %v, want nil with a stub opener on PATH", err)
	}
}

func TestOpenBrowserErrorsWithoutAnOpener(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := OpenBrowser("https://example.com"); err == nil {
		t.Error("OpenBrowser succeeded with no opener available")
	}
}

func TestDeviceFlowSucceeds(t *testing.T) {
	var polls int
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/device/code":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("scope") != "repo" {
				t.Errorf("requested scope = %q, want repo", r.Form.Get("scope"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"device_code":"dev-code","user_code":"ABCD-1234","verification_uri":"https://example.invalid/device","expires_in":30,"interval":1}`))
		case "/login/oauth/access_token":
			polls++
			w.Header().Set("Content-Type", "application/json")
			if polls == 1 {
				w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			w.Write([]byte(`{"access_token":"ghp_device_token","token_type":"bearer","scope":"repo"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	token, err := DeviceFlow()
	if err != nil {
		t.Fatalf("DeviceFlow: %v", err)
	}
	if token != "ghp_device_token" {
		t.Errorf("DeviceFlow = %q, want the granted token", token)
	}
	if polls < 2 {
		t.Errorf("polled %d times, want it to keep polling past authorization_pending", polls)
	}
}

func TestDeviceFlowFallsBackWhenBrowserWontOpen(t *testing.T) {
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":30,"interval":1}`))
			return
		}
		w.Write([]byte(`{"access_token":"tok"}`))
	})
	openBrowser = func(string) error { return os.ErrPermission }

	token, err := DeviceFlow()
	if err != nil {
		t.Fatalf("DeviceFlow = %v, want it to carry on and print the URL instead", err)
	}
	if token != "tok" {
		t.Errorf("token = %q", token)
	}
}

func TestDeviceFlowRejectsMissingDeviceCode(t *testing.T) {
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"user_code":"ABCD"}`))
	})
	_, err := DeviceFlow()
	if err == nil {
		t.Fatal("DeviceFlow succeeded without a device code")
	}
	if !strings.Contains(err.Error(), "device code") {
		t.Errorf("error = %v, want it to mention the missing device code", err)
	}
}

func TestDeviceFlowRejectsMalformedResponse(t *testing.T) {
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	})
	if _, err := DeviceFlow(); err == nil {
		t.Error("DeviceFlow accepted a malformed device-code response")
	}
}

func TestDeviceFlowUnreachableHost(t *testing.T) {
	prevBase, prevOpen := oauthBase, openBrowser
	oauthBase = "http://127.0.0.1:1"
	openBrowser = func(string) error { return nil }
	t.Cleanup(func() { oauthBase, openBrowser = prevBase, prevOpen })

	if _, err := DeviceFlow(); err == nil {
		t.Error("DeviceFlow succeeded against a dead host")
	}
}

func TestDeviceFlowTimesOut(t *testing.T) {
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		// expires_in 0 means the deadline has already passed.
		w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":0,"interval":1}`))
	})
	_, err := DeviceFlow()
	if err == nil {
		t.Fatal("DeviceFlow succeeded with an already-expired device code")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want a timeout", err)
	}
}

func TestPollForToken(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantToken string
		wantErr   bool
	}{
		{"granted", `{"access_token":"tok"}`, "tok", false},
		{"pending keeps polling", `{"error":"authorization_pending"}`, "", false},
		{"slow down asks for a back-off", `{"error":"slow_down"}`, "", true},
		{"hard error", `{"error":"access_denied"}`, "", true},
		{"empty response", `{}`, "", false},
		{"malformed", `{not json`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err == nil {
					if got := r.Form.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
						t.Errorf("grant_type = %q", got)
					}
				}
				w.Write([]byte(tt.body))
			})

			token, err := pollForToken("dev-code")
			if tt.wantErr && err == nil {
				t.Fatalf("pollForToken = (%q, nil), want an error", token)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("pollForToken = %v, want no error", err)
			}
			if token != tt.wantToken {
				t.Errorf("token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}

func TestPollForTokenUnreachable(t *testing.T) {
	prev := oauthBase
	oauthBase = "http://127.0.0.1:1"
	t.Cleanup(func() { oauthBase = prev })
	if _, err := pollForToken("d"); err == nil {
		t.Error("pollForToken succeeded against a dead host")
	}
}

func TestSaveTokenWithUserStoresTheAccount(t *testing.T) {
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam-mohammed"}`))
	})

	saveTokenWithUser("ghp_x", "oauth")

	accounts := LoadAllAccounts()
	if len(accounts) != 1 || accounts[0].Username != "izam-mohammed" {
		t.Fatalf("accounts = %+v, want the validated user saved", accounts)
	}
	if accounts[0].Method != "oauth" {
		t.Errorf("Method = %q, want oauth", accounts[0].Method)
	}
}

func TestSaveTokenWithUserSkipsInvalidTokens(t *testing.T) {
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	saveTokenWithUser("bad", "oauth")
	if len(LoadAllAccounts()) != 0 {
		t.Error("a token the API rejected was saved anyway")
	}
}

func TestOAuthClientIDIsSet(t *testing.T) {
	if oauthClientID == "" {
		t.Fatal("oauthClientID is empty — DeviceFlow would refuse to start")
	}
	if _, err := url.Parse(oauthBase); err != nil {
		t.Errorf("oauthBase is not a URL: %v", err)
	}
}

// writeStub creates an executable shell stub named name inside dir.
func writeStub(dir, name, body string) error {
	return os.WriteFile(dir+"/"+name, []byte(body), 0o755)
}

func TestDeviceFlowStopsWhenTheUserDenies(t *testing.T) {
	// Polling on after access_denied left the user watching "Waiting for
	// authorization..." until the code expired -- 15 minutes, by GitHub's
	// default expires_in -- and then reported a timeout instead of the denial.
	var polls int
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":600,"interval":1}`))
			return
		}
		polls++
		w.Write([]byte(`{"error":"access_denied"}`))
	})

	start := time.Now()
	_, err := DeviceFlow()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("DeviceFlow succeeded after the user denied the request")
	}
	if !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("error = %v, want it to report the denial", err)
	}
	if polls != 1 {
		t.Errorf("polled %d times, want it to stop after the first denial", polls)
	}
	// The deadline is far out; returning promptly is the whole point.
	if elapsed > 5*time.Second {
		t.Errorf("took %v to report a denial", elapsed)
	}
}

func TestDeviceFlowStopsOnAnExpiredCode(t *testing.T) {
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":600,"interval":1}`))
			return
		}
		w.Write([]byte(`{"error":"expired_token"}`))
	})

	_, err := DeviceFlow()
	if err == nil || !strings.Contains(err.Error(), "expired_token") {
		t.Errorf("error = %v, want it to report the expired code", err)
	}
}

func TestDeviceFlowKeepsPollingThroughATransportFailure(t *testing.T) {
	// A network hiccup is not a reason to abandon the flow.
	var polls int
	srv := serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":600,"interval":1}`))
			return
		}
		polls++
		if polls == 1 {
			// Hang up mid-response so the client sees a transport error.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Skip("test server does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Skip("could not hijack the connection")
			}
			conn.Close()
			return
		}
		w.Write([]byte(`{"access_token":"ghp_eventually"}`))
	})
	_ = srv

	token, err := DeviceFlow()
	if err != nil {
		t.Fatalf("DeviceFlow gave up on a transport error: %v", err)
	}
	if token != "ghp_eventually" {
		t.Errorf("token = %q, want the one granted on the retry", token)
	}
	if polls < 2 {
		t.Errorf("polled %d times, want it to have retried", polls)
	}
}

func TestDeviceFlowBacksOffOnSlowDown(t *testing.T) {
	// slow_down must widen the gap between polls, so measure the gaps rather
	// than the total: without a back-off both would be one interval.
	var pollTimes []time.Time
	serveOAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri":"https://example.invalid/d","expires_in":600,"interval":1}`))
			return
		}
		pollTimes = append(pollTimes, time.Now())
		if len(pollTimes) == 1 {
			w.Write([]byte(`{"error":"slow_down"}`))
			return
		}
		w.Write([]byte(`{"access_token":"ghp_after_backoff"}`))
	})
	// A coarser unit than the package default, so the two gaps separate
	// cleanly without making the test slow.
	pollUnit = 20 * time.Millisecond

	start := time.Now()
	token, err := DeviceFlow()
	if err != nil {
		t.Fatalf("DeviceFlow: %v", err)
	}
	if token != "ghp_after_backoff" {
		t.Errorf("token = %q", token)
	}
	if len(pollTimes) != 2 {
		t.Fatalf("polled %d times, want 2", len(pollTimes))
	}

	first := pollTimes[0].Sub(start)
	second := pollTimes[1].Sub(pollTimes[0])
	t.Logf("gap before the first poll: %v, before the second: %v", first, second)

	// The interval doubles from one floor to two, so allow generous slack and
	// still catch a missing back-off, which would leave the gaps equal.
	if second < first*3/2 {
		t.Errorf("poll gaps were %v then %v — slow_down did not widen the interval", first, second)
	}
}

func TestPromptPATAcceptsAFineGrainedTokenWithoutWarning(t *testing.T) {
	// GitHub lists no scopes for fine-grained tokens, which used to be read
	// as "missing the repo scope" and produced a spurious retry prompt.
	isolateStore(t)
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"login":"izam-mohammed"}`)) // no X-OAuth-Scopes
	})
	stubStdin(t, "github_pat_valid\n")
	stubConfirm(t) // any confirmation prompt would answer "no"

	token, err := promptPAT()
	if err != nil {
		t.Fatalf("promptPAT rejected a fine-grained token: %v", err)
	}
	if token != "github_pat_valid" {
		t.Errorf("token = %q", token)
	}
	if names := ListAccountUsernames(); len(names) != 1 {
		t.Errorf("accounts = %v, want the token saved", names)
	}
}

func TestPromptPATStillWarnsOnAClassicTokenWithoutRepo(t *testing.T) {
	// The fix above must not silence the case it was meant to keep.
	isolateStore(t)
	var calls int
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("X-OAuth-Scopes", "gist, read:org")
		} else {
			w.Header().Set("X-OAuth-Scopes", "repo")
		}
		w.Write([]byte(`{"login":"izam"}`))
	})
	stubStdin(t, "ghp_narrow\nghp_wide\n")
	stubConfirm(t, true) // yes, enter a new token

	token, err := promptPAT()
	if err != nil {
		t.Fatal(err)
	}
	if token != "ghp_wide" {
		t.Errorf("token = %q, want the retry to have been offered and taken", token)
	}
}
