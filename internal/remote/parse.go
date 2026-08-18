package remote

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Target represents a parsed remote GitHub target.
type Target struct {
	Kind     string // "repo", "pr", "user" (inferred from URL structure)
	Owner    string
	Repo     string
	Branch   string // specific branch (from /tree/<branch> URL)
	PRNum    int
	CloneURL string // resolved HTTPS clone URL
}

// String returns a human-readable label for the target.
func (t Target) String() string {
	if t.PRNum > 0 {
		return fmt.Sprintf("%s/%s#%d", t.Owner, t.Repo, t.PRNum)
	}
	base := fmt.Sprintf("%s/%s", t.Owner, t.Repo)
	if t.Branch != "" {
		return fmt.Sprintf("%s@%s", base, t.Branch)
	}
	return base
}

var (
	// owner/repo pattern
	ownerRepoRe = regexp.MustCompile(`^([a-zA-Z0-9\-_.]+)/([a-zA-Z0-9\-_.]+)$`)
	// owner/repo#123 pattern
	ownerRepoPRRe = regexp.MustCompile(`^([a-zA-Z0-9\-_.]+)/([a-zA-Z0-9\-_.]+)#(\d+)$`)
	// SSH URL: git@github.com:owner/repo.git
	sshRe = regexp.MustCompile(`^git@github\.com:([a-zA-Z0-9\-_.]+)/([a-zA-Z0-9\-_.]+?)(?:\.git)?$`)
)

// IsURL returns true if the input looks like a URL (not a local path).
func IsURL(input string) bool {
	return strings.HasPrefix(input, "https://") ||
		strings.HasPrefix(input, "http://") ||
		strings.HasPrefix(input, "git@") ||
		strings.HasPrefix(input, "github.com/")
}

// ParseURL parses any GitHub URL into a Target.
// Supports HTTPS, HTTP, SSH, and bare github.com/ URLs.
// Auto-detects PR URLs (/pull/N).
func ParseURL(input string) (*Target, error) {
	// SSH URL
	if m := sshRe.FindStringSubmatch(input); m != nil {
		owner, repo := m[1], m[2]
		return &Target{
			Kind:     "repo",
			Owner:    owner,
			Repo:     repo,
			CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		}, nil
	}

	// Normalize bare github.com/ to https://
	if strings.HasPrefix(input, "github.com/") {
		input = "https://" + input
	}

	u, err := url.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if u.Host != "github.com" && u.Host != "www.github.com" {
		return nil, fmt.Errorf("only github.com URLs are supported, got %q", u.Host)
	}

	// Split path: /owner[/repo[/pull/N]][.git]
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")

	// Filter out empty parts (trailing slashes)
	var cleaned []string
	for _, p := range parts {
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	parts = cleaned

	if len(parts) == 0 {
		return nil, fmt.Errorf("cannot extract owner from URL %q", input)
	}

	// Org URL pattern: /orgs/my-org or /orgs/my-org/repositories
	if parts[0] == "orgs" && len(parts) >= 2 {
		return &Target{
			Kind:  "user",
			Owner: parts[1],
		}, nil
	}

	// Single segment = user/org profile URL (e.g. https://github.com/izam-mohammed)
	if len(parts) == 1 {
		return &Target{
			Kind:  "user",
			Owner: parts[0],
		}, nil
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	target := &Target{
		Kind:     "repo",
		Owner:    owner,
		Repo:     repo,
		CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
	}

	// Check for /pull/N
	if len(parts) >= 4 && parts[2] == "pull" {
		prNum, err := strconv.Atoi(parts[3])
		if err == nil {
			target.Kind = "pr"
			target.PRNum = prNum
		}
	}

	// Check for /tree/<branch> — branch can contain slashes (e.g. fix/multi-bar)
	if len(parts) >= 4 && parts[2] == "tree" {
		target.Branch = strings.Join(parts[3:], "/")
	}

	return target, nil
}

// ParseRepo parses input for `claim repo <arg>`.
// Accepts: owner/repo, any URL format.
func ParseRepo(input string) (*Target, error) {
	if IsURL(input) {
		return ParseURL(input)
	}

	// owner/repo shorthand
	if m := ownerRepoRe.FindStringSubmatch(input); m != nil {
		owner, repo := m[1], m[2]
		return &Target{
			Owner:    owner,
			Repo:     repo,
			CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		}, nil
	}

	return nil, fmt.Errorf("cannot parse %q as a repo — use owner/repo or a GitHub URL", input)
}

// ParsePR parses input for `claim pr <arg>`.
// Accepts: owner/repo#123, PR URL.
func ParsePR(input string) (*Target, error) {
	if IsURL(input) {
		target, err := ParseURL(input)
		if err != nil {
			return nil, err
		}
		if target.PRNum == 0 {
			return nil, fmt.Errorf("URL %q does not contain a PR number — use .../pull/N format", input)
		}
		return target, nil
	}

	// owner/repo#123
	if m := ownerRepoPRRe.FindStringSubmatch(input); m != nil {
		prNum, _ := strconv.Atoi(m[3])
		owner, repo := m[1], m[2]
		return &Target{
			Owner:    owner,
			Repo:     repo,
			PRNum:    prNum,
			CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		}, nil
	}

	return nil, fmt.Errorf("cannot parse %q as a PR — use owner/repo#N or a GitHub PR URL", input)
}
