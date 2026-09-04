package remote

import "testing"

func TestTargetString(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		want   string
	}{
		{"plain repo", Target{Owner: "izam-mohammed", Repo: "claim-your-code"}, "izam-mohammed/claim-your-code"},
		{"repo with branch", Target{Owner: "o", Repo: "r", Branch: "fix/multi-bar"}, "o/r@fix/multi-bar"},
		{"pull request", Target{Owner: "o", Repo: "r", PRNum: 42}, "o/r#42"},
		{"PR number wins over branch", Target{Owner: "o", Repo: "r", Branch: "main", PRNum: 7}, "o/r#7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsURLRejectsLocalPaths(t *testing.T) {
	for _, in := range []string{".", "..", "./my-repo", "/abs/path", "my-repo", "~/code", "owner/repo"} {
		if IsURL(in) {
			t.Errorf("IsURL(%q) = true, want false — that is a local path or shorthand", in)
		}
	}
}

func TestParseURLOrgForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		owner string
	}{
		{"orgs path", "https://github.com/orgs/my-org", "my-org"},
		{"orgs repositories tab", "https://github.com/orgs/my-org/repositories", "my-org"},
		{"bare profile", "https://github.com/izam-mohammed", "izam-mohammed"},
		{"profile with trailing slash", "https://github.com/izam-mohammed/", "izam-mohammed"},
		{"www host", "https://www.github.com/izam-mohammed", "izam-mohammed"},
		{"scheme-less github.com", "github.com/izam-mohammed", "izam-mohammed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ParseURL(tt.input)
			if err != nil {
				t.Fatalf("ParseURL(%q): %v", tt.input, err)
			}
			if target.Kind != "user" {
				t.Errorf("Kind = %q, want \"user\"", target.Kind)
			}
			if target.Owner != tt.owner {
				t.Errorf("Owner = %q, want %q", target.Owner, tt.owner)
			}
		})
	}
}

func TestParseURLBranchWithSlashes(t *testing.T) {
	target, err := ParseURL("https://github.com/o/r/tree/fix/multi/bar")
	if err != nil {
		t.Fatal(err)
	}
	if target.Branch != "fix/multi/bar" {
		t.Errorf("Branch = %q, want %q — slashes in a branch name must be preserved", target.Branch, "fix/multi/bar")
	}
}

func TestParseURLNonNumericPRIsPlainRepo(t *testing.T) {
	target, err := ParseURL("https://github.com/o/r/pull/not-a-number")
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != "repo" {
		t.Errorf("Kind = %q, want \"repo\" when the PR segment is not a number", target.Kind)
	}
	if target.PRNum != 0 {
		t.Errorf("PRNum = %d, want 0", target.PRNum)
	}
}

func TestParseURLRejectsOtherHosts(t *testing.T) {
	for _, in := range []string{"https://gitlab.com/o/r", "https://example.com/o/r"} {
		if _, err := ParseURL(in); err == nil {
			t.Errorf("ParseURL(%q) succeeded, want an error — only github.com is supported", in)
		}
	}
}

func TestParseURLRejectsEmptyPath(t *testing.T) {
	if _, err := ParseURL("https://github.com/"); err == nil {
		t.Error("ParseURL of a bare host succeeded, want an error")
	}
}

func TestParseURLInvalid(t *testing.T) {
	if _, err := ParseURL("https://github.com/o\x7f/r"); err == nil {
		t.Error("ParseURL accepted a malformed URL")
	}
}

func TestParseURLSSHDropsGitSuffix(t *testing.T) {
	target, err := ParseURL("git@github.com:izam-mohammed/claim-your-code.git")
	if err != nil {
		t.Fatal(err)
	}
	if target.Repo != "claim-your-code" {
		t.Errorf("Repo = %q, want the .git suffix stripped", target.Repo)
	}
	if target.CloneURL != "https://github.com/izam-mohammed/claim-your-code.git" {
		t.Errorf("CloneURL = %q", target.CloneURL)
	}
}

func TestParseURLStripsGitSuffixFromHTTPS(t *testing.T) {
	target, err := ParseURL("https://github.com/o/r.git")
	if err != nil {
		t.Fatal(err)
	}
	if target.Repo != "r" {
		t.Errorf("Repo = %q, want %q", target.Repo, "r")
	}
}

func TestParseRepoRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "just-a-name", "too/many/parts", "owner/", "/repo", "own er/repo"} {
		if _, err := ParseRepo(in); err == nil {
			t.Errorf("ParseRepo(%q) succeeded, want an error", in)
		}
	}
}

func TestParseRepoFromURL(t *testing.T) {
	target, err := ParseRepo("https://github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if target.Owner != "o" || target.Repo != "r" {
		t.Errorf("got %s/%s, want o/r", target.Owner, target.Repo)
	}
}

func TestParsePRURLWithoutNumberIsRejected(t *testing.T) {
	_, err := ParsePR("https://github.com/o/r")
	if err == nil {
		t.Fatal("ParsePR accepted a repo URL with no PR number")
	}
}

func TestParsePRPropagatesURLError(t *testing.T) {
	if _, err := ParsePR("https://gitlab.com/o/r/pull/1"); err == nil {
		t.Error("ParsePR accepted a non-github host")
	}
}

func TestParsePRRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "owner/repo", "owner/repo#", "owner/repo#abc"} {
		if _, err := ParsePR(in); err == nil {
			t.Errorf("ParsePR(%q) succeeded, want an error", in)
		}
	}
}

func TestParsePRShorthandSetsCloneURL(t *testing.T) {
	target, err := ParsePR("izam-mohammed/claim-your-code#42")
	if err != nil {
		t.Fatal(err)
	}
	if target.PRNum != 42 {
		t.Errorf("PRNum = %d, want 42", target.PRNum)
	}
	if target.CloneURL != "https://github.com/izam-mohammed/claim-your-code.git" {
		t.Errorf("CloneURL = %q", target.CloneURL)
	}
}
