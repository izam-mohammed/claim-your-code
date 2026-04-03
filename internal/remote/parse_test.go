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
		owner   string
		repo    string
		prNum   int
		wantErr bool
	}{
		{"https", "https://github.com/izam/repo", "izam", "repo", 0, false},
		{"https .git", "https://github.com/izam/repo.git", "izam", "repo", 0, false},
		{"http", "http://github.com/izam/repo", "izam", "repo", 0, false},
		{"bare github.com", "github.com/izam/repo", "izam", "repo", 0, false},
		{"ssh", "git@github.com:izam/repo.git", "izam", "repo", 0, false},
		{"ssh no .git", "git@github.com:izam/repo", "izam", "repo", 0, false},
		{"pr url", "https://github.com/izam/repo/pull/42", "izam", "repo", 42, false},
		{"pr url trailing", "https://github.com/izam/repo/pull/42/", "izam", "repo", 42, false},
		{"www prefix", "https://www.github.com/izam/repo", "izam", "repo", 0, false},
		{"not github", "https://gitlab.com/izam/repo", "", "", 0, true},
		{"no repo", "https://github.com/izam", "", "", 0, true},
		{"hyphenated", "https://github.com/izam-mohammed/claim-your-code", "izam-mohammed", "claim-your-code", 0, false},
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
			if target.Owner != tt.owner {
				t.Errorf("owner = %q, want %q", target.Owner, tt.owner)
			}
			if target.Repo != tt.repo {
				t.Errorf("repo = %q, want %q", target.Repo, tt.repo)
			}
			if target.PRNum != tt.prNum {
				t.Errorf("prNum = %d, want %d", target.PRNum, tt.prNum)
			}
			if target.CloneURL == "" {
				t.Error("cloneURL should not be empty")
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
		{"izam/repo", "", "", 0, true},            // no PR number
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
