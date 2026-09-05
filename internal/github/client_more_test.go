package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve starts a test API server and points the package at it for the test's duration.
func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := APIBase()
	SetAPIBase(srv.URL)
	t.Cleanup(func() {
		SetAPIBase(prev)
		srv.Close()
	})
	return srv
}

func TestNewPublicClientIsUnauthenticated(t *testing.T) {
	c := NewPublicClient()
	if c.IsAuthenticated() {
		t.Error("NewPublicClient().IsAuthenticated() = true, want false")
	}
	if c.Token() != "" {
		t.Errorf("Token() = %q, want empty", c.Token())
	}
}

func TestNewClientCarriesToken(t *testing.T) {
	c := NewClient("ghp_secret")
	if !c.IsAuthenticated() {
		t.Error("IsAuthenticated() = false for a client built with a token")
	}
	if c.Token() != "ghp_secret" {
		t.Errorf("Token() = %q, want %q", c.Token(), "ghp_secret")
	}
}

func TestGetSendsAuthorizationOnlyWhenTokenPresent(t *testing.T) {
	var gotAuth string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept header = %q", r.Header.Get("Accept"))
		}
		w.Write([]byte(`{}`))
	})

	resp, err := NewClient("tok").get("/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok")
	}

	resp, err = NewPublicClient().get("/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotAuth != "" {
		t.Errorf("Authorization = %q for a public client, want it unset", gotAuth)
	}
}

func TestGetJSONSurfacesNon200(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	})
	var v struct{}
	err := NewPublicClient().getJSON("/x", &v)
	if err == nil {
		t.Fatal("getJSON succeeded on a 500")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to carry the status and body", err)
	}
}

func TestGetJSONRequestError(t *testing.T) {
	prev := APIBase()
	SetAPIBase("http://127.0.0.1:1") // nothing listening
	t.Cleanup(func() { SetAPIBase(prev) })

	var v struct{}
	if err := NewPublicClient().getJSON("/x", &v); err == nil {
		t.Error("getJSON succeeded against a dead server")
	}
}

func TestGetJSONMalformedBody(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	})
	var v struct{}
	if err := NewPublicClient().getJSON("/x", &v); err == nil {
		t.Error("getJSON accepted malformed JSON")
	}
}

func TestListReposFollowsPagination(t *testing.T) {
	var srv *httptest.Server
	srv = serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/acme/repos?page=2>; rel="next"`, srv.URL))
			w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
		case "2":
			w.Write([]byte(`[{"name":"two","owner":{"login":"acme"},"archived":true}]`))
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
		}
	})

	repos, err := NewPublicClient().ListOrgRepos("acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("ListOrgRepos returned %d repos, want 2 across both pages", len(repos))
	}
	if repos[0].Name != "one" || repos[0].Owner != "acme" || repos[0].DefaultBranch != "main" {
		t.Errorf("first repo = %+v", repos[0])
	}
	if !repos[1].Archived {
		t.Error("second repo should be marked archived")
	}
}

func TestListUserReposRequestsOwnerType(t *testing.T) {
	var gotPath string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(`[{"name":"solo","owner":{"login":"izam"}}]`))
	})

	repos, err := NewPublicClient().ListUserRepos("izam")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "solo" {
		t.Fatalf("ListUserRepos = %+v", repos)
	}
	if !strings.Contains(gotPath, "type=owner") {
		t.Errorf("request path = %q, want it to restrict to owned repos", gotPath)
	}
}

func TestListReposSkipsUndecodableEntries(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		// The middle entry is a string, not an object.
		w.Write([]byte(`[{"name":"ok"},"garbage",{"name":"also-ok"}]`))
	})
	repos, err := NewPublicClient().ListOrgRepos("acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Errorf("got %d repos, want the 2 decodable ones (bad entries are skipped)", len(repos))
	}
}

func TestListReposPropagatesHTTPError(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`rate limited`))
	})
	if _, err := NewPublicClient().ListOrgRepos("acme"); err == nil {
		t.Error("ListOrgRepos succeeded on a 403")
	}
}

func TestListReposPropagatesMalformedPage(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"not":"an array"}`))
	})
	if _, err := NewPublicClient().ListOrgRepos("acme"); err == nil {
		t.Error("ListOrgRepos accepted a non-array page")
	}
}

func TestListReposRequestError(t *testing.T) {
	prev := APIBase()
	SetAPIBase("http://127.0.0.1:1")
	t.Cleanup(func() { SetAPIBase(prev) })
	if _, err := NewPublicClient().ListOrgRepos("acme"); err == nil {
		t.Error("ListOrgRepos succeeded against a dead server")
	}
}

func TestGetPRFailsWhenRepoLookupFails(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":7,"title":"t","head":{"ref":"f","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := NewPublicClient().GetPR("o", "r", 7); err == nil {
		t.Error("GetPR succeeded even though the repo lookup 404'd")
	}
}

func TestListCommitsClampsLimit(t *testing.T) {
	var gotPath string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Write([]byte(`[]`))
	})
	for _, limit := range []int{0, -5, 1000} {
		if _, err := NewPublicClient().ListCommits("o", "r", "main", limit); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(gotPath, "per_page=100") {
			t.Errorf("limit %d produced path %q, want it clamped to per_page=100", limit, gotPath)
		}
	}
}

func TestListCommitsPropagatesError(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := NewPublicClient().ListCommits("o", "r", "main", 10); err == nil {
		t.Error("ListCommits succeeded on a 404")
	}
}

func TestGetUser(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %q, want /user", r.URL.Path)
		}
		w.Write([]byte(`{"login":"izam-mohammed"}`))
	})
	user, err := NewClient("tok").GetUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Login != "izam-mohammed" {
		t.Errorf("Login = %q", user.Login)
	}
}

func TestGetUserPropagatesError(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := NewClient("bad").GetUser(); err == nil {
		t.Error("GetUser succeeded on a 401")
	}
}

func TestIsOrg(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/orgs/real-org" {
			w.Write([]byte(`{"login":"real-org"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c := NewPublicClient()
	if !c.IsOrg("real-org") {
		t.Error("IsOrg(real-org) = false, want true")
	}
	if c.IsOrg("just-a-user") {
		t.Error("IsOrg(just-a-user) = true, want false")
	}
}

func TestIsOrgFalseWhenUnreachable(t *testing.T) {
	prev := APIBase()
	SetAPIBase("http://127.0.0.1:1")
	t.Cleanup(func() { SetAPIBase(prev) })
	if NewPublicClient().IsOrg("anything") {
		t.Error("IsOrg = true when the API is unreachable, want false")
	}
}

func TestNextPageURLNoNext(t *testing.T) {
	for _, header := range []string{"", `<https://api.github.com/x?page=1>; rel="prev"`} {
		if got := nextPageURL(header); got != "" {
			t.Errorf("nextPageURL(%q) = %q, want empty", header, got)
		}
	}
}

func TestParseRateLimit(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Remaining", "4321")
	resp.Header.Set("X-RateLimit-Reset", "1700000000")
	remaining, reset := ParseRateLimit(resp)
	if remaining != 4321 {
		t.Errorf("remaining = %d, want 4321", remaining)
	}
	if reset != 1700000000 {
		t.Errorf("reset = %d, want 1700000000", reset)
	}
}

func TestParseRateLimitMissingAndMalformedHeaders(t *testing.T) {
	empty := &http.Response{Header: http.Header{}}
	if remaining, reset := ParseRateLimit(empty); remaining != 0 || reset != 0 {
		t.Errorf("ParseRateLimit(no headers) = (%d, %d), want (0, 0)", remaining, reset)
	}

	bad := &http.Response{Header: http.Header{}}
	bad.Header.Set("X-RateLimit-Remaining", "not-a-number")
	bad.Header.Set("X-RateLimit-Reset", "also-not")
	if remaining, reset := ParseRateLimit(bad); remaining != 0 || reset != 0 {
		t.Errorf("ParseRateLimit(garbage) = (%d, %d), want (0, 0)", remaining, reset)
	}
}

func TestStripTokenFromURLLeavesNonHTTPAlone(t *testing.T) {
	ssh := "git@github.com:owner/repo.git"
	if got := StripTokenFromURL(ssh); got != ssh {
		t.Errorf("StripTokenFromURL(%q) = %q, want it unchanged", ssh, got)
	}
}

func TestStripTokenFromURLHTTP(t *testing.T) {
	in := "http://x-access-token:secret@github.com/o/r.git"
	want := "http://github.com/o/r.git"
	if got := StripTokenFromURL(in); got != want {
		t.Errorf("StripTokenFromURL() = %q, want %q", got, want)
	}
}

func TestStripTokenFromURLWithoutToken(t *testing.T) {
	in := "https://github.com/o/r.git"
	if got := StripTokenFromURL(in); got != in {
		t.Errorf("StripTokenFromURL() = %q, want it unchanged", got)
	}
}
