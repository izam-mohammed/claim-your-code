package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripTokenFromURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"https://x-access-token:ghp_abc123@github.com/owner/repo.git",
			"https://github.com/owner/repo.git",
		},
		{
			"https://github.com/owner/repo.git",
			"https://github.com/owner/repo.git",
		},
		{
			"http://x-access-token:tok@github.com/owner/repo",
			"http://github.com/owner/repo",
		},
		{
			"git@github.com:owner/repo.git",
			"git@github.com:owner/repo.git",
		},
	}
	for _, tt := range tests {
		got := StripTokenFromURL(tt.input)
		if got != tt.want {
			t.Errorf("StripTokenFromURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAuthCloneURL(t *testing.T) {
	c := NewClient("ghp_test123")
	url := c.AuthCloneURL("owner", "repo")
	want := "https://x-access-token:ghp_test123@github.com/owner/repo.git"
	if url != want {
		t.Errorf("AuthCloneURL = %q, want %q", url, want)
	}
}

func TestGetRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/myrepo" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":           "myrepo",
			"full_name":      "owner/myrepo",
			"clone_url":      "https://github.com/owner/myrepo.git",
			"ssh_url":        "git@github.com:owner/myrepo.git",
			"default_branch": "main",
			"description":    "A test repo",
			"private":        false,
			"fork":           false,
			"archived":       false,
		})
	}))
	defer srv.Close()

	c := &Client{token: "test-token", http: srv.Client()}
	// Override API base for testing
	origBase := apiBase
	defer func() { SetAPIBase(origBase) }()
	SetAPIBase(srv.URL)

	repo, err := c.GetRepo("owner", "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if repo.Name != "myrepo" {
		t.Errorf("name = %q, want %q", repo.Name, "myrepo")
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("default_branch = %q, want %q", repo.DefaultBranch, "main")
	}
	if repo.Private {
		t.Error("expected private=false")
	}
}

func TestGetRepo404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := &Client{token: "test-token", http: srv.Client()}
	origBase := apiBase
	defer func() { SetAPIBase(origBase) }()
	SetAPIBase(srv.URL)

	_, err := c.GetRepo("owner", "nonexistent")
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestListCommits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"sha": "abc123",
				"commit": map[string]string{
					"message": "Fix bug\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>",
				},
			},
			{
				"sha": "def456",
				"commit": map[string]string{
					"message": "Clean commit",
				},
			},
		})
	}))
	defer srv.Close()

	c := &Client{token: "test-token", http: srv.Client()}
	origBase := apiBase
	defer func() { SetAPIBase(origBase) }()
	SetAPIBase(srv.URL)

	commits, err := c.ListCommits("owner", "repo", "main", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].SHA != "abc123" {
		t.Errorf("SHA = %q, want %q", commits[0].SHA, "abc123")
	}
	if commits[1].Message != "Clean commit" {
		t.Errorf("message = %q, want %q", commits[1].Message, "Clean commit")
	}
}

func TestGetPR(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.URL.Path {
		case "/repos/owner/repo/pulls/42":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"number": 42,
				"title":  "Fix auth",
				"head": map[string]string{
					"ref": "fix-auth",
					"sha": "abc123",
				},
				"base": map[string]string{
					"ref": "main",
				},
			})
		case "/repos/owner/repo":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":           "repo",
				"full_name":      "owner/repo",
				"clone_url":      "https://github.com/owner/repo.git",
				"default_branch": "main",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{token: "test-token", http: srv.Client()}
	origBase := apiBase
	defer func() { SetAPIBase(origBase) }()
	SetAPIBase(srv.URL)

	pr, err := c.GetPR("owner", "repo", 42)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 42 {
		t.Errorf("number = %d, want 42", pr.Number)
	}
	if pr.Title != "Fix auth" {
		t.Errorf("title = %q, want %q", pr.Title, "Fix auth")
	}
	if pr.HeadBranch != "fix-auth" {
		t.Errorf("head = %q, want %q", pr.HeadBranch, "fix-auth")
	}
	if pr.BaseBranch != "main" {
		t.Errorf("base = %q, want %q", pr.BaseBranch, "main")
	}
}

func TestNextPageURL(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{
			`<https://api.github.com/orgs/foo/repos?page=2>; rel="next", <https://api.github.com/orgs/foo/repos?page=5>; rel="last"`,
			"https://api.github.com/orgs/foo/repos?page=2",
		},
		{
			`<https://api.github.com/orgs/foo/repos?page=5>; rel="last"`,
			"",
		},
		{"", ""},
	}
	for _, tt := range tests {
		got := nextPageURL(tt.header)
		if got != tt.want {
			t.Errorf("nextPageURL(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
