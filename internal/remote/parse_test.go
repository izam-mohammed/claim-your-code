package remote

import "testing"

func TestIsURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/owner/repo", true},
		{"http://github.com/owner/repo", true},
		{"git@github.com:owner/repo.git", true},
		{"github.com/owner/repo", true},
		{"./my-folder", false},
		{"/home/user/project", false},
		{".", false},
		{"owner/repo", false},
		{"some-folder", false},
	}
	for _, tt := range tests {
		if got := IsURL(tt.input); got != tt.want {
			t.Errorf("IsURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		kind    string
		owner   string
		repo    string
		branch  string
		prNum   int
		wantErr bool
	}{
		{"https", "https://github.com/izam/repo", "repo", "izam", "repo", "", 0, false},
		{"https .git", "https://github.com/izam/repo.git", "repo", "izam", "repo", "", 0, false},
		{"http", "http://github.com/izam/repo", "repo", "izam", "repo", "", 0, false},
		{"bare github.com", "github.com/izam/repo", "repo", "izam", "repo", "", 0, false},
		{"ssh", "git@github.com:izam/repo.git", "repo", "izam", "repo", "", 0, false},
		{"ssh no .git", "git@github.com:izam/repo", "repo", "izam", "repo", "", 0, false},
		{"pr url", "https://github.com/izam/repo/pull/42", "pr", "izam", "repo", "", 42, false},
		{"pr url trailing", "https://github.com/izam/repo/pull/42/", "pr", "izam", "repo", "", 42, false},
		{"www prefix", "https://www.github.com/izam/repo", "repo", "izam", "repo", "", 0, false},
		{"not github", "https://gitlab.com/izam/repo", "", "", "", "", 0, true},
		{"user profile", "https://github.com/izam-mohammed", "user", "izam-mohammed", "", "", 0, false},
		{"user profile trailing", "https://github.com/izam-mohammed/", "user", "izam-mohammed", "", "", 0, false},
		{"org url", "https://github.com/orgs/my-org", "user", "my-org", "", "", 0, false},
		{"org repos url", "https://github.com/orgs/my-org/repositories", "user", "my-org", "", "", 0, false},
		{"hyphenated", "https://github.com/izam-mohammed/claim-your-code", "repo", "izam-mohammed", "claim-your-code", "", 0, false},
		{"empty path", "https://github.com/", "", "", "", "", 0, true},
		{"branch simple", "https://github.com/izam/repo/tree/main", "repo", "izam", "repo", "main", 0, false},
		{"branch with slash", "https://github.com/izam/repo/tree/fix/multi-bar", "repo", "izam", "repo", "fix/multi-bar", 0, false},
		{"branch deep", "https://github.com/izam/repo/tree/feature/auth/v2", "repo", "izam", "repo", "feature/auth/v2", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ParseURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if target.Kind != tt.kind {
				t.Errorf("kind = %q, want %q", target.Kind, tt.kind)
			}
			if target.Owner != tt.owner {
				t.Errorf("owner = %q, want %q", target.Owner, tt.owner)
			}
			if target.Repo != tt.repo {
				t.Errorf("repo = %q, want %q", target.Repo, tt.repo)
			}
			if target.Branch != tt.branch {
				t.Errorf("branch = %q, want %q", target.Branch, tt.branch)
			}
			if target.PRNum != tt.prNum {
				t.Errorf("prNum = %d, want %d", target.PRNum, tt.prNum)
			}
			if tt.kind == "repo" || tt.kind == "pr" {
				if target.CloneURL == "" {
					t.Error("cloneURL should not be empty for repo/pr targets")
				}
			}
		})
	}
}

func TestParseRepo(t *testing.T) {
	tests := []struct {
		input   string
		owner   string
		repo    string
		wantErr bool
	}{
		{"izam/repo", "izam", "repo", false},
		{"izam-mohammed/claim-your-code", "izam-mohammed", "claim-your-code", false},
		{"https://github.com/izam/repo", "izam", "repo", false},
		{"git@github.com:izam/repo.git", "izam", "repo", false},
		{"not-valid", "", "", true},
		{"too/many/slashes", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			target, err := ParseRepo(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target.Owner != tt.owner || target.Repo != tt.repo {
				t.Errorf("got %s/%s, want %s/%s", target.Owner, target.Repo, tt.owner, tt.repo)
			}
		})
	}
}

func TestParsePR(t *testing.T) {
	tests := []struct {
		input   string
		owner   string
		repo    string
		prNum   int
		wantErr bool
	}{
		{"izam/repo#42", "izam", "repo", 42, false},
		{"izam-mohammed/claim-your-code#1", "izam-mohammed", "claim-your-code", 1, false},
		{"https://github.com/izam/repo/pull/99", "izam", "repo", 99, false},
		{"izam/repo", "", "", 0, true},                    // no PR number
		{"https://github.com/izam/repo", "", "", 0, true}, // URL without /pull/N
		{"not-valid", "", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			target, err := ParsePR(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target.Owner != tt.owner || target.Repo != tt.repo || target.PRNum != tt.prNum {
				t.Errorf("got %s/%s#%d, want %s/%s#%d", target.Owner, target.Repo, target.PRNum, tt.owner, tt.repo, tt.prNum)
			}
		})
	}
}
