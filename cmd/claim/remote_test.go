package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/izam-mohammed/claim-your-code/internal/auth"
	githubpkg "github.com/izam-mohammed/claim-your-code/internal/github"
	"github.com/izam-mohammed/claim-your-code/internal/remote"
)

// serveGitHub points the GitHub client at a test server for the test's duration.
func serveGitHub(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := githubpkg.APIBase()
	githubpkg.SetAPIBase(srv.URL)
	t.Cleanup(func() {
		githubpkg.SetAPIBase(prev)
		srv.Close()
	})
	return srv
}

// stubToken makes requireAuth return the given token without prompting.
func stubToken(t *testing.T, token string, err error) {
	t.Helper()
	prev := getToken
	getToken = func() (string, error) { return token, err }
	t.Cleanup(func() { getToken = prev })
}

// repoJSON is a minimal GET /repos/:owner/:repo body.
func repoJSON(name string, private bool) string {
	b, _ := json.Marshal(map[string]any{
		"name": name, "full_name": "owner/" + name,
		"default_branch": "main", "private": private,
		"clone_url": "https://github.com/owner/" + name + ".git",
	})
	return string(b)
}

func TestParseRemoteFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want remoteFlags
	}{
		{"none", []string{"claim", "repo", "o/r"}, remoteFlags{}},
		{"dry run", []string{"claim", "repo", "o/r", "--dry-run"}, remoteFlags{dryRun: true}},
		{"force long", []string{"claim", "repo", "o/r", "--force"}, remoteFlags{force: true}},
		{"force short", []string{"claim", "repo", "o/r", "-f"}, remoteFlags{force: true}},
		{"apply", []string{"claim", "repo", "o/r", "--apply"}, remoteFlags{apply: true}},
		{"api only", []string{"claim", "repo", "o/r", "--api-only"}, remoteFlags{apiOnly: true}},
		{"all together", []string{"claim", "repo", "o/r", "--dry-run", "-f", "--apply", "--api-only"},
			remoteFlags{dryRun: true, force: true, apply: true, apiOnly: true}},
		{"unknown flags ignored", []string{"claim", "repo", "o/r", "--nope"}, remoteFlags{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got remoteFlags
			runMain(t, tt.args, func() { got = parseRemoteFlags() })
			if got != tt.want {
				t.Errorf("parseRemoteFlags() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNonFlagArg(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"claim", "repo", "owner/repo"}, "owner/repo"},
		{[]string{"claim", "repo", "--apply", "owner/repo"}, "owner/repo"},
		{[]string{"claim", "repo", "--apply"}, ""},
		{[]string{"claim", "repo"}, ""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var got string
			runMain(t, tt.args, func() { got = nonFlagArg() })
			if got != tt.want {
				t.Errorf("nonFlagArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOwnerArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bare name", []string{"claim", "org", "my-org"}, "my-org"},
		{"profile URL", []string{"claim", "user", "https://github.com/izam-mohammed"}, "izam-mohammed"},
		{"orgs URL", []string{"claim", "org", "https://github.com/orgs/my-org"}, "my-org"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			runMain(t, tt.args, func() { got = ownerArg("usage") })
			if got != tt.want {
				t.Errorf("ownerArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOwnerArgMissingArgument(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim", "org"}, func() {
		ownerArg("missing argument — run `claim org --help`")
	})
	if !exited || code != 1 {
		t.Errorf("a missing argument should fail, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "claim org --help") {
		t.Errorf("the error should point at help:\n%s", out)
	}
}

func TestOwnerArgInvalidURL(t *testing.T) {
	_, code, exited := runMain(t, []string{"claim", "org", "https://gitlab.com/x"}, func() {
		ownerArg("usage")
	})
	if !exited || code != 1 {
		t.Errorf("a non-github URL should fail, got (%d, %v)", code, exited)
	}
}

func TestRequireAuthPublicClientWhenNoToken(t *testing.T) {
	stubToken(t, "", nil)
	var client *githubpkg.Client
	runMain(t, []string{"claim"}, func() { client = requireAuth() })
	if client.IsAuthenticated() {
		t.Error("requireAuth returned an authenticated client for an empty token")
	}
}

func TestRequireAuthWithToken(t *testing.T) {
	stubToken(t, "ghp_x", nil)
	var client *githubpkg.Client
	runMain(t, []string{"claim"}, func() { client = requireAuth() })
	if !client.IsAuthenticated() || client.Token() != "ghp_x" {
		t.Error("requireAuth did not carry the token through")
	}
}

func TestRequireAuthFailure(t *testing.T) {
	stubToken(t, "", fmt.Errorf("cancelled"))
	_, code, exited := runMain(t, []string{"claim"}, func() { requireAuth() })
	if !exited || code != 1 {
		t.Errorf("a failed authentication should exit, got (%d, %v)", code, exited)
	}
}

func TestRunLogoutNoAccounts(t *testing.T) {
	isolateData(t)
	out, _, _ := runMain(t, []string{"claim", "logout"}, runLogout)
	if !strings.Contains(out, "No saved accounts") {
		t.Errorf("output = %q", out)
	}
}

func TestRunLogoutNamedAccount(t *testing.T) {
	isolateData(t)
	if err := auth.SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}
	out, _, _ := runMain(t, []string{"claim", "logout", "izam"}, runLogout)
	if !strings.Contains(out, "Removed account") {
		t.Errorf("output = %q", out)
	}
	if len(auth.ListAccountUsernames()) != 0 {
		t.Error("the account was not removed")
	}
}

func TestRunLogoutInteractivePick(t *testing.T) {
	isolateData(t)
	for _, n := range []string{"alice", "bob"} {
		if err := auth.SaveAccount(n, "t", "pat"); err != nil {
			t.Fatal(err)
		}
	}
	var offered []huh.Option[string]
	stubPrompts(t, prompts{selectOne: func(_ string, opts []huh.Option[string], _ int) (string, error) {
		offered = opts
		return "alice", nil
	}})

	out, _, _ := runMain(t, []string{"claim", "logout"}, runLogout)
	if !strings.Contains(out, "Removed account") {
		t.Errorf("output = %q", out)
	}
	if names := auth.ListAccountUsernames(); len(names) != 1 || names[0] != "bob" {
		t.Errorf("accounts = %v, want alice removed", names)
	}
	if len(offered) != 3 || offered[2].Value != "__all__" {
		t.Errorf("options = %+v, want both accounts plus a remove-all entry", offered)
	}
}

func TestRunLogoutRemoveAll(t *testing.T) {
	isolateData(t)
	for _, n := range []string{"alice", "bob"} {
		if err := auth.SaveAccount(n, "t", "pat"); err != nil {
			t.Fatal(err)
		}
	}
	stubPrompts(t, prompts{selectOne: answer("__all__")})
	out, _, _ := runMain(t, []string{"claim", "logout"}, runLogout)
	if !strings.Contains(out, "Removed all 2 accounts") {
		t.Errorf("output = %q", out)
	}
	if len(auth.ListAccountUsernames()) != 0 {
		t.Error("accounts survived a remove-all")
	}
}

func TestRunLogoutCancelled(t *testing.T) {
	isolateData(t)
	for _, n := range []string{"alice", "bob"} {
		if err := auth.SaveAccount(n, "t", "pat"); err != nil {
			t.Fatal(err)
		}
	}
	stubPrompts(t, prompts{}) // selection fails
	runMain(t, []string{"claim", "logout"}, runLogout)
	if len(auth.ListAccountUsernames()) != 2 {
		t.Error("a cancelled logout removed accounts")
	}
}

func TestRunRemoteRepoMissingArgument(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim", "repo"}, runRemoteRepo)
	if !exited || code != 1 {
		t.Errorf("expected a usage failure, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "claim repo --help") {
		t.Errorf("the error should point at help:\n%s", out)
	}
}

func TestRunRemoteRepoUnparseableArgument(t *testing.T) {
	_, code, exited := runMain(t, []string{"claim", "repo", "not-a-repo"}, runRemoteRepo)
	if !exited || code != 1 {
		t.Errorf("expected a parse failure, got (%d, %v)", code, exited)
	}
}

func TestRunRemotePRMissingArgument(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim", "pr"}, runRemotePR)
	if !exited || code != 1 {
		t.Errorf("expected a usage failure, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "claim pr --help") {
		t.Errorf("the error should point at help:\n%s", out)
	}
}

func TestRunRemotePRUnparseableArgument(t *testing.T) {
	_, code, exited := runMain(t, []string{"claim", "pr", "owner/repo"}, runRemotePR)
	if !exited || code != 1 {
		t.Errorf("expected a parse failure, got (%d, %v)", code, exited)
	}
}

func TestScanRepoViaAPIFindsTrailers(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"sha":"aaaaaaaabbbbbbbbccccccccddddddddeeeeeeee","commit":{"message":"feat: one\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"}},
			{"sha":"bbbbbbbbccccccccddddddddeeeeeeeeffffffff","commit":{"message":"fix: clean"}}
		]`))
	})
	repo := &githubpkg.RepoInfo{Owner: "owner", Name: "repo", FullName: "owner/repo", DefaultBranch: "main"}

	out, _, _ := runMain(t, []string{"claim"}, func() {
		scanRepoViaAPI(githubpkg.NewPublicClient(), repo)
	})

	for _, want := range []string{"1 commit(s) co-authored by Claude", "aaaaaaaa", "feat: one", "Claude Opus 4.6", "on main"} {
		if !strings.Contains(out, want) {
			t.Errorf("API scan output is missing %q\n---\n%s", want, out)
		}
	}
}

func TestScanRepoViaAPICleanRepo(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"sha":"aaaaaaaabbbbbbbb","commit":{"message":"fix: clean"}}]`))
	})
	repo := &githubpkg.RepoInfo{Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main"}

	out, _, _ := runMain(t, []string{"claim"}, func() {
		scanRepoViaAPI(githubpkg.NewPublicClient(), repo)
	})
	if !strings.Contains(out, "No Claude co-authorship found") {
		t.Errorf("output = %q", out)
	}
}

func TestScanRepoViaAPIHonoursAnExplicitBranch(t *testing.T) {
	var gotSHA string
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		gotSHA = r.URL.Query().Get("sha")
		w.Write([]byte(`[]`))
	})
	repo := &githubpkg.RepoInfo{Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main"}

	runMain(t, []string{"claim"}, func() {
		scanRepoViaAPI(githubpkg.NewPublicClient(), repo, "release")
	})
	if gotSHA != "release" {
		t.Errorf("scanned branch = %q, want the branch that was asked for", gotSHA)
	}
}

func TestScanRepoViaAPIEmptyBranchFallsBackToDefault(t *testing.T) {
	var gotSHA string
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		gotSHA = r.URL.Query().Get("sha")
		w.Write([]byte(`[]`))
	})
	repo := &githubpkg.RepoInfo{Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "trunk"}
	runMain(t, []string{"claim"}, func() {
		scanRepoViaAPI(githubpkg.NewPublicClient(), repo, "")
	})
	if gotSHA != "trunk" {
		t.Errorf("scanned branch = %q, want the default branch", gotSHA)
	}
}

func TestScanRepoViaAPIReportsErrors(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	repo := &githubpkg.RepoInfo{Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main"}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		scanRepoViaAPI(githubpkg.NewPublicClient(), repo)
	})
	if !strings.Contains(out, "Error:") {
		t.Errorf("output = %q, want the API error surfaced", out)
	}
}

func TestScanRepoViaAPITruncatesLongLists(t *testing.T) {
	var commits []map[string]any
	for i := 0; i < 8; i++ {
		commits = append(commits, map[string]any{
			"sha":    fmt.Sprintf("%040d", i),
			"commit": map[string]string{"message": "feat: x\n\n" + claudeTrailer},
		})
	}
	body, _ := json.Marshal(commits)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) { w.Write(body) })

	repo := &githubpkg.RepoInfo{Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main"}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		scanRepoViaAPI(githubpkg.NewPublicClient(), repo)
	})
	if !strings.Contains(out, "... and 3 more") {
		t.Errorf("8 matches should be truncated at 5:\n%s", out)
	}
}

func TestScanAPIConcurrentSummarises(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/dirty/") {
			w.Write([]byte(`[{"sha":"aaaaaaaabbbbbbbb","commit":{"message":"x\n\n` + claudeTrailer + `"}}]`))
			return
		}
		w.Write([]byte(`[{"sha":"bbbbbbbbcccccccc","commit":{"message":"clean"}}]`))
	})

	repos := []githubpkg.RepoInfo{
		{Owner: "owner", Name: "dirty", DefaultBranch: "main"},
		{Owner: "owner", Name: "clean", DefaultBranch: "main"},
	}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		scanAPIConcurrent(githubpkg.NewPublicClient(), repos)
	})

	if !strings.Contains(out, "owner/dirty") {
		t.Errorf("the affected repo is missing:\n%s", out)
	}
	if !strings.Contains(out, "1 repo(s) clean") {
		t.Errorf("the clean count is missing:\n%s", out)
	}
}

func TestScanAPIConcurrentCountsErrorsAsClean(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	repos := []githubpkg.RepoInfo{{Owner: "o", Name: "r", DefaultBranch: "main"}}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		scanAPIConcurrent(githubpkg.NewPublicClient(), repos)
	})
	if !strings.Contains(out, "No Claude co-authorship found") {
		t.Errorf("output = %q", out)
	}
}

func TestListReposDelegatesByKind(t *testing.T) {
	var paths []string
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Write([]byte(`[]`))
	})
	client := githubpkg.NewPublicClient()

	if _, err := listRepos(client, "acme", true); err != nil {
		t.Fatal(err)
	}
	if _, err := listRepos(client, "izam", false); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/orgs/acme/repos" || paths[1] != "/users/izam/repos" {
		t.Errorf("requested paths = %v, want the org then the user endpoint", paths)
	}
}

func TestPublicOrAuthedClientStaysPublicWhenReposAreVisible(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"one","owner":{"login":"acme"}}]`))
	})
	stubToken(t, "should-not-be-used", nil)

	var client *githubpkg.Client
	runMain(t, []string{"claim"}, func() { client = publicOrAuthedClient("acme", true) })
	if client.IsAuthenticated() {
		t.Error("publicOrAuthedClient authenticated even though the public listing worked")
	}
}

func TestPublicOrAuthedClientFallsBackToAuth(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`)) // nothing visible publicly
	})
	stubToken(t, "ghp_x", nil)

	var client *githubpkg.Client
	runMain(t, []string{"claim"}, func() { client = publicOrAuthedClient("acme", true) })
	if !client.IsAuthenticated() {
		t.Error("publicOrAuthedClient should have authenticated when nothing was visible")
	}
}

func TestRunRemoteMultiRepoNoActiveRepos(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"old","owner":{"login":"acme"},"archived":true},{"name":"fork","owner":{"login":"acme"},"fork":true}]`))
	})
	stubPrompts(t, prompts{})

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !strings.Contains(out, "No active repos found") {
		t.Errorf("archived repos and forks should be excluded:\n%s", out)
	}
}

func TestRunRemoteMultiRepoListFailure(t *testing.T) {
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, code, exited := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !exited || code != 1 {
		t.Errorf("a failed listing should exit, got (%d, %v)", code, exited)
	}
}

func TestRunRemoteMultiRepoAPIScan(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/repos"):
			w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
		default:
			w.Write([]byte(`[{"sha":"aaaaaaaabbbbbbbb","commit":{"message":"x\n\n` + claudeTrailer + `"}}]`))
		}
	})

	answers := []string{"all", "api"}
	i := 0
	stubPrompts(t, prompts{selectOne: func(string, []huh.Option[string], int) (string, error) {
		a := answers[i]
		i++
		return a, nil
	}})

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !strings.Contains(out, "acme/one") {
		t.Errorf("the affected repo is missing:\n%s", out)
	}
}

func TestRunRemoteMultiRepoForceSkipsSelection(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/repos") {
			w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
			return
		}
		w.Write([]byte(`[]`))
	})
	stubPrompts(t, prompts{selectOne: answer("api")})

	out, _, _ := runMain(t, []string{"claim", "org", "acme", "--force"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !strings.Contains(out, "Found 1 repo(s)") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteMultiRepoIndividualSelection(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/repos") {
			w.Write([]byte(`[
				{"name":"one","owner":{"login":"acme"},"default_branch":"main"},
				{"name":"two","owner":{"login":"acme"},"default_branch":"main","private":true}
			]`))
			return
		}
		w.Write([]byte(`[]`))
	})

	answers := []string{"select", "api"}
	i := 0
	var multiOpts []huh.Option[string]
	stubPrompts(t, prompts{
		selectOne: func(string, []huh.Option[string], int) (string, error) {
			a := answers[i]
			i++
			return a, nil
		},
		selectMany: func(_ string, opts []huh.Option[string], _ int) ([]string, error) {
			multiOpts = opts
			return []string{"two"}, nil
		},
	})

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})

	if len(multiOpts) != 2 {
		t.Fatalf("offered %d repos, want 2", len(multiOpts))
	}
	if !strings.Contains(multiOpts[1].Key, "[private]") {
		t.Errorf("private repos should be labelled: %q", multiOpts[1].Key)
	}
	if !strings.Contains(out, "Scanning 1 repo(s)") {
		t.Errorf("only the selected repo should be scanned:\n%s", out)
	}
}

func TestRunRemoteMultiRepoNoSelectionStops(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
	})
	stubPrompts(t, prompts{
		selectOne:  answer("select"),
		selectMany: func(string, []huh.Option[string], int) ([]string, error) { return nil, nil },
	})

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !strings.Contains(out, "No repos selected") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteMultiRepoOffersAuthWhenUnauthenticated(t *testing.T) {
	isolateData(t)
	var listings int
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/repos") {
			listings++
			w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
			return
		}
		w.Write([]byte(`[]`))
	})
	stubToken(t, "ghp_x", nil)
	stubPrompts(t, prompts{
		confirm:   func(string) bool { return true }, // yes, authenticate
		selectOne: answer("api"),
	})

	out, _, _ := runMain(t, []string{"claim", "org", "acme", "--force"}, func() {
		runRemoteMultiRepo(githubpkg.NewPublicClient(), "acme", true)
	})
	if listings < 2 {
		t.Errorf("repos were listed %d times, want a re-fetch after authenticating", listings)
	}
	if !strings.Contains(out, "Re-fetching repos") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteMultiRepoCancelledScopeStops(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
	})
	stubPrompts(t, prompts{}) // every selection fails

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if strings.Contains(out, "Scanning") {
		t.Errorf("a cancelled scope prompt should stop before scanning:\n%s", out)
	}
}

func TestRunRemoteMultiRepoByNameDetectsOrg(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/orgs/acme" {
			w.Write([]byte(`{"login":"acme"}`))
			return
		}
		w.Write([]byte(`[]`))
	})
	stubToken(t, "", nil)
	stubPrompts(t, prompts{})

	out, _, _ := runMain(t, []string{"claim", "acme"}, func() {
		runRemoteMultiRepoByName("acme")
	})
	if !strings.Contains(out, "org") {
		t.Errorf("acme should have been detected as an org:\n%s", out)
	}
}

func TestRunRemoteOrgAndUserReadTheirArgument(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})
	stubToken(t, "", nil)
	stubPrompts(t, prompts{})

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, runRemoteOrg)
	if !strings.Contains(out, "acme") {
		t.Errorf("runRemoteOrg output = %q", out)
	}

	out, _, _ = runMain(t, []string{"claim", "user", "izam"}, runRemoteUser)
	if !strings.Contains(out, "izam") {
		t.Errorf("runRemoteUser output = %q", out)
	}
}

func TestRunRemoteRepoAPIOnly(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/owner/repo/commits") {
			w.Write([]byte(`[{"sha":"aaaaaaaabbbbbbbb","commit":{"message":"x\n\n` + claudeTrailer + `"}}]`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, err := remote.ParseRepo("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo", "--api-only"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "via API") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "1 commit(s) co-authored by Claude") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteRepoNotFound(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	stubToken(t, "ghp_x", nil)

	target, err := remote.ParseRepo("owner/missing")
	if err != nil {
		t.Fatal(err)
	}
	_, code, exited := runMain(t, []string{"claim", "repo", "owner/missing"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !exited || code != 1 {
		t.Errorf("a missing repo should exit, got (%d, %v)", code, exited)
	}
}

func TestRunRemotePRAPIOnly(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pulls/"):
			w.Write([]byte(`{"number":42,"title":"a change","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
		case strings.HasPrefix(r.URL.Path, "/repos/owner/repo/commits"):
			w.Write([]byte(`[]`))
		default:
			w.Write([]byte(repoJSON("repo", false)))
		}
	})

	target, err := remote.ParsePR("owner/repo#42")
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42", "--api-only"}, func() {
		runRemotePRWithTarget(target)
	})
	for _, want := range []string{"a change", "feature", "main", "via API"} {
		if !strings.Contains(out, want) {
			t.Errorf("PR output is missing %q\n---\n%s", want, out)
		}
	}
}

func TestRunRemotePRNotFound(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	stubToken(t, "ghp_x", nil)

	target, err := remote.ParsePR("owner/repo#99")
	if err != nil {
		t.Fatal(err)
	}
	_, code, exited := runMain(t, []string{"claim", "pr", "owner/repo#99"}, func() {
		runRemotePRWithTarget(target)
	})
	if !exited || code != 1 {
		t.Errorf("a missing PR should exit, got (%d, %v)", code, exited)
	}
}

func TestRunRemotePRAPIOnlyScansTheHeadBranch(t *testing.T) {
	// --api-only asked for the repository's default branch, so `claim pr`
	// over the API reported main's commits and none of the pull request's.
	isolateData(t)
	var scanned string
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pulls/"):
			w.Write([]byte(`{"number":42,"title":"a change","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
		case strings.HasPrefix(r.URL.Path, "/repos/owner/repo/commits"):
			scanned = r.URL.Query().Get("sha")
			w.Write([]byte(`[]`))
		default:
			w.Write([]byte(repoJSON("repo", false)))
		}
	})

	target, err := remote.ParsePR("owner/repo#42")
	if err != nil {
		t.Fatal(err)
	}
	runMain(t, []string{"claim", "pr", "owner/repo#42", "--api-only"}, func() {
		runRemotePRWithTarget(target)
	})
	if scanned != "feature" {
		t.Errorf("scanned branch = %q, want the pull request's head branch", scanned)
	}
}

func TestLogoutWithoutATerminalNamesTheDirectForm(t *testing.T) {
	// The picker used to fail silently, so `claim logout` in a script or an
	// agent printed nothing at all and exited 0.
	isolateData(t)
	if err := auth.SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}

	out, code, exited := runMain(t, []string{"claim", "logout"}, runLogout)
	if !exited || code != 1 {
		t.Errorf("a picker that cannot run should exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "claim logout <username>") {
		t.Errorf("the message should name the direct form:\n%s", out)
	}
}

func TestRunRemoteMultiRepoForceTakesEveryDefault(t *testing.T) {
	// --force selected every repo and then stopped on the scan-method
	// prompt, so the flag could not carry a multi-repo scan on its own.
	isolateData(t)
	var commitRequests int
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits") {
			commitRequests++
			w.Write([]byte(`[]`))
			return
		}
		w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
	})

	out, _, exited := runMain(t, []string{"claim", "org", "acme", "--force"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if exited {
		t.Errorf("--force should carry the whole scan:\n%s", out)
	}
	if !strings.Contains(out, "Scanning 1 repo(s)") {
		t.Errorf("output = %q", out)
	}
	if commitRequests == 0 {
		t.Error("no repository was actually scanned")
	}
}

func TestRunRemoteMultiRepoWithoutATerminalNamesForce(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
	})

	out, code, exited := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !exited || code != 1 {
		t.Errorf("a picker that cannot run should exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("the message should name --force:\n%s", out)
	}
}

func TestRunRemoteRepoAPIOnlyExitsWhenTheBranchIsGone(t *testing.T) {
	// A failed listing printed an error and exited 0, so a script could not
	// tell "no Claude commits" from "that branch does not exist".
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/owner/repo/commits") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, err := remote.ParseRepo("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	out, code, exited := runMain(t, []string{"claim", "repo", "owner/repo", "--api-only"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !exited || code != 1 {
		t.Errorf("a failed API scan should exit, got (%d, %v)\n%s", code, exited, out)
	}
}
