package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var apiBase = "https://api.github.com"

// setAPIBase overrides the API base URL (for testing).
func setAPIBase(url string) { apiBase = url }

// Client is a lightweight GitHub API client.
type Client struct {
	token string
	http  *http.Client
}

// NewClient creates a GitHub API client with the given token.
func NewClient(token string) *Client {
	return &Client{token: token, http: http.DefaultClient}
}

// NewPublicClient creates an unauthenticated client for public repo access.
func NewPublicClient() *Client {
	return &Client{token: "", http: http.DefaultClient}
}

// IsAuthenticated returns true if the client has a token.
func (c *Client) IsAuthenticated() bool {
	return c.token != ""
}

// Token returns the client's auth token (for clone operations).
func (c *Client) Token() string {
	return c.token
}

func (c *Client) get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	return c.http.Do(req)
}

func (c *Client) getJSON(path string, v interface{}) error {
	resp, err := c.get(path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("not found: %s", path)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

// getPaginated fetches all pages of a paginated list endpoint.
func (c *Client) getPaginated(path string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	url := apiBase + path

	for url != "" {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
		}

		var page []json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		all = append(all, page...)

		url = nextPageURL(resp.Header.Get("Link"))
	}

	return all, nil
}

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextPageURL(linkHeader string) string {
	if m := linkNextRe.FindStringSubmatch(linkHeader); m != nil {
		return m[1]
	}
	return ""
}

// GetRepo returns info about a single repository.
func (c *Client) GetRepo(owner, repo string) (*RepoInfo, error) {
	var raw struct {
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		DefaultBranch string `json:"default_branch"`
		Description   string `json:"description"`
		Private       bool   `json:"private"`
		Fork          bool   `json:"fork"`
		Archived      bool   `json:"archived"`
	}
	if err := c.getJSON(fmt.Sprintf("/repos/%s/%s", owner, repo), &raw); err != nil {
		return nil, err
	}
	return &RepoInfo{
		Owner:         owner,
		Name:          raw.Name,
		FullName:      raw.FullName,
		CloneURL:      raw.CloneURL,
		SSHURL:        raw.SSHURL,
		DefaultBranch: raw.DefaultBranch,
		Description:   raw.Description,
		Private:       raw.Private,
		Fork:          raw.Fork,
		Archived:      raw.Archived,
	}, nil
}

// ListOrgRepos returns all repositories for an organization.
func (c *Client) ListOrgRepos(org string) ([]RepoInfo, error) {
	return c.listRepos(fmt.Sprintf("/orgs/%s/repos?per_page=100", org))
}

// ListUserRepos returns all owned repositories for a user.
func (c *Client) ListUserRepos(user string) ([]RepoInfo, error) {
	return c.listRepos(fmt.Sprintf("/users/%s/repos?per_page=100&type=owner", user))
}

func (c *Client) listRepos(path string) ([]RepoInfo, error) {
	pages, err := c.getPaginated(path)
	if err != nil {
		return nil, err
	}

	var repos []RepoInfo
	for _, raw := range pages {
		var r struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			CloneURL      string `json:"clone_url"`
			SSHURL        string `json:"ssh_url"`
			DefaultBranch string `json:"default_branch"`
			Description   string `json:"description"`
			Private       bool   `json:"private"`
			Fork          bool   `json:"fork"`
			Archived      bool   `json:"archived"`
			Owner         struct {
				Login string `json:"login"`
			} `json:"owner"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		repos = append(repos, RepoInfo{
			Owner:         r.Owner.Login,
			Name:          r.Name,
			FullName:      r.FullName,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			DefaultBranch: r.DefaultBranch,
			Description:   r.Description,
			Private:       r.Private,
			Fork:          r.Fork,
			Archived:      r.Archived,
		})
	}
	return repos, nil
}

// GetPR returns info about a pull request.
func (c *Client) GetPR(owner, repo string, num int) (*PRInfo, error) {
	var raw struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := c.getJSON(fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, num), &raw); err != nil {
		return nil, err
	}

	repoInfo, err := c.GetRepo(owner, repo)
	if err != nil {
		return nil, err
	}

	return &PRInfo{
		Number:     raw.Number,
		Title:      raw.Title,
		HeadBranch: raw.Head.Ref,
		HeadSHA:    raw.Head.SHA,
		BaseBranch: raw.Base.Ref,
		Repo:       *repoInfo,
	}, nil
}

// ListCommits returns recent commits for a branch (up to limit).
func (c *Client) ListCommits(owner, repo, branch string, limit int) ([]CommitInfo, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	path := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=%d", owner, repo, branch, limit)

	var raw []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := c.getJSON(path, &raw); err != nil {
		return nil, err
	}

	commits := make([]CommitInfo, len(raw))
	for i, r := range raw {
		commits[i] = CommitInfo{SHA: r.SHA, Message: r.Commit.Message}
	}
	return commits, nil
}

// GetUser returns the authenticated user's info.
func (c *Client) GetUser() (*UserInfo, error) {
	var user UserInfo
	if err := c.getJSON("/user", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// IsOrg checks if a name is a GitHub organization (vs a user).
func (c *Client) IsOrg(name string) bool {
	resp, err := c.get(fmt.Sprintf("/orgs/%s", name))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// AuthCloneURL returns a clone URL with the token embedded for authenticated cloning.
// Format: https://x-access-token:{token}@github.com/owner/repo.git
func (c *Client) AuthCloneURL(owner, repo string) string {
	return fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", c.token, owner, repo)
}

// StripTokenFromURL removes embedded tokens from clone URLs.
func StripTokenFromURL(rawURL string) string {
	// https://x-access-token:TOKEN@github.com/... → https://github.com/...
	// Only strip from HTTP(S) URLs, not SSH URLs
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	if idx := strings.Index(rawURL, "@github.com"); idx > 0 {
		prefix := "https://"
		if strings.HasPrefix(rawURL, "http://") {
			prefix = "http://"
		}
		return prefix + "github.com" + rawURL[idx+len("@github.com"):]
	}
	return rawURL
}

// ParseRateLimit extracts rate limit info from response headers.
func ParseRateLimit(resp *http.Response) (remaining int, reset int64) {
	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		remaining, _ = strconv.Atoi(v)
	}
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		reset, _ = strconv.ParseInt(v, 10, 64)
	}
	return
}
